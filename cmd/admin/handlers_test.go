package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

// --- buildImpersonationToken ---

func TestBuildImpersonationTokenClaims(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	secret := "test-secret"
	tok, err := buildImpersonationToken("42", "7", secret, 10*time.Minute, now)
	if err != nil {
		t.Fatalf("buildImpersonationToken: %v", err)
	}

	// Skip exp validation — the fixed `now` is in the past so the token would
	// otherwise appear expired at test time.
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	parsed, err := parser.Parse(tok, func(*jwt.Token) (interface{}, error) { return []byte(secret), nil })
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)

	if claims["user_id"] != "42" {
		t.Errorf("user_id: got %v", claims["user_id"])
	}
	if claims["impersonation"] != true {
		t.Errorf("impersonation: got %v", claims["impersonation"])
	}
	if claims["actor_admin_id"] != "7" {
		t.Errorf("actor_admin_id: got %v", claims["actor_admin_id"])
	}
	exp, _ := claims["exp"].(float64)
	if int64(exp) != now.Add(10*time.Minute).Unix() {
		t.Errorf("exp: got %d, want %d", int64(exp), now.Add(10*time.Minute).Unix())
	}
	iat, _ := claims["iat"].(float64)
	if int64(iat) != now.Unix() {
		t.Errorf("iat: got %d, want %d", int64(iat), now.Unix())
	}
}

// --- impersonationTargetCheck ---

func TestImpersonationTargetCheck(t *testing.T) {
	cases := []struct {
		name   string
		target impersonationTarget
		okWant bool
	}{
		{"normal user", impersonationTarget{Found: true}, true},
		{"not found", impersonationTarget{Found: false}, false},
		{"system", impersonationTarget{Found: true, IsSystem: true}, false},
		{"bot", impersonationTarget{Found: true, IsBot: true}, false},
		{"banned", impersonationTarget{Found: true, Banned: true}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := impersonationTargetCheck(c.target)
			if got != c.okWant {
				t.Errorf("got ok=%v, want %v", got, c.okWant)
			}
		})
	}
}

// --- handleImpersonate ---

// serveImpersonate drives the handler with chi's URL-param plumbing in place
// and the DB-lookup hook swapped for an in-memory stub.
func serveImpersonate(t *testing.T, userID string, target impersonationTarget, lookupErr error) *httptest.ResponseRecorder {
	t.Helper()
	prevSecret := parleyJWTSecret
	parleyJWTSecret = "test-secret"
	t.Cleanup(func() { parleyJWTSecret = prevSecret })

	prev := lookupImpersonationTarget
	lookupImpersonationTarget = func(r *http.Request, id int64) (impersonationTarget, error) {
		return target, lookupErr
	}
	t.Cleanup(func() { lookupImpersonationTarget = prev })

	r := chi.NewRouter()
	r.Post("/users/{id}/impersonate", handleImpersonate)

	req := httptest.NewRequest(http.MethodPost, "/users/"+userID+"/impersonate", nil)
	req = req.WithContext(context.WithValue(req.Context(), adminIDKey, int64(7)))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func TestHandleImpersonateNormalUserReturnsToken(t *testing.T) {
	rr := serveImpersonate(t, "42", impersonationTarget{Found: true}, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	tok, ok := resp["token"]
	if !ok || tok == "" {
		t.Fatal("expected non-empty token")
	}

	parsed, err := jwt.Parse(tok, func(*jwt.Token) (interface{}, error) { return []byte("test-secret"), nil })
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	if claims["user_id"] != "42" {
		t.Errorf("user_id: got %v", claims["user_id"])
	}
	if claims["impersonation"] != true {
		t.Errorf("impersonation: got %v", claims["impersonation"])
	}
	// serveImpersonate stuffs admin_id=7 into the request context; the
	// handler should surface that as actor_admin_id on the minted token.
	if claims["actor_admin_id"] != "7" {
		t.Errorf("actor_admin_id: got %v, want \"7\"", claims["actor_admin_id"])
	}
	exp, _ := claims["exp"].(float64)
	iat, _ := claims["iat"].(float64)
	ttlSec := int64(exp) - int64(iat)
	const want = int64(10 * 60)
	// Allow ±2s slack for scheduling jitter between token mint and assertion.
	if math.Abs(float64(ttlSec-want)) > 2 {
		t.Errorf("ttl: got %ds, want ~%ds", ttlSec, want)
	}
}

func TestHandleImpersonateBotTargetForbidden(t *testing.T) {
	rr := serveImpersonate(t, "42", impersonationTarget{Found: true, IsBot: true}, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleImpersonateSystemTargetForbidden(t *testing.T) {
	rr := serveImpersonate(t, "42", impersonationTarget{Found: true, IsSystem: true}, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleImpersonateBannedTargetForbidden(t *testing.T) {
	rr := serveImpersonate(t, "42", impersonationTarget{Found: true, Banned: true}, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleImpersonateDeletedTargetForbidden(t *testing.T) {
	// Hard-deleted users surface as Found=false; the handler should refuse
	// rather than leak existence via a different error code.
	rr := serveImpersonate(t, "42", impersonationTarget{Found: false}, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleImpersonateInvalidID(t *testing.T) {
	rr := serveImpersonate(t, "not-a-number", impersonationTarget{}, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, body=%s", rr.Code, rr.Body.String())
	}
}
