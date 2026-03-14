package websocket

import "encoding/json"

// WSMessage represents a WebSocket message structure
type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Message is the internal message struct used for broadcasting
type Message struct {
	Type      string
	ChannelID string
	Payload   []byte
}