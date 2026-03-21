package notification

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/httputil"
)

// Handler handles HTTP requests for notifications.
type Handler struct {
	repo *db.Repository
}

// NewHandler creates a notification Handler.
func NewHandler(repo *db.Repository) *Handler {
	return &Handler{repo: repo}
}

// GetNotifications handles GET /api/notifications
func (h *Handler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	if userIDStr == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	notifications, err := h.repo.GetUserNotifications(r.Context(), userID, limit)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}

	resp := make([]Response, 0, len(notifications))
	for _, n := range notifications {
		resp = append(resp, ToResponse(n))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// MarkAllRead handles PATCH /api/notifications/read-all
func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	if userIDStr == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	if err := h.repo.MarkAllNotificationsRead(r.Context(), userID); err != nil {
		httputil.InternalError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// MarkRead handles PATCH /api/notifications/{id}/read
func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	if userIDStr == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	notifID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid notification ID", http.StatusBadRequest)
		return
	}

	if err := h.repo.MarkNotificationRead(r.Context(), notifID, userID); err != nil {
		httputil.InternalError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
