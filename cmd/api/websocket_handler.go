package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"

	gorillawebsocket "github.com/gorilla/websocket"

	"parley/internal/auth"
	"parley/internal/db"
	ws "parley/internal/websocket"
)

func handleWebSocket(hub *ws.Hub, authService *auth.AuthService, repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Accept token from Authorization header OR query param (browser WS can't set headers)
		tokenString := ""
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			tokenString = authHeader[7:]
		} else {
			tokenString = r.URL.Query().Get("token")
		}

		if tokenString == "" {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		userID, iat, err := authService.ValidateTokenFull(tokenString)
		if err != nil {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Check whether the user has been force-logged out since this token was issued
		forceLoggedOut, err := authService.IsForceLoggedOut(r.Context(), userID, iat)
		if err != nil {
			log.Printf("handleWebSocket: IsForceLoggedOut error for user %s: %v", userID, err)
			http.Error(w, "authorization check failed", http.StatusInternalServerError)
			return
		}
		if forceLoggedOut {
			http.Error(w, "session has been invalidated", http.StatusUnauthorized)
			return
		}

		// Resolve server-side display name so the client cannot spoof it
		var displayName string
		row := repo.DB().QueryRowContext(
			context.Background(),
			"SELECT COALESCE(display_name, username) FROM users WHERE id = $1",
			userID,
		)
		if scanErr := row.Scan(&displayName); scanErr != nil {
			if scanErr != sql.ErrNoRows {
				log.Printf("handleWebSocket: failed to query display name for user %s: %v", userID, scanErr)
			}
			displayName = userID // safe fallback
		}

		upgrader := gorillawebsocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true // native clients / same-origin requests have no Origin
				}
				return allowedOrigins[origin]
			},
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
			return
		}

		wsClient := ws.NewClient(hub, conn, userID, displayName)
		hub.RegisterClient(wsClient)

		go wsClient.WritePump()
		go wsClient.ReadPump()
	}
}
