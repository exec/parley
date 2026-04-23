package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"parley/internal/auth"
)

// TestHandleWebSocketRejectsBannedUserOnJWTFallback verifies the WS handler's
// JWT-fallback (no ticket, Authorization: Bearer) rejects a banned user even
// when the JWT is still valid. Regression guard for F-ws-ban-check: before the
// fix, the fallback only checked force-logout.
func TestHandleWebSocketRejectsBannedUserOnJWTFallback(t *testing.T) {
	svc := auth.NewAuthService(nil)
	const userID = "4242"

	token, err := svc.GenerateTokenForUser(userID)
	if err != nil {
		t.Fatalf("GenerateTokenForUser: %v", err)
	}

	// Prime the session-status cache so GetSessionStatus short-circuits
	// before the DB lookup. Banned, not force-logged-out.
	svc.PrimeSessionCacheForTests(userID, &auth.SessionStatus{
		BannedAt:  sql.NullTime{Time: time.Now().Add(-time.Minute), Valid: true},
		BanReason: sql.NullString{String: "test", Valid: true},
	})

	// repo/hub/tickets are nil: the reject path short-circuits before
	// any of them are used.
	h := handleWebSocket(nil, svc, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want 403 (banned)", rr.Code)
	}
}

// TestHandleWebSocketRejectsForceLoggedOutUserOnJWTFallback keeps the
// force-logout reject path covered alongside the new ban check.
func TestHandleWebSocketRejectsForceLoggedOutUserOnJWTFallback(t *testing.T) {
	svc := auth.NewAuthService(nil)
	const userID = "4243"

	token, err := svc.GenerateTokenForUser(userID)
	if err != nil {
		t.Fatalf("GenerateTokenForUser: %v", err)
	}

	// Force-logout stamped in the future so IssuedAt <= ForceLogoutAt.Unix()
	// (the middleware's invalidation condition) holds for the just-minted JWT.
	svc.PrimeSessionCacheForTests(userID, &auth.SessionStatus{
		ForceLogoutAt: sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true},
	})

	h := handleWebSocket(nil, svc, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401 (force-logout)", rr.Code)
	}
}
