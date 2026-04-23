package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fireRequest runs a single request through adminRateLimitMiddleware+rl and
// returns the IP bucket key that the limiter observed.
func fireRequest(t *testing.T, rl *adminRateLimiter, remoteAddr, xRealIP, xff string) {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := adminRateLimitMiddleware(rl)(next)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = remoteAddr
	if xRealIP != "" {
		req.Header.Set("X-Real-IP", xRealIP)
	}
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
}

// bucketCount returns the number of recorded hits for a given key.
func bucketCount(rl *adminRateLimiter, key string) int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.requests[key])
}

func TestAdminRateLimitMiddleware_UsesXRealIP(t *testing.T) {
	rl := newAdminRateLimiter(100, time.Minute)
	fireRequest(t, rl, "127.0.0.1:55123", "198.51.100.42", "")

	if got := bucketCount(rl, "198.51.100.42"); got != 1 {
		t.Errorf("bucket 198.51.100.42: got %d hits, want 1", got)
	}
	if got := bucketCount(rl, "127.0.0.1"); got != 0 {
		t.Errorf("bucket 127.0.0.1: got %d hits, want 0 (should not be used when X-Real-IP is set)", got)
	}
}

func TestAdminRateLimitMiddleware_FallsBackToRemoteAddr(t *testing.T) {
	rl := newAdminRateLimiter(100, time.Minute)
	fireRequest(t, rl, "203.0.113.7:40000", "", "")

	if got := bucketCount(rl, "203.0.113.7"); got != 1 {
		t.Errorf("bucket 203.0.113.7: got %d hits, want 1", got)
	}
}

func TestAdminRateLimitMiddleware_XForwardedForIgnored(t *testing.T) {
	// Both X-Real-IP and X-Forwarded-For present — X-Real-IP must win.
	// XFF is attacker-spoofable via Cloudflare (finding F6) and must never
	// be used as the bucket key.
	rl := newAdminRateLimiter(100, time.Minute)
	fireRequest(t, rl, "127.0.0.1:55123", "198.51.100.42", "203.0.113.99, 10.0.0.1")

	if got := bucketCount(rl, "198.51.100.42"); got != 1 {
		t.Errorf("bucket 198.51.100.42: got %d hits, want 1", got)
	}
	if got := bucketCount(rl, "203.0.113.99"); got != 0 {
		t.Errorf("bucket 203.0.113.99 (from XFF): got %d hits, want 0 — XFF must be ignored", got)
	}
	if got := bucketCount(rl, "127.0.0.1"); got != 0 {
		t.Errorf("bucket 127.0.0.1: got %d hits, want 0", got)
	}
}

func TestAdminRateLimitMiddleware_XRealIPTrimmed(t *testing.T) {
	// Defensive: header values sometimes arrive with surrounding whitespace.
	rl := newAdminRateLimiter(100, time.Minute)
	fireRequest(t, rl, "127.0.0.1:55123", "  198.51.100.42  ", "")

	if got := bucketCount(rl, "198.51.100.42"); got != 1 {
		t.Errorf("bucket 198.51.100.42: got %d hits, want 1 (X-Real-IP should be trimmed)", got)
	}
}

func TestAdminRateLimitMiddleware_EnforcesLimit(t *testing.T) {
	// Sanity check that the limiter still rejects after the threshold and
	// that different X-Real-IPs get independent buckets (the core F7 fix).
	rl := newAdminRateLimiter(2, time.Minute)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := adminRateLimitMiddleware(rl)(next)

	do := func(xri string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		req.RemoteAddr = "127.0.0.1:55123"
		req.Header.Set("X-Real-IP", xri)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}

	// IP A burns through its 2 allowed requests.
	if got := do("198.51.100.1"); got != http.StatusOK {
		t.Errorf("A req1: got %d, want 200", got)
	}
	if got := do("198.51.100.1"); got != http.StatusOK {
		t.Errorf("A req2: got %d, want 200", got)
	}
	if got := do("198.51.100.1"); got != http.StatusTooManyRequests {
		t.Errorf("A req3: got %d, want 429", got)
	}

	// IP B must still be allowed — separate bucket.
	if got := do("198.51.100.2"); got != http.StatusOK {
		t.Errorf("B req1: got %d, want 200 (different IP must have separate bucket)", got)
	}
}
