package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"parley/internal/auth"
)

func TestTokenBucketAllows(t *testing.T) {
	rl := newRateLimiter(10, time.Minute) // 10 req/min

	// First 10 requests should pass
	for i := 0; i < 10; i++ {
		if !rl.Allow("192.0.2.1") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestTokenBucketDenies(t *testing.T) {
	rl := newRateLimiter(5, time.Minute) // 5 req/min burst

	// Exhaust the burst
	for i := 0; i < 5; i++ {
		rl.Allow("192.0.2.1")
	}

	// 6th should be denied
	if rl.Allow("192.0.2.1") {
		t.Error("6th request should be denied after burst exhausted")
	}
}

func TestTokenBucketIsolation(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)

	rl.Allow("10.0.0.1")
	rl.Allow("10.0.0.1")
	// 10.0.0.1 is exhausted

	// 10.0.0.2 should still have full bucket
	if !rl.Allow("10.0.0.2") {
		t.Error("10.0.0.2 should not be affected by 10.0.0.1 exhaustion")
	}
}

func TestTokenBucketRefills(t *testing.T) {
	rl := newRateLimiter(60, time.Minute) // 1 token/sec refill

	// Exhaust burst
	for i := 0; i < 60; i++ {
		rl.Allow("192.0.2.1")
	}

	if rl.Allow("192.0.2.1") {
		t.Error("should be denied immediately after exhaustion")
	}

	// Wait 1.5 seconds — generous margin so at least 1 token refills
	// even on a loaded CI runner (1 token/sec rate, 500ms slack).
	time.Sleep(1500 * time.Millisecond)

	if !rl.Allow("192.0.2.1") {
		t.Error("should be allowed after 1 second refill")
	}
}

func TestUserRateLimiterIsolation(t *testing.T) {
	rl := newRateLimiter(3, time.Minute) // 3/min per user

	// User A exhausts their bucket (using the "u:" prefix that userRateLimitMiddleware produces)
	for i := 0; i < 3; i++ {
		if !rl.Allow("u:user_a") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if rl.Allow("u:user_a") {
		t.Error("4th request for user_a should be denied")
	}

	// User B's bucket is independent
	if !rl.Allow("u:user_b") {
		t.Error("user_b should not be affected by user_a exhaustion")
	}
}

// --- denyImpersonation ---

// impersonationCtx wires the same context values AuthMiddlewareWith would
// set; lets us drive denyImpersonation in isolation.
func impersonationCtx(r *http.Request, actor, target string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, auth.UserIDKey, target)
	ctx = context.WithValue(ctx, auth.IsImpersonationKey, true)
	ctx = context.WithValue(ctx, auth.ActorAdminIDKey, actor)
	return r.WithContext(ctx)
}

func TestDenyImpersonationBlocksImpersonationToken(t *testing.T) {
	called := false
	h := denyImpersonation(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := impersonationCtx(httptest.NewRequest(http.MethodDelete, "/api/auth/password", nil), "7", "42")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if called {
		t.Fatal("handler should not be called for impersonation token")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want 403", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body=%q", err, rr.Body.String())
	}
	if body["error"] != "endpoint disallowed for impersonation sessions" {
		t.Errorf("error: got %q", body["error"])
	}
}

func TestDenyImpersonationPassesNormalToken(t *testing.T) {
	called := false
	h := denyImpersonation(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodDelete, "/api/auth/password", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserIDKey, "42"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called {
		t.Fatal("handler should have been called for normal token")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rr.Code)
	}
}

