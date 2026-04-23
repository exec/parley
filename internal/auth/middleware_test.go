package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
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
