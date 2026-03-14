package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisPubSub provides Redis pub/sub functionality for cross-instance WebSocket communication
type RedisPubSub struct {
	client *redis.Client
	mu     sync.RWMutex
}

// NewRedisPubSub creates a new RedisPubSub instance
func NewRedisPubSub(redisURL string) (*RedisPubSub, error) {
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opt)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisPubSub{
		client: client,
	}, nil
}

// Publish publishes a message to a Redis channel
func (r *RedisPubSub) Publish(channel string, message interface{}) error {
	var data []byte
	var err error

	// Convert message to JSON if it's not already a string/[]byte
	switch msg := message.(type) {
	case string:
		data = []byte(msg)
	case []byte:
		data = msg
	default:
		data, err = json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}
	}

	ctx := context.Background()
	err = r.client.Publish(ctx, channel, data).Err()
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}

// Subscribe creates a new subscription to a Redis channel
func (r *RedisPubSub) Subscribe(channel string) *redis.PubSub {
	ctx := context.Background()
	return r.client.Subscribe(ctx, channel)
}

// StartSubscriber starts a background goroutine that listens to a channel
// and calls the handler for each received message
func (r *RedisPubSub) StartSubscriber(channel string, handler func(message []byte)) {
	go func() {
		// Create a new subscription
		pubsub := r.Subscribe(channel)
		ch := pubsub.Channel()

		log.Printf("Started Redis subscriber for channel: %s", channel)

		for msg := range ch {
			if msg == nil {
				continue
			}

			handler([]byte(msg.Payload))
		}

		log.Printf("Redis subscriber stopped for channel: %s", channel)
	}()
}

// Close closes the Redis client connection
func (r *RedisPubSub) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.client.Close()
}

// Client returns the underlying Redis client
func (r *RedisPubSub) Client() *redis.Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.client
}