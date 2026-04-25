package voice

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/httputil"
	ws "parley/internal/websocket"
)

// Handler handles voice HTTP endpoints. All routes accept a virtual-channel ID.
type Handler struct {
	svc         *Service
	repo        *db.Repository
	hub         *ws.Hub
	authz       *Authorizer
	dmCallEnder DmCallEnder
}

func NewHandler(svc *Service, repo *db.Repository, hub *ws.Hub) *Handler {
	return &Handler{
		svc:   svc,
		repo:  repo,
		hub:   hub,
		authz: NewAuthorizer(repo),
	}
}

// DmCallEnder is the optional callback the wiring layer sets to dm.Service.EmitCallEnded.
// Kept as a function-typed field so internal/voice doesn't import internal/dm directly,
// avoiding an import cycle.
type DmCallEnder func(ctx context.Context, dmChannelID, lastLeaverUserID, durationMs, startedAtMs int64) error

// SetDmCallEnder is called from cmd/api wiring after both services exist.
func (h *Handler) SetDmCallEnder(f DmCallEnder) { h.dmCallEnder = f }

// nowMs is a tiny seam so tests could fake time later.
var nowMs = func() int64 { return timeNow().UnixMilli() }

// timeNow is the underlying time func; declared here to keep the import contained.
var timeNow = func() time.Time { return time.Now() }

// parseVC extracts and validates the virtual channel from the URL path.
// All voice routes use {vc} as the path parameter name.
func (h *Handler) parseVC(w http.ResponseWriter, r *http.Request) (VirtualChannel, string, bool) {
	raw := r.PathValue("vc")
	vc, err := ParseVirtualChannel(raw)
	if err != nil {
		httputil.JSONError(w, "invalid virtual channel id", http.StatusBadRequest)
		return VirtualChannel{}, "", false
	}
	return vc, raw, true
}

// userFromCtx returns (userID int64, userIDStr string, ok). On !ok, an error response has been written.
func (h *Handler) userFromCtx(w http.ResponseWriter, r *http.Request) (int64, string, bool) {
	s := auth.GetUserIDFromContext(r)
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return 0, "", false
	}
	return id, s, true
}

// broadcastTarget returns the WS topic for voice-state events for this vc.
// Server channels broadcast to "server:{serverID}" (existing behavior so all
// members see green dots in sidebars). DMs broadcast to "dm:{id}" which DM
// members already subscribe to.
func (h *Handler) broadcastTarget(r *http.Request, vc VirtualChannel) (string, bool) {
	switch vc.Kind {
	case KindDM:
		return vc.String(), true
	case KindServer:
		ch, err := h.repo.GetChannelByID(r.Context(), vc.ID)
		if err != nil || ch == nil {
			return "", false
		}
		return "server:" + strconv.FormatInt(ch.ServerID, 10), true
	}
	return "", false
}

// Token issues a LiveKit token for the requesting user to join vc.
// GET /api/voice/{vc}/token
func (h *Handler) Token(w http.ResponseWriter, r *http.Request) {
	if !h.svc.Configured() {
		httputil.JSONError(w, "voice not configured", http.StatusServiceUnavailable)
		return
	}
	userID, userIDStr, ok := h.userFromCtx(w, r)
	if !ok {
		return
	}
	vc, vcStr, ok := h.parseVC(w, r)
	if !ok {
		return
	}
	allowed, err := h.authz.AuthorizeJoin(r.Context(), vc, userID)
	if err != nil || !allowed {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	user, err := h.repo.GetUserByID(r.Context(), userID)
	if err != nil {
		log.Printf("voice handler: failed to get user: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	tokenName := user.DisplayName
	if tokenName == "" {
		tokenName = user.Username
	}
	token, err := h.svc.IssueToken(userIDStr, tokenName, vcStr)
	if err != nil {
		log.Printf("voice handler: failed to generate token: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token, "url": h.svc.ServerURL()})
}

// Join records a participant + broadcasts a join state event.
// POST /api/voice/{vc}/join
func (h *Handler) Join(w http.ResponseWriter, r *http.Request) {
	userID, userIDStr, ok := h.userFromCtx(w, r)
	if !ok {
		return
	}
	vc, vcStr, ok := h.parseVC(w, r)
	if !ok {
		return
	}
	if allowed, err := h.authz.AuthorizeJoin(r.Context(), vc, userID); err != nil || !allowed {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	user, err := h.repo.GetUserByID(r.Context(), userID)
	if err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.Username
	}
	if err := h.svc.Join(r.Context(), vcStr, userIDStr, displayName, user.AvatarURL); err != nil {
		log.Printf("voice handler: failed to join: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if topic, ok := h.broadcastTarget(r, vc); ok {
		h.broadcastVoiceState(topic, vcStr, userIDStr, displayName, user.AvatarURL, "join")
	}
	w.WriteHeader(http.StatusNoContent)
}

// Leave removes a participant + broadcasts. If the room is now empty AND the
// vc is a DM, emits a call_ended system message via the DM service.
// POST /api/voice/{vc}/leave
func (h *Handler) Leave(w http.ResponseWriter, r *http.Request) {
	userID, userIDStr, ok := h.userFromCtx(w, r)
	if !ok {
		return
	}
	vc, vcStr, ok := h.parseVC(w, r)
	if !ok {
		return
	}
	user, _ := h.repo.GetUserByID(r.Context(), userID)
	displayName := ""
	avatarURL := ""
	if user != nil {
		displayName = user.DisplayName
		if displayName == "" {
			displayName = user.Username
		}
		avatarURL = user.AvatarURL
	}
	if err := h.svc.Leave(r.Context(), vcStr, userIDStr); err != nil {
		log.Printf("voice handler: failed to leave: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if topic, ok := h.broadcastTarget(r, vc); ok {
		h.broadcastVoiceState(topic, vcStr, userIDStr, displayName, avatarURL, "leave")
	}

	// Last-leaver detection — emit call_ended for DMs.
	if vc.Kind == KindDM {
		if startedAtMs, ended, err := h.svc.EndIfEmpty(r.Context(), vcStr); err == nil && ended {
			if h.dmCallEnder != nil {
				durationMs := nowMs() - startedAtMs
				if durationMs < 0 {
					durationMs = 0
				}
				_ = h.dmCallEnder(r.Context(), vc.ID, userID, durationMs, startedAtMs)
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// Participants is unchanged in semantics; just takes vc instead of channelID.
// GET /api/voice/{vc}/participants
func (h *Handler) Participants(w http.ResponseWriter, r *http.Request) {
	_, vcStr, ok := h.parseVC(w, r)
	if !ok {
		return
	}
	parts, err := h.svc.Participants(r.Context(), vcStr)
	if err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(parts)
}

// Heartbeat refreshes the per-user voice presence TTL.
// POST /api/voice/{vc}/heartbeat
func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	_, userIDStr, ok := h.userFromCtx(w, r)
	if !ok {
		return
	}
	_, vcStr, ok := h.parseVC(w, r)
	if !ok {
		return
	}
	if err := h.svc.RefreshHeartbeat(r.Context(), vcStr, userIDStr); err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MuteParticipant force-mutes a target via WS event.
// POST /api/voice/{vc}/participants/{targetUserId}/mute
func (h *Handler) MuteParticipant(w http.ResponseWriter, r *http.Request) {
	requesterID, _, ok := h.userFromCtx(w, r)
	if !ok {
		return
	}
	vc, vcStr, ok := h.parseVC(w, r)
	if !ok {
		return
	}
	targetUserIDStr := r.PathValue("targetUserId")
	targetID, err := strconv.ParseInt(targetUserIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid target user id", http.StatusBadRequest)
		return
	}
	allowed, err := h.authz.AuthorizeMute(r.Context(), vc, requesterID, targetID)
	if err != nil || !allowed {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"channel_id": vcStr,
		"muted":      true,
	})
	if err := h.hub.SendToUser(targetUserIDStr, ws.EventVoiceForceMute, payload); err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// KickParticipant force-disconnects a target.
// POST /api/voice/{vc}/participants/{targetUserId}/kick
func (h *Handler) KickParticipant(w http.ResponseWriter, r *http.Request) {
	requesterID, _, ok := h.userFromCtx(w, r)
	if !ok {
		return
	}
	vc, vcStr, ok := h.parseVC(w, r)
	if !ok {
		return
	}
	targetUserIDStr := r.PathValue("targetUserId")
	targetID, err := strconv.ParseInt(targetUserIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid target user id", http.StatusBadRequest)
		return
	}
	allowed, err := h.authz.AuthorizeKick(r.Context(), vc, requesterID, targetID)
	if err != nil || !allowed {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}

	// Remove + broadcast leave on the right topic.
	targetUser, _ := h.repo.GetUserByID(r.Context(), targetID)
	displayName := ""
	avatarURL := ""
	if targetUser != nil {
		displayName = targetUser.DisplayName
		if displayName == "" {
			displayName = targetUser.Username
		}
		avatarURL = targetUser.AvatarURL
	}
	if err := h.svc.Leave(r.Context(), vcStr, targetUserIDStr); err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if topic, ok := h.broadcastTarget(r, vc); ok {
		h.broadcastVoiceState(topic, vcStr, targetUserIDStr, displayName, avatarURL, "leave")
	}

	disc, _ := json.Marshal(map[string]any{"channel_id": vcStr})
	_ = h.hub.SendToUser(targetUserIDStr, ws.EventVoiceForceDisconnect, disc)

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) broadcastVoiceState(topic, channelID, userID, username, avatarURL, action string) {
	payload, _ := json.Marshal(map[string]string{
		"channel_id": channelID,
		"user_id":    userID,
		"username":   username,
		"avatar_url": avatarURL,
		"action":     action,
	})
	h.hub.BroadcastToChannel(topic, ws.EventVoiceStateUpdate, payload)
}
