package websocket

// This file exposes test-only helpers so that other packages (e.g. the server
// service) can drive the Hub in their own unit tests without standing up a
// real WebSocket connection. These are intentionally minimal wrappers over
// otherwise-private state — do NOT use them in production code paths.

// NewTestClient creates a Client with no underlying connection, suitable only
// for tests. Its send channel is buffered and can be drained with
// len(client.send) in the local test file. The hub's ReadPump / WritePump are
// never started against this client.
func NewTestClient(hub *Hub, userID string) *Client {
	return &Client{
		hub:    hub,
		send:   make(chan []byte, 1024),
		userID: userID,
	}
}

// ClientSubscribed reports whether the given client currently has a
// subscription to channelID. Safe for concurrent use; intended for tests.
func (h *Hub) ClientSubscribed(client *Client, channelID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.channelSubs[channelID][client]
	return ok
}
