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

func handleWebSocket(hub *ws.Hub, authService *auth.AuthService, repo *db.Repository, tickets *ticketStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var userID string

		if ticketStr := r.URL.Query().Get("ticket"); ticketStr != "" {
			// Preferred path: short-lived single-use ticket (JWT never hits the URL)
			var ok bool
			userID, ok = tickets.Consume(ticketStr)
			if !ok {
				http.Error(w, "invalid or expired ticket", http.StatusUnauthorized)
				return
			}
		} else {
			// Authorization header only — never accept JWT via URL query params
			// (URL query params are recorded in proxy/server access logs).
			tokenString := ""
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				tokenString = authHeader[7:]
			}
			if tokenString == "" {
				http.Error(w, "authorization required", http.StatusUnauthorized)
				return
			}

			var iat int64
			var err error
			userID, iat, err = authService.ValidateTokenFull(tokenString)
			if err != nil {
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}

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
