package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSEvent is a message received from the server.
type WSEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// WSClient manages a single WebSocket connection to Parley.
type WSClient struct {
	conn      *websocket.Conn
	events    chan WSEvent
	done      chan struct{}
	closeOnce sync.Once
	writeMu   sync.Mutex
}

const (
	wsWriteTimeout = 10 * time.Second
	wsPongWait     = 60 * time.Second
	wsPingInterval = 54 * time.Second
)

// NewWSClient connects to the Parley WebSocket endpoint using a pre-fetched ticket.
func NewWSClient(ctx context.Context, host, ticket string) (*WSClient, error) {
	wsURL := toWSURL(host) + "/ws?ticket=" + ticket

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ws dial: %w", err)
	}

	wsc := &WSClient{
		conn:   conn,
		events: make(chan WSEvent, 256),
		done:   make(chan struct{}),
	}

	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(wsPongWait))
		return nil
	})

	go wsc.readLoop()
	go wsc.pingLoop()

	return wsc, nil
}

func (c *WSClient) readLoop() {
	defer c.Close()
	c.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		c.conn.SetReadDeadline(time.Now().Add(wsPongWait))
		var ev WSEvent
		if err := json.Unmarshal(msg, &ev); err != nil {
			continue
		}
		select {
		case c.events <- ev:
		default:
			// Drop event if consumer is slow — bench tool may not consume all events.
		}
	}
}

func (c *WSClient) pingLoop() {
	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.writeMu.Lock()
			c.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
			err := c.conn.WriteMessage(websocket.PingMessage, nil)
			c.writeMu.Unlock()
			if err != nil {
				c.Close()
				return
			}
		}
	}
}

// Events returns the channel of incoming events.
func (c *WSClient) Events() <-chan WSEvent { return c.events }

// Done returns a channel that is closed when the WS connection closes.
func (c *WSClient) Done() <-chan struct{} { return c.done }

// Subscribe sends a CHANNEL_SUBSCRIBE message.
func (c *WSClient) Subscribe(channelID int64) error {
	select {
	case <-c.done:
		return fmt.Errorf("ws connection is closed")
	default:
	}
	msg, _ := json.Marshal(map[string]any{
		"type": "CHANNEL_SUBSCRIBE",
		"payload": map[string]string{
			"channel_id": fmt.Sprintf("%d", channelID),
		},
	})
	c.writeMu.Lock()
	c.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
	err := c.conn.WriteMessage(websocket.TextMessage, msg)
	c.writeMu.Unlock()
	return err
}

// SendTyping sends a TYPING event.
func (c *WSClient) SendTyping(channelID int64) error {
	select {
	case <-c.done:
		return fmt.Errorf("ws connection is closed")
	default:
	}
	msg, _ := json.Marshal(map[string]any{
		"type": "TYPING",
		"payload": map[string]string{
			"channel_id": fmt.Sprintf("%d", channelID),
		},
	})
	c.writeMu.Lock()
	c.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
	err := c.conn.WriteMessage(websocket.TextMessage, msg)
	c.writeMu.Unlock()
	return err
}

// Close closes the WebSocket connection.
func (c *WSClient) Close() {
	c.closeOnce.Do(func() {
		c.conn.Close()
		close(c.done)
	})
}

func toWSURL(host string) string {
	host = strings.Replace(host, "https://", "wss://", 1)
	host = strings.Replace(host, "http://", "ws://", 1)
	return host
}
