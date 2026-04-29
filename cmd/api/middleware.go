package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"parley/internal/auth"
	"parley/internal/db"
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
// It keys on auth.ClientIP(r), which returns X-Real-IP when we're behind the
// DMZ nginx (nginx overwrites X-Real-IP to $remote_addr after real_ip_header
// CF-Connecting-IP, so the header is the trusted real-client IP) and falls
// back to r.RemoteAddr otherwise. X-Forwarded-For is never read — Cloudflare
// preserves client-supplied XFF as the leftmost token, making it
// attacker-controlled (see audit finding F6).
func rateLimitMiddleware(rl rateLimiterI) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := auth.ClientIP(r)
			if !rl.Allow(ip) {
				if auth.ShouldLogAuditOnce("ratelimit:ip:" + ip) {
					log.Printf("audit: rate_limited ip=%s path=%s scope=ip", ip, r.URL.Path)
				}
				http.Error(w, "rate limit exceeded, please slow down", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ownerRateLimitMiddleware applies per-owner rate limiting: writes from
// a user and writes from every API-key bot they own share a single bucket.
// Without this, an owner with N bots would get N+1 independent 5/s user
// buckets (6/s × 11 = 66 msg-writes/s per attacker before hitting any
// wall — the D4 finding). The aggregate cap is intentionally a little
// higher than the per-user limit so that a legitimate user running one
// or two helpful bots still has headroom.
//
// Key prefix is "o:" to avoid collision with userRateLimitMiddleware's
// "u:" prefix and rateLimitMiddleware's IP keys.
func ownerRateLimitMiddleware(rl rateLimiterI) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ownerID := auth.GetOwnerUserIDFromContext(r)
			key := ownerID
			if key == "" {
				// Fallback to IP — should only happen on unauthenticated routes
				// that mistakenly stack this middleware.
				key = auth.ClientIP(r)
			} else {
				key = "o:" + key
			}
			if !rl.Allow(key) {
				userID := auth.GetUserIDFromContext(r)
				if ownerID != "" && auth.ShouldLogAuditOnce("ratelimit:o:"+ownerID) {
					log.Printf("audit: rate_limited owner_user_id=%s user_id=%s ip=%s path=%s scope=owner", ownerID, userID, auth.ClientIP(r), r.URL.Path)
				}
				http.Error(w, "rate limit exceeded, slow down", http.StatusTooManyRequests)
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
			userID := auth.GetUserIDFromContext(r)
			key := userID
			if key == "" {
				// Fallback to IP (should not occur on authenticated routes)
				key = auth.ClientIP(r)
			} else {
				key = "u:" + key // namespace to avoid collision with IP keys
			}
			if !rl.Allow(key) {
				if userID != "" && auth.ShouldLogAuditOnce("ratelimit:u:"+userID) {
					log.Printf("audit: rate_limited user_id=%s ip=%s path=%s scope=user", userID, auth.ClientIP(r), r.URL.Path)
				} else if userID == "" && auth.ShouldLogAuditOnce("ratelimit:ip:"+key) {
					log.Printf("audit: rate_limited ip=%s path=%s scope=user_fallback_ip", key, r.URL.Path)
				}
				http.Error(w, "rate limit exceeded, slow down", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// denyImpersonation returns 403 if the request is authenticated with an
// admin-minted impersonation token. Applied to routes that modify the
// target user's account, credentials, keys, or other state the admin
// should never cause to change while acting as the user. Support-mode
// viewing + low-risk remediation remains allowed; anything account-
// mutating is denied. See audit finding F-impersonation-claim.
func denyImpersonation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.IsImpersonation(r) {
			actor := auth.ActorAdminID(r)
			target := auth.GetUserIDFromContext(r)
			if auth.ShouldLogAuditOnce("impersonation_deny:" + actor + ":" + target + ":" + r.URL.Path) {
				log.Printf("audit: impersonation_denied actor_admin_id=%s target_user_id=%s method=%s path=%s",
					actor, target, r.Method, r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"endpoint disallowed for impersonation sessions"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireBetaFeatures returns 403 unless the authenticated user has opted
// into beta features (user_preferences.beta_features = TRUE). Wrap routes
// that should only be visible to opt-in users while a feature is in beta.
//
// Phase A.A1+A2 (projects + synthesis) is gated this way so the half-built
// dev-platform UX doesn't leak to the general user base while VC-side
// activities (A3) are still pending. Drop the wrap when the surface goes GA.
func requireBetaFeatures(repo *db.Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uidStr := auth.GetUserIDFromContext(r)
			if uidStr == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			uid, err := strconv.ParseInt(uidStr, 10, 64)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			ok, err := repo.IsBetaUser(r.Context(), uid)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"failed to check beta access"}`))
				return
			}
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"beta features not enabled for this account"}`))
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

// defaultBodyLimit (64 KB) is the global cap for any request that doesn't carry
// a per-route override. Sized for typical JSON payloads (messages, profile
// patches, settings); large uploads (file/avatar/soundboard, AI theme
// generation) carry their own per-route maxBodyMiddleware.
const defaultBodyLimitBytes = 64 * 1024

// largeBodyPaths lists routes that handle bigger-than-default payloads and so
// must NOT be wrapped by the global cap. http.MaxBytesReader composes such
// that the OUTERMOST wrapper wins (its limit fires first when the handler
// reads the body), so a 64 KB outer cap would defeat a per-route 50 MB cap if
// applied unconditionally. Skipping these paths lets the per-route
// maxBodyMiddleware be the only wrapper and remain authoritative.
//
// Suffix-matched so it works under any chi mount prefix (we mount under /api,
// but tests bench routes elsewhere).
var largeBodyPathSuffixes = []string{
	"/api/upload",
	"/api/me/themes/generate",
	"/soundboard", // server soundboard upload: /api/servers/{id}/soundboard
}

// globalBodyLimitMiddleware wraps every request body with
// http.MaxBytesReader(defaultBodyLimitBytes) except for the upload-class
// routes listed in largeBodyPathSuffixes. Applied as a router-level
// r.Use(...) so it covers every endpoint without per-route opt-in.
func globalBodyLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLargeBodyPath(r.URL.Path) {
			r.Body = http.MaxBytesReader(w, r.Body, defaultBodyLimitBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func isLargeBodyPath(path string) bool {
	for _, suffix := range largeBodyPathSuffixes {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
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
