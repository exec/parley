package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"parley/internal/audit"
	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/httputil"
	"parley/internal/permissions"
)

// Handler handles HTTP requests for server operations
type Handler struct {
	service  *ServerService
	auditSvc *audit.AuditService
}

// NewHandler creates a new server handler
func NewHandler(service *ServerService, auditSvc *audit.AuditService) *Handler {
	return &Handler{service: service, auditSvc: auditSvc}
}

// Request/Response types

type CreateServerRequest struct {
	Name    string `json:"name"`
	IconURL string `json:"icon_url"`
}

type UpdateServerRequest struct {
	Name        string `json:"name"`
	IconURL     string `json:"icon_url"`
	Description string `json:"description"`
	IsPublic    bool   `json:"is_public"`
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
		if errors.Is(err, ErrServerNotFound) {
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
	beforeServer, err := h.service.GetServer(r.Context(), id)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" || beforeServer.OwnerID != userID {
		httputil.JSONError(w, "only the server owner can update the server", http.StatusForbidden)
		return
	}

	updatedServer, err := h.service.UpdateServer(r.Context(), id, req.Name, req.IconURL, req.Description, req.IsPublic)
	if err != nil {
		msg := err.Error()
		switch msg {
		case "server name is required", "description must be 200 characters or fewer",
			"a vanity URL is required to list your server publicly", "server not found":
			httputil.JSONError(w, msg, http.StatusBadRequest)
		default:
			httputil.InternalError(w, err)
		}
		return
	}

	if beforeServer != nil {
		updateActorIDInt, _ := strconv.ParseInt(userID, 10, 64)
		updateActorUsername, _ := h.service.Repo().GetUsernameByID(r.Context(), updateActorIDInt)
		serverIDInt, _ := strconv.ParseInt(id, 10, 64)
		h.auditSvc.Log(r.Context(), audit.Entry{
			ServerID:      serverIDInt,
			ActorID:       &updateActorIDInt,
			ActorUsername: updateActorUsername,
			Action:        "server.update",
			TargetType:    "server",
			Changes: map[string]any{
				"before": map[string]any{
					"name":        beforeServer.Name,
					"icon_url":    beforeServer.IconURL,
					"description": beforeServer.Description,
					"is_public":   beforeServer.IsPublic,
				},
				"after": map[string]any{
					"name":        updatedServer.Name,
					"icon_url":    updatedServer.IconURL,
					"description": updatedServer.Description,
					"is_public":   updatedServer.IsPublic,
				},
			},
		})
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
		if errors.Is(err, ErrMemberNotFound) {
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

	actorIDInt, _ := strconv.ParseInt(userID, 10, 64)
	actorUsername, _ := h.service.Repo().GetUsernameByID(r.Context(), actorIDInt)
	invite, err := h.service.CreateInvite(r.Context(), serverID, userID, body.MaxUses, expiresAt, actorUsername)
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
		if errors.Is(err, ErrBanned) {
			httputil.JSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		// Try as vanity URL
		server, err = h.service.JoinServerByVanityURL(r.Context(), code, userID)
		if err != nil {
			if errors.Is(err, ErrBanned) {
				httputil.JSONError(w, err.Error(), http.StatusForbidden)
				return
			}
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

	// Hierarchy: non-owners cannot grant Administrator (bit 0).
	if !isOwner {
		sID, _ := permissions.ParseInt64(serverID)
		aID, _ := permissions.ParseInt64(userID)
		actorHighest, _ := h.service.Repo().GetHighestRolePosition(r.Context(), sID, aID)
		_ = actorHighest
		if req.Permissions&permissions.PermAdministrator != 0 {
			httputil.JSONError(w, "only the server owner can grant Administrator", http.StatusForbidden)
			return
		}
	}

	actorIDInt, _ := strconv.ParseInt(userID, 10, 64)
	actorUsername, _ := h.service.Repo().GetUsernameByID(r.Context(), actorIDInt)
	role, err := h.service.CreateServerRole(r.Context(), serverID, req.Name, req.Color, req.Permissions, actorIDInt, actorUsername)
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

	actorIDInt, _ := strconv.ParseInt(userID, 10, 64)
	actorUsername, _ := h.service.Repo().GetUsernameByID(r.Context(), actorIDInt)
	if err := h.service.DeleteServerRole(r.Context(), serverID, roleID, actorIDInt, actorUsername); err != nil {
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

	actorIDInt, _ := strconv.ParseInt(userID, 10, 64)
	actorUsername, _ := h.service.Repo().GetUsernameByID(r.Context(), actorIDInt)
	role, err := h.service.UpdateServerRole(r.Context(), serverID, roleID, req.Name, req.Color, req.Permissions, req.Hoist, req.Position, actorIDInt, actorUsername)
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

	actorIDInt, _ := strconv.ParseInt(userID, 10, 64)
	actorUsername, _ := h.service.Repo().GetUsernameByID(r.Context(), actorIDInt)
	if err := h.service.AssignRoleToMember(r.Context(), serverID, targetUserID, req.RoleID, actorIDInt, actorUsername); err != nil {
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

	actorIDInt, _ := strconv.ParseInt(userID, 10, 64)
	actorUsername, _ := h.service.Repo().GetUsernameByID(r.Context(), actorIDInt)
	if err := h.service.RemoveRoleFromMember(r.Context(), serverID, targetUserID, roleID, actorIDInt, actorUsername); err != nil {
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
	targetIDInt, err := strconv.ParseInt(targetUserID, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user ID", http.StatusBadRequest)
		return
	}
	badges, err := h.service.Repo().GetUserBadges(r.Context(), targetIDInt)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	if badges&db.BadgeAdmin != 0 {
		httputil.JSONError(w, "cannot kick a Parley Admin", http.StatusForbidden)
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

	actorIDInt, _ := strconv.ParseInt(currentUserID, 10, 64)
	actorUsername, _ := h.service.Repo().GetUsernameByID(r.Context(), actorIDInt)
	if err := h.service.KickMember(r.Context(), serverID, targetUserID, actorIDInt, actorUsername); err != nil {
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
	targetIDInt, err := strconv.ParseInt(targetUserID, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user ID", http.StatusBadRequest)
		return
	}
	badges, err := h.service.Repo().GetUserBadges(r.Context(), targetIDInt)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	if badges&db.BadgeAdmin != 0 {
		httputil.JSONError(w, "cannot ban a Parley Admin", http.StatusForbidden)
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

	actorIDInt, _ := strconv.ParseInt(currentUserID, 10, 64)
	actorUsername, _ := h.service.Repo().GetUsernameByID(r.Context(), actorIDInt)
	if err := h.service.BanMember(r.Context(), serverID, targetUserID, actorIDInt, actorUsername, req.Reason); err != nil {
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
	actorIDInt, _ := strconv.ParseInt(currentUserID, 10, 64)
	actorUsername, _ := h.service.Repo().GetUsernameByID(r.Context(), actorIDInt)
	if err := h.service.UnbanMember(r.Context(), serverID, targetUserID, actorIDInt, actorUsername); err != nil {
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
	revokeActorIDInt, _ := strconv.ParseInt(userID, 10, 64)
	revokeActorUsername, _ := h.service.Repo().GetUsernameByID(r.Context(), revokeActorIDInt)
	err := h.service.RevokeInvite(r.Context(), serverID, code, userID, revokeActorUsername)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) || errors.Is(err, ErrInviteNotFound) {
			httputil.JSONError(w, "invite not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrNotMember) {
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
		if errors.Is(err, ErrNotMember) {
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

// ListServerCategories handles GET /server-categories (public, no auth)
func (h *Handler) ListServerCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.service.ListServerCategories(r.Context())
	if err != nil {
		httputil.JSONError(w, "failed to load categories", http.StatusInternalServerError)
		return
	}
	render.JSON(w, r, cats)
}

// Discover handles GET /discover (public, no auth)
func (h *Handler) Discover(w http.ResponseWriter, r *http.Request) {
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	q := r.URL.Query().Get("q")

	var categoryID *int64
	if catStr := r.URL.Query().Get("category_id"); catStr != "" {
		if v, err := strconv.ParseInt(catStr, 10, 64); err == nil {
			categoryID = &v
		}
	}

	servers, total, err := h.service.Discover(r.Context(), categoryID, q, page)
	if err != nil {
		httputil.JSONError(w, "failed to load servers", http.StatusInternalServerError)
		return
	}
	render.JSON(w, r, map[string]interface{}{
		"servers": servers,
		"total":   total,
	})
}

// SetServerCategories handles PUT /servers/{id}/categories (owner only)
func (h *Handler) SetServerCategories(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}
	userID := auth.GetUserIDFromContext(r)
	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}
	if server.OwnerID != userID {
		httputil.JSONError(w, "only the server owner can update categories", http.StatusForbidden)
		return
	}

	var req struct {
		CategoryIDs []int64 `json:"category_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.CategoryIDs == nil {
		req.CategoryIDs = []int64{}
	}

	cats, err := h.service.SetServerCategories(r.Context(), serverID, req.CategoryIDs)
	if err != nil {
		msg := err.Error()
		if msg == "maximum 3 categories allowed" || msg == "invalid category" {
			httputil.JSONError(w, msg, http.StatusBadRequest)
		} else {
			httputil.InternalError(w, err)
		}
		return
	}
	render.JSON(w, r, cats)
}

// GetServerCategoriesForServer handles GET /servers/{id}/categories (auth required)
func (h *Handler) GetServerCategoriesForServer(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}
	cats, err := h.service.GetServerCategoryAssignments(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "failed to load categories", http.StatusInternalServerError)
		return
	}
	render.JSON(w, r, cats)
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

	vanityActorIDInt, _ := strconv.ParseInt(userID, 10, 64)
	vanityActorUsername, _ := h.service.Repo().GetUsernameByID(r.Context(), vanityActorIDInt)
	server, err := h.service.SetVanityURL(r.Context(), serverID, req.VanityURL, vanityActorIDInt, vanityActorUsername)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrOwnerOnly) {
			status = http.StatusForbidden
		} else if errors.Is(err, ErrServerNotFound) {
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

// GetAuditLog handles GET /servers/{id}/audit-log
func (h *Handler) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	serverIDInt, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid server ID", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	srv, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	isOwner := srv.OwnerID == userID
	if !isOwner {
		sID, _ := permissions.ParseInt64(serverID)
		aID, _ := permissions.ParseInt64(userID)
		ownerIDInt, _ := permissions.ParseInt64(srv.OwnerID)
		allowed, err := permissions.HasPermission(r.Context(), h.service.Repo(), sID, aID, ownerIDInt, permissions.PermViewAuditLog)
		if err != nil || !allowed {
			httputil.JSONError(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	q := r.URL.Query()
	limit := 50
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 {
		if l > 100 {
			l = 100
		}
		limit = l
	}
	offset := 0
	if o, err := strconv.Atoi(q.Get("offset")); err == nil && o >= 0 {
		offset = o
	}
	action := q.Get("action")
	var actorFilter *int64
	if a := q.Get("actor_id"); a != "" {
		if v, err := strconv.ParseInt(a, 10, 64); err == nil {
			actorFilter = &v
		}
	}

	logs, total, err := h.service.Repo().ListAuditLogs(r.Context(), serverIDInt, actorFilter, action, limit, offset)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}

	type logEntry struct {
		ID            int64      `json:"id,string"`
		ServerID      int64      `json:"server_id,string"`
		ActorID       *int64     `json:"actor_id,omitempty"`
		ActorUsername string     `json:"actor_username"`
		Action        string     `json:"action"`
		TargetID      string     `json:"target_id,omitempty"`
		TargetType    string     `json:"target_type,omitempty"`
		TargetName    string     `json:"target_name,omitempty"`
		Changes       json.RawMessage `json:"changes,omitempty"`
		Reason        string     `json:"reason,omitempty"`
		CreatedAt     time.Time  `json:"created_at"`
	}

	out := make([]logEntry, len(logs))
	for i, l := range logs {
		var changes json.RawMessage
		if len(l.Changes) > 0 {
			changes = json.RawMessage(l.Changes)
		}
		out[i] = logEntry{
			ID:            l.ID,
			ServerID:      l.ServerID,
			ActorID:       l.ActorID,
			ActorUsername: l.ActorUsername,
			Action:        l.Action,
			TargetID:      l.TargetID,
			TargetType:    l.TargetType,
			TargetName:    l.TargetName,
			Changes:       changes,
			Reason:        l.Reason,
			CreatedAt:     l.CreatedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"logs": out, "total": total})
}
