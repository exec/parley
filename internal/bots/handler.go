// internal/bots/handler.go
package bots

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"parley/internal/server"
)

// Handler holds the Service for HTTP dispatch.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// callerID extracts the authenticated user ID from the request context.
// The auth middleware stores a string user ID under server.UserIDKey, placed
// there by bridgeUserIDMiddleware in cmd/api/routes.go.
func callerID(r *http.Request) (int64, bool) {
	s, ok := r.Context().Value(server.UserIDKey).(string)
	if !ok || s == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// serverIDParam parses the {id} URL param as int64.
func serverIDParam(r *http.Request) (int64, bool) {
	n, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return n, err == nil
}

// writeErr writes a JSON error response.
func writeErr(w http.ResponseWriter, r *http.Request, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// handleSvcErr maps service errors to HTTP status codes.
func handleSvcErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeErr(w, r, 404, "not found")
	case errors.Is(err, ErrForbidden):
		writeErr(w, r, 403, "forbidden")
	case errors.Is(err, ErrAlreadyExists):
		writeErr(w, r, 409, "already exists")
	default:
		writeErr(w, r, 500, "internal server error")
	}
}

// ListBots handles GET /api/servers/{id}/bots
func (h *Handler) ListBots(w http.ResponseWriter, r *http.Request) {
	sid, ok := serverIDParam(r)
	if !ok {
		writeErr(w, r, 400, "invalid server id")
		return
	}
	bots, err := h.svc.ListBots(r.Context(), sid)
	if err != nil {
		handleSvcErr(w, r, err)
		return
	}
	render.JSON(w, r, bots)
}

// AddBot handles POST /api/servers/{id}/bots
func (h *Handler) AddBot(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	sid, ok := serverIDParam(r)
	if !ok {
		writeErr(w, r, 400, "invalid server id")
		return
	}
	var req struct {
		InviteToken string `json:"invite_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.InviteToken == "" {
		writeErr(w, r, 400, "invite_token required")
		return
	}
	if err := h.svc.AddBot(r.Context(), sid, uid, req.InviteToken); err != nil {
		handleSvcErr(w, r, err)
		return
	}
	w.WriteHeader(204)
}

// RemoveBot handles DELETE /api/servers/{id}/bots/{botId}
func (h *Handler) RemoveBot(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	sid, ok := serverIDParam(r)
	if !ok {
		writeErr(w, r, 400, "invalid server id")
		return
	}
	botID, err := strconv.ParseInt(chi.URLParam(r, "botId"), 10, 64)
	if err != nil {
		writeErr(w, r, 400, "invalid bot id")
		return
	}
	if err := h.svc.RemoveBot(r.Context(), sid, botID, uid); err != nil {
		handleSvcErr(w, r, err)
		return
	}
	w.WriteHeader(204)
}

// GetAIConfig handles GET /api/servers/{id}/ai-config
func (h *Handler) GetAIConfig(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	sid, ok := serverIDParam(r)
	if !ok {
		writeErr(w, r, 400, "invalid server id")
		return
	}
	cfg, err := h.svc.GetAIConfig(r.Context(), sid, uid)
	if err != nil {
		handleSvcErr(w, r, err)
		return
	}
	render.JSON(w, r, cfg)
}

// SetAIConfig handles PUT /api/servers/{id}/ai-config
func (h *Handler) SetAIConfig(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	sid, ok := serverIDParam(r)
	if !ok {
		writeErr(w, r, 400, "invalid server id")
		return
	}
	var req struct {
		Provider     string `json:"provider"`
		Model        string `json:"model"`
		APIKey       string `json:"api_key"`
		SystemPrompt string `json:"system_prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, 400, "invalid body")
		return
	}
	if req.Provider == "" || req.Model == "" {
		writeErr(w, r, 400, "provider and model required")
		return
	}
	if err := h.svc.SetAIConfig(r.Context(), sid, uid, req.Provider, req.Model, req.SystemPrompt, req.APIKey); err != nil {
		handleSvcErr(w, r, err)
		return
	}
	w.WriteHeader(204)
}

// GetAIUsage handles GET /api/servers/{id}/ai-usage
func (h *Handler) GetAIUsage(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	sid, ok := serverIDParam(r)
	if !ok {
		writeErr(w, r, 400, "invalid server id")
		return
	}
	usage, err := h.svc.GetAIUsage(r.Context(), sid, uid)
	if err != nil {
		handleSvcErr(w, r, err)
		return
	}
	render.JSON(w, r, usage)
}

// GetMyBots handles GET /api/bots/mine
func (h *Handler) GetMyBots(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	bots, err := h.svc.GetMyBots(r.Context(), uid)
	if err != nil {
		handleSvcErr(w, r, err)
		return
	}
	render.JSON(w, r, bots)
}

// ResolveInvite handles GET /api/bots/invite/{token} (public)
func (h *Handler) ResolveInvite(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	info, err := h.svc.ResolveInvite(r.Context(), token)
	if err != nil {
		handleSvcErr(w, r, err)
		return
	}
	render.JSON(w, r, info)
}

// AcceptInvite handles POST /api/bots/invite/{token}/accept
func (h *Handler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	token := chi.URLParam(r, "token")
	var req struct {
		ServerID int64 `json:"server_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ServerID == 0 {
		writeErr(w, r, 400, "server_id required")
		return
	}
	if err := h.svc.AcceptInvite(r.Context(), token, req.ServerID, uid); err != nil {
		handleSvcErr(w, r, err)
		return
	}
	w.WriteHeader(204)
}
