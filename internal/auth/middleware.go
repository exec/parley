package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type contextKey string

const (
	// UserIDKey is the context key for storing user ID
	UserIDKey contextKey = "userID"
	// IsAPIKeyAuthKey is the context key for marking API key authenticated requests
	IsAPIKeyAuthKey contextKey = "isAPIKeyAuth"
	// OwnerUserIDKey is the context key for the request's *owner* user ID:
	// - for JWT-auth requests, the owner is the authenticated user themself;
	// - for API-key-auth requests, the owner is the user who minted the key
	//   (different from the authenticated user when the key belongs to a bot).
	// The aggregate per-owner rate limiter (D4) keys on this value so that
	// one user plus all their bots share a single write-rate bucket.
	OwnerUserIDKey contextKey = "ownerUserID"
	// IsImpersonationKey marks the request as carrying an admin-minted
	// impersonation token rather than the real user's session. Handlers that
	// guard destructive / profile-mutating actions use this to refuse work;
	// the audit-log path uses it to record every request an admin makes on
	// another user's behalf.
	IsImpersonationKey contextKey = "isImpersonation"
	// ActorAdminIDKey is the admin_users.id of the admin who minted the
	// impersonation token currently being exercised. Present only when
	// IsImpersonationKey is true.
	ActorAdminIDKey contextKey = "actorAdminID"
	// AuthorizationHeader is the HTTP header name for authorization
	AuthorizationHeader = "Authorization"
	// BearerPrefix is the prefix for Bearer token
	BearerPrefix = "Bearer "
)

// SHA256Hex returns the hex-encoded SHA-256 digest of s.
func SHA256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// auditDedup throttles audit log emissions keyed by an arbitrary string so a
// client generating the same reject reason repeatedly doesn't produce one log
// line per request. Callers namespace their keys (e.g. "banned:<user_id>",
// "ratelimit:u:<user_id>", "ratelimit:ip:<ip>").
// Value: time.Time of the last emission.
var auditDedup sync.Map

// AuditDedupInterval is the per-key suppression window. First occurrence after
// an interval is always logged; subsequent hits within the window are silent.
// The deny/reject action itself is NOT suppressed — only the log.
const AuditDedupInterval = 5 * time.Minute

// ShouldLogAuditOnce returns true if we haven't logged this key in the last
// AuditDedupInterval, and records the time if so.
func ShouldLogAuditOnce(key string) bool {
	now := time.Now()
	if prev, ok := auditDedup.Load(key); ok {
		if last, ok := prev.(time.Time); ok && now.Sub(last) < AuditDedupInterval {
			return false
		}
	}
	auditDedup.Store(key, now)
	return true
}

// ClientIP returns the real client IP. Behind the DMZ nginx (which sets
// X-Real-IP to $remote_addr after real_ip_header CF-Connecting-IP), that
// header is the trusted real-client IP — client-supplied copies are
// overwritten at the proxy. We do NOT read X-Forwarded-For because
// Cloudflare preserves client-supplied XFF as the leftmost token (see
// audit finding F6); it is attacker-controlled.
func ClientIP(r *http.Request) string {
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// IsAPIKeyAuth returns true if the request was authenticated via an API key.
func IsAPIKeyAuth(r *http.Request) bool {
	v, _ := r.Context().Value(IsAPIKeyAuthKey).(bool)
	return v
}

// extractToken extracts the JWT token from the Authorization header
func extractToken(r *http.Request) string {
	authHeader := r.Header.Get(AuthorizationHeader)
	if authHeader == "" {
		return ""
	}

	// Check for Bearer prefix
	if strings.HasPrefix(authHeader, BearerPrefix) {
		return strings.TrimPrefix(authHeader, BearerPrefix)
	}

	return ""
}

// impersonationAuditDedupInterval bounds how often the per-request audit
// line fires for the same actor/target/path triple. A single page load can
// fan out into dozens of XHRs; we want one line per distinct action within
// a short window, not one line per resource fetched.
const impersonationAuditDedupInterval = 5 * time.Second

// AuthMiddlewareWith returns an auth middleware that also checks for force logout
// using the provided AuthService (which must have a non-nil repo).
func AuthMiddlewareWith(svc *AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := extractToken(r)
			if tokenString == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}

			// API key authentication
			if strings.HasPrefix(tokenString, "plk_") {
				if svc.repo == nil {
					http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
					return
				}
				keyHash := SHA256Hex(tokenString)
				keyID, userID, ownerID, scopes, err := svc.repo.GetAPIKeyByHash(r.Context(), keyHash)
				if err != nil {
					http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
					return
				}
				userIDStr := strconv.FormatInt(userID, 10)
				ownerIDStr := strconv.FormatInt(ownerID, 10)
				if banned, reason, _ := svc.IsBanned(r.Context(), userIDStr); banned {
					if ShouldLogAuditOnce("banned:" + userIDStr) {
						log.Printf("audit: blocked_banned_user user_id=%s ip=%s via=api_key reason=%q path=%s", userIDStr, ClientIP(r), reason, r.URL.Path)
					}
					http.Error(w, "Account banned", http.StatusForbidden)
					return
				}
				// Update last_used_at asynchronously
				go svc.repo.UpdateAPIKeyLastUsed(context.Background(), keyID)
				ctx := context.WithValue(r.Context(), UserIDKey, userIDStr)
				ctx = context.WithValue(ctx, IsAPIKeyAuthKey, true)
				// The owner is the user who created this key; for bot
				// keys ownerID != userID. Needed by the aggregate
				// per-owner write limiter (D4) so the bot's writes
				// count against the owner's bucket.
				ctx = context.WithValue(ctx, OwnerUserIDKey, ownerIDStr)
				// Stash the key's scopes so RequireScope (and any
				// HasScope callers) can enforce them. Empty slice
				// for a pre-migration row means "no scopes" — the
				// safe failure mode: nothing HasScope-checks will
				// permit, and the owner must rotate the key.
				ctx = context.WithValue(ctx, ScopesKey, scopes)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			info, err := svc.ValidateTokenFull(tokenString)
			if err != nil {
				http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
				return
			}
			userID := info.UserID
			if svc.repo != nil {
				if st, err := svc.GetSessionStatus(r.Context(), userID); err == nil {
					if st.ForceLogoutAt.Valid && info.IssuedAt <= st.ForceLogoutAt.Time.Unix() {
						http.Error(w, "Session expired", http.StatusUnauthorized)
						return
					}
					if st.BannedAt.Valid {
						reason := st.BanReason.String
						if ShouldLogAuditOnce("banned:" + userID) {
							log.Printf("audit: blocked_banned_user user_id=%s ip=%s via=jwt reason=%q path=%s", userID, ClientIP(r), reason, r.URL.Path)
						}
						http.Error(w, "Account banned", http.StatusForbidden)
						return
					}
				}
			}
			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			// For JWT auth the owner is the user themselves; the owner key
			// is set so the aggregate per-owner write limiter (D4) has a
			// consistent shape for both auth paths.
			ctx = context.WithValue(ctx, OwnerUserIDKey, userID)
			if info.IsImpersonation {
				ctx = context.WithValue(ctx, IsImpersonationKey, true)
				ctx = context.WithValue(ctx, ActorAdminIDKey, info.ActorAdminID)
				// Deduped so a single page-load's many XHRs collapse to
				// one line per (actor, target, path) in a 5s window; a
				// truly novel action still emits immediately.
				key := "impersonation:" + info.ActorAdminID + ":" + userID + ":" + r.URL.Path
				if shouldLogKeyedOnce(key, impersonationAuditDedupInterval) {
					log.Printf("audit: impersonation_request actor_admin_id=%s target_user_id=%s method=%s path=%s",
						info.ActorAdminID, userID, r.Method, r.URL.Path)
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// shouldLogKeyedOnce is a variant of ShouldLogAuditOnce that takes an
// explicit interval — used for the impersonation request logger, which
// needs a much tighter window than the 5-minute default so distinct
// actions aren't silently suppressed.
func shouldLogKeyedOnce(key string, interval time.Duration) bool {
	now := time.Now()
	if prev, ok := auditDedup.Load(key); ok {
		if last, ok := prev.(time.Time); ok && now.Sub(last) < interval {
			return false
		}
	}
	auditDedup.Store(key, now)
	return true
}

// IsImpersonation reports whether the request is authenticated with an
// admin-minted impersonation token.
func IsImpersonation(r *http.Request) bool {
	v, _ := r.Context().Value(IsImpersonationKey).(bool)
	return v
}

// ActorAdminID returns the admin_users.id of the admin driving the
// impersonation session, or "" when the request isn't an impersonation.
func ActorAdminID(r *http.Request) string {
	v, _ := r.Context().Value(ActorAdminIDKey).(string)
	return v
}

// GetUserIDFromContext retrieves the userID from the request context
func GetUserIDFromContext(r *http.Request) string {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		return ""
	}
	return userID
}

// GetOwnerUserIDFromContext retrieves the owner's user ID from the request
// context. For JWT-authenticated requests the owner is the user themself;
// for API-key-authenticated requests (including bot keys) the owner is the
// user who minted the key. Used by the aggregate per-owner write limiter
// (D4) so a user plus all their bots share one write-rate bucket. Falls
// back to the authenticated user ID when unset (older middleware paths).
func GetOwnerUserIDFromContext(r *http.Request) string {
	if ownerID, ok := r.Context().Value(OwnerUserIDKey).(string); ok && ownerID != "" {
		return ownerID
	}
	return GetUserIDFromContext(r)
}

// GetUserIDFromContextWithError retrieves the userID from the request context with error
func GetUserIDFromContextWithError(r *http.Request) (string, error) {
	userID := GetUserIDFromContext(r)
	if userID == "" {
		return "", errors.New("user not authenticated")
	}
	return userID, nil
}