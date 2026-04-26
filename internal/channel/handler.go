package channel

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"parley/internal/audit"
	"parley/internal/auth"
	"parley/internal/httputil"
	"parley/internal/permissions"
)

// Handler handles HTTP requests for channels
type Handler struct {
	service  *ChannelService
	auditSvc *audit.AuditService
}

// NewHandler creates a new channel handler
func NewHandler(service *ChannelService, auditSvc *audit.AuditService) *Handler {
	return &Handler{service: service, auditSvc: auditSvc}
}

// ServerRoutes returns the chi router with channel routes mounted at /servers/:serverID/channels
func (h *Handler) ServerRoutes() http.Handler {
	r := chi.NewRouter()

	r.Post("/", h.CreateChannel)
	r.Patch("/reorder", h.ReorderChannels)
	r.Get("/", h.GetServerChannels)

	return r
}

// ChannelRoutes returns the chi router with channel routes mounted at /channels/:id
func (h *Handler) ChannelRoutes() http.Handler {
	r := chi.NewRouter()

	r.Get("/", h.GetChannel)
	r.Put("/", h.UpdateChannel)
	r.Delete("/", h.DeleteChannel)

	return r
}

// CreateChannelRequest represents the request body for creating a channel
type CreateChannelRequest struct {
	Name     string  `json:"name"`
	Type     int     `json:"type"`
	ParentID *string `json:"parent_id"`
	Topic    string  `json:"topic"`
}

// CreateChannel handles POST /servers/:serverID/channels
func (h *Handler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	if serverID == "" {
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ch, err := h.service.CreateChannel(r.Context(), serverID, req.Name, req.Type, req.ParentID, req.Topic, userID)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			httputil.JSONError(w, "you do not have permission to create channels", http.StatusForbidden)
			return
		}
		httputil.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	actorIDInt, _ := strconv.ParseInt(userID, 10, 64)
	actorUsername, _ := h.service.Repo().GetUsernameByID(r.Context(), actorIDInt)
	serverIDInt, _ := strconv.ParseInt(serverID, 10, 64)
	h.auditSvc.Log(r.Context(), audit.Entry{
		ServerID:      serverIDInt,
		ActorID:       &actorIDInt,
		ActorUsername: actorUsername,
		Action:        "channel.create",
		TargetID:      ch.ID,
		TargetType:    "channel",
		TargetName:    ch.Name,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ch)
}

// GetChannel handles GET /channels/:id
func (h *Handler) GetChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.JSONError(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ch, err := h.service.GetChannel(r.Context(), id)
	if err != nil {
		httputil.JSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	// Verify server membership + ViewChannel permission
	channelIDInt, _ := strconv.ParseInt(id, 10, 64)
	serverIDInt, _ := strconv.ParseInt(ch.ServerID, 10, 64)
	userIDInt, _ := strconv.ParseInt(userID, 10, 64)

	srv, err := h.service.Repo().GetServerByID(r.Context(), serverIDInt)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	canView, err := permissions.HasChannelPermission(r.Context(), h.service.Repo(), serverIDInt, userIDInt, srv.OwnerID, channelIDInt, permissions.PermViewChannel)
	if err != nil || !canView {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ch)
}

// GetServerChannels handles GET /servers/:serverID/channels
func (h *Handler) GetServerChannels(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	if serverID == "" {
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	// ownerID is resolved inside the service via the server record; we pass userID and let the service look up ownerID.
	// To do the filtering we need the ownerID here — fetch it from the service.
	ownerID := h.service.GetServerOwnerID(r.Context(), serverID)
	if ownerID == "" {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	channels, err := h.service.GetServerChannels(r.Context(), serverID, userID, ownerID)
	if err != nil {
		if errors.Is(err, ErrServerNotFound) {
			httputil.JSONError(w, "server not found", http.StatusNotFound)
			return
		}
		httputil.InternalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channels)
}

// UpdateChannelRequest represents the request body for updating a channel
type UpdateChannelRequest struct {
	Name  string `json:"name"`
	Topic string `json:"topic"`
}

// UpdateChannel handles PUT /channels/:id
func (h *Handler) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.JSONError(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req UpdateChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	beforeCh, _ := h.service.GetChannel(r.Context(), id)

	ch, err := h.service.UpdateChannel(r.Context(), id, req.Name, req.Topic, userID)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			httputil.JSONError(w, "you do not have permission to update channels", http.StatusForbidden)
			return
		}
		httputil.JSONError(w, "invalid request", http.StatusBadRequest)
		return
	}

	if beforeCh != nil {
		actorIDInt, _ := strconv.ParseInt(userID, 10, 64)
		actorUsername, _ := h.service.Repo().GetUsernameByID(r.Context(), actorIDInt)
		serverIDInt, _ := strconv.ParseInt(ch.ServerID, 10, 64)
		h.auditSvc.Log(r.Context(), audit.Entry{
			ServerID:      serverIDInt,
			ActorID:       &actorIDInt,
			ActorUsername: actorUsername,
			Action:        "channel.update",
			TargetID:      ch.ID,
			TargetType:    "channel",
			TargetName:    ch.Name,
			Changes: map[string]any{
				"before": map[string]any{"name": beforeCh.Name, "topic": beforeCh.Topic},
				"after":  map[string]any{"name": ch.Name, "topic": ch.Topic},
			},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ch)
}

// DeleteChannel handles DELETE /channels/:id
func (h *Handler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.JSONError(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck // reason is optional; DELETE may have no body

	chToDelete, _ := h.service.GetChannel(r.Context(), id)

	if err := h.service.DeleteChannel(r.Context(), id, userID); err != nil {
		if errors.Is(err, ErrChannelNotFound) {
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
		} else if errors.Is(err, ErrForbidden) {
			httputil.JSONError(w, "only the server owner can delete channels", http.StatusForbidden)
		} else {
			httputil.InternalError(w, err)
		}
		return
	}

	if chToDelete != nil {
		actorIDInt, _ := strconv.ParseInt(userID, 10, 64)
		actorUsername, _ := h.service.Repo().GetUsernameByID(r.Context(), actorIDInt)
		serverIDInt, _ := strconv.ParseInt(chToDelete.ServerID, 10, 64)
		h.auditSvc.Log(r.Context(), audit.Entry{
			ServerID:      serverIDInt,
			ActorID:       &actorIDInt,
			ActorUsername: actorUsername,
			Action:        "channel.delete",
			TargetID:      chToDelete.ID,
			TargetType:    "channel",
			TargetName:    chToDelete.Name,
			Reason:        req.Reason,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// ReorderChannels handles PATCH /servers/:serverID/channels/reorder
func (h *Handler) ReorderChannels(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var orders []ChannelOrder
	if err := json.NewDecoder(r.Body).Decode(&orders); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	channels, err := h.service.ReorderChannels(r.Context(), serverID, orders, userID)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			httputil.JSONError(w, "forbidden", http.StatusForbidden)
			return
		}
		httputil.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channels)
}
