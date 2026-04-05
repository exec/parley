package websocket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestSubscriptionLimitEnforced verifies that SubscribeToChannel silently
// drops subscriptions beyond maxSubscriptionsPerClient.
func TestSubscriptionLimitEnforced(t *testing.T) {
	hub := NewHub()
	c := newTestClient(hub, "user1")
	hub.RegisterClient(c)

	// Subscribe up to the limit
	for i := 0; i < maxSubscriptionsPerClient; i++ {
		hub.SubscribeToChannel(fmt.Sprintf("ch%d", i), c)
	}

	hub.mu.RLock()
	count := len(hub.clientChannels[c])
	hub.mu.RUnlock()

	if count != maxSubscriptionsPerClient {
		t.Fatalf("expected %d subscriptions, got %d", maxSubscriptionsPerClient, count)
	}

	// One more should be rejected
	hub.SubscribeToChannel("ch-overflow", c)

	hub.mu.RLock()
	countAfter := len(hub.clientChannels[c])
	_, inSubs := hub.channelSubs["ch-overflow"][c]
	hub.mu.RUnlock()

	if countAfter != maxSubscriptionsPerClient {
		t.Errorf("subscription count should remain %d, got %d", maxSubscriptionsPerClient, countAfter)
	}
	if inSubs {
		t.Error("overflow channel should not appear in channelSubs")
	}
}

// TestSubscriptionLimitIdempotentResub verifies that resubscribing to an
// already-subscribed channel does not count twice toward the limit.
func TestSubscriptionLimitIdempotentResub(t *testing.T) {
	hub := NewHub()
	c := newTestClient(hub, "user1")
	hub.RegisterClient(c)

	// Fill to limit-1
	for i := 0; i < maxSubscriptionsPerClient-1; i++ {
		hub.SubscribeToChannel(fmt.Sprintf("ch%d", i), c)
	}

	// Subscribe to ch0 again (already subscribed)
	hub.SubscribeToChannel("ch0", c)

	hub.mu.RLock()
	count := len(hub.clientChannels[c])
	hub.mu.RUnlock()

	// ch0 is a re-sub so count should still be maxSubscriptionsPerClient-1
	// (the map overwrites the existing key, no new entry created)
	if count != maxSubscriptionsPerClient-1 {
		t.Errorf("expected %d subscriptions after resub, got %d", maxSubscriptionsPerClient-1, count)
	}
}

// TestMaxConnectionsPerUserRejected verifies that RegisterClient rejects
// a connection when the user already has maxConnectionsPerUser connections,
// and that the rejected client's send channel is closed.
func TestMaxConnectionsPerUserRejected(t *testing.T) {
	hub := NewHub()

	// We need a real WS connection for the rejected client because
	// RegisterClient calls client.conn.Close() on rejection. Set up a
	// minimal WebSocket server.
	upgrader := websocket.Upgrader{}
	serverConns := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConns <- conn
	}))
	defer srv.Close()

	// Register maxConnectionsPerUser clients (no real conn needed for these)
	for i := 0; i < maxConnectionsPerUser; i++ {
		c := newTestClient(hub, "user1")
		hub.RegisterClient(c)
	}

	hub.mu.RLock()
	countBefore := len(hub.userToClient["user1"])
	hub.mu.RUnlock()

	if countBefore != maxConnectionsPerUser {
		t.Fatalf("expected %d connections, got %d", maxConnectionsPerUser, countBefore)
	}

	// Dial a real WS connection for the client that will be rejected
	wsURL := "ws" + srv.URL[len("http"):]
	dialer := websocket.Dialer{
		HandshakeTimeout: 2 * time.Second,
	}
	clientConn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	// Wait for server-side connection
	serverConn := <-serverConns
	defer serverConn.Close()

	rejected := &Client{
		hub:    hub,
		conn:   serverConn,
		send:   make(chan []byte, 1024),
		userID: "user1",
	}

	hub.RegisterClient(rejected)

	// The rejected client should have its send channel closed
	select {
	case _, ok := <-rejected.send:
		if ok {
			t.Error("expected send channel to be closed for rejected client")
		}
	default:
		t.Error("send channel should be closed (not blocking)")
	}

	// The connection count should not have increased
	hub.mu.RLock()
	countAfter := len(hub.userToClient["user1"])
	hub.mu.RUnlock()

	if countAfter != maxConnectionsPerUser {
		t.Errorf("expected %d connections after rejection, got %d", maxConnectionsPerUser, countAfter)
	}

	// Also verify rejected client is not in hub.clients
	hub.mu.RLock()
	_, inClients := hub.clients[rejected]
	hub.mu.RUnlock()
	if inClients {
		t.Error("rejected client should not be in hub.clients")
	}

	// Clean up the real WS connection
	clientConn.Close()
}

// TestMaxConnectionsPerUserAllowsDifferentUsers verifies that different users
// can each have up to maxConnectionsPerUser connections independently.
func TestMaxConnectionsPerUserAllowsDifferentUsers(t *testing.T) {
	hub := NewHub()

	// Fill user1 to the limit
	for i := 0; i < maxConnectionsPerUser; i++ {
		c := newTestClient(hub, "user1")
		hub.RegisterClient(c)
	}

	// user2 should still be able to connect
	c2 := newTestClient(hub, "user2")
	hub.RegisterClient(c2)

	hub.mu.RLock()
	u1Count := len(hub.userToClient["user1"])
	u2Count := len(hub.userToClient["user2"])
	hub.mu.RUnlock()

	if u1Count != maxConnectionsPerUser {
		t.Errorf("user1 should have %d connections, got %d", maxConnectionsPerUser, u1Count)
	}
	if u2Count != 1 {
		t.Errorf("user2 should have 1 connection, got %d", u2Count)
	}
}

// TestChannelAccessCheckerFailsClosed verifies that CheckChannelAccess returns
// false when no checker is set (fail-closed behavior).
func TestChannelAccessCheckerFailsClosed(t *testing.T) {
	hub := NewHub()
	if hub.CheckChannelAccess("user1", "ch1") {
		t.Error("should return false when no checker is configured")
	}
}

// TestChannelAccessCheckerDelegates verifies that CheckChannelAccess delegates
// to the configured function.
func TestChannelAccessCheckerDelegates(t *testing.T) {
	hub := NewHub()
	hub.SetChannelAccessChecker(func(userID, channelID string) bool {
		return userID == "allowed" && channelID == "ch1"
	})

	if !hub.CheckChannelAccess("allowed", "ch1") {
		t.Error("should return true for allowed user+channel")
	}
	if hub.CheckChannelAccess("denied", "ch1") {
		t.Error("should return false for denied user")
	}
	if hub.CheckChannelAccess("allowed", "ch2") {
		t.Error("should return false for wrong channel")
	}
}

// drainSend removes all pending messages from a client's send channel.
// It stops when the channel is empty (buffer drained) or closed (all buffered
// messages consumed). Safe for both open and closed channels.
func drainSend(c *Client) {
	for {
		select {
		case _, ok := <-c.send:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

// TestSendToUserDeliversToAllConnections verifies that SendToUser delivers
// to every connection of a multi-connection user.
func TestSendToUserDeliversToAllConnections(t *testing.T) {
	hub := NewHub()

	clients := make([]*Client, 3)
	for i := range clients {
		clients[i] = newTestClient(hub, "user1")
		hub.RegisterClient(clients[i])
	}

	// Drain PRESENCE_SNAPSHOT and USER_ONLINE messages from registration
	for _, c := range clients {
		drainSend(c)
	}

	if err := hub.SendToUser("user1", "TEST", []byte(`{"msg":"hi"}`)); err != nil {
		t.Fatalf("SendToUser: %v", err)
	}

	for i, c := range clients {
		select {
		case msg := <-c.send:
			var parsed WSMessage
			if err := json.Unmarshal(msg, &parsed); err != nil {
				t.Errorf("client %d: unmarshal: %v", i, err)
			}
			if parsed.Type != "TEST" {
				t.Errorf("client %d: type=%q, want TEST", i, parsed.Type)
			}
		default:
			t.Errorf("client %d: no message received", i)
		}
	}
}

// TestDisconnectUserClosesAllSendChannels verifies that DisconnectUser closes
// the send channel for every connection of a user.
func TestDisconnectUserClosesAllSendChannels(t *testing.T) {
	hub := NewHub()

	clients := make([]*Client, 3)
	for i := range clients {
		clients[i] = newTestClient(hub, "user1")
		hub.RegisterClient(clients[i])
	}

	hub.DisconnectUser("user1")

	for i, c := range clients {
		// Drain any buffered messages (PRESENCE_SNAPSHOT, USER_ONLINE)
		drainSend(c)
		// After draining, the next read should see a closed channel
		select {
		case _, ok := <-c.send:
			if ok {
				t.Errorf("client %d: send channel should be closed", i)
			}
		default:
			t.Errorf("client %d: send channel should be closed (not blocking)", i)
		}
	}
}

// TestBroadcastStatusUpdateDeliversToAll verifies that BroadcastStatusUpdate
// sends a USER_STATUS_UPDATE event to all connected clients.
func TestBroadcastStatusUpdateDeliversToAll(t *testing.T) {
	hub := NewHub()

	c1 := newTestClient(hub, "user1")
	c2 := newTestClient(hub, "user2")
	hub.RegisterClient(c1)
	hub.RegisterClient(c2)

	// Drain PRESENCE_SNAPSHOT messages
	drainSend(c1)
	drainSend(c2)

	hub.BroadcastStatusUpdate("user1", "idle", "brb")

	for _, c := range []*Client{c1, c2} {
		select {
		case msg := <-c.send:
			var parsed struct {
				Type    string `json:"type"`
				Payload struct {
					UserID     string `json:"user_id"`
					StatusType string `json:"status_type"`
					StatusText string `json:"status_text"`
				} `json:"payload"`
			}
			if err := json.Unmarshal(msg, &parsed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if parsed.Type != EventUserStatusUpdate {
				t.Errorf("type=%q, want %q", parsed.Type, EventUserStatusUpdate)
			}
			if parsed.Payload.UserID != "user1" {
				t.Errorf("user_id=%q, want user1", parsed.Payload.UserID)
			}
		default:
			t.Error("no status update received")
		}
	}
}
