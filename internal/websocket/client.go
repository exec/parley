package websocket

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 4096
)

// Client represents a WebSocket client
type Client struct {
	hub  *Hub
	conn *websocket.Conn

	// sendMu protects send and closed together so that close(send) and
	// send<-msg never execute concurrently.  RLock for sends, Lock for close.
	sendMu sync.RWMutex

	// Buffered channel of outbound messages
	send chan []byte

	// closed tracks whether send has been closed. Checked under sendMu.RLock.
	closed bool

	// closeOnce ensures client.send is only closed once, regardless of which
	// code path triggers the teardown (buffer-full eviction vs normal unregister).
	closeOnce sync.Once

	// User ID associated with this client
	userID string

	// displayName is the server-resolved display name for the user (COALESCE(display_name, username)).
	// It is set at connection time and never overwritten by client-supplied data.
	displayName string

	// wsMsgBucket is a token-bucket rate limiter for inbound WebSocket messages.
	// Zero value is safe: the IsZero check in allowWSMessage handles initialization.
	wsMsgBucket struct {
		mu       sync.Mutex
		tokens   float64
		lastSeen time.Time
	}
}

// NewClient creates a new client
func NewClient(hub *Hub, conn *websocket.Conn, userID string, displayName string) *Client {
	return &Client{
		hub:         hub,
		conn:        conn,
		send:        make(chan []byte, 1024),
		userID:      userID,
		displayName: displayName,
	}
}

// closeSend closes the send channel exactly once.
// It acquires a write lock on sendMu so that no concurrent safeSend is
// mid-send on the channel when we close it, eliminating the close-vs-send
// data race detected by -race.
func (c *Client) closeSend() {
	c.closeOnce.Do(func() {
		c.sendMu.Lock()
		c.closed = true
		close(c.send)
		c.sendMu.Unlock()
	})
}

// ReadPump reads messages from the WebSocket connection
func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Parse the incoming message
		var wsMsg WSMessage
		if err := json.Unmarshal(message, &wsMsg); err != nil {
			log.Printf("Error parsing message: %v", err)
			continue
		}

		// Enforce per-connection message rate limit (30 msg/s, burst 60).
		if !c.allowWSMessage() {
			log.Printf("ReadPump: user %s exceeded WS message rate limit, disconnecting", c.userID)
			return
		}

		// Handle different message types
		c.handleMessage(wsMsg)
	}
}

// handleMessage handles incoming WebSocket messages
func (c *Client) handleMessage(msg WSMessage) {
	switch msg.Type {
	case "CHANNEL_SUBSCRIBE":
		var payload struct {
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			log.Printf("CHANNEL_SUBSCRIBE from user %s: failed to parse payload: %v", c.userID, err)
			return
		}
		if payload.ChannelID == "" {
			log.Printf("CHANNEL_SUBSCRIBE from user %s: missing channel_id", c.userID)
			return
		}
		if !c.hub.CheckChannelAccess(c.userID, payload.ChannelID) {
			log.Printf("CHANNEL_SUBSCRIBE from user %s: access denied for channel %s", c.userID, payload.ChannelID)
			return
		}
		c.hub.SubscribeToChannel(payload.ChannelID, c)

	case "CHANNEL_UNSUBSCRIBE":
		var payload struct {
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			log.Printf("CHANNEL_UNSUBSCRIBE from user %s: failed to parse payload: %v", c.userID, err)
			return
		}
		if payload.ChannelID == "" {
			log.Printf("CHANNEL_UNSUBSCRIBE from user %s: missing channel_id", c.userID)
			return
		}
		c.hub.UnsubscribeFromChannel(payload.ChannelID, c)

	case "TYPING":
		var payload struct {
			ChannelID string `json:"channel_id"`
			Username  string `json:"username"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			log.Printf("TYPING from user %s: failed to parse payload: %v", c.userID, err)
			return
		}
		if payload.ChannelID == "" {
			return
		}
		if !c.hub.CheckChannelAccess(c.userID, payload.ChannelID) {
			log.Printf("TYPING from user %s: access denied for channel %s", c.userID, payload.ChannelID)
			return
		}
		// Build broadcast payload using server-side user ID and display name (not client-supplied)
		broadcastPayload, err := json.Marshal(map[string]string{
			"channel_id": payload.ChannelID,
			"user_id":    c.userID,
			"username":   c.displayName,
		})
		if err != nil {
			log.Printf("TYPING from user %s: failed to marshal broadcast payload: %v", c.userID, err)
			return
		}
		c.hub.BroadcastToChannel(payload.ChannelID, EventUserTyping, broadcastPayload)

	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}

// allowWSMessage implements a token-bucket rate limiter for inbound messages.
// It allows up to 30 messages/second with a burst of 60. Returns true if the
// message is within limits (consuming one token), false if the bucket is empty.
func (c *Client) allowWSMessage() bool {
	const rate = 30.0  // tokens per second
	const burst = 60.0
	c.wsMsgBucket.mu.Lock()
	defer c.wsMsgBucket.mu.Unlock()
	now := time.Now()
	if c.wsMsgBucket.lastSeen.IsZero() {
		c.wsMsgBucket.tokens = burst
		c.wsMsgBucket.lastSeen = now
	}
	elapsed := now.Sub(c.wsMsgBucket.lastSeen).Seconds()
	c.wsMsgBucket.tokens += elapsed * rate
	if c.wsMsgBucket.tokens > burst {
		c.wsMsgBucket.tokens = burst
	}
	c.wsMsgBucket.lastSeen = now
	if c.wsMsgBucket.tokens >= 1.0 {
		c.wsMsgBucket.tokens--
		return true
	}
	return false
}

// WritePump writes messages to the WebSocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

			// Flush any additional queued messages as separate frames.
			// No sendMu needed here: WritePump is the sole reader of c.send,
			// so there is no concurrent receive race. A concurrent closeSend()
			// closing the channel causes <-c.send to return the zero value safely.
			n := len(c.send)
			for i := 0; i < n; i++ {
				c.conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := c.conn.WriteMessage(websocket.TextMessage, <-c.send); err != nil {
					return
				}
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Send sends a message to the client.
// It routes through safeSend so that the sendMu read-lock is held during the
// channel send, preventing a close-vs-send data race with closeSend.
func (c *Client) Send(messageType string, payload []byte) error {
	wsMsg := WSMessage{
		Type:    messageType,
		Payload: payload,
	}

	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		return err
	}

	safeSend(c, msgBytes)
	return nil
}

// GetUserID returns the user ID associated with this client
func (c *Client) GetUserID() string {
	return c.userID
}