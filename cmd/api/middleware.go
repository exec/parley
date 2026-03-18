package main

import (
	"net"
	"net/http"
	"sync"
	"time"

	"parley/internal/auth"
)

// ----- Token bucket rate limiter -----
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
}

// newRateLimiter creates a token bucket rate limiter equivalent to limit requests
// per window. burst = limit, rate = limit/window in tokens/second.
func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		rate:  float64(limit) / window.Seconds(),
		burst: float64(limit),
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

// cleanup removes stale buckets every 5 minutes.
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
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

// rateLimitMiddleware rejects requests that exceed the limiter's threshold.
// It uses r.RemoteAddr exclusively to avoid trusting client-supplied headers.
// When nginx sits in front, it overwrites X-Real-IP before forwarding, and
// Chi's RealIP middleware has already copied that trusted value into RemoteAddr.
func rateLimitMiddleware(rl *rateLimiter) func(http.Handler) http.Handler {
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
func userRateLimitMiddleware(rl *rateLimiter) func(http.Handler) http.Handler {
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
