package auth

import (
	"net/http"
)

// Bot API-key scope model.
//
// A scope is a short string of the form "<resource>:<action>" that bounds
// what a bot's API key can do. The set below is intentionally coarse — each
// scope maps to a block of endpoints whose blast radius is similar — so
// future endpoints slot into an existing scope without a version bump.
//
// The meta-scope "full" grants every other scope and is how pre-scope keys
// are grandfathered. It is a valid input to new-key creation too, but it
// exists mostly so the migration has something to backfill with: callers
// minting new keys should pick narrow scopes.
//
// Scopes apply only to requests authenticated via API key. Normal user JWTs
// and impersonation tokens pass every scope check unconditionally — the
// user's own session is never scope-limited (see HasScope).
const (
	ScopeMessagesRead        = "messages:read"
	ScopeMessagesWrite       = "messages:write"
	ScopeCommandsWrite       = "commands:write"
	ScopeInteractionsRespond = "interactions:respond"
	ScopeProfileWrite        = "profile:write"
	ScopeServersRead         = "servers:read"
	ScopeDeveloperManage     = "developer:manage"

	// ScopeFull is the meta-scope that implies every other scope. Existing
	// keys are backfilled to this during the scopes-column migration; new
	// keys accept it but callers should prefer narrow scopes.
	ScopeFull = "full"
)

// KnownScopes is the set of scope strings the API accepts on key creation.
// Any scope not in this map is rejected with 400. The "full" meta-scope is
// included here so backfilled keys validate and so callers can still mint
// a wide-open key when they explicitly want one.
var KnownScopes = map[string]struct{}{
	ScopeMessagesRead:        {},
	ScopeMessagesWrite:       {},
	ScopeCommandsWrite:       {},
	ScopeInteractionsRespond: {},
	ScopeProfileWrite:        {},
	ScopeServersRead:         {},
	ScopeDeveloperManage:     {},
	ScopeFull:                {},
}

// ScopesKey is the context value where the middleware stashes the scopes
// array that came with the API key used to authenticate the request. Only
// present for API-key requests; absent for normal user JWTs. Downstream
// callers should go through HasScope instead of reading this directly.
const ScopesKey contextKey = "apiKeyScopes"

// ValidateScopes checks that every element of the input is a known scope
// and that the set is non-empty. Returns the offending scope on the first
// failure so the caller can surface it in the 400 response.
//
// Empty-set is rejected: a scope-less key would be useless and is almost
// certainly a client bug (forgot to send the field). Callers that want a
// universal key must pass "full" explicitly.
func ValidateScopes(scopes []string) (bad string, ok bool) {
	if len(scopes) == 0 {
		return "", false
	}
	for _, s := range scopes {
		if _, known := KnownScopes[s]; !known {
			return s, false
		}
	}
	return "", true
}

// GetScopesFromContext returns the scopes stashed by the auth middleware.
// Returns nil when the request is not API-key-authenticated (normal user
// JWTs never set ScopesKey).
func GetScopesFromContext(r *http.Request) []string {
	v, _ := r.Context().Value(ScopesKey).([]string)
	return v
}

// HasScope reports whether the request is allowed to exercise `scope`.
//
// For user-JWT requests (IsAPIKeyAuth is false), HasScope always returns
// true — a user's own session has no scope constraints. For API-key
// requests, HasScope returns true iff the key's scopes include either
// the specific `scope` or the "full" meta-scope.
func HasScope(r *http.Request, scope string) bool {
	if !IsAPIKeyAuth(r) {
		return true
	}
	for _, s := range GetScopesFromContext(r) {
		if s == ScopeFull || s == scope {
			return true
		}
	}
	return false
}

// RequireScope returns middleware that rejects an API-key-authenticated
// request whose scopes do not include `scope` (or the "full" meta-scope).
// Non-API-key requests pass through unchanged.
//
// The 403 body is intentionally parseable — clients (and bot authors
// staring at a CLI error) need to know which scope is missing so they can
// re-issue the key with the right grants.
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !HasScope(r, scope) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				// Hand-rolled so this file doesn't need to import encoding/json;
				// the scope name is constrained to [a-z:] so injection is moot.
				_, _ = w.Write([]byte(`{"error":"bot token missing scope: ` + scope + `"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
