package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const adminIDKey contextKey = "adminID"
const adminUsernameKey contextKey = "adminUsername"

func adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return []byte(adminJWTSecret), nil
		})
		if err != nil || !token.Valid {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		adminIDFloat, ok := claims["admin_id"].(float64)
		if !ok {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		adminID := int64(adminIDFloat)
		adminUsername, ok := claims["username"].(string)
		if !ok {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := r.Context()
		ctx = context.WithValue(ctx, adminIDKey, adminID)
		ctx = context.WithValue(ctx, adminUsernameKey, adminUsername)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getAdminID(r *http.Request) int64 {
	v, _ := r.Context().Value(adminIDKey).(int64)
	return v
}
