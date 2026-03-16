package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"parley/internal/permissions"
)

// Handler handles HTTP requests for server operations
type Handler struct {
	service *ServerService
}

// NewHandler creates a new server handler
func NewHandler(service *ServerService) *Handler {
	return &Handler{service: service}
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

	// Only owner or Administrator can add bots
	if server.OwnerID != userID {
		ownerID, _ := permissions.ParseInt64(server.OwnerID)
		actorID, _ := permissions.ParseInt64(userID)
		sID, _ := permissions.ParseInt64(serverID)
		isAdmin, _ := permissions.HasPermission(r.Context(), h.service.Repo(), sID, actorID, ownerID, permissions.PermAdministrator)
		if !isAdmin {
			w.WriteHeader(http.StatusForbidden)
			render.JSON(w, r, ErrorResponse{Error: "Administrator permission required to add bots"})
			return
		}
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

	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	// Verify the server exists
	_, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}

	// Verify the requester is a member of the server
	sIDInt, _ := idToInt64(serverID)
	uIDInt, _ := idToInt64(userID)
	_, err = h.service.Repo().GetMember(r.Context(), sIDInt, uIDInt)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		render.JSON(w, r, ErrorResponse{Error: "you are not a member of this server"})
		return
	}

	members, err := h.service.GetMembersWithRoles(r.Context(), serverID)
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

	// Verify the server exists
	_, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}

	// Verify the user is a member of the server
	sIDInt, _ := idToInt64(serverID)
	uIDInt, _ := idToInt64(userID)
	_, err = h.service.Repo().GetMember(r.Context(), sIDInt, uIDInt)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		render.JSON(w, r, ErrorResponse{Error: "you are not a member of this server"})
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

// GetServerRoles handles GET /servers/:id/roles
func (h *Handler) GetServerRoles(w http.ResponseWriter, r *http.Request) {
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

	// Verify the server exists
	_, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}

	// Verify the requester is a member of the server
	sIDInt, _ := idToInt64(serverID)
	uIDInt, _ := idToInt64(userID)
	_, err = h.service.Repo().GetMember(r.Context(), sIDInt, uIDInt)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		render.JSON(w, r, ErrorResponse{Error: "you are not a member of this server"})
		return
	}

	roles, err := h.service.GetServerRoles(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}
	if roles == nil {
		roles = []Role{}
	}
	render.JSON(w, r, roles)
}

// CreateServerRole handles POST /servers/:id/roles
func (h *Handler) CreateServerRole(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")

	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}

	isOwner := server.OwnerID == userID
	if !isOwner {
		sID, _ := permissions.ParseInt64(serverID)
		aID, _ := permissions.ParseInt64(userID)
		ownerID, _ := permissions.ParseInt64(server.OwnerID)
		hasPerm, err := permissions.HasPermission(r.Context(), h.service.Repo(), sID, aID, ownerID, permissions.PermManageRoles)
		if err != nil || !hasPerm {
			w.WriteHeader(http.StatusForbidden)
			render.JSON(w, r, ErrorResponse{Error: "manage roles permission required"})
			return
		}
	}

	var req struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Permissions int64  `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "invalid request body"})
		return
	}

	// Non-owners can only grant permissions they themselves have.
	if !isOwner {
		sID, _ := permissions.ParseInt64(serverID)
		aID, _ := permissions.ParseInt64(userID)
		ownerID, _ := permissions.ParseInt64(server.OwnerID)
		actorPerms, _ := permissions.GetEffectivePermissions(r.Context(), h.service.Repo(), sID, aID, ownerID)
		req.Permissions &= actorPerms
	}

	// Hierarchy: new role position must be below actor's highest role (unless owner).
	if !isOwner {
		sID, _ := permissions.ParseInt64(serverID)
		aID, _ := permissions.ParseInt64(userID)
		actorHighest, _ := h.service.Repo().GetHighestRolePosition(r.Context(), sID, aID)
		_ = actorHighest // position is assigned by DB; we can't enforce pre-creation without knowing final position
	}

	role, err := h.service.CreateServerRole(r.Context(), serverID, req.Name, req.Color, req.Permissions)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	render.JSON(w, r, role)
}

// DeleteServerRole handles DELETE /servers/:id/roles/:roleId
func (h *Handler) DeleteServerRole(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	roleID := chi.URLParam(r, "roleId")

	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}

	isOwner := server.OwnerID == userID
	if !isOwner {
		sID, _ := permissions.ParseInt64(serverID)
		aID, _ := permissions.ParseInt64(userID)
		ownerID, _ := permissions.ParseInt64(server.OwnerID)
		hasPerm, err := permissions.HasPermission(r.Context(), h.service.Repo(), sID, aID, ownerID, permissions.PermManageRoles)
		if err != nil || !hasPerm {
			w.WriteHeader(http.StatusForbidden)
			render.JSON(w, r, ErrorResponse{Error: "manage roles permission required"})
			return
		}
	}

	// Prevent deleting @everyone role.
	sID, _ := permissions.ParseInt64(serverID)
	rID, _ := permissions.ParseInt64(roleID)
	roles, err := h.service.Repo().GetServerRoles(r.Context(), sID)
	if err == nil {
		for _, role := range roles {
			if role.ID == rID && role.IsEveryone {
				w.WriteHeader(http.StatusForbidden)
				render.JSON(w, r, ErrorResponse{Error: "cannot delete the @everyone role"})
				return
			}
		}
	}

	// Hierarchy enforcement: non-owners cannot delete a role at or above their own highest position.
	if !isOwner {
		aID, _ := permissions.ParseInt64(userID)
		actorHighest, _ := h.service.Repo().GetHighestRolePosition(r.Context(), sID, aID)
		for _, role := range roles {
			if role.ID == rID && role.Position >= actorHighest {
				w.WriteHeader(http.StatusForbidden)
				render.JSON(w, r, ErrorResponse{Error: "cannot delete a role at or above your highest role"})
				return
			}
		}
	}

	if err := h.service.DeleteServerRole(r.Context(), serverID, roleID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateServerRole handles PATCH /servers/:id/roles/:roleId
func (h *Handler) UpdateServerRole(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	roleID := chi.URLParam(r, "roleId")

	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}

	isOwner := server.OwnerID == userID
	sID, _ := permissions.ParseInt64(serverID)
	rID, _ := permissions.ParseInt64(roleID)
	aID, _ := permissions.ParseInt64(userID)
	ownerID, _ := permissions.ParseInt64(server.OwnerID)

	// Determine if this is the @everyone role.
	roles, _ := h.service.Repo().GetServerRoles(r.Context(), sID)
	isEveryoneRole := false
	for _, role := range roles {
		if role.ID == rID && role.IsEveryone {
			isEveryoneRole = true
			break
		}
	}

	if !isOwner {
		// @everyone role requires ManageServer; other roles require ManageRoles.
		requiredPerm := permissions.PermManageRoles
		if isEveryoneRole {
			requiredPerm = permissions.PermManageServer
		}
		hasPerm, err := permissions.HasPermission(r.Context(), h.service.Repo(), sID, aID, ownerID, requiredPerm)
		if err != nil || !hasPerm {
			w.WriteHeader(http.StatusForbidden)
			if isEveryoneRole {
				render.JSON(w, r, ErrorResponse{Error: "manage server permission required to edit @everyone"})
			} else {
				render.JSON(w, r, ErrorResponse{Error: "manage roles permission required"})
			}
			return
		}

		// Hierarchy enforcement: non-owners cannot edit a role at or above their highest position.
		actorHighest, _ := h.service.Repo().GetHighestRolePosition(r.Context(), sID, aID)
		for _, role := range roles {
			if role.ID == rID && role.Position >= actorHighest {
				w.WriteHeader(http.StatusForbidden)
				render.JSON(w, r, ErrorResponse{Error: "cannot edit a role at or above your highest role"})
				return
			}
		}
	}

	var req struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Permissions int64  `json:"permissions"`
		Hoist       bool   `json:"hoist"`
		Position    int    `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "name is required"})
		return
	}

	// @everyone cannot be renamed; always keep the canonical name.
	if isEveryoneRole {
		req.Name = "@everyone"
	}

	// Non-owners can only grant permissions they themselves have.
	if !isOwner {
		actorPerms, _ := permissions.GetEffectivePermissions(r.Context(), h.service.Repo(), sID, aID, ownerID)
		req.Permissions &= actorPerms
	}

	role, err := h.service.UpdateServerRole(r.Context(), serverID, roleID, req.Name, req.Color, req.Permissions, req.Hoist, req.Position)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}

	render.JSON(w, r, role)
}

// GetMemberRoles handles GET /servers/:id/members/:userId/roles
func (h *Handler) GetMemberRoles(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userID")

	roles, err := h.service.GetMemberRoles(r.Context(), serverID, targetUserID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}
	if roles == nil {
		roles = []Role{}
	}
	render.JSON(w, r, roles)
}

// AssignRoleToMember handles POST /servers/:id/members/:userId/roles
func (h *Handler) AssignRoleToMember(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userID")

	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}

	isOwner := server.OwnerID == userID
	if !isOwner {
		sID, _ := permissions.ParseInt64(serverID)
		aID, _ := permissions.ParseInt64(userID)
		ownerID, _ := permissions.ParseInt64(server.OwnerID)
		hasPerm, err := permissions.HasPermission(r.Context(), h.service.Repo(), sID, aID, ownerID, permissions.PermManageRoles)
		if err != nil || !hasPerm {
			w.WriteHeader(http.StatusForbidden)
			render.JSON(w, r, ErrorResponse{Error: "manage roles permission required"})
			return
		}
	}

	var req struct {
		RoleID string `json:"role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RoleID == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "role_id is required"})
		return
	}

	// Hierarchy: non-owners can only assign roles below their own highest position.
	if !isOwner {
		sID, _ := permissions.ParseInt64(serverID)
		aID, _ := permissions.ParseInt64(userID)
		rID, _ := permissions.ParseInt64(req.RoleID)
		actorHighest, _ := h.service.Repo().GetHighestRolePosition(r.Context(), sID, aID)
		roles, _ := h.service.Repo().GetServerRoles(r.Context(), sID)
		for _, role := range roles {
			if role.ID == rID && role.Position >= actorHighest {
				w.WriteHeader(http.StatusForbidden)
				render.JSON(w, r, ErrorResponse{Error: "cannot assign a role at or above your highest role"})
				return
			}
		}
	}

	if err := h.service.AssignRoleToMember(r.Context(), serverID, targetUserID, req.RoleID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RemoveRoleFromMember handles DELETE /servers/:id/members/:userId/roles/:roleId
func (h *Handler) RemoveRoleFromMember(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userID")
	roleID := chi.URLParam(r, "roleId")

	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}

	isOwner := server.OwnerID == userID
	if !isOwner {
		sID, _ := permissions.ParseInt64(serverID)
		aID, _ := permissions.ParseInt64(userID)
		ownerID, _ := permissions.ParseInt64(server.OwnerID)
		hasPerm, err := permissions.HasPermission(r.Context(), h.service.Repo(), sID, aID, ownerID, permissions.PermManageRoles)
		if err != nil || !hasPerm {
			w.WriteHeader(http.StatusForbidden)
			render.JSON(w, r, ErrorResponse{Error: "manage roles permission required"})
			return
		}
	}

	// Hierarchy: non-owners can only remove roles below their own highest position.
	if !isOwner {
		sID, _ := permissions.ParseInt64(serverID)
		aID, _ := permissions.ParseInt64(userID)
		rID, _ := permissions.ParseInt64(roleID)
		actorHighest, _ := h.service.Repo().GetHighestRolePosition(r.Context(), sID, aID)
		roles, _ := h.service.Repo().GetServerRoles(r.Context(), sID)
		for _, role := range roles {
			if role.ID == rID && role.Position >= actorHighest {
				w.WriteHeader(http.StatusForbidden)
				render.JSON(w, r, ErrorResponse{Error: "cannot remove a role at or above your highest role"})
				return
			}
		}
	}

	if err := h.service.RemoveRoleFromMember(r.Context(), serverID, targetUserID, roleID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetMembersWithRoles handles GET /servers/:id/members-with-roles
func (h *Handler) GetMembersWithRoles(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")

	_, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}

	members, err := h.service.GetMembersWithRoles(r.Context(), serverID)
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

// LeaveServer handles DELETE /servers/:id/leave — allows the current user to leave a server
func (h *Handler) LeaveServer(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, ErrorResponse{Error: "server ID is required"})
		return
	}

	currentUserID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || currentUserID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}
	if server.OwnerID == currentUserID {
		w.WriteHeader(http.StatusForbidden)
		render.JSON(w, r, ErrorResponse{Error: "server owner cannot leave; delete the server instead"})
		return
	}

	if err := h.service.RemoveMember(r.Context(), serverID, currentUserID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// KickMember handles POST /servers/:id/members/:userID/kick
func (h *Handler) KickMember(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userID")

	currentUserID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || currentUserID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}
	if targetUserID == server.OwnerID {
		w.WriteHeader(http.StatusForbidden)
		render.JSON(w, r, ErrorResponse{Error: "cannot kick the server owner"})
		return
	}
	_, allowed, err := h.service.CanKick(r.Context(), serverID, currentUserID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}
	if !allowed {
		w.WriteHeader(http.StatusForbidden)
		render.JSON(w, r, ErrorResponse{Error: "you do not have permission to kick members"})
		return
	}

	// Role hierarchy check: actor must outrank target.
	_, hierarchyOK, err := h.service.RoleHierarchyCheck(r.Context(), serverID, currentUserID, targetUserID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}
	if !hierarchyOK {
		w.WriteHeader(http.StatusForbidden)
		render.JSON(w, r, ErrorResponse{Error: "your role is not high enough to kick this member"})
		return
	}

	if err := h.service.KickMember(r.Context(), serverID, targetUserID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BanMember handles POST /servers/:id/members/:userID/ban
func (h *Handler) BanMember(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userID")

	currentUserID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || currentUserID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		render.JSON(w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		render.JSON(w, r, ErrorResponse{Error: "server not found"})
		return
	}
	if targetUserID == server.OwnerID {
		w.WriteHeader(http.StatusForbidden)
		render.JSON(w, r, ErrorResponse{Error: "cannot ban the server owner"})
		return
	}
	_, allowed, err := h.service.CanBan(r.Context(), serverID, currentUserID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}
	if !allowed {
		w.WriteHeader(http.StatusForbidden)
		render.JSON(w, r, ErrorResponse{Error: "you do not have permission to ban members"})
		return
	}

	// Role hierarchy check: actor must outrank target.
	_, hierarchyOK, err := h.service.RoleHierarchyCheck(r.Context(), serverID, currentUserID, targetUserID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}
	if !hierarchyOK {
		w.WriteHeader(http.StatusForbidden)
		render.JSON(w, r, ErrorResponse{Error: "your role is not high enough to ban this member"})
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if err := h.service.BanMember(r.Context(), serverID, targetUserID, currentUserID, req.Reason); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetMyPermissions handles GET /servers/:id/my-permissions
func (h *Handler) GetMyPermissions(w http.ResponseWriter, r *http.Request) {
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
	perms, isOwner, err := h.service.GetMyPermissions(r.Context(), serverID, userID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, ErrorResponse{Error: err.Error()})
		return
	}
	render.JSON(w, r, map[string]interface{}{
		"permissions": perms,
		"is_owner":    isOwner,
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