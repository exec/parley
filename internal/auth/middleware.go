package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

const (
	// UserIDKey is the context key for storing user ID
	UserIDKey = "userID"
	// AuthorizationHeader is the HTTP header name for authorization
	AuthorizationHeader = "Authorization"
	// BearerPrefix is the prefix for Bearer token
	BearerPrefix = "Bearer "
)

// AuthMiddleware creates HTTP middleware for authentication
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract token from Authorization header
		tokenString := extractToken(r)
		if tokenString == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		// Validate token (doesn't need repo, pass nil)
		authService := NewAuthService(nil)
		userID, err := authService.ValidateToken(tokenString)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Add userID to request context
		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
			userID, iat, err := svc.ValidateTokenFull(tokenString)
			if err != nil {
				http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
				return
			}
			if svc.repo != nil {
				if kicked, _ := svc.IsForceLoggedOut(r.Context(), userID, iat); kicked {
					http.Error(w, "Session expired", http.StatusUnauthorized)
					return
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