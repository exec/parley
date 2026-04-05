package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"parley/internal/auth"
)

// ----- Rate limiter interface -----

// rateLimiterI is implemented by both the in-memory token bucket (dev/fallback)
// and the Redis fixed-window limiter (production, shared across all API nodes).
type rateLimiterI interface {
	Allow(key string) bool
}

// ----- Redis fixed-window rate limiter (production) -----
//
// Uses INCR + EXPIRE in a pipeline for atomic, cross-node limiting.
// Fixed window: counts requests in [now - window, now]. On Redis error,
// fails open so Redis unavailability does not block legitimate traffic.

type redisRateLimiter struct {
	rdb    *goredis.Client
	limit  int
	window time.Duration
}

func newRedisRateLimiter(rdb *goredis.Client, limit int, window time.Duration) *redisRateLimiter {
	return &redisRateLimiter{rdb: rdb, limit: limit, window: window}
}

func (r *redisRateLimiter) Allow(key string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	// Fixed-window bucket: one counter per IP per window slot.
	// Window slot = unix seconds / window_seconds (integer division = floor).
	windowSec := int64(r.window.Seconds())
	if windowSec < 1 {
		windowSec = 1
	}
	bucket := fmt.Sprintf("parley:rl:%s:%d", key, time.Now().Unix()/windowSec)

	pipe := r.rdb.TxPipeline()
	incr := pipe.Incr(ctx, bucket)
	// TTL is 2× window so the key expires well after the window closes.
	pipe.Expire(ctx, bucket, r.window*2)
	if _, err := pipe.Exec(ctx); err != nil {
		// Fail open: if Redis is down, don't block users.
		return true
	}

	return incr.Val() <= int64(r.limit)
}

// ----- In-memory token bucket rate limiter (dev / Redis unavailable fallback) -----
//
// Token bucket: allows burst up to `burst` requests, then refills at
// `rate` requests per second. O(1) per Allow call, O(1) memory per key.
//
// Sharded across 64 independent buckets to reduce lock contention under
// high concurrency. Different IP/user keys almost never share a shard.

const numShards = 64

type tokenBucket struct {
	tokens   float64
	lastSeen time.Time
}

type rateShard struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
}

type rateLimiter struct {
	shards [numShards]rateShard
	rate   float64 // tokens per second
	burst  float64 // maximum token capacity
	done   chan struct{}
}

// newRateLimiter creates a token bucket rate limiter equivalent to limit requests
// per window. burst = limit, rate = limit/window in tokens/second.
func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		rate:  float64(limit) / window.Seconds(),
		burst: float64(limit),
		done:  make(chan struct{}),
	}
	for i := range rl.shards {
		rl.shards[i].buckets = make(map[string]*tokenBucket)
	}
	go rl.cleanup()
	return rl
}

// shard returns the rateShard responsible for key using an FNV-1a hash.
func (rl *rateLimiter) shard(key string) *rateShard {
	h := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return &rl.shards[h%numShards]
}

// Allow returns true if the key has a token available.
func (rl *rateLimiter) Allow(key string) bool {
	s := rl.shard(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	b, ok := s.buckets[key]
	if !ok {
		b = &tokenBucket{tokens: rl.burst, lastSeen: now}
		s.buckets[key] = b
	}

	// Refill tokens based on elapsed time.
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.lastSeen = now

	if b.tokens >= 1.0 {
		b.tokens--
		return true
	}
	return false
}

// cleanup removes stale buckets every 5 minutes. It exits when rl.done is closed.
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-rl.done:
			return
		case <-ticker.C:
			threshold := time.Now().Add(-10 * time.Minute)
			for i := range rl.shards {
				s := &rl.shards[i]
				s.mu.Lock()
				for key, b := range s.buckets {
					if b.lastSeen.Before(threshold) {
						delete(s.buckets, key)
					}
				}
				s.mu.Unlock()
			}
		}
	}
}

// Stop signals the cleanup goroutine to exit.
func (rl *rateLimiter) Stop() {
	close(rl.done)
}

// ----- Middleware helpers -----

// rateLimitMiddleware rejects requests that exceed the limiter's threshold.
// It uses r.RemoteAddr exclusively to avoid trusting client-supplied headers.
// When nginx sits in front, it overwrites X-Real-IP before forwarding, and
// Chi's RealIP middleware has already copied that trusted value into RemoteAddr.
func rateLimitMiddleware(rl rateLimiterI) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if ip == "" {
				ip = r.RemoteAddr
			}
			if !rl.Allow(ip) {
				http.Error(w, "rate limit exceeded, please slow down", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// userRateLimitMiddleware applies per-authenticated-user rate limiting.
// It uses the user ID from the JWT context as the bucket key so the limit
// applies per account regardless of IP (defeats multi-IP bypass attempts).
func userRateLimitMiddleware(rl rateLimiterI) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use authenticated user ID as the rate limit key so the limit applies
			// per account regardless of IP (defeats multi-IP bypass attempts).
			key := auth.GetUserIDFromContext(r)
			if key == "" {
				// Fallback to IP (should not occur on authenticated routes)
				key, _, _ = net.SplitHostPort(r.RemoteAddr)
			} else {
				key = "u:" + key // namespace to avoid collision with IP keys
			}
			if !rl.Allow(key) {
				http.Error(w, "rate limit exceeded, slow down", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ----- Request body size limiter -----

// maxBodyMiddleware caps request bodies at maxBytes. Requests that exceed the
// limit receive a 413 response before the handler reads any body content.
func maxBodyMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// newRateLimiterFor returns a Redis-backed rate limiter if rdb is non-nil,
// otherwise an in-memory token bucket. Used so production gets cross-node
// consistency while dev/single-node gets the cheaper in-memory implementation.
func newRateLimiterFor(rdb *goredis.Client, limit int, window time.Duration) rateLimiterI {
	if rdb != nil {
		return newRedisRateLimiter(rdb, limit, window)
	}
	return newRateLimiter(limit, window)
}
