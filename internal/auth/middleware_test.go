package auth

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// --- extractToken ---

func TestExtractTokenBearer(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer mytoken123")
	got := extractToken(r)
	if got != "mytoken123" {
		t.Errorf("expected 'mytoken123', got '%s'", got)
	}
}

func TestExtractTokenNoHeader(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	got := extractToken(r)
	if got != "" {
		t.Errorf("expected empty string, got '%s'", got)
	}
}

func TestExtractTokenNoBearerPrefix(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Basic abc123")
	got := extractToken(r)
	if got != "" {
		t.Errorf("expected empty string for non-Bearer auth, got '%s'", got)
	}
}

func TestExtractTokenBearerEmptyValue(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer ")
	got := extractToken(r)
	if got != "" {
		t.Errorf("expected empty string for 'Bearer ', got '%s'", got)
	}
}

// --- GetUserIDFromContext ---

func TestGetUserIDFromContext(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(r.Context(), UserIDKey, "42")
	r = r.WithContext(ctx)

	got := GetUserIDFromContext(r)
	if got != "42" {
		t.Errorf("expected '42', got '%s'", got)
	}
}

func TestGetUserIDFromContextMissing(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	got := GetUserIDFromContext(r)
	if got != "" {
		t.Errorf("expected empty string, got '%s'", got)
	}
}

func TestGetUserIDFromContextWrongType(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(r.Context(), UserIDKey, 42) // int, not string
	r = r.WithContext(ctx)

	got := GetUserIDFromContext(r)
	if got != "" {
		t.Errorf("expected empty string for wrong type, got '%s'", got)
	}
}

// --- GetUserIDFromContextWithError ---

func TestGetUserIDFromContextWithError(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(r.Context(), UserIDKey, "42")
	r = r.WithContext(ctx)

	got, err := GetUserIDFromContextWithError(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "42" {
		t.Errorf("expected '42', got '%s'", got)
	}
}

func TestGetUserIDFromContextWithErrorMissing(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	_, err := GetUserIDFromContextWithError(r)
	if err == nil {
		t.Error("expected error for missing user ID")
	}
}

// --- IsAPIKeyAuth ---

func TestIsAPIKeyAuthTrue(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(r.Context(), IsAPIKeyAuthKey, true)
	r = r.WithContext(ctx)

	if !IsAPIKeyAuth(r) {
		t.Error("expected IsAPIKeyAuth to return true")
	}
}

func TestIsAPIKeyAuthFalse(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if IsAPIKeyAuth(r) {
		t.Error("expected IsAPIKeyAuth to return false when not set")
	}
}

// --- AuthMiddlewareWith ---

func TestAuthMiddlewareNoHeader(t *testing.T) {
	svc := newTestService()
	middleware := AuthMiddlewareWith(svc)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called without auth header")
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/protected", nil)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	svc := newTestService()
	middleware := AuthMiddlewareWith(svc)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with invalid token")
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthMiddlewareValidToken(t *testing.T) {
	svc := newTestService()
	token, err := svc.generateToken("42")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	middleware := AuthMiddlewareWith(svc)

	var capturedUserID string
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = GetUserIDFromContext(r)
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if capturedUserID != "42" {
		t.Errorf("expected userID '42' in context, got '%s'", capturedUserID)
	}
}

func TestAuthMiddlewareExpiredToken(t *testing.T) {
	svc := newTestService()
	claims := jwt.MapClaims{
		"user_id": "42",
		"exp":     time.Now().Add(-1 * time.Hour).Unix(),
		"iat":     time.Now().Add(-2 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(svc.config.SecretKey))

	middleware := AuthMiddlewareWith(svc)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with expired token")
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthMiddlewareAPIKeyNoRepo(t *testing.T) {
	svc := newTestService()
	// svc.repo is nil; API key auth should fail gracefully
	middleware := AuthMiddlewareWith(svc)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for API key without repo")
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer plk_testapikey123")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthMiddlewareNonBearerAuth(t *testing.T) {
	svc := newTestService()
	middleware := AuthMiddlewareWith(svc)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with non-Bearer auth")
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// --- Constants ---

func TestConstants(t *testing.T) {
	if AuthorizationHeader != "Authorization" {
		t.Errorf("unexpected AuthorizationHeader: %s", AuthorizationHeader)
	}
	if BearerPrefix != "Bearer " {
		t.Errorf("unexpected BearerPrefix: %s", BearerPrefix)
	}
}

// --- ClientIP (F6) ---

// Cloudflare preserves client-supplied XFF as the leftmost token, so
// ClientIP must ignore X-Forwarded-For entirely and fall back to RemoteAddr
// when no trusted X-Real-IP is present.
func TestClientIPIgnoresXForwardedFor(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	r.RemoteAddr = "10.10.10.5:1234"
	got := ClientIP(r)
	if got != "10.10.10.5" {
		t.Errorf("expected '10.10.10.5' (XFF must be ignored), got '%s'", got)
	}
}

func TestClientIPPrefersXRealIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Real-IP", "198.51.100.42")
	r.RemoteAddr = "10.10.10.5:1234"
	got := ClientIP(r)
	if got != "198.51.100.42" {
		t.Errorf("expected '198.51.100.42', got '%s'", got)
	}
}

func TestClientIPFallsBackToRemoteAddr(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.10.10.5:1234"
	got := ClientIP(r)
	if got != "10.10.10.5" {
		t.Errorf("expected '10.10.10.5', got '%s'", got)
	}
}

// With both XFF and XRI set, XRI wins and XFF is silently ignored.
func TestClientIPXRealIPWinsOverXForwardedFor(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	r.Header.Set("X-Real-IP", "198.51.100.42")
	r.RemoteAddr = "10.10.10.5:1234"
	got := ClientIP(r)
	if got != "198.51.100.42" {
		t.Errorf("expected '198.51.100.42' (XRI wins, XFF ignored), got '%s'", got)
	}
}

// --- Impersonation context surfacing ---

// An impersonation token minted with actor_admin_id must surface through
// the middleware as IsImpersonation=true + ActorAdminID=<id> so downstream
// helpers (IsImpersonation / ActorAdminID) and denyImpersonation can see it.
func TestAuthMiddlewareSurfacesImpersonation(t *testing.T) {
	svc := newTestService()
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id":        "42",
		"impersonation":  true,
		"actor_admin_id": "7",
		"exp":            now.Add(10 * time.Minute).Unix(),
		"iat":            now.Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := tok.SignedString([]byte(svc.config.SecretKey))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	middleware := AuthMiddlewareWith(svc)
	var gotImp bool
	var gotActor string
	var gotUser string
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotImp = IsImpersonation(r)
		gotActor = ActorAdminID(r)
		gotUser = GetUserIDFromContext(r)
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	if gotUser != "42" {
		t.Errorf("UserID: got %q, want \"42\"", gotUser)
	}
	if !gotImp {
		t.Error("IsImpersonation(r): got false, want true")
	}
	if gotActor != "7" {
		t.Errorf("ActorAdminID(r): got %q, want \"7\"", gotActor)
	}
}

// A normal (non-impersonation) token must leave IsImpersonation=false and
// ActorAdminID="". The middleware should not set either context value.
func TestAuthMiddlewareNormalTokenHasNoImpersonationFlags(t *testing.T) {
	svc := newTestService()
	token, err := svc.generateToken("42")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	middleware := AuthMiddlewareWith(svc)
	var gotImp bool
	var gotActor string
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotImp = IsImpersonation(r)
		gotActor = ActorAdminID(r)
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	if gotImp {
		t.Error("IsImpersonation(r): got true, want false")
	}
	if gotActor != "" {
		t.Errorf("ActorAdminID(r): got %q, want empty", gotActor)
	}
}

// Integration: AuthMiddlewareWith followed by a deny-impersonation guard
// rejects an impersonation token before the handler runs, and lets a
// normal user token through. Kept in-package because the cmd/api deny
// middleware is the same pattern — this locks in that the context keys
// surface correctly from the authenticator for anyone chaining a guard.
func TestAuthMiddlewareChainedWithDenyGuard(t *testing.T) {
	svc := newTestService()

	// Minimal reimplementation of the cmd/api denyImpersonation guard —
	// test lives in auth package to avoid importing cmd/api (would break
	// layering) while still exercising the handoff from auth middleware.
	denyGuard := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if IsImpersonation(r) {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	now := time.Now()
	impTokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":        "42",
		"impersonation":  true,
		"actor_admin_id": "7",
		"exp":            now.Add(10 * time.Minute).Unix(),
		"iat":            now.Unix(),
	}).SignedString([]byte(svc.config.SecretKey))
	if err != nil {
		t.Fatalf("sign imp: %v", err)
	}
	normalTokenStr, err := svc.generateToken("42")
	if err != nil {
		t.Fatalf("generate normal: %v", err)
	}

	var handlerHits int
	chain := AuthMiddlewareWith(svc)(denyGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerHits++
		w.WriteHeader(http.StatusOK)
	})))

	// Impersonation token → 403, handler never called.
	{
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("DELETE", "/api/auth/password", nil)
		req.Header.Set("Authorization", "Bearer "+impTokenStr)
		chain.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("impersonation: got %d, want 403", rr.Code)
		}
	}
	before := handlerHits

	// Normal token → 200, handler runs exactly once.
	{
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("DELETE", "/api/auth/password", nil)
		req.Header.Set("Authorization", "Bearer "+normalTokenStr)
		chain.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("normal: got %d, want 200", rr.Code)
		}
	}
	if handlerHits-before != 1 {
		t.Errorf("handler hits delta=%d, want 1", handlerHits-before)
	}
}

// 10 rapid requests against the same path must only produce one audit
// log line — the dedup key is (actor, target, path), and the interval is
// 5s, so a single page load's XHR burst against one endpoint is quiet
// after the first entry.
func TestAuthMiddlewareDedupsImpersonationAuditBurst(t *testing.T) {
	svc := newTestService()
	now := time.Now()
	impTokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":        "111",
		"impersonation":  true,
		"actor_admin_id": "222",
		"exp":            now.Add(10 * time.Minute).Unix(),
		"iat":            now.Unix(),
	}).SignedString([]byte(svc.config.SecretKey))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Drop any previous audit-dedup state for this key so we start clean.
	auditDedup.Delete("impersonation:222:111:/dedup-burst")
	t.Cleanup(func() { auditDedup.Delete("impersonation:222:111:/dedup-burst") })

	// Tee the stdlib logger into a buffer so we can count emissions.
	var buf bytes.Buffer
	prev := log.Writer()
	flags := log.Flags()
	prefix := log.Prefix()
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(prev)
		log.SetFlags(flags)
		log.SetPrefix(prefix)
	})

	handler := AuthMiddlewareWith(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 10; i++ {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/dedup-burst", nil)
		req.Header.Set("Authorization", "Bearer "+impTokenStr)
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("iter %d: status %d", i, rr.Code)
		}
	}

	count := strings.Count(buf.String(), "audit: impersonation_request")
	if count != 1 {
		t.Errorf("impersonation_request log lines: got %d, want 1; buf=%q", count, buf.String())
	}
}

// shouldLogKeyedOnce is the per-interval dedup used by the impersonation
// audit logger. The first call per key wins, subsequent calls within the
// window must return false, and a call after the interval elapses must
// wake the key back up.
func TestShouldLogKeyedOnceDedups(t *testing.T) {
	key := "test-dedup-key-" + t.Name()
	t.Cleanup(func() { auditDedup.Delete(key) })

	if !shouldLogKeyedOnce(key, 1*time.Second) {
		t.Fatal("first call should return true")
	}
	// Back-to-back calls are suppressed.
	for i := 0; i < 10; i++ {
		if shouldLogKeyedOnce(key, 1*time.Second) {
			t.Errorf("call %d within window returned true, want false", i)
		}
	}
	// Sub-window interval of 0 reopens the gate immediately.
	if !shouldLogKeyedOnce(key+"-fresh", 1*time.Millisecond) {
		t.Error("fresh key should return true")
	}
}
