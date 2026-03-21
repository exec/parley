package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/httputil"
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

// HTTP Handlers

// CreateServer handles POST /servers
func (h *Handler) CreateServer(w http.ResponseWriter, r *http.Request) {
	var req CreateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		httputil.JSONError(w, "server name is required", http.StatusBadRequest)
		return
	}

	// Get owner ID from auth context
	ownerID := auth.GetUserIDFromContext(r)
	if ownerID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	server, err := h.service.CreateServer(r.Context(), req.Name, req.IconURL, ownerID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	render.JSON(w, r, server)
}

// GetServer handles GET /servers/:id
func (h *Handler) GetServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}

	server, err := h.service.GetServer(r.Context(), id)
	if err != nil {
		if err.Error() == "server not found" {
			httputil.JSONError(w, "server not found", http.StatusNotFound)
			return
		}
		httputil.InternalError(w, err)
		return
	}

	render.JSON(w, r, server)
}

// GetUserServers handles GET /servers
func (h *Handler) GetUserServers(w http.ResponseWriter, r *http.Request) {
	// Get user ID from auth context
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	servers, err := h.service.GetUserServers(r.Context(), userID)
	if err != nil {
		httputil.InternalError(w, err)
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
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}

	var req UpdateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		httputil.JSONError(w, "server name is required", http.StatusBadRequest)
		return
	}

	// Verify the user is the owner
	server, err := h.service.GetServer(r.Context(), id)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" || server.OwnerID != userID {
		httputil.JSONError(w, "only the server owner can update the server", http.StatusForbidden)
		return
	}

	updatedServer, err := h.service.UpdateServer(r.Context(), id, req.Name, req.IconURL)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}

	render.JSON(w, r, updatedServer)
}

// DeleteServer handles DELETE /servers/:id
func (h *Handler) DeleteServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}

	// Verify the user is the owner
	server, err := h.service.GetServer(r.Context(), id)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" || server.OwnerID != userID {
		httputil.JSONError(w, "only the server owner can delete the server", http.StatusForbidden)
		return
	}

	err = h.service.DeleteServer(r.Context(), id)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AddMember handles POST /servers/:id/members
func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}

	var req AddMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.UserID == "" {
		httputil.JSONError(w, "user ID is required", http.StatusBadRequest)
		return
	}

	// Verify the user is the owner or a member
	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Only owner or Administrator can add bots
	if server.OwnerID != userID {
		ownerID, _ := permissions.ParseInt64(server.OwnerID)
		actorID, _ := permissions.ParseInt64(userID)
		sID, _ := permissions.ParseInt64(serverID)
		isAdmin, _ := permissions.HasPermission(r.Context(), h.service.Repo(), sID, actorID, ownerID, permissions.PermAdministrator)
		if !isAdmin {
			httputil.JSONError(w, "Administrator permission required to add bots", http.StatusForbidden)
			return
		}
	}

	err = h.service.AddMember(r.Context(), serverID, req.UserID, req.Nickname)
	if err != nil {
		httputil.InternalError(w, err)
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
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}

	if userID == "" {
		httputil.JSONError(w, "user ID is required", http.StatusBadRequest)
		return
	}

	// Verify the user is the owner
	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	currentUserID := auth.GetUserIDFromContext(r)
	if currentUserID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Only owner can remove members (or the member themselves)
	if server.OwnerID != currentUserID && userID != currentUserID {
		httputil.JSONError(w, "only the server owner or the member themselves can remove members", http.StatusForbidden)
		return
	}

	// Cannot remove the owner
	if userID == server.OwnerID {
		httputil.JSONError(w, "cannot remove the server owner", http.StatusForbidden)
		return
	}

	err = h.service.RemoveMember(r.Context(), serverID, userID)
	if err != nil {
		if err.Error() == "member not found" {
			httputil.JSONError(w, "member not found", http.StatusNotFound)
			return
		}
		httputil.InternalError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetMembers handles GET /servers/:id/members
func (h *Handler) GetMembers(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify the server exists
	_, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	// Verify the requester is a member of the server
	sIDInt, _ := idToInt64(serverID)
	uIDInt, _ := idToInt64(userID)
	_, err = h.service.Repo().GetMember(r.Context(), sIDInt, uIDInt)
	if err != nil {
		httputil.JSONError(w, "you are not a member of this server", http.StatusForbidden)
		return
	}

	members, err := h.service.GetMembersWithRoles(r.Context(), serverID)
	if err != nil {
		httputil.InternalError(w, err)
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
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}

	// Verify the user is authenticated
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify the server exists
	_, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	// Verify the user is a member of the server
	sIDInt, _ := idToInt64(serverID)
	uIDInt, _ := idToInt64(userID)
	_, err = h.service.Repo().GetMember(r.Context(), sIDInt, uIDInt)
	if err != nil {
		httputil.JSONError(w, "you are not a member of this server", http.StatusForbidden)
		return
	}

	var body struct {
		MaxUses   *int   `json:"max_uses"`
		ExpiresIn string `json:"expires_in"` // e.g. "30m", "1h", "6h", "12h", "1d", "7d", "30d"
	}
	// Ignore decode error (body is optional)
	json.NewDecoder(r.Body).Decode(&body)

	var expiresAt *time.Time
	if body.ExpiresIn != "" {
		dur, err := parseDuration(body.ExpiresIn)
		if err == nil {
			t := time.Now().Add(dur)
			expiresAt = &t
		}
	}

	invite, err := h.service.CreateInvite(r.Context(), serverID, userID, body.MaxUses, expiresAt)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	render.JSON(w, r, invite)
}

// PreviewInvite handles GET /api/invites/:code — returns server info without joining.
// Works for both regular invite codes and server vanity URLs.
func (h *Handler) PreviewInvite(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		httputil.JSONError(w, "invite code is required", http.StatusBadRequest)
		return
	}

	server, err := h.service.GetServerByInviteCode(r.Context(), code)
	if err != nil {
		// Try as vanity URL
		server, err = h.service.GetServerByVanityURL(r.Context(), code)
		if err != nil {
			httputil.JSONError(w, "invite not found or invalid", http.StatusNotFound)
			return
		}
	}

	render.JSON(w, r, map[string]interface{}{"server": server})
}

// JoinInvite handles POST /api/invites/:code — joins the server for the authenticated user.
// Works for both regular invite codes and server vanity URLs.
func (h *Handler) JoinInvite(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		httputil.JSONError(w, "invite code is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// First try as a regular invite code, then fall back to vanity URL
	server, err := h.service.JoinServerByInvite(r.Context(), code, userID)
	if err != nil {
		// Try as vanity URL
		server, err = h.service.JoinServerByVanityURL(r.Context(), code, userID)
		if err != nil {
			httputil.JSONError(w, "invite not found or invalid", http.StatusNotFound)
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
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify the server exists
	_, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	// Verify the requester is a member of the server
	sIDInt, _ := idToInt64(serverID)
	uIDInt, _ := idToInt64(userID)
	_, err = h.service.Repo().GetMember(r.Context(), sIDInt, uIDInt)
	if err != nil {
		httputil.JSONError(w, "you are not a member of this server", http.StatusForbidden)
		return
	}

	roles, err := h.service.GetServerRoles(r.Context(), serverID)
	if err != nil {
		httputil.InternalError(w, err)
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

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	isOwner := server.OwnerID == userID
	if !isOwner {
		sID, _ := permissions.ParseInt64(serverID)
		aID, _ := permissions.ParseInt64(userID)
		ownerID, _ := permissions.ParseInt64(server.OwnerID)
		hasPerm, err := permissions.HasPermission(r.Context(), h.service.Repo(), sID, aID, ownerID, permissions.PermManageRoles)
		if err != nil || !hasPerm {
			httputil.JSONError(w, "manage roles permission required", http.StatusForbidden)
			return
		}
	}

	var req struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Permissions int64  `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
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
		httputil.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
	render.JSON(w, r, role)
}

// DeleteServerRole handles DELETE /servers/:id/roles/:roleId
func (h *Handler) DeleteServerRole(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	roleID := chi.URLParam(r, "roleId")

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	isOwner := server.OwnerID == userID
	if !isOwner {
		sID, _ := permissions.ParseInt64(serverID)
		aID, _ := permissions.ParseInt64(userID)
		ownerID, _ := permissions.ParseInt64(server.OwnerID)
		hasPerm, err := permissions.HasPermission(r.Context(), h.service.Repo(), sID, aID, ownerID, permissions.PermManageRoles)
		if err != nil || !hasPerm {
			httputil.JSONError(w, "manage roles permission required", http.StatusForbidden)
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
				httputil.JSONError(w, "cannot delete the @everyone role", http.StatusForbidden)
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
				httputil.JSONError(w, "cannot delete a role at or above your highest role", http.StatusForbidden)
				return
			}
		}
	}

	if err := h.service.DeleteServerRole(r.Context(), serverID, roleID); err != nil {
		httputil.InternalError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateServerRole handles PATCH /servers/:id/roles/:roleId
func (h *Handler) UpdateServerRole(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	roleID := chi.URLParam(r, "roleId")

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
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
			if isEveryoneRole {
				httputil.JSONError(w, "manage server permission required to edit @everyone", http.StatusForbidden)
			} else {
				httputil.JSONError(w, "manage roles permission required", http.StatusForbidden)
			}
			return
		}

		// Hierarchy enforcement: non-owners cannot edit a role at or above their highest position.
		actorHighest, _ := h.service.Repo().GetHighestRolePosition(r.Context(), sID, aID)
		for _, role := range roles {
			if role.ID == rID && role.Position >= actorHighest {
				httputil.JSONError(w, "cannot edit a role at or above your highest role", http.StatusForbidden)
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
		httputil.JSONError(w, "name is required", http.StatusBadRequest)
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
		httputil.InternalError(w, err)
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
		httputil.InternalError(w, err)
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

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	isOwner := server.OwnerID == userID
	if !isOwner {
		sID, _ := permissions.ParseInt64(serverID)
		aID, _ := permissions.ParseInt64(userID)
		ownerID, _ := permissions.ParseInt64(server.OwnerID)
		hasPerm, err := permissions.HasPermission(r.Context(), h.service.Repo(), sID, aID, ownerID, permissions.PermManageRoles)
		if err != nil || !hasPerm {
			httputil.JSONError(w, "manage roles permission required", http.StatusForbidden)
			return
		}
	}

	var req struct {
		RoleID string `json:"role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RoleID == "" {
		httputil.JSONError(w, "role_id is required", http.StatusBadRequest)
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
				httputil.JSONError(w, "cannot assign a role at or above your highest role", http.StatusForbidden)
				return
			}
		}
	}

	if err := h.service.AssignRoleToMember(r.Context(), serverID, targetUserID, req.RoleID); err != nil {
		httputil.InternalError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RemoveRoleFromMember handles DELETE /servers/:id/members/:userId/roles/:roleId
func (h *Handler) RemoveRoleFromMember(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userID")
	roleID := chi.URLParam(r, "roleId")

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	isOwner := server.OwnerID == userID
	if !isOwner {
		sID, _ := permissions.ParseInt64(serverID)
		aID, _ := permissions.ParseInt64(userID)
		ownerID, _ := permissions.ParseInt64(server.OwnerID)
		hasPerm, err := permissions.HasPermission(r.Context(), h.service.Repo(), sID, aID, ownerID, permissions.PermManageRoles)
		if err != nil || !hasPerm {
			httputil.JSONError(w, "manage roles permission required", http.StatusForbidden)
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
				httputil.JSONError(w, "cannot remove a role at or above your highest role", http.StatusForbidden)
				return
			}
		}
	}

	if err := h.service.RemoveRoleFromMember(r.Context(), serverID, targetUserID, roleID); err != nil {
		httputil.InternalError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetMembersWithRoles handles GET /servers/:id/members-with-roles
func (h *Handler) GetMembersWithRoles(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")

	_, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	members, err := h.service.GetMembersWithRoles(r.Context(), serverID)
	if err != nil {
		httputil.InternalError(w, err)
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
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}

	currentUserID := auth.GetUserIDFromContext(r)
	if currentUserID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}
	if server.OwnerID == currentUserID {
		httputil.JSONError(w, "server owner cannot leave; delete the server instead", http.StatusForbidden)
		return
	}

	if err := h.service.RemoveMember(r.Context(), serverID, currentUserID); err != nil {
		httputil.InternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// KickMember handles POST /servers/:id/members/:userID/kick
func (h *Handler) KickMember(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userID")

	currentUserID := auth.GetUserIDFromContext(r)
	if currentUserID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}
	if targetUserID == server.OwnerID {
		httputil.JSONError(w, "cannot kick the server owner", http.StatusForbidden)
		return
	}
	_, allowed, err := h.service.CanKick(r.Context(), serverID, currentUserID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	if !allowed {
		httputil.JSONError(w, "you do not have permission to kick members", http.StatusForbidden)
		return
	}

	// Role hierarchy check: actor must outrank target.
	_, hierarchyOK, err := h.service.RoleHierarchyCheck(r.Context(), serverID, currentUserID, targetUserID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	if !hierarchyOK {
		httputil.JSONError(w, "your role is not high enough to kick this member", http.StatusForbidden)
		return
	}

	if err := h.service.KickMember(r.Context(), serverID, targetUserID); err != nil {
		httputil.InternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BanMember handles POST /servers/:id/members/:userID/ban
func (h *Handler) BanMember(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userID")

	currentUserID := auth.GetUserIDFromContext(r)
	if currentUserID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}
	if targetUserID == server.OwnerID {
		httputil.JSONError(w, "cannot ban the server owner", http.StatusForbidden)
		return
	}
	_, allowed, err := h.service.CanBan(r.Context(), serverID, currentUserID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	if !allowed {
		httputil.JSONError(w, "you do not have permission to ban members", http.StatusForbidden)
		return
	}

	// Role hierarchy check: actor must outrank target.
	_, hierarchyOK, err := h.service.RoleHierarchyCheck(r.Context(), serverID, currentUserID, targetUserID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	if !hierarchyOK {
		httputil.JSONError(w, "your role is not high enough to ban this member", http.StatusForbidden)
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if err := h.service.BanMember(r.Context(), serverID, targetUserID, currentUserID, req.Reason); err != nil {
		httputil.InternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListBans handles GET /servers/:id/bans
func (h *Handler) ListBans(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	currentUserID := auth.GetUserIDFromContext(r)
	if currentUserID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	_, allowed, err := h.service.CanBan(r.Context(), serverID, currentUserID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	if !allowed {
		httputil.JSONError(w, "you do not have permission to view bans", http.StatusForbidden)
		return
	}
	bans, err := h.service.ListBans(r.Context(), serverID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	render.JSON(w, r, bans)
}

// UnbanMember handles DELETE /servers/:id/bans/:userID
func (h *Handler) UnbanMember(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userID")
	currentUserID := auth.GetUserIDFromContext(r)
	if currentUserID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	_, allowed, err := h.service.CanBan(r.Context(), serverID, currentUserID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	if !allowed {
		httputil.JSONError(w, "you do not have permission to unban members", http.StatusForbidden)
		return
	}
	if err := h.service.UnbanMember(r.Context(), serverID, targetUserID); err != nil {
		httputil.InternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetMyPermissions handles GET /servers/:id/my-permissions
func (h *Handler) GetMyPermissions(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	perms, isOwner, err := h.service.GetMyPermissions(r.Context(), serverID, userID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	render.JSON(w, r, map[string]interface{}{
		"permissions": perms,
		"is_owner":    isOwner,
	})
}

// ListServerInvites handles GET /servers/:id/invites
func (h *Handler) ListServerInvites(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// Check membership
	sIDInt, _ := idToInt64(serverID)
	uIDInt, _ := idToInt64(userID)
	_, err := h.service.Repo().GetMember(r.Context(), sIDInt, uIDInt)
	if err != nil {
		httputil.JSONError(w, "not a member", http.StatusForbidden)
		return
	}
	invites, err := h.service.GetServerInvites(r.Context(), serverID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	if invites == nil {
		invites = []*Invite{}
	}
	render.JSON(w, r, invites)
}

// RevokeInvite handles DELETE /servers/:id/invites/:code
func (h *Handler) RevokeInvite(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	code := chi.URLParam(r, "code")
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	err := h.service.RevokeInvite(r.Context(), serverID, code, userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) || err.Error() == "invite not found" {
			httputil.JSONError(w, "invite not found", http.StatusNotFound)
			return
		}
		if err.Error() == "not a member of this server" {
			httputil.JSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		httputil.InternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetInviteMembers handles GET /servers/:id/invites/:code/members
func (h *Handler) GetInviteMembers(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	code := chi.URLParam(r, "code")
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	members, err := h.service.GetInviteMembers(r.Context(), serverID, code, userID)
	if err != nil {
		if err.Error() == "not a member of this server" {
			httputil.JSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		httputil.InternalError(w, err)
		return
	}
	if members == nil {
		members = []*db.InviteMember{}
	}
	render.JSON(w, r, members)
}

// parseDuration parses a friendly duration string used for invite expiry.
func parseDuration(s string) (time.Duration, error) {
	switch s {
	case "30m":
		return 30 * time.Minute, nil
	case "1h":
		return time.Hour, nil
	case "6h":
		return 6 * time.Hour, nil
	case "12h":
		return 12 * time.Hour, nil
	case "1d":
		return 24 * time.Hour, nil
	case "7d":
		return 7 * 24 * time.Hour, nil
	case "30d":
		return 30 * 24 * time.Hour, nil
	}
	return 0, errors.New("unknown duration: " + s)
}

// SetVanityURL handles PUT /servers/:id/vanity
func (h *Handler) SetVanityURL(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		VanityURL string `json:"vanity_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
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
		if status == http.StatusInternalServerError {
			httputil.InternalError(w, err)
		} else {
			httputil.JSONError(w, err.Error(), status)
		}
		return
	}

	render.JSON(w, r, server)
}
