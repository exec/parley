package channel

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Handler handles HTTP requests for channels
type Handler struct {
	service *ChannelService
}

// NewHandler creates a new channel handler
func NewHandler(service *ChannelService) *Handler {
	return &Handler{service: service}
}

// ServerRoutes returns the chi router with channel routes mounted at /servers/:serverID/channels
func (h *Handler) ServerRoutes() http.Handler {
	r := chi.NewRouter()

	// Apply auth middleware to all routes
	r.Use(AuthMiddleware)

	r.Post("/", h.CreateChannel)
	r.Get("/", h.GetServerChannels)

	return r
}

// ChannelRoutes returns the chi router with channel routes mounted at /channels/:id
func (h *Handler) ChannelRoutes() http.Handler {
	r := chi.NewRouter()

	// Apply auth middleware to all routes
	r.Use(AuthMiddleware)

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
}

// CreateChannel handles POST /servers/:serverID/channels
func (h *Handler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	if serverID == "" {
		http.Error(w, "server ID is required", http.StatusBadRequest)
		return
	}

	var req CreateChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ch, err := h.service.CreateChannel(r.Context(), serverID, req.Name, req.Type, req.ParentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ch)
}

// GetChannel handles GET /channels/:id
func (h *Handler) GetChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	ch, err := h.service.GetChannel(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ch)
}

// GetServerChannels handles GET /servers/:serverID/channels
func (h *Handler) GetServerChannels(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	if serverID == "" {
		http.Error(w, "server ID is required", http.StatusBadRequest)
		return
	}

	channels, err := h.service.GetServerChannels(r.Context(), serverID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channels)
}

// UpdateChannelRequest represents the request body for updating a channel
type UpdateChannelRequest struct {
	Name string `json:"name"`
}

// UpdateChannel handles PUT /channels/:id
func (h *Handler) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	var req UpdateChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ch, err := h.service.UpdateChannel(r.Context(), id, req.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ch)
}

// DeleteChannel handles DELETE /channels/:id
func (h *Handler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	if err := h.service.DeleteChannel(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AuthMiddleware is a simple authentication middleware
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		// In a real implementation, you would validate the token
		// For now, we just require any authorization header to be present

		next.ServeHTTP(w, r)
	})
}