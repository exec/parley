package main

import (
	"encoding/json"
	"net/http"

	"parley/internal/auth"
	"parley/internal/websocket"
)

func handleUpdateStatus(authService *auth.AuthService, hub *websocket.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			StatusType string `json:"status_type"`
			StatusText string `json:"status_text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := authService.UpdateStatus(r.Context(), userIDStr, req.StatusType, req.StatusText); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Broadcast status update to all connected clients
		hub.BroadcastStatusUpdate(userIDStr, req.StatusType, req.StatusText)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Status updated"})
	}
}
