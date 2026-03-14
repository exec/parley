package websocket

import (
	"encoding/json"
	"log"
)

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
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
}

// NewHub creates a new Hub
func NewHub() *Hub {
	return &Hub{
		clients:        make(map[*Client]bool),
		userToClient:   make(map[string]map[*Client]bool),
		channelSubs:   make(map[string]map[*Client]bool),
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		broadcast:      make(chan *Message),
	}
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
	if h.channelSubs[channelID] == nil {
		h.channelSubs[channelID] = make(map[*Client]bool)
	}
	h.channelSubs[channelID][client] = true
	log.Printf("Client %s subscribed to channel: %s", client.userID, channelID)
}

// UnsubscribeFromChannel removes a client from a channel's subscriber list
func (h *Hub) UnsubscribeFromChannel(channelID string, client *Client) {
	if h.channelSubs[channelID] != nil {
		delete(h.channelSubs[channelID], client)
		if len(h.channelSubs[channelID]) == 0 {
			delete(h.channelSubs, channelID)
		}
		log.Printf("Client %s unsubscribed from channel: %s", client.userID, channelID)
	}
}

// SendToUser sends a message to a specific user by their userID
func (h *Hub) SendToUser(userID string, messageType string, payload []byte) error {
	clients := h.userToClient[userID]
	if clients == nil || len(clients) == 0 {
		return nil // No clients found for user, not an error
	}

	// Create WSMessage
	wsMsg := WSMessage{
		Type:    messageType,
		Payload: payload,
	}

	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
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

	return nil
}

// BroadcastToChannel sends a message to all clients subscribed to a channel
func (h *Hub) BroadcastToChannel(channelID string, messageType string, payload []byte) {
	clients := h.channelSubs[channelID]
	if clients == nil || len(clients) == 0 {
		return
	}

	// Create WSMessage
	wsMsg := WSMessage{
		Type:    messageType,
		Payload: payload,
	}

	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
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
}