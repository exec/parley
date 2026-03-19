package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"parley/internal/auth"
	"parley/internal/db"
	ws "parley/internal/websocket"
)

func handleUpdateStatus(authService *auth.AuthService, hub *ws.Hub, repo *db.Repository) http.HandlerFunc {
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

		// Broadcast USER_UPDATE (with status fields) to every server the user belongs to,
		// matching the same pattern as profile updates in user_handlers.go.
		if hub != nil {
			userID, parseErr := strconv.ParseInt(userIDStr, 10, 64)
			if parseErr == nil {
				servers, serversErr := repo.GetServersByUserID(r.Context(), userID)
				if serversErr == nil {
					payload, marshalErr := json.Marshal(map[string]string{
						"user_id":     userIDStr,
						"status_type": req.StatusType,
						"status_text": req.StatusText,
					})
					if marshalErr == nil {
						for _, srv := range servers {
							hub.BroadcastToChannel(fmt.Sprintf("server:%d", srv.ID), ws.EventUserUpdate, payload)
						}
					}
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Status updated"})
	}
}
