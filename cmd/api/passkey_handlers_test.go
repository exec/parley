package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"parley/internal/auth"
)

// TestHandleRemovePasswordRequiresAuth covers the unauthenticated rejection
// path for the remove-password handler. The atomicity guarantee for
// F-auth-removepw-race is provided by the single SQL UPDATE with EXISTS
// subquery in Repository.ClearPasswordIfPasskeyExists — Postgres evaluates
// the EXISTS predicate and the UPDATE in one statement, so a concurrent
// passkey DELETE cannot interleave between the check and the clear.
func TestHandleRemovePasswordRequiresAuth(t *testing.T) {
	svc := auth.NewAuthService(nil)
	h := handleRemovePassword(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/auth/password", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}
}
