package websocket

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
)

// newTestClient creates a Client without a real WS connection — safe for
// testing hub map logic since ReadPump/WritePump are never started.
func newTestClient(hub *Hub, userID string) *Client {
	return &Client{
		hub:    hub,
		send:   make(chan []byte, 1024),
		userID: userID,
	}
}

func TestInverseIndexMaintained(t *testing.T) {
	hub := NewHub()
	c := newTestClient(hub, "user1")

	hub.SubscribeToChannel("ch1", c)
	hub.SubscribeToChannel("ch2", c)

	hub.mu.RLock()
	chans := hub.clientChannels[c]
	hub.mu.RUnlock()

	if !chans["ch1"] || !chans["ch2"] {
		t.Fatal("inverse index not populated after subscribe")
	}

	hub.UnsubscribeFromChannel("ch1", c)

	hub.mu.RLock()
	chans = hub.clientChannels[c]
	hub.mu.RUnlock()

	if chans["ch1"] {
		t.Error("ch1 should have been removed from inverse index")
	}
	if !chans["ch2"] {
		t.Error("ch2 should still be in inverse index")
	}
}

func TestUnregisterCleansInverseIndex(t *testing.T) {
	hub := NewHub()
	c := newTestClient(hub, "user1")

	hub.RegisterClient(c)
	hub.SubscribeToChannel("ch1", c)
	hub.SubscribeToChannel("ch2", c)
	hub.UnregisterClient(c)

	hub.mu.RLock()
	_, inChannelSubs1 := hub.channelSubs["ch1"][c]
	_, inChannelSubs2 := hub.channelSubs["ch2"][c]
	_, inClientChannels := hub.clientChannels[c]
	hub.mu.RUnlock()

	if inChannelSubs1 || inChannelSubs2 {
		t.Error("client still in channelSubs after unregister")
	}
	if inClientChannels {
		t.Error("client still in clientChannels after unregister")
	}
}

func TestSafeSendToClosedChannel(t *testing.T) {
	c := &Client{send: make(chan []byte, 4)}
	c.closeSend() // mark closed and close channel

	// Must not panic
	sent := safeSend(c, []byte("hello"))
	if sent {
		t.Error("safeSend should return false for closed channel")
	}
}

func TestSafeSendToFullChannel(t *testing.T) {
	c := &Client{send: make(chan []byte, 1)}
	c.send <- []byte("full")

	sent := safeSend(c, []byte("overflow"))
	if sent {
		t.Error("safeSend should return false for full channel")
	}
}

func TestSafeSendDelivers(t *testing.T) {
	c := &Client{send: make(chan []byte, 1)}
	sent := safeSend(c, []byte("hello"))
	if !sent {
		t.Error("safeSend should return true when channel has capacity")
	}
	if msg := <-c.send; string(msg) != "hello" {
		t.Errorf("got %q, want %q", msg, "hello")
	}
}

func TestBroadcastToChannelConcurrentUnregister(t *testing.T) {
	hub := NewHub()
	const N = 50

	clients := make([]*Client, N)
	for i := range clients {
		clients[i] = newTestClient(hub, fmt.Sprintf("user%d", i))
		hub.RegisterClient(clients[i])
		hub.SubscribeToChannel("ch1", clients[i])
	}

	var wg sync.WaitGroup

	// Concurrent broadcasts
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hub.BroadcastToChannel("ch1", "TEST", []byte(`{"x":1}`))
		}()
	}

	// Concurrent unregisters
	for _, c := range clients[:25] {
		wg.Add(1)
		c := c
		go func() {
			defer wg.Done()
			hub.UnregisterClient(c)
		}()
	}

	wg.Wait()
	// Success: no panic, no race (run with -race)
}

func TestPresenceSnapshotCapped(t *testing.T) {
	// RegisterClient on a hub without a publisher should send a snapshot
	// with at most presenceSnapshotMax user IDs.
	hub := NewHub()

	// Populate userToClient with more than presenceSnapshotMax users
	hub.mu.Lock()
	for i := 0; i < presenceSnapshotMax+50; i++ {
		uid := fmt.Sprintf("user%d", i)
		c := newTestClient(hub, uid)
		hub.clients[c] = true
		hub.userToClient[uid] = map[*Client]bool{c: true}
	}
	hub.mu.Unlock()

	incoming := newTestClient(hub, "newcomer")
	hub.RegisterClient(incoming)

	// Drain the send channel to find the PRESENCE_SNAPSHOT message
	var snapshotMsg []byte
	for {
		select {
		case msg := <-incoming.send:
			var parsed struct {
				Type    string `json:"type"`
				Payload struct {
					UserIDs []string `json:"user_ids"`
				} `json:"payload"`
			}
			if err := json.Unmarshal(msg, &parsed); err == nil && parsed.Type == EventPresenceSnapshot {
				snapshotMsg = msg
			}
		default:
			goto done
		}
	}
done:
	if snapshotMsg == nil {
		t.Fatal("no PRESENCE_SNAPSHOT received")
	}

	var wrapper struct {
		Payload struct {
			UserIDs []string `json:"user_ids"`
		} `json:"payload"`
	}
	json.Unmarshal(snapshotMsg, &wrapper)
	if len(wrapper.Payload.UserIDs) > presenceSnapshotMax {
		t.Errorf("snapshot contains %d user IDs, want at most %d",
			len(wrapper.Payload.UserIDs), presenceSnapshotMax)
	}
}

// TestBroadcastToChannelSlowClientMinimalEviction verifies that a slow client
// (full send buffer) is removed from channelSubs for that channel but is NOT
// immediately removed from h.clients — USER_OFFLINE is deferred to UnregisterClient
// via the natural WritePump-exit teardown chain.
//
// This test FAILS with the old code (which deleted from h.clients in the eviction
// block, bypassing USER_OFFLINE), and PASSES with the new minimal-eviction approach.
func TestBroadcastToChannelSlowClientMinimalEviction(t *testing.T) {
	hub := NewHub()

	slow := &Client{
		hub:    hub,
		send:   make(chan []byte, 0), // zero-capacity: always "full"
		userID: "slow",
	}
	fast := newTestClient(hub, "fast")

	hub.RegisterClient(slow)
	hub.RegisterClient(fast)
	hub.SubscribeToChannel("ch1", slow)
	hub.SubscribeToChannel("ch1", fast)

	hub.BroadcastToChannel("ch1", "TEST", []byte(`{}`))

	hub.mu.RLock()
	_, slowInChannelSubs := hub.channelSubs["ch1"][slow]
	_, slowInClients := hub.clients[slow]
	_, fastInClients := hub.clients[fast]
	hub.mu.RUnlock()

	if slowInChannelSubs {
		t.Error("slow client should be removed from channelSubs (won't receive future broadcasts)")
	}
	if !slowInClients {
		// If we prematurely delete from h.clients, UnregisterClient's guard
		// fails and USER_OFFLINE never fires — user appears permanently online.
		t.Error("slow client must remain in h.clients until UnregisterClient fires naturally")
	}
	if !fastInClients {
		t.Error("fast client should be unaffected")
	}
}

// TestUnsubscribeUserFromServer verifies that dropping a user from a server
// removes subscriptions to channels in that server while leaving subs to
// other servers and DMs intact.
func TestUnsubscribeUserFromServer(t *testing.T) {
	hub := NewHub()
	hub.SetChannelServerResolver(func(channelID string) (string, bool) {
		switch channelID {
		case "101", "102":
			return "A", true
		case "201":
			return "B", true
		}
		return "", false
	})

	client := newTestClient(hub, "user1")
	hub.RegisterClient(client)

	for _, ch := range []string{"server:A", "server:B", "101", "102", "201", "dm:5"} {
		hub.SubscribeToChannel(ch, client)
	}

	hub.UnsubscribeUserFromServer("user1", "A")

	hub.mu.RLock()
	clientSubs := make(map[string]bool, len(hub.clientChannels[client]))
	for ch := range hub.clientChannels[client] {
		clientSubs[ch] = true
	}
	_, serverAInIndex := hub.channelSubs["server:A"][client]
	_, ch101InIndex := hub.channelSubs["101"][client]
	_, ch102InIndex := hub.channelSubs["102"][client]
	_, ch201InIndex := hub.channelSubs["201"][client]
	_, serverBInIndex := hub.channelSubs["server:B"][client]
	_, dmInIndex := hub.channelSubs["dm:5"][client]
	_, stillConnected := hub.clients[client]
	hub.mu.RUnlock()

	if clientSubs["server:A"] || serverAInIndex {
		t.Error("server:A subscription should have been dropped")
	}
	if clientSubs["101"] || ch101InIndex {
		t.Error("channel 101 (server A) subscription should have been dropped")
	}
	if clientSubs["102"] || ch102InIndex {
		t.Error("channel 102 (server A) subscription should have been dropped")
	}
	if !clientSubs["server:B"] || !serverBInIndex {
		t.Error("server:B subscription must be preserved")
	}
	if !clientSubs["201"] || !ch201InIndex {
		t.Error("channel 201 (server B) subscription must be preserved")
	}
	if !clientSubs["dm:5"] || !dmInIndex {
		t.Error("DM subscription must be preserved")
	}
	if !stillConnected {
		t.Error("UnsubscribeUserFromServer must not close the WS connection")
	}
}

// TestUnsubscribeUserFromServer_NoResolver verifies that without a resolver,
// only the "server:{id}" virtual channel is dropped — numeric channels
// cannot be matched to a server and are left alone (fail-safe).
func TestUnsubscribeUserFromServer_NoResolver(t *testing.T) {
	hub := NewHub()
	client := newTestClient(hub, "user1")
	hub.RegisterClient(client)

	for _, ch := range []string{"server:A", "101", "dm:5"} {
		hub.SubscribeToChannel(ch, client)
	}

	hub.UnsubscribeUserFromServer("user1", "A")

	hub.mu.RLock()
	_, serverAIn := hub.channelSubs["server:A"][client]
	_, ch101In := hub.channelSubs["101"][client]
	_, dmIn := hub.channelSubs["dm:5"][client]
	hub.mu.RUnlock()

	if serverAIn {
		t.Error("server:A should be dropped even without a resolver")
	}
	if !ch101In {
		t.Error("numeric channel should be left alone when no resolver is configured")
	}
	if !dmIn {
		t.Error("DM subscription must be preserved")
	}
}

// TestUnsubscribeUserFromServer_MultipleClients verifies the method walks
// all of a user's active WS connections (multi-tab / multi-device).
func TestUnsubscribeUserFromServer_MultipleClients(t *testing.T) {
	hub := NewHub()
	hub.SetChannelServerResolver(func(channelID string) (string, bool) {
		if channelID == "101" {
			return "A", true
		}
		return "", false
	})

	c1 := newTestClient(hub, "user1")
	c2 := newTestClient(hub, "user1")
	hub.RegisterClient(c1)
	hub.RegisterClient(c2)
	hub.SubscribeToChannel("server:A", c1)
	hub.SubscribeToChannel("101", c1)
	hub.SubscribeToChannel("server:A", c2)
	hub.SubscribeToChannel("101", c2)

	hub.UnsubscribeUserFromServer("user1", "A")

	hub.mu.RLock()
	_, c1HasServer := hub.channelSubs["server:A"][c1]
	_, c2HasServer := hub.channelSubs["server:A"][c2]
	_, c1Has101 := hub.channelSubs["101"][c1]
	_, c2Has101 := hub.channelSubs["101"][c2]
	hub.mu.RUnlock()

	if c1HasServer || c2HasServer || c1Has101 || c2Has101 {
		t.Error("all of user1's clients should be unsubscribed from server A channels")
	}
}
