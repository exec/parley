package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient wraps the Redis client for caching operations
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient creates a new Redis client using the REDIS_URL environment variable.
// Default URL is redis://localhost:6379 if REDIS_URL is not set.
func NewRedisClient() (*RedisClient, error) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisClient{client: client}, nil
}

// NewRedisClientWithURL creates a new Redis client with the given URL
func NewRedisClientWithURL(redisURL string) (*RedisClient, error) {
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisClient{client: client}, nil
}

// GetCached retrieves a string value from the cache
func (r *RedisClient) GetCached(key string) (string, error) {
	ctx := context.Background()
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get cached value: %w", err)
	}
	return val, nil
}

// SetCached stores a string value in the cache with an expiration time
func (r *RedisClient) SetCached(key string, value string, expiration time.Duration) error {
	ctx := context.Background()
	err := r.client.Set(ctx, key, value, expiration).Err()
	if err != nil {
		return fmt.Errorf("failed to set cached value: %w", err)
	}
	return nil
}

// DeleteCached removes a key from the cache
func (r *RedisClient) DeleteCached(key string) error {
	ctx := context.Background()
	err := r.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete cached value: %w", err)
	}
	return nil
}

// GetCachedJSON retrieves a JSON-encoded value from the cache and unmarshals it into dest
func (r *RedisClient) GetCachedJSON(key string, dest interface{}) error {
	ctx := context.Background()
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get cached JSON value: %w", err)
	}

	if err := json.Unmarshal([]byte(val), dest); err != nil {
		return fmt.Errorf("failed to unmarshal cached JSON: %w", err)
	}

	return nil
}

// SetCachedJSON marshals value to JSON and stores it in the cache with an expiration time
func (r *RedisClient) SetCachedJSON(key string, value interface{}, expiration time.Duration) error {
	ctx := context.Background()

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value to JSON: %w", err)
	}

	err = r.client.Set(ctx, key, data, expiration).Err()
	if err != nil {
		return fmt.Errorf("failed to set cached JSON value: %w", err)
	}

	return nil
}

// Close closes the Redis connection
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// Client returns the underlying Redis client
func (r *RedisClient) Client() *redis.Client {
	return r.client
}