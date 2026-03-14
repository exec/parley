package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
)

// Context keys
type contextKey string

const (
	UserIDKey contextKey = "user_id"
)

// AuthMiddleware validates the authorization token and extracts user ID
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// For development, allow a test token
			testToken := r.URL.Query().Get("token")
			if testToken != "" {
				userID := validateToken(testToken)
				if userID != "" {
					ctx := context.WithValue(r.Context(), UserIDKey, userID)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			w.WriteHeader(http.StatusUnauthorized)
			render.JSON(w, r, map[string]string{"error": "authorization header required"})
			return
		}

		// Extract token from "Bearer <token>" format
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			w.WriteHeader(http.StatusUnauthorized)
			render.JSON(w, r, map[string]string{"error": "invalid authorization header format"})
			return
		}

		token := parts[1]
		userID := validateToken(token)
		if userID == "" {
			w.WriteHeader(http.StatusUnauthorized)
			render.JSON(w, r, map[string]string{"error": "invalid or expired token"})
			return
		}

		// Add user ID to request context
		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// validateToken validates the token and returns the user ID
// In a real implementation, this would validate a JWT or session token
func validateToken(token string) string {
	// For development/testing, accept any non-empty token
	// In production, this would validate against a JWT or database
	if token != "" {
		// For testing, we'll use the token as the user ID if it looks like a UUID
		return token
	}
	return ""
}

// RequireRoleMiddleware creates middleware that checks user has required role
func RequireRoleMiddleware(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get user role from context
			role, ok := r.Context().Value("user_role").(string)
			if !ok || role != requiredRole {
				w.WriteHeader(http.StatusForbidden)
				render.JSON(w, r, map[string]string{"error": "insufficient permissions"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetUserID extracts the user ID from the request context
func GetUserID(r *http.Request) string {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		return ""
	}
	return userID
}

// CORSMiddleware adds CORS headers to responses
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequestIDMiddleware is exposed for external use
var RequestIDMiddleware = middleware.RequestID