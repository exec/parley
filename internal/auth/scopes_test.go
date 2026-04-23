package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- ValidateScopes ---

func TestValidateScopesRejectsEmpty(t *testing.T) {
	bad, ok := ValidateScopes(nil)
	if ok {
		t.Error("empty scopes must be rejected")
	}
	if bad != "" {
		t.Errorf("bad should be empty for empty-input case, got %q", bad)
	}

	bad, ok = ValidateScopes([]string{})
	if ok {
		t.Error("empty scopes slice must be rejected")
	}
	if bad != "" {
		t.Errorf("bad should be empty for empty-slice case, got %q", bad)
	}
}

func TestValidateScopesAcceptsKnown(t *testing.T) {
	if bad, ok := ValidateScopes([]string{ScopeMessagesRead, ScopeMessagesWrite}); !ok {
		t.Errorf("known scopes rejected: bad=%q", bad)
	}
	if _, ok := ValidateScopes([]string{ScopeFull}); !ok {
		t.Error("full scope must be accepted")
	}
}

func TestValidateScopesRejectsUnknown(t *testing.T) {
	bad, ok := ValidateScopes([]string{ScopeMessagesRead, "admin:god"})
	if ok {
		t.Error("unknown scope must be rejected")
	}
	if bad != "admin:god" {
		t.Errorf("expected bad=admin:god, got %q", bad)
	}
}

// --- HasScope ---

// A non-API-key request (normal JWT) is never scope-limited. HasScope must
// return true without inspecting ScopesKey.
func TestHasScopeTrueForNonAPIKeyAuth(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	// Explicitly not setting IsAPIKeyAuthKey — represents a user JWT request.
	if !HasScope(r, ScopeMessagesWrite) {
		t.Error("HasScope should pass for user JWT requests")
	}
}

func TestHasScopeMatchesExactScope(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(r.Context(), IsAPIKeyAuthKey, true)
	ctx = context.WithValue(ctx, ScopesKey, []string{ScopeMessagesRead})
	r = r.WithContext(ctx)

	if !HasScope(r, ScopeMessagesRead) {
		t.Error("HasScope should pass when scope is present")
	}
	if HasScope(r, ScopeMessagesWrite) {
		t.Error("HasScope should fail when scope is missing")
	}
}

// "full" grants every other scope — covers the grandfather path.
func TestHasScopeFullGrantsAll(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(r.Context(), IsAPIKeyAuthKey, true)
	ctx = context.WithValue(ctx, ScopesKey, []string{ScopeFull})
	r = r.WithContext(ctx)

	for scope := range KnownScopes {
		if !HasScope(r, scope) {
			t.Errorf("full scope should grant %s", scope)
		}
	}
}

// Empty scopes on an API-key-authed request fail closed: HasScope always
// false. This is the safe failure mode for a pre-migration key in flight
// during a rolling deploy.
func TestHasScopeEmptyScopesFailsClosed(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(r.Context(), IsAPIKeyAuthKey, true)
	ctx = context.WithValue(ctx, ScopesKey, []string{})
	r = r.WithContext(ctx)

	if HasScope(r, ScopeMessagesRead) {
		t.Error("empty scopes on API-key auth must fail closed")
	}
}

// --- RequireScope middleware ---

func TestRequireScopePassesUserJWT(t *testing.T) {
	mw := RequireScope(ScopeMessagesRead)
	var called bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rr, req)

	if !called {
		t.Error("user JWT request must reach the handler")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rr.Code)
	}
}

func TestRequireScopeRejectsMissingScope(t *testing.T) {
	mw := RequireScope(ScopeMessagesWrite)
	var called bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	ctx := context.WithValue(req.Context(), IsAPIKeyAuthKey, true)
	ctx = context.WithValue(ctx, ScopesKey, []string{ScopeMessagesRead})
	req = req.WithContext(ctx)
	h.ServeHTTP(rr, req)

	if called {
		t.Error("handler must not be called when scope is missing")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want 403", rr.Code)
	}
	// Body is parseable JSON and names the missing scope so callers can
	// re-issue the key with the right grants.
	body, _ := io.ReadAll(rr.Body)
	var errResp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("response body not JSON: %q", body)
	}
	if !strings.Contains(errResp.Error, ScopeMessagesWrite) {
		t.Errorf("error body should name missing scope, got %q", errResp.Error)
	}
}

func TestRequireScopePassesWithMatchingScope(t *testing.T) {
	mw := RequireScope(ScopeMessagesWrite)
	var called bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	ctx := context.WithValue(req.Context(), IsAPIKeyAuthKey, true)
	ctx = context.WithValue(ctx, ScopesKey, []string{ScopeMessagesWrite, ScopeMessagesRead})
	req = req.WithContext(ctx)
	h.ServeHTTP(rr, req)

	if !called {
		t.Error("handler must be called when scope is present")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rr.Code)
	}
}

// A "full" scope key must pass every RequireScope gate (grandfather path).
func TestRequireScopePassesWithFullScope(t *testing.T) {
	mw := RequireScope(ScopeDeveloperManage)
	var called bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	ctx := context.WithValue(req.Context(), IsAPIKeyAuthKey, true)
	ctx = context.WithValue(ctx, ScopesKey, []string{ScopeFull})
	req = req.WithContext(ctx)
	h.ServeHTTP(rr, req)

	if !called {
		t.Error("full scope must pass RequireScope")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rr.Code)
	}
}

// --- GetScopesFromContext ---

func TestGetScopesFromContextReturnsNilForUserJWT(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if got := GetScopesFromContext(r); got != nil {
		t.Errorf("expected nil for user JWT, got %v", got)
	}
}

func TestGetScopesFromContextReturnsStash(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	want := []string{ScopeMessagesRead, ScopeServersRead}
	ctx := context.WithValue(r.Context(), ScopesKey, want)
	r = r.WithContext(ctx)
	got := GetScopesFromContext(r)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}

// --- Simulated end-to-end: an API-key-auth stage followed by RequireScope
// rejects writes for a key scoped only to messages:read but allows reads.
// Mirrors the production middleware chain (AuthMiddlewareWith stashes
// ScopesKey, per-route RequireScope enforces) without needing a DB.
func TestRequireScopeChainedWithAPIKeyStash(t *testing.T) {
	// Stand-in for the API-key branch of AuthMiddlewareWith: stashes
	// IsAPIKeyAuthKey + ScopesKey just like the real middleware does,
	// so we can test the downstream RequireScope without wiring a DB.
	apiKeyStage := func(scopes []string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := context.WithValue(r.Context(), IsAPIKeyAuthKey, true)
				ctx = context.WithValue(ctx, ScopesKey, scopes)
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		}
	}

	reached := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached++
		w.WriteHeader(http.StatusOK)
	})

	// Read-only key: passes a RequireScope(messages:read) gate, fails on
	// a RequireScope(messages:write) gate.
	readOnly := apiKeyStage([]string{ScopeMessagesRead})

	rr := httptest.NewRecorder()
	readOnly(RequireScope(ScopeMessagesRead)(handler)).ServeHTTP(rr, httptest.NewRequest("GET", "/messages", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("read gate: got %d, want 200", rr.Code)
	}
	if reached != 1 {
		t.Errorf("handler reach count after read: got %d, want 1", reached)
	}

	rr = httptest.NewRecorder()
	readOnly(RequireScope(ScopeMessagesWrite)(handler)).ServeHTTP(rr, httptest.NewRequest("POST", "/messages", nil))
	if rr.Code != http.StatusForbidden {
		t.Errorf("write gate: got %d, want 403", rr.Code)
	}
	if reached != 1 {
		t.Errorf("write should not reach handler; reach count: got %d, want 1", reached)
	}
}
