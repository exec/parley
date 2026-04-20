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

// ClientIP extracts the real client IP from the request. When Parley sits
// behind the DMZ nginx (which sets X-Forwarded-For from CF-Connecting-IP),
// r.RemoteAddr is the DMZ's internal IP, not the user's — so we prefer the
// forwarded headers.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// XFF can be a chain; the leftmost is the original client.
		if idx := strings.Index(xff, ","); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
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
				keyID, userID, err := svc.repo.GetAPIKeyByHash(r.Context(), keyHash)
				if err != nil {
					http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
					return
				}
				userIDStr := strconv.FormatInt(userID, 10)
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
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			userID, iat, err := svc.ValidateTokenFull(tokenString)
			if err != nil {
				http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
				return
			}
			if svc.repo != nil {
				if st, err := svc.GetSessionStatus(r.Context(), userID); err == nil {
					if st.ForceLogoutAt.Valid && iat <= st.ForceLogoutAt.Time.Unix() {
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
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserIDFromContext retrieves the userID from the request context
func GetUserIDFromContext(r *http.Request) string {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		return ""
	}
	return userID
}

// GetUserIDFromContextWithError retrieves the userID from the request context with error
func GetUserIDFromContextWithError(r *http.Request) (string, error) {
	userID := GetUserIDFromContext(r)
	if userID == "" {
		return "", errors.New("user not authenticated")
	}
	return userID, nil
}