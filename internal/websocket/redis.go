package websocket

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
)

const redisChannel = "parley:events"

// RedisEnvelope is the message envelope published to Redis
type RedisEnvelope struct {
	NodeID    string          `json:"node_id"`
	EventType string          `json:"event_type"` // "channel" or "user"
	ChannelID string          `json:"channel_id,omitempty"`
	UserID    string          `json:"user_id,omitempty"`
	Event     string          `json:"event"`
	Data      json.RawMessage `json:"data"`
}

// RedisHub wraps Hub and adds Redis pub/sub for cross-node broadcasting
type RedisHub struct {
	hub    *Hub
	pubsub *RedisPubSub
	nodeID string
}

// newNodeID generates a random node identifier.
func newNodeID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// NewRedisHub creates a RedisHub. If Redis is unavailable it returns nil and logs a warning.
func NewRedisHub(hub *Hub) *RedisHub {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.Printf("WARNING: REDIS_URL not set, falling back to redis://localhost:6379 — cross-node broadcasting will silently fail if Redis is not running locally")
		redisURL = "redis://localhost:6379"
	}

	nodeID := newNodeID()

	pubsub, err := NewRedisPubSub(redisURL)
	if err != nil {
		log.Printf("WARNING: Redis unavailable (%v) — falling back to local-only WebSocket broadcasting", err)
		return nil
	}

	log.Printf("Connected to Redis for cross-node WebSocket broadcasting (node ID: %s)", nodeID)

	return &RedisHub{
		hub:    hub,
		pubsub: pubsub,
		nodeID: nodeID,
	}
}

// Subscribe starts the background goroutine that receives events from Redis
// and dispatches them to local clients.
func (r *RedisHub) Subscribe() {
	r.pubsub.StartSubscriber(redisChannel, func(message []byte) {
		var env RedisEnvelope
		if err := json.Unmarshal(message, &env); err != nil {
			log.Printf("RedisHub: failed to unmarshal envelope: %v", err)
			return
		}

		// Skip events we published ourselves to avoid duplicates
		if env.NodeID == r.nodeID {
			return
		}

		// Deliver to local clients ONLY — never re-publish to Redis.
		// Routing through hub.broadcast → BroadcastToChannel would re-publish
		// the event to Redis, causing every node to echo it back indefinitely.
		switch env.EventType {
		case "channel":
			r.hub.BroadcastLocalToChannel(env.ChannelID, env.Event, []byte(env.Data))
		case "user":
			r.hub.SendLocalToUser(env.UserID, env.Event, []byte(env.Data))
		default:
			log.Printf("RedisHub: unknown event_type %q, dropping", env.EventType)
		}
	})
}

// PublishToChannel publishes a channel event to Redis so other nodes receive it.
func (r *RedisHub) PublishToChannel(channelID, event string, data []byte) {
	env := RedisEnvelope{
		NodeID:    r.nodeID,
		EventType: "channel",
		ChannelID: channelID,
		Event:     event,
		Data:      json.RawMessage(data),
	}
	if err := r.pubsub.Publish(redisChannel, env); err != nil {
		log.Printf("RedisHub: failed to publish channel event: %v", err)
	}
}

// PublishToUser publishes a user-directed event to Redis so other nodes receive it.
func (r *RedisHub) PublishToUser(userID, event string, data []byte) {
	env := RedisEnvelope{
		NodeID:    r.nodeID,
		EventType: "user",
		UserID:    userID,
		Event:     event,
		Data:      json.RawMessage(data),
	}
	if err := r.pubsub.Publish(redisChannel, env); err != nil {
		log.Printf("RedisHub: failed to publish user event: %v", err)
	}
}

// Close shuts down the Redis connection.
func (r *RedisHub) Close() error {
	return r.pubsub.Close()
}
