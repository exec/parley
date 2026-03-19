package voice

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/httputil"
	"parley/internal/permissions"
	ws "parley/internal/websocket"
)

// Handler handles voice channel HTTP endpoints.
type Handler struct {
	svc  *Service
	repo *db.Repository
	hub  *ws.Hub
}

func NewHandler(svc *Service, repo *db.Repository, hub *ws.Hub) *Handler {
	return &Handler{svc: svc, repo: repo, hub: hub}
}

// Token issues a LiveKit token for the requesting user to join a voice channel.
// GET /api/channels/{channelId}/voice/token
func (h *Handler) Token(w http.ResponseWriter, r *http.Request) {
	if !h.svc.Configured() {
		httputil.JSONError(w, "voice not configured", http.StatusServiceUnavailable)
		return
	}

	userIDStr := auth.GetUserIDFromContext(r)
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelIDStr := r.PathValue("channelId")
	channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	ch, err := h.repo.GetChannelByID(ctx, channelID)
	if err != nil {
		httputil.JSONError(w, "channel not found", http.StatusNotFound)
		return
	}

	// Verify user is a member of the server
	member, err := h.repo.GetMember(ctx, ch.ServerID, userID)
	if err != nil || member == nil {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}

	user, err := h.repo.GetUserByID(ctx, userID)
	if err != nil {
		log.Printf("voice handler: failed to get user: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	tokenName := user.DisplayName
	if tokenName == "" {
		tokenName = user.Username
	}
	token, err := h.svc.IssueToken(userIDStr, tokenName, channelIDStr)
	if err != nil {
		log.Printf("voice handler: failed to generate token: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token": token,
		"url":   h.svc.ServerURL(),
	})
}

// Join records a participant in a voice channel and broadcasts a WS event.
// POST /api/channels/{channelId}/voice/join
func (h *Handler) Join(w http.ResponseWriter, r *http.Request) {
	userIDStr, username, avatarURL, channelIDStr, serverVirtualChannelID, ok := h.parseVoiceRequest(w, r)
	if !ok {
		return
	}

	if err := h.svc.Join(r.Context(), channelIDStr, userIDStr, username, avatarURL); err != nil {
		log.Printf("voice handler: failed to join: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	h.broadcastVoiceState(serverVirtualChannelID, channelIDStr, userIDStr, username, avatarURL, "join")
	w.WriteHeader(http.StatusNoContent)
}

// Leave removes a participant from a voice channel and broadcasts a WS event.
// POST /api/channels/{channelId}/voice/leave
func (h *Handler) Leave(w http.ResponseWriter, r *http.Request) {
	userIDStr, username, avatarURL, channelIDStr, serverVirtualChannelID, ok := h.parseVoiceRequest(w, r)
	if !ok {
		return
	}

	if err := h.svc.Leave(r.Context(), channelIDStr, userIDStr); err != nil {
		log.Printf("voice handler: failed to leave: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	h.broadcastVoiceState(serverVirtualChannelID, channelIDStr, userIDStr, username, avatarURL, "leave")
	w.WriteHeader(http.StatusNoContent)
}

// Participants returns who is currently in a voice channel.
// GET /api/channels/{channelId}/voice/participants
func (h *Handler) Participants(w http.ResponseWriter, r *http.Request) {
	channelIDStr := r.PathValue("channelId")

	participants, err := h.svc.Participants(r.Context(), channelIDStr)
	if err != nil {
		log.Printf("voice handler: failed to fetch participants: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(participants)
}

// parseVoiceRequest extracts and validates auth + channel for join/leave.
func (h *Handler) parseVoiceRequest(w http.ResponseWriter, r *http.Request) (userIDStr, username, avatarURL, channelIDStr, serverVirtualChannelID string, ok bool) {
	userIDStr = auth.GetUserIDFromContext(r)
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelIDStr = r.PathValue("channelId")
	channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	ch, err := h.repo.GetChannelByID(ctx, channelID)
	if err != nil {
		httputil.JSONError(w, "channel not found", http.StatusNotFound)
		return
	}

	member, err := h.repo.GetMember(ctx, ch.ServerID, userID)
	if err != nil || member == nil {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}

	user, err := h.repo.GetUserByID(ctx, userID)
	if err != nil {
		log.Printf("voice handler: failed to get user: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.Username
	}
	serverVirtualChannelID = "server:" + strconv.FormatInt(ch.ServerID, 10)
	return userIDStr, displayName, user.AvatarURL, channelIDStr, serverVirtualChannelID, true
}

// MuteParticipant force-mutes a participant in a voice channel.
// POST /channels/{channelId}/voice/participants/{targetUserId}/mute
func (h *Handler) MuteParticipant(w http.ResponseWriter, r *http.Request) {
	requesterIDStr := auth.GetUserIDFromContext(r)
	requesterID, err := strconv.ParseInt(requesterIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelIDStr := r.PathValue("channelId")
	channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	targetUserIDStr := r.PathValue("targetUserId")
	if _, err := strconv.ParseInt(targetUserIDStr, 10, 64); err != nil {
		httputil.JSONError(w, "invalid target user id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	ch, err := h.repo.GetChannelByID(ctx, channelID)
	if err != nil {
		httputil.JSONError(w, "channel not found", http.StatusNotFound)
		return
	}

	srv, err := h.repo.GetServerByID(ctx, ch.ServerID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	ok, err := permissions.HasChannelPermission(ctx, h.repo, ch.ServerID, requesterID, srv.OwnerID, channelID, permissions.PermMuteMembers)
	if err != nil || !ok {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"channel_id": channelIDStr,
		"muted":      true,
	})
	if err := h.hub.SendToUser(targetUserIDStr, ws.EventVoiceForceMute, payload); err != nil {
		log.Printf("voice handler: failed to send mute event: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// KickParticipant force-disconnects a participant from a voice channel.
// POST /channels/{channelId}/voice/participants/{targetUserId}/kick
func (h *Handler) KickParticipant(w http.ResponseWriter, r *http.Request) {
	requesterIDStr := auth.GetUserIDFromContext(r)
	requesterID, err := strconv.ParseInt(requesterIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelIDStr := r.PathValue("channelId")
	channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	targetUserIDStr := r.PathValue("targetUserId")
	targetUserID, err := strconv.ParseInt(targetUserIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid target user id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	ch, err := h.repo.GetChannelByID(ctx, channelID)
	if err != nil {
		httputil.JSONError(w, "channel not found", http.StatusNotFound)
		return
	}

	srv, err := h.repo.GetServerByID(ctx, ch.ServerID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	ok, err := permissions.HasChannelPermission(ctx, h.repo, ch.ServerID, requesterID, srv.OwnerID, channelID, permissions.PermMoveMembers)
	if err != nil || !ok {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}

	// Look up target member for the broadcast payload
	targetMember, err := h.repo.GetMember(ctx, ch.ServerID, targetUserID)
	if err != nil || targetMember == nil {
		httputil.JSONError(w, "target member not found", http.StatusNotFound)
		return
	}

	serverVirtualChannelID := "server:" + strconv.FormatInt(ch.ServerID, 10)

	// Remove from DB and broadcast leave state
	if err := h.svc.Leave(ctx, channelIDStr, targetUserIDStr); err != nil {
		log.Printf("voice handler: failed to remove participant: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	targetDisplayName := targetMember.DisplayName
	if targetDisplayName == "" {
		targetDisplayName = targetMember.Username
	}
	h.broadcastVoiceState(serverVirtualChannelID, channelIDStr, targetUserIDStr, targetDisplayName, targetMember.AvatarURL, "leave")

	// Send disconnect event to the target user
	payload, _ := json.Marshal(map[string]interface{}{
		"channel_id": channelIDStr,
	})
	if err := h.hub.SendToUser(targetUserIDStr, ws.EventVoiceForceDisconnect, payload); err != nil {
		// Non-fatal: user may have already left; state is already cleaned up
		_ = err
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) broadcastVoiceState(serverVirtualChannelID, channelID, userID, username, avatarURL, action string) {
	payload, _ := json.Marshal(map[string]string{
		"channel_id": channelID,
		"user_id":    userID,
		"username":   username,
		"avatar_url": avatarURL,
		"action":     action,
	})
	h.hub.BroadcastToChannel(serverVirtualChannelID, ws.EventVoiceStateUpdate, payload)
}
