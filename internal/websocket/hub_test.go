package websocket

import (
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
	ch := make(chan []byte, 4)
	close(ch)

	// Must not panic
	sent := safeSend(ch, []byte("hello"))
	if sent {
		t.Error("safeSend should return false for closed channel")
	}
}

func TestSafeSendToFullChannel(t *testing.T) {
	ch := make(chan []byte, 1)
	ch <- []byte("full")

	sent := safeSend(ch, []byte("overflow"))
	if sent {
		t.Error("safeSend should return false for full channel")
	}
}

// Ensure fmt and sync are used so Task 2's tests can extend this file.
var _ = fmt.Sprintf
var _ sync.Mutex
