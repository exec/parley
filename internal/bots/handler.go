// internal/bots/handler.go
package bots

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"fmt"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"parley/internal/auth"
	"parley/internal/httputil"
	ws "parley/internal/websocket"
)

// hub is the minimal WS interface the handler needs.
type hub interface {
	BroadcastToChannel(channelID, event string, payload []byte)
}

// Handler holds the Service for HTTP dispatch.
type Handler struct {
	svc *Service
	hub hub
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// SetHub wires in the WebSocket hub for broadcasting bot membership events.
func (h *Handler) SetHub(hub hub) {
	h.hub = hub
}

// callerID extracts the authenticated user ID from the request context.
func callerID(r *http.Request) (int64, bool) {
	s := auth.GetUserIDFromContext(r)
	if s == "" {
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

// handleSvcErr maps service errors to HTTP status codes.
func handleSvcErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httputil.JSONError(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrForbidden):
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, ErrAlreadyExists):
		httputil.JSONError(w, "already exists", http.StatusConflict)
	default:
		httputil.InternalError(w, err)
	}
}

// ListBots handles GET /api/servers/{id}/bots
func (h *Handler) ListBots(w http.ResponseWriter, r *http.Request) {
	sid, ok := serverIDParam(r)
	if !ok {
		httputil.JSONError(w, "invalid server id", http.StatusBadRequest)
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
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	sid, ok := serverIDParam(r)
	if !ok {
		httputil.JSONError(w, "invalid server id", http.StatusBadRequest)
		return
	}
	var req struct {
		InviteToken string `json:"invite_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.InviteToken == "" {
		httputil.JSONError(w, "invite_token required", http.StatusBadRequest)
		return
	}
	botUserID, err := h.svc.AddBot(r.Context(), sid, uid, req.InviteToken)
	if err != nil {
		handleSvcErr(w, r, err)
		return
	}
	if h.hub != nil {
		payload, _ := json.Marshal(map[string]string{
			"server_id": fmt.Sprintf("%d", sid),
			"user_id":   fmt.Sprintf("%d", botUserID),
		})
		h.hub.BroadcastToChannel(fmt.Sprintf("server:%d", sid), ws.EventMemberJoin, payload)
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveBot handles DELETE /api/servers/{id}/bots/{botId}
func (h *Handler) RemoveBot(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	sid, ok := serverIDParam(r)
	if !ok {
		httputil.JSONError(w, "invalid server id", http.StatusBadRequest)
		return
	}
	botID, err := strconv.ParseInt(chi.URLParam(r, "botId"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid bot id", http.StatusBadRequest)
		return
	}
	if err := h.svc.RemoveBot(r.Context(), sid, botID, uid); err != nil {
		handleSvcErr(w, r, err)
		return
	}
	if h.hub != nil {
		payload, _ := json.Marshal(map[string]string{
			"server_id": fmt.Sprintf("%d", sid),
			"user_id":   fmt.Sprintf("%d", botID),
		})
		h.hub.BroadcastToChannel(fmt.Sprintf("server:%d", sid), ws.EventMemberLeave, payload)
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetAIConfig handles GET /api/servers/{id}/ai-config
func (h *Handler) GetAIConfig(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	sid, ok := serverIDParam(r)
	if !ok {
		httputil.JSONError(w, "invalid server id", http.StatusBadRequest)
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
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	sid, ok := serverIDParam(r)
	if !ok {
		httputil.JSONError(w, "invalid server id", http.StatusBadRequest)
		return
	}
	var req struct {
		Provider          string `json:"provider"`
		Model             string `json:"model"`
		APIKey            string `json:"api_key"`
		PresetVerbosity   string `json:"preset_verbosity"`
		PresetPersonality string `json:"preset_personality"`
		PresetRole        string `json:"preset_role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.Provider == "" || req.Model == "" {
		httputil.JSONError(w, "provider and model required", http.StatusBadRequest)
		return
	}
	if req.PresetVerbosity == "" {
		req.PresetVerbosity = "concise"
	}
	if req.PresetPersonality == "" {
		req.PresetPersonality = "friendly"
	}
	if req.PresetRole == "" {
		req.PresetRole = "assistant"
	}
	if err := h.svc.SetAIConfig(r.Context(), sid, uid, req.Provider, req.Model, req.PresetVerbosity, req.PresetPersonality, req.PresetRole, req.APIKey); err != nil {
		handleSvcErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetAIUsage handles GET /api/servers/{id}/ai-usage
func (h *Handler) GetAIUsage(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	sid, ok := serverIDParam(r)
	if !ok {
		httputil.JSONError(w, "invalid server id", http.StatusBadRequest)
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
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
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
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	token := chi.URLParam(r, "token")
	var req struct {
		ServerID int64 `json:"server_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ServerID == 0 {
		httputil.JSONError(w, "server_id required", http.StatusBadRequest)
		return
	}
	if _, err := h.svc.AcceptInvite(r.Context(), token, req.ServerID, uid, 0); err != nil {
		handleSvcErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
