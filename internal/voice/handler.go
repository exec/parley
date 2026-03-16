package voice

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"parley/internal/auth"
	"parley/internal/db"
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

func jsonErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// Token issues a LiveKit token for the requesting user to join a voice channel.
// GET /api/channels/{channelId}/voice/token
func (h *Handler) Token(w http.ResponseWriter, r *http.Request) {
	if !h.svc.Configured() {
		jsonErr(w, "voice not configured", http.StatusServiceUnavailable)
		return
	}

	userIDStr := auth.GetUserIDFromContext(r)
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		jsonErr(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelIDStr := r.PathValue("channelId")
	channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
	if err != nil {
		jsonErr(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	ch, err := h.repo.GetChannelByID(ctx, channelID)
	if err != nil {
		jsonErr(w, "channel not found", http.StatusNotFound)
		return
	}

	// Verify user is a member of the server
	member, err := h.repo.GetMember(ctx, ch.ServerID, userID)
	if err != nil || member == nil {
		jsonErr(w, "forbidden", http.StatusForbidden)
		return
	}

	token, err := h.svc.IssueToken(userIDStr, member.Username, channelIDStr)
	if err != nil {
		jsonErr(w, "failed to generate token", http.StatusInternalServerError)
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
		jsonErr(w, "failed to join", http.StatusInternalServerError)
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
		jsonErr(w, "failed to leave", http.StatusInternalServerError)
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
		jsonErr(w, "failed to fetch participants", http.StatusInternalServerError)
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
		jsonErr(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelIDStr = r.PathValue("channelId")
	channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
	if err != nil {
		jsonErr(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	ch, err := h.repo.GetChannelByID(ctx, channelID)
	if err != nil {
		jsonErr(w, "channel not found", http.StatusNotFound)
		return
	}

	member, err := h.repo.GetMember(ctx, ch.ServerID, userID)
	if err != nil || member == nil {
		jsonErr(w, "forbidden", http.StatusForbidden)
		return
	}

	serverVirtualChannelID = "server:" + strconv.FormatInt(ch.ServerID, 10)
	return userIDStr, member.Username, member.AvatarURL, channelIDStr, serverVirtualChannelID, true
}

// MuteParticipant force-mutes a participant in a voice channel.
// POST /channels/{channelId}/voice/participants/{targetUserId}/mute
func (h *Handler) MuteParticipant(w http.ResponseWriter, r *http.Request) {
	requesterIDStr := auth.GetUserIDFromContext(r)
	requesterID, err := strconv.ParseInt(requesterIDStr, 10, 64)
	if err != nil {
		jsonErr(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelIDStr := r.PathValue("channelId")
	channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
	if err != nil {
		jsonErr(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	targetUserIDStr := r.PathValue("targetUserId")
	if _, err := strconv.ParseInt(targetUserIDStr, 10, 64); err != nil {
		jsonErr(w, "invalid target user id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	ch, err := h.repo.GetChannelByID(ctx, channelID)
	if err != nil {
		jsonErr(w, "channel not found", http.StatusNotFound)
		return
	}

	srv, err := h.repo.GetServerByID(ctx, ch.ServerID)
	if err != nil {
		jsonErr(w, "server not found", http.StatusNotFound)
		return
	}

	ok, err := permissions.HasChannelPermission(ctx, h.repo, ch.ServerID, requesterID, srv.OwnerID, channelID, permissions.PermMuteMembers)
	if err != nil || !ok {
		jsonErr(w, "forbidden", http.StatusForbidden)
		return
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"channel_id": channelIDStr,
		"muted":      true,
	})
	if err := h.hub.SendToUser(targetUserIDStr, ws.EventVoiceForceMute, payload); err != nil {
		jsonErr(w, "failed to send mute event", http.StatusInternalServerError)
		return
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
