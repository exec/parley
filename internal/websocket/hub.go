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
}

// NewHub creates a new Hub
func NewHub() *Hub {
	return &Hub{
		clients:      make(map[*Client]bool),
		userToClient: make(map[string]map[*Client]bool),
		channelSubs:  make(map[string]map[*Client]bool),
		register:     make(chan *Client),
		unregister:   make(chan *Client),
		broadcast:    make(chan *Message),
	}
}

// SetPublisher sets the cross-node publisher (e.g. RedisHub).
// Call this before starting the hub's Run loop.
func (h *Hub) SetPublisher(p Publisher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.publisher = p
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

// RegisterClient adds a client to the hub
func (h *Hub) RegisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[client] = true

	// Add to user map
	if h.userToClient[client.userID] == nil {
		h.userToClient[client.userID] = make(map[*Client]bool)
	}
	h.userToClient[client.userID][client] = true

	log.Printf("Client registered for user: %s", client.userID)
}

// UnregisterClient removes a client from the hub
func (h *Hub) UnregisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)

		// Remove from user map
		if h.userToClient[client.userID] != nil {
			delete(h.userToClient[client.userID], client)
			if len(h.userToClient[client.userID]) == 0 {
				delete(h.userToClient, client.userID)
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
}

// SubscribeToChannel adds a client to a channel's subscriber list
func (h *Hub) SubscribeToChannel(channelID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.channelSubs[channelID] == nil {
		h.channelSubs[channelID] = make(map[*Client]bool)
	}
	h.channelSubs[channelID][client] = true
	log.Printf("Client %s subscribed to channel: %s", client.userID, channelID)
}

// UnsubscribeFromChannel removes a client from a channel's subscriber list
func (h *Hub) UnsubscribeFromChannel(channelID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.channelSubs[channelID] != nil {
		delete(h.channelSubs[channelID], client)
		if len(h.channelSubs[channelID]) == 0 {
			delete(h.channelSubs, channelID)
		}
		log.Printf("Client %s unsubscribed from channel: %s", client.userID, channelID)
	}
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
			close(client.send)
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
			close(client.send)

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