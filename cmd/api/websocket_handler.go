package main

import (
	"net/http"

	gorillawebsocket "github.com/gorilla/websocket"

	"parley/internal/auth"
	ws "parley/internal/websocket"
)

func handleWebSocket(hub *ws.Hub) http.HandlerFunc {
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

		authService := auth.NewAuthService(nil)
		userID, err := authService.ValidateToken(tokenString)
		if err != nil {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
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

		wsClient := ws.NewClient(hub, conn, userID)
		hub.RegisterClient(wsClient)

		go wsClient.WritePump()
		go wsClient.ReadPump()
	}
}
