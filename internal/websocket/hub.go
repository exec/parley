package websocket

import (
	"encoding/json"
	"log"
	"sync"
)

// Publisher is implemented by RedisHub to cross-publish events to other nodes.
// Hub holds an optional reference to it.
type Publisher interface {
	PublishToChannel(channelID, event string, data []byte)
	PublishToUser(userID, event string, data []byte)
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	mu sync.RWMutex

	// Registered clients
	clients map[*Client]bool

	// User ID to client mapping (one user can have multiple connections)
	userToClient map[string]map[*Client]bool

	// Channel subscribers
	channelSubs map[string]map[*Client]bool

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Broadcast messages to clients
	broadcast chan *Message

	// publisher is optional; if set, events are also published cross-node via Redis
	publisher Publisher

	// channelAccessChecker is an optional function to verify whether a user is
	// allowed to subscribe to a channel. If nil, access is denied (fail closed).
	channelAccessChecker func(userID, channelID string) bool
}

// NewHub creates a new Hub
func NewHub() *Hub {
	return &Hub{
		clients:      make(map[*Client]bool),
		userToClient: make(map[string]map[*Client]bool),
		channelSubs:  make(map[string]map[*Client]bool),
		register:     make(chan *Client, 64),
		unregister:   make(chan *Client, 64),
		broadcast:    make(chan *Message, 1024),
	}
}

// SetPublisher sets the cross-node publisher (e.g. RedisHub).
// Call this before starting the hub's Run loop.
func (h *Hub) SetPublisher(p Publisher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.publisher = p
}

// SetChannelAccessChecker sets the function used to decide whether a user may
// subscribe to a given channel. Call this before starting the hub's Run loop.
func (h *Hub) SetChannelAccessChecker(fn func(userID, channelID string) bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.channelAccessChecker = fn
}

// CheckChannelAccess returns true only if the channelAccessChecker confirms the
// user has access. Fails closed (returns false) when no checker is configured.
func (h *Hub) CheckChannelAccess(userID, channelID string) bool {
	h.mu.RLock()
	fn := h.channelAccessChecker
	h.mu.RUnlock()
	if fn == nil {
		return false
	}
	return fn(userID, channelID)
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.RegisterClient(client)

		case client := <-h.unregister:
			h.UnregisterClient(client)

		case message := <-h.broadcast:
			h.BroadcastToChannel(message.ChannelID, message.Type, message.Payload)
		}
	}
}

// RegisterClient adds a client to the hub, broadcasts USER_ONLINE globally,
// and sends a PRESENCE_SNAPSHOT of all online users to the new client.
func (h *Hub) RegisterClient(client *Client) {
	h.mu.Lock()

	h.clients[client] = true

	// Add to user map
	if h.userToClient[client.userID] == nil {
		h.userToClient[client.userID] = make(map[*Client]bool)
	}
	isFirstConnection := len(h.userToClient[client.userID]) == 0
	h.userToClient[client.userID][client] = true

	// Collect all unique online user IDs for the snapshot
	seen := make(map[string]bool)
	onlineUserIDs := make([]string, 0, len(h.userToClient))
	for uid := range h.userToClient {
		if !seen[uid] {
			seen[uid] = true
			onlineUserIDs = append(onlineUserIDs, uid)
		}
	}

	h.mu.Unlock()

	// Send the new client a snapshot of everyone currently online
	if snapshotPayload, err := json.Marshal(map[string]interface{}{
		"user_ids": onlineUserIDs,
	}); err == nil {
		client.Send(EventPresenceSnapshot, snapshotPayload)
	}

	// Announce arrival globally (only once per user, not once per tab/connection)
	if isFirstConnection {
		if onlinePayload, err := json.Marshal(map[string]string{
			"user_id": client.userID,
		}); err == nil {
			h.broadcastToAllLocal(EventUserOnline, onlinePayload)
		}
	}

	log.Printf("Client registered for user: %s", client.userID)
}

// UnregisterClient removes a client from the hub and broadcasts USER_OFFLINE
// globally when the user has no remaining connections.
func (h *Hub) UnregisterClient(client *Client) {
	h.mu.Lock()

	userFullyOffline := false

	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		client.closeSend()

		// Remove from user map
		if h.userToClient[client.userID] != nil {
			delete(h.userToClient[client.userID], client)
			if len(h.userToClient[client.userID]) == 0 {
				delete(h.userToClient, client.userID)
				userFullyOffline = true
			}
		}

		// Remove from all channel subscriptions
		for channelID, clients := range h.channelSubs {
			if _, ok := clients[client]; ok {
				delete(h.channelSubs[channelID], client)
				if len(h.channelSubs[channelID]) == 0 {
					delete(h.channelSubs, channelID)
				}
			}
		}

		log.Printf("Client unregistered for user: %s", client.userID)
	}

	h.mu.Unlock()

	// Broadcast USER_OFFLINE globally only when the user has no remaining connections.
	if userFullyOffline {
		if offlinePayload, err := json.Marshal(map[string]string{
			"user_id": client.userID,
		}); err == nil {
			h.broadcastToAllLocal(EventUserOffline, offlinePayload)
		}
	}
}

// SubscribeToChannel adds a client to a channel's subscriber list.
// Presence events are now handled globally (on connect/disconnect), not per-channel.
func (h *Hub) SubscribeToChannel(channelID string, client *Client) {
	h.mu.Lock()

	if h.channelSubs[channelID] == nil {
		h.channelSubs[channelID] = make(map[*Client]bool)
	}
	h.channelSubs[channelID][client] = true

	h.mu.Unlock()

	log.Printf("Client %s subscribed to channel: %s", client.userID, channelID)
}

// UnsubscribeFromChannel removes a client from a channel's subscriber list.
// Presence events are now handled globally (on connect/disconnect), not per-channel.
func (h *Hub) UnsubscribeFromChannel(channelID string, client *Client) {
	h.mu.Lock()

	if h.channelSubs[channelID] != nil {
		delete(h.channelSubs[channelID], client)
		if len(h.channelSubs[channelID]) == 0 {
			delete(h.channelSubs, channelID)
		}
	}

	h.mu.Unlock()

	log.Printf("Client %s unsubscribed from channel: %s", client.userID, channelID)
}

// SendToUser sends a message to a specific user by their userID.
// It also publishes to Redis (if a publisher is set) so other nodes deliver it too.
func (h *Hub) SendToUser(userID string, messageType string, payload []byte) error {
	h.mu.Lock()

	// Capture publisher reference while holding lock, then release before calling it
	pub := h.publisher

	clients := h.userToClient[userID]
	if clients == nil || len(clients) == 0 {
		h.mu.Unlock()
		// Still publish cross-node — the user may be on a different node
		if pub != nil {
			pub.PublishToUser(userID, messageType, payload)
		}
		return nil
	}

	// Create WSMessage
	wsMsg := WSMessage{
		Type:    messageType,
		Payload: payload,
	}

	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		h.mu.Unlock()
		return err
	}

	// Send to all connected clients for this user
	for client := range clients {
		select {
		case client.send <- msgBytes:
		default:
			// Client's send buffer is full, close the connection
			delete(h.clients, client)
			client.closeSend()
			delete(h.userToClient[userID], client)
		}
	}

	h.mu.Unlock()

	// Publish cross-node so other nodes can deliver to their local clients
	if pub != nil {
		pub.PublishToUser(userID, messageType, payload)
	}

	return nil
}

// DisconnectUser closes all WebSocket connections for the given user.
// The natural teardown chain (WritePump exit → conn close → ReadPump unregister)
// handles map cleanup, so we only need to close the send channels here.
func (h *Hub) DisconnectUser(userID string) {
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.userToClient[userID]))
	for c := range h.userToClient[userID] {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		c.closeSend()
	}
}

// broadcastToAllLocal sends a message to every locally-connected client.
// Used for global presence events (USER_ONLINE / USER_OFFLINE).
func (h *Hub) broadcastToAllLocal(messageType string, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	wsMsg := WSMessage{Type: messageType, Payload: payload}
	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		return
	}

	for client := range h.clients {
		select {
		case client.send <- msgBytes:
		default:
			delete(h.clients, client)
			client.closeSend()
			if h.userToClient[client.userID] != nil {
				delete(h.userToClient[client.userID], client)
			}
		}
	}
}

// BroadcastLocalToChannel sends to local clients subscribed to a channel ONLY.
// No Redis publish — use this when delivering events received from Redis to avoid
// the infinite re-broadcast loop that would occur if we published back to Redis.
func (h *Hub) BroadcastLocalToChannel(channelID string, messageType string, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	clients := h.channelSubs[channelID]
	if clients == nil || len(clients) == 0 {
		return
	}

	wsMsg := WSMessage{Type: messageType, Payload: payload}
	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		log.Printf("BroadcastLocalToChannel: marshal error: %v", err)
		return
	}

	for client := range clients {
		select {
		case client.send <- msgBytes:
		default:
			delete(h.clients, client)
			client.closeSend()
			if h.userToClient[client.userID] != nil {
				delete(h.userToClient[client.userID], client)
			}
			delete(h.channelSubs[channelID], client)
		}
	}
}

// SendLocalToUser delivers to local clients for a user ONLY — no Redis publish.
func (h *Hub) SendLocalToUser(userID string, messageType string, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	clients := h.userToClient[userID]
	if clients == nil || len(clients) == 0 {
		return
	}

	wsMsg := WSMessage{Type: messageType, Payload: payload}
	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		return
	}

	for client := range clients {
		select {
		case client.send <- msgBytes:
		default:
			delete(h.clients, client)
			client.closeSend()
			delete(h.userToClient[userID], client)
		}
	}
}

// BroadcastToChannel sends a message to all clients subscribed to a channel.
// It also publishes to Redis (if a publisher is set) so other nodes deliver it too.
func (h *Hub) BroadcastToChannel(channelID string, messageType string, payload []byte) {
	h.mu.Lock()

	// Capture publisher reference while holding lock, then release before calling it
	pub := h.publisher

	clients := h.channelSubs[channelID]
	if clients == nil || len(clients) == 0 {
		h.mu.Unlock()
		// Still publish cross-node — subscribers may be on other nodes
		if pub != nil {
			pub.PublishToChannel(channelID, messageType, payload)
		}
		return
	}

	// Create WSMessage
	wsMsg := WSMessage{
		Type:    messageType,
		Payload: payload,
	}

	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		h.mu.Unlock()
		log.Printf("Error marshaling broadcast message: %v", err)
		return
	}

	// Send to all subscribed clients
	for client := range clients {
		select {
		case client.send <- msgBytes:
		default:
			// Client's send buffer is full, close the connection
			delete(h.clients, client)
			client.closeSend()

			// Remove from user map
			if h.userToClient[client.userID] != nil {
				delete(h.userToClient[client.userID], client)
			}

			delete(h.channelSubs[channelID], client)
		}
	}

	h.mu.Unlock()

	// Publish cross-node so other nodes can deliver to their local channel subscribers
	if pub != nil {
		pub.PublishToChannel(channelID, messageType, payload)
	}
}