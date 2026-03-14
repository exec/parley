package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
)

// Handler handles HTTP requests for server operations
type Handler struct {
	service *ServerService
}

// NewHandler creates a new server handler
func NewHandler(service *ServerService) *Handler {
	return &Handler{service: service}
}

// Router returns a chi router with all server routes
func (h *Handler) Router() *chi.Mux {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(render.SetContentType(render.ContentTypeJSON))

	// Routes
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware) // Require authentication for all routes

		r.Post("/servers", h.CreateServer)
		r.Get("/servers", h.GetUserServers)
		r.Get("/servers/{id}", h.GetServer)
		r.Put("/servers/{id}", h.UpdateServer)
		r.Delete("/servers/{id}", h.DeleteServer)

		// Member routes
		r.Post("/servers/{id}/members", h.AddMember)
		r.Delete("/servers/{id}/members/{userID}", h.RemoveMember)
		r.Get("/servers/{id}/members", h.GetMembers)

		// Invite routes
		r.Post("/servers/{id}/invites", h.CreateInvite)
	})

	// Public invite route (doesn't require auth, but joins if authenticated)
	r.Get("/invites/{code}", h.GetInvite)

	return r
}

// Request/Response types

type CreateServerRequest struct {
	Name    string `json:"name"`
	IconURL string `json:"icon_url"`
}

type UpdateServerRequest struct {
	Name    string `json:"name"`
	IconURL string `json:"icon_url"`
}

type AddMemberRequest struct {
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// HTTP Handlers

// CreateServer handles POST /servers
func (h *Handler) CreateServer(w http.ResponseWriter, r *http.Request) {
	var req CreateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "invalid request body"})
		return
	}

	if req.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "server name is required"})
		return
	}

	// Get owner ID from auth context
	ownerID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || ownerID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	server, err := h.service.CreateServer(r.Context(), req.Name, req.IconURL, ownerID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	render.JSON(w, r, server)
}

// GetServer handles GET /servers/:id
func (h *Handler) GetServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "server ID is required"})
		return
	}

	server, err := h.service.GetServer(r.Context(), id)
	if err != nil {
		if errors.Is(err, errors.New("server not found")) {
			w.WriteHeader(http.StatusNotFound)
			render.JSON(w, r, ErrorResponse{Error: "server not found"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}

	render.JSON(w, r, server)
}

// GetUserServers handles GET /servers
func (h *Handler) GetUserServers(w http.ResponseWriter, r *http.Request) {
	// Get user ID from auth context
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	servers, err := h.service.GetUserServers(r.Context(), userID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if servers == nil {
		servers = []*Server{}
	}

	render.JSON(w, r, servers)
}

// UpdateServer handles PUT /servers/:id
func (h *Handler) UpdateServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "server ID is required"})
		return
	}

	var req UpdateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "invalid request body"})
		return
	}

	if req.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "server name is required"})
		return
	}

	// Verify the user is the owner
	server, err := h.service.GetServer(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}

	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" || server.OwnerID != userID {
		w.WriteHeader(http.StatusForbidden)
		render.JSON(w, r, ErrorResponse{Error: "only the server owner can update the server"})
		return
	}

	updatedServer, err := h.service.UpdateServer(r.Context(), id, req.Name, req.IconURL)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}

	render.JSON(w, r, updatedServer)
}

// DeleteServer handles DELETE /servers/:id
func (h *Handler) DeleteServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "server ID is required"})
		return
	}

	// Verify the user is the owner
	server, err := h.service.GetServer(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}

	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" || server.OwnerID != userID {
		w.WriteHeader(http.StatusForbidden)
		render.JSON(w, r, ErrorResponse{Error: "only the server owner can delete the server"})
		return
	}

	err = h.service.DeleteServer(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AddMember handles POST /servers/:id/members
func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "server ID is required"})
		return
	}

	var req AddMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "invalid request body"})
		return
	}

	if req.UserID == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "user ID is required"})
		return
	}

	// Verify the user is the owner or a member
	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}

	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	// Only owner can add members
	if server.OwnerID != userID {
		w.WriteHeader(http.StatusForbidden)
		render.JSON(w, r, ErrorResponse{Error: "only the server owner can add members"})
		return
	}

	err = h.service.AddMember(r.Context(), serverID, req.UserID, req.Nickname)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	render.JSON(w, r, map[string]string{
		"server_id": serverID,
		"user_id":   req.UserID,
		"nickname":  req.Nickname,
	})
}

// RemoveMember handles DELETE /servers/:id/members/:userID
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	userID := chi.URLParam(r, "userID")

	if serverID == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "server ID is required"})
		return
	}

	if userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "user ID is required"})
		return
	}

	// Verify the user is the owner
	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}

	currentUserID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || currentUserID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	// Only owner can remove members (or the member themselves)
	if server.OwnerID != currentUserID && userID != currentUserID {
		w.WriteHeader(http.StatusForbidden)
		render.JSON(w, r, ErrorResponse{Error: "only the server owner or the member themselves can remove members"})
		return
	}

	// Cannot remove the owner
	if userID == server.OwnerID {
		w.WriteHeader(http.StatusForbidden)
		render.JSON(w, r, ErrorResponse{Error: "cannot remove the server owner"})
		return
	}

	err = h.service.RemoveMember(r.Context(), serverID, userID)
	if err != nil {
		if errors.Is(err, errors.New("member not found")) {
			w.WriteHeader(http.StatusNotFound)
			render.JSON(w, r, ErrorResponse{Error: "member not found"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetMembers handles GET /servers/:id/members
func (h *Handler) GetMembers(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "server ID is required"})
		return
	}

	// Verify the server exists
	_, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}

	members, err := h.service.GetMembers(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if members == nil {
		members = []*ServerMember{}
	}

	render.JSON(w, r, members)
}

// CreateInvite handles POST /servers/:id/invites
func (h *Handler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "server ID is required"})
		return
	}

	// Verify the user is authenticated
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	// Verify the server exists and user is a member
	_, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}

	invite, err := h.service.CreateInvite(r.Context(), serverID, userID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	render.JSON(w, r, invite)
}

// GetInvite handles GET /api/invites/:code
// Works for both regular invite codes and server vanity URLs.
func (h *Handler) GetInvite(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "invite code is required"})
		return
	}

	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	// First try as a regular invite code, then fall back to vanity URL
	server, err := h.service.JoinServerByInvite(r.Context(), code, userID)
	if err != nil {
		// Try as vanity URL
		server, err = h.service.JoinServerByVanityURL(r.Context(), code, userID)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			render.JSON(w, r, ErrorResponse{Error: "invite not found or invalid"})
			return
		}
	}

	render.JSON(w, r, map[string]interface{}{
		"server":  server,
		"message": "Successfully joined server",
	})
}

// SetVanityURL handles PUT /servers/:id/vanity
func (h *Handler) SetVanityURL(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "server ID is required"})
		return
	}

	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	var req struct {
		VanityURL string `json:"vanity_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "invalid request body"})
		return
	}

	server, err := h.service.SetVanityURL(r.Context(), serverID, userID, req.VanityURL)
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "only the server owner can set the vanity URL" {
			status = http.StatusForbidden
		} else if err.Error() == "server not found" {
			status = http.StatusNotFound
		}
		w.WriteHeader(status)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}

	render.JSON(w, r, server)
}