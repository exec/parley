package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
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

	// If no password in URL but REDIS_PASSWORD env var is set, use that
	if opt.Password == "" {
		if password := os.Getenv("REDIS_PASSWORD"); password != "" {
			opt.Password = password
		}
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

// StartSubscriber starts a background goroutine that listens to a Redis channel
// and calls handler for each message. It reconnects automatically on failure
// using exponential backoff capped at 30s.
func (r *RedisPubSub) StartSubscriber(channel string, handler func(message []byte)) {
	go func() {
		backoff := time.Second
		for {
			pubsub := r.client.Subscribe(context.Background(), channel)
			ch := pubsub.Channel()
			log.Printf("Redis: subscribed to %s", channel)
			backoff = time.Second // reset on successful connect

			for msg := range ch {
				if msg == nil {
					continue
				}
				handler([]byte(msg.Payload))
			}

			// Channel closed — connection dropped or Redis restarted.
			pubsub.Close()
			log.Printf("Redis: subscription to %s lost, reconnecting in %s", channel, backoff)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}()

	// Periodic ping to detect silent connection failures quickly.
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := r.client.Ping(ctx).Err(); err != nil {
				log.Printf("Redis: ping failed: %v", err)
			}
			cancel()
		}
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