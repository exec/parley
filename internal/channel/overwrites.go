package channel

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"parley/internal/audit"
	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/permissions"
	ws "parley/internal/websocket"
)

// UpsertOverwriteRequest represents the request body for upserting a permission overwrite.
type UpsertOverwriteRequest struct {
	TargetType int    `json:"target_type"`
	TargetID   string `json:"target_id"`
	Allow      int64  `json:"allow"`
	Deny       int64  `json:"deny"`
}

// GetOverwrites handles GET /channels/{id}/overwrites
func (h *Handler) GetOverwrites(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "id")
	if channelID == "" {
		http.Error(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelIDInt, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		http.Error(w, "invalid channel ID", http.StatusBadRequest)
		return
	}

	ch, err := h.service.repo.GetChannelByID(r.Context(), channelIDInt)
	if err != nil {
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}

	userIDInt, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	srv, err := h.service.repo.GetServerByID(r.Context(), ch.ServerID)
	if err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	canView, err := permissions.HasChannelPermission(r.Context(), h.service.repo, ch.ServerID, userIDInt, srv.OwnerID, channelIDInt, permissions.PermViewChannel)
	if err != nil || !canView {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	overwrites, err := h.service.repo.GetRawChannelOverwrites(r.Context(), channelIDInt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if overwrites == nil {
		w.Write([]byte("[]"))
		return
	}
	json.NewEncoder(w).Encode(overwrites)
}

// UpsertOverwrite handles PUT /channels/{id}/overwrites
func (h *Handler) UpsertOverwrite(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "id")
	if channelID == "" {
		http.Error(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelIDInt, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		http.Error(w, "invalid channel ID", http.StatusBadRequest)
		return
	}

	userIDInt, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	ch, err := h.service.repo.GetChannelByID(r.Context(), channelIDInt)
	if err != nil {
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}

	srv, err := h.service.repo.GetServerByID(r.Context(), ch.ServerID)
	if err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	// Check permission: must have ManageRoles or ManageChannels
	hasManageRoles, err := permissions.HasPermission(r.Context(), h.service.repo, ch.ServerID, userIDInt, srv.OwnerID, permissions.PermManageRoles)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hasManageChannels, err := permissions.HasPermission(r.Context(), h.service.repo, ch.ServerID, userIDInt, srv.OwnerID, permissions.PermManageChannels)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !hasManageRoles && !hasManageChannels {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var req UpsertOverwriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	targetIDInt, err := strconv.ParseInt(req.TargetID, 10, 64)
	if err != nil {
		http.Error(w, "invalid target_id", http.StatusBadRequest)
		return
	}

	// Mask out server-only bits
	req.Allow &= ^permissions.PermServerOnlyMask
	req.Deny &= ^permissions.PermServerOnlyMask

	// Clear conflicting bits: if a bit is in both allow and deny, clear it from deny
	req.Deny &= ^req.Allow

	// Check actor can only set bits they themselves have
	actorPerms, err := permissions.GetEffectivePermissions(r.Context(), h.service.repo, ch.ServerID, userIDInt, srv.OwnerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if (req.Allow|req.Deny)&(^actorPerms) != 0 {
		http.Error(w, "forbidden: cannot set permissions you do not have", http.StatusForbidden)
		return
	}

	// Snapshot the prior overwrite (if any) BEFORE the upsert so we can record before-state in the audit log.
	beforeAllow, beforeDeny := int64(0), int64(0)
	if priorOWs, err := h.service.repo.GetRawChannelOverwrites(r.Context(), channelIDInt); err == nil {
		for _, prior := range priorOWs {
			if prior.TargetType == req.TargetType && prior.TargetID == targetIDInt {
				beforeAllow, beforeDeny = prior.Allow, prior.Deny
				break
			}
		}
	}

	ow, err := h.service.repo.UpsertOverwrite(r.Context(), channelIDInt, req.TargetType, targetIDInt, req.Allow, req.Deny)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Sync handling
	if err := h.handleOverwriteSync(r, ch.ServerID, channelIDInt, ch.ParentID.Valid, ch.ParentID.Int64); err != nil {
		// Log but don't fail the request
		_ = err
	}

	// Broadcast overwrite update to channel subscribers
	h.broadcastChannelOverwriteUpdate(r, ch.ServerID, channelIDInt)

	// Audit
	overwriteTargetName := strconv.FormatInt(targetIDInt, 10)
	overwriteTypeStr := "role"
	if req.TargetType == 1 {
		overwriteTypeStr = "user"
		if name, err := h.service.repo.GetUsernameByID(r.Context(), targetIDInt); err == nil && name != "" {
			overwriteTargetName = name
		}
	} else {
		if role, err := h.service.repo.GetServerRoleByID(r.Context(), targetIDInt); err == nil && role != nil {
			overwriteTargetName = role.Name
		}
	}
	actorIDInt := userIDInt
	actorUsername, _ := h.service.repo.GetUsernameByID(r.Context(), actorIDInt)
	h.auditSvc.Log(r.Context(), audit.Entry{
		ServerID:      ch.ServerID,
		ActorID:       &actorIDInt,
		ActorUsername: actorUsername,
		Action:        "channel.overwrite_set",
		TargetID:      strconv.FormatInt(channelIDInt, 10),
		TargetType:    "channel",
		TargetName:    ch.Name,
		Changes: map[string]any{
			"overwrite_target_type": overwriteTypeStr,
			"overwrite_target_id":   strconv.FormatInt(targetIDInt, 10),
			"overwrite_target_name": overwriteTargetName,
			"before":                map[string]any{"allow": beforeAllow, "deny": beforeDeny},
			"after":                 map[string]any{"allow": req.Allow, "deny": req.Deny},
		},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ow)
}

// DeleteOverwrite handles DELETE /channels/{id}/overwrites/{overwriteId}
func (h *Handler) DeleteOverwrite(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "id")
	overwriteID := chi.URLParam(r, "overwriteId")
	if channelID == "" {
		http.Error(w, "channel ID is required", http.StatusBadRequest)
		return
	}
	if overwriteID == "" {
		http.Error(w, "overwrite ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelIDInt, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		http.Error(w, "invalid channel ID", http.StatusBadRequest)
		return
	}

	overwriteIDInt, err := strconv.ParseInt(overwriteID, 10, 64)
	if err != nil {
		http.Error(w, "invalid overwrite ID", http.StatusBadRequest)
		return
	}

	userIDInt, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	ch, err := h.service.repo.GetChannelByID(r.Context(), channelIDInt)
	if err != nil {
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}

	srv, err := h.service.repo.GetServerByID(r.Context(), ch.ServerID)
	if err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	// Check permission: must have ManageRoles or ManageChannels
	hasManageRoles, err := permissions.HasPermission(r.Context(), h.service.repo, ch.ServerID, userIDInt, srv.OwnerID, permissions.PermManageRoles)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hasManageChannels, err := permissions.HasPermission(r.Context(), h.service.repo, ch.ServerID, userIDInt, srv.OwnerID, permissions.PermManageChannels)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !hasManageRoles && !hasManageChannels {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Snapshot the overwrite BEFORE deletion so we can record before-state in the audit log.
	// GetOverwriteByID does not exist; read all channel overwrites and find the matching ID.
	var beforeOW *db.PermissionOverwrite
	if priorOWs, err := h.service.repo.GetRawChannelOverwrites(r.Context(), channelIDInt); err == nil {
		for i := range priorOWs {
			if priorOWs[i].ID == overwriteIDInt {
				beforeOW = &priorOWs[i]
				break
			}
		}
	}

	if err := h.service.repo.DeleteOverwrite(r.Context(), overwriteIDInt); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Sync handling
	if err := h.handleOverwriteSync(r, ch.ServerID, channelIDInt, ch.ParentID.Valid, ch.ParentID.Int64); err != nil {
		_ = err
	}

	// Broadcast overwrite update to channel subscribers
	h.broadcastChannelOverwriteUpdate(r, ch.ServerID, channelIDInt)

	// Audit (skip if we couldn't snapshot the prior overwrite — better than logging incomplete data)
	if beforeOW != nil {
		overwriteTargetName := strconv.FormatInt(beforeOW.TargetID, 10)
		overwriteTypeStr := "role"
		if beforeOW.TargetType == 1 {
			overwriteTypeStr = "user"
			if name, err := h.service.repo.GetUsernameByID(r.Context(), beforeOW.TargetID); err == nil && name != "" {
				overwriteTargetName = name
			}
		} else {
			if role, err := h.service.repo.GetServerRoleByID(r.Context(), beforeOW.TargetID); err == nil && role != nil {
				overwriteTargetName = role.Name
			}
		}
		actorIDInt := userIDInt
		actorUsername, _ := h.service.repo.GetUsernameByID(r.Context(), actorIDInt)
		h.auditSvc.Log(r.Context(), audit.Entry{
			ServerID:      ch.ServerID,
			ActorID:       &actorIDInt,
			ActorUsername: actorUsername,
			Action:        "channel.overwrite_delete",
			TargetID:      strconv.FormatInt(channelIDInt, 10),
			TargetType:    "channel",
			TargetName:    ch.Name,
			Changes: map[string]any{
				"overwrite_target_type": overwriteTypeStr,
				"overwrite_target_id":   strconv.FormatInt(beforeOW.TargetID, 10),
				"overwrite_target_name": overwriteTargetName,
				"before":                map[string]any{"allow": beforeOW.Allow, "deny": beforeOW.Deny},
			},
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetMyChannelPermissions handles GET /channels/{id}/my-permissions
func (h *Handler) GetMyChannelPermissions(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "id")
	if channelID == "" {
		http.Error(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelIDInt, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		http.Error(w, "invalid channel ID", http.StatusBadRequest)
		return
	}

	userIDInt, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	ch, err := h.service.repo.GetChannelByID(r.Context(), channelIDInt)
	if err != nil {
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}

	srv, err := h.service.repo.GetServerByID(r.Context(), ch.ServerID)
	if err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	basePerms, err := permissions.GetEffectivePermissions(r.Context(), h.service.repo, ch.ServerID, userIDInt, srv.OwnerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	everyoneRole, _ := h.service.repo.GetEveryoneRole(r.Context(), ch.ServerID)
	memberRoles, _ := h.service.repo.GetMemberRoles(r.Context(), ch.ServerID, userIDInt)
	overwrites, _ := h.service.repo.GetChannelOverwrites(r.Context(), channelIDInt)

	everyoneID := int64(0)
	if everyoneRole != nil {
		everyoneID = everyoneRole.ID
	}

	roleIDs := make([]int64, len(memberRoles))
	for i, role := range memberRoles {
		roleIDs[i] = role.ID
	}

	channelPerms := permissions.ComputeChannelPermissions(basePerms, userIDInt, roleIDs, everyoneID, overwrites)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"permissions": channelPerms})
}

// broadcastChannelOverwriteUpdate fetches the current overwrites for a channel and broadcasts
// a CHANNEL_OVERWRITE_UPDATE event to the channel's subscribers.
func (h *Handler) broadcastChannelOverwriteUpdate(r *http.Request, serverID, channelID int64) {
	// Invalidate cached permission masks for this channel — an overwrite
	// change can flip any user's effective bits in this channel.
	if h.service.memberCache != nil {
		h.service.memberCache.InvalidateChannelMasks(channelID)
	}
	if h.service.hub == nil {
		return
	}
	overwrites, err := h.service.repo.GetRawChannelOverwrites(r.Context(), channelID)
	if err != nil {
		log.Printf("broadcastChannelOverwriteUpdate: failed to get overwrites for channel %d: %v", channelID, err)
		return
	}
	payload, err := json.Marshal(map[string]interface{}{
		"channel_id": strconv.FormatInt(channelID, 10),
		"overwrites": overwrites,
	})
	if err != nil {
		log.Printf("broadcastChannelOverwriteUpdate: failed to marshal payload: %v", err)
		return
	}
	// Broadcast on the server topic so every member gets it. The previous
	// "channel:N" topic was never subscribed to by any client, so the event
	// silently dropped on the floor.
	serverIDStr := strconv.FormatInt(serverID, 10)
	h.service.hub.BroadcastToChannel("server:"+serverIDStr, ws.EventChannelOverwriteUpdate, payload)
}

// handleOverwriteSync handles the synced flag logic after an overwrite change.
// If the channel has a parent (is in a category), mark it as unsynced.
// If the channel IS a category (no parent, has children), propagate to synced children.
func (h *Handler) handleOverwriteSync(r *http.Request, serverID, channelID int64, hasParent bool, parentID int64) error {
	if hasParent {
		// Channel is in a category — mark as unsynced
		return h.service.repo.SetChannelSynced(r.Context(), channelID, false)
	}

	// Channel might be a category — propagate to synced children
	children, err := h.service.repo.GetSyncedChildrenByParent(r.Context(), channelID)
	if err != nil {
		return err
	}
	for _, child := range children {
		if copyErr := h.service.repo.CopyOverwrites(r.Context(), channelID, child.ID); copyErr != nil {
			return copyErr
		}
		// CopyOverwrites mutated this child's overwrites — drop its cached masks.
		if h.service.memberCache != nil {
			h.service.memberCache.InvalidateChannelMasks(child.ID)
		}
	}
	return nil
}
