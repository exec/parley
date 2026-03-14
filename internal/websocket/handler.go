package websocket

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow all origins in development; in production, configure appropriately
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Handler handles WebSocket connections
type Handler struct {
	hub *Hub
}

// NewHandler creates a new WebSocket handler
func NewHandler(hub *Hub) *Handler {
	return &Handler{
		hub: hub,
	}
}

// HandleWebSocket upgrades an HTTP connection to WebSocket
func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Extract token from query parameter or header
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("Authorization")
		// Remove "Bearer " prefix if present
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}
	}

	if token == "" {
		http.Error(w, "Missing authentication token", http.StatusUnauthorized)
		return
	}

	// Validate token and extract user ID
	// This is a placeholder - implement your own token validation logic
	userID, err := validateToken(token)
	if err != nil {
		log.Printf("Invalid token: %v", err)
		http.Error(w, "Invalid authentication token", http.StatusUnauthorized)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// Create client
	client := NewClient(h.hub, conn, userID)

	// Register client with hub
	h.hub.register <- client

	// Start pumps in goroutines
	go client.WritePump()
	go client.ReadPump()
}

// validateToken validates the token and returns the user ID
// This is a placeholder - implement your own token validation logic
func validateToken(token string) (string, error) {
	// Example implementation:
	// 1. Parse the JWT token
	// 2. Validate the signature
	// 3. Extract the user ID from claims
	// 4. Return the user ID

	// For now, we'll use the token directly as the user ID
	// Replace this with actual JWT validation
	if token == "" {
		return "", nil
	}

	// Placeholder: return token as user ID
	// In production, you would validate the JWT and extract the user ID
	return token, nil
}

// HandleWebSocket is a standalone function for handling WebSocket connections
// It uses a global hub instance if needed
func HandleWebSocket(hub *Hub, w http.ResponseWriter, r *http.Request) {
	// Extract token from query parameter or header
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("Authorization")
		// Remove "Bearer " prefix if present
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}
	}

	if token == "" {
		http.Error(w, "Missing authentication token", http.StatusUnauthorized)
		return
	}

	// Validate token and extract user ID
	userID, err := validateToken(token)
	if err != nil {
		log.Printf("Invalid token: %v", err)
		http.Error(w, "Invalid authentication token", http.StatusUnauthorized)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// Create client
	client := NewClient(hub, conn, userID)

	// Register client with hub
	hub.register <- client

	// Start pumps in goroutines
	go client.WritePump()
	go client.ReadPump()
}