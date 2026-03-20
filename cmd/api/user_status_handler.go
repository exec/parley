package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"parley/internal/auth"
	"parley/internal/db"
	ws "parley/internal/websocket"
)

func handleUpdateStatus(hub *ws.Hub, repo *db.Repository) http.HandlerFunc {
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

		// Reject offline — only the hub can write that.
		if req.StatusType == "offline" {
			jsonError(w, "offline status is managed by the server", http.StatusBadRequest)
			return
		}

		// Validate allowed values. "idle" is included (authService.UpdateStatus
		// uses an older allowlist that omits it, so we call repo directly).
		switch req.StatusType {
		case "online", "idle", "dnd", "invisible":
			// valid
		default:
			jsonError(w, "invalid status type", http.StatusBadRequest)
			return
		}

		// Trim status_text to 128 chars.
		if len(req.StatusText) > 128 {
			req.StatusText = req.StatusText[:128]
		}
		// Trim any trailing multi-byte boundary issues from the 128-char cut.
		req.StatusText = strings.TrimSpace(req.StatusText)

		userID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if err := repo.UpdateUserStatus(r.Context(), userID, req.StatusType, req.StatusText); err != nil {
			jsonError(w, "failed to update status", http.StatusInternalServerError)
			return
		}

		// Broadcast USER_STATUS_UPDATE cross-node via BroadcastStatusUpdate.
		if hub != nil {
			hub.BroadcastStatusUpdate(userIDStr, req.StatusType, req.StatusText)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status_type": req.StatusType,
			"status_text": req.StatusText,
		})
	}
}
