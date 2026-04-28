package dm

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/httputil"
	"parley/internal/validation"
	ws "parley/internal/websocket"
)

// DmNotifyFunc is called after a DM is sent to create a notification for the recipient.
// authorID is threaded so the notification layer can suppress delivery when the
// recipient has blocked the sender.
type DmNotifyFunc func(ctx context.Context, recipientID, authorID int64, authorUsername, authorAvatarURL string, dmChannelID int64)

// ForwardSourceResolver resolves a forward request's source message into a
// server-derived snapshot, after verifying the actor can read the source.
// Implemented by *message.MessageService; injected via SetForwardSourceResolver
// to avoid pulling internal/message into internal/dm at compile time.
type ForwardSourceResolver func(ctx context.Context, messageID, sourceChannelID, actorID string) (*db.ForwardedMessageData, error)

// Handler handles HTTP requests for DMs
type Handler struct {
	repo            *db.Repository
	hub             *ws.Hub
	svc             *Service
	dmNotify        DmNotifyFunc
	resolveForward  ForwardSourceResolver
}

// NewHandler creates a new DM handler
func NewHandler(repo *db.Repository, hub *ws.Hub) *Handler {
	return &Handler{repo: repo, hub: hub, svc: NewService(repo, hub)}
}

// SetForwardSourceResolver wires the cross-package resolver used by
// ForwardToDm to verify and snapshot the source message server-side.
func (h *Handler) SetForwardSourceResolver(fn ForwardSourceResolver) {
	h.resolveForward = fn
}

// SetDmNotify registers a function to call after a DM is sent.
func (h *Handler) SetDmNotify(fn DmNotifyFunc) {
	h.dmNotify = fn
}

// Service returns the underlying dm.Service. Used by the wiring layer to
// reach EmitCallEnded for the voice handler's last-leaver hook.
func (h *Handler) Service() *Service {
	return h.svc
}

// OpenDmChannelRequest represents the request to open/start a DM.
// Supports both single (`user_id`) and multi (`user_ids`) shapes;
// when len(user_ids) > 1 a group DM is created.
type OpenDmChannelRequest struct {
	UserID  string   `json:"user_id,omitempty"`
	UserIDs []string `json:"user_ids,omitempty"`
	Name    string   `json:"name,omitempty"`
}

// GetDmChannels handles GET /dms
func (h *Handler) GetDmChannels(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	if userIDStr == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	channels, err := h.repo.GetUserDmChannels(r.Context(), userID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}

	if channels == nil {
		channels = []db.DmChannel{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channels)
}

// OpenDmChannel handles POST /dms - start/open a DM conversation
func (h *Handler) OpenDmChannel(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	if userIDStr == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	currentUserID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	var req OpenDmChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var otherIDs []int64
	if len(req.UserIDs) > 0 {
		for _, s := range req.UserIDs {
			id, perr := strconv.ParseInt(s, 10, 64)
			if perr != nil {
				httputil.JSONError(w, "invalid user_ids entry", http.StatusBadRequest)
				return
			}
			otherIDs = append(otherIDs, id)
		}
	} else if req.UserID != "" {
		id, perr := strconv.ParseInt(req.UserID, 10, 64)
		if perr != nil {
			httputil.JSONError(w, "invalid user_id", http.StatusBadRequest)
			return
		}
		otherIDs = append(otherIDs, id)
	} else {
		httputil.JSONError(w, "user_id or user_ids is required", http.StatusBadRequest)
		return
	}

	channel, err := h.svc.CreateChannel(r.Context(), currentUserID, otherIDs, req.Name)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFriend):
			httputil.JSONError(w, "can only group-DM friends", http.StatusForbidden)
			return
		case errors.Is(err, ErrBlocked):
			httputil.JSONError(w, "blocked", http.StatusForbidden)
			return
		}
		if err.Error() == "must include at least one other user" {
			httputil.JSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		httputil.InternalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channel)
}

// GetDmMessages handles GET /dms/{id}/messages
func (h *Handler) GetDmMessages(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	if userIDStr == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	dmID := chi.URLParam(r, "id")
	if dmID == "" {
		httputil.JSONError(w, "DM channel ID is required", http.StatusBadRequest)
		return
	}

	dmChannelID, err := strconv.ParseInt(dmID, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid DM channel ID", http.StatusBadRequest)
		return
	}

	// Verify user is part of this DM channel
	if _, err := h.repo.GetDmChannelByID(r.Context(), dmChannelID); err != nil {
		httputil.JSONError(w, "DM channel not found", http.StatusNotFound)
		return
	}

	// Membership is re-checked per request — once a user is kicked from a
	// group DM their dm_channel_members row is deleted and IsMember returns
	// false, blocking any further history fetch (including by-cursor pagination
	// from a cached message ID).
	//
	// TODO(privacy): bound history by joined_at..removed_at on
	// dm_channel_members so re-added members can't see messages from the
	// window when they weren't a member. Requires adding a removed_at column
	// (currently dm_channel_members has no soft-delete) and updating
	// AddMembers/RemoveDmMember to manage it.
	isMember, err := h.svc.IsMember(r.Context(), dmChannelID, userID)
	if err != nil || !isMember {
		httputil.JSONError(w, "not authorized to view this DM channel", http.StatusForbidden)
		return
	}

	// Parse query parameters
	limit := 50
	var beforeID int64

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			if l > 200 {
				l = 200
			}
			limit = l
		}
	}

	if beforeStr := r.URL.Query().Get("before"); beforeStr != "" {
		if b, err := strconv.ParseInt(beforeStr, 10, 64); err == nil && b > 0 {
			beforeID = b
		}
	}

	messages, err := h.repo.GetDmMessages(r.Context(), dmChannelID, limit, beforeID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}

	if messages == nil {
		messages = []db.DmMessage{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// SendDmMessageRequest represents the request to send a DM
type SendDmMessageRequest struct {
	Content        string  `json:"content"`
	AttachmentURL  string  `json:"attachment_url"`
	AttachmentName string  `json:"attachment_name"`
	AttachmentType string  `json:"attachment_type"`
	ParentID       *string `json:"parent_id"`
}

// SendDmMessage handles POST /dms/{id}/messages
func (h *Handler) SendDmMessage(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	if userIDStr == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	currentUserID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	dmID := chi.URLParam(r, "id")
	if dmID == "" {
		httputil.JSONError(w, "DM channel ID is required", http.StatusBadRequest)
		return
	}

	dmChannelID, err := strconv.ParseInt(dmID, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid DM channel ID", http.StatusBadRequest)
		return
	}

	// Verify user is part of this DM channel
	channel, err := h.repo.GetDmChannelByID(r.Context(), dmChannelID)
	if err != nil {
		httputil.JSONError(w, "DM channel not found", http.StatusNotFound)
		return
	}

	isMember, err := h.svc.IsMember(r.Context(), dmChannelID, currentUserID)
	if err != nil || !isMember {
		httputil.JSONError(w, "not authorized to send messages in this DM channel", http.StatusForbidden)
		return
	}

	var req SendDmMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content == "" && req.AttachmentURL == "" {
		httputil.JSONError(w, "content or attachment is required", http.StatusBadRequest)
		return
	}
	if len(req.Content) > 4000 {
		httputil.JSONError(w, "message content exceeds maximum length of 4000 characters", http.StatusBadRequest)
		return
	}
	if validation.HasSpoofedLink(req.Content) {
		httputil.JSONError(w, "message contains a spoofed link", http.StatusBadRequest)
		return
	}

	var parentID *int64
	if req.ParentID != nil && *req.ParentID != "" {
		pid, err := strconv.ParseInt(*req.ParentID, 10, 64)
		if err == nil {
			parentID = &pid
		}
	}

	msg, err := h.repo.CreateDmMessage(r.Context(), dmChannelID, currentUserID, req.Content, req.AttachmentURL, req.AttachmentName, req.AttachmentType, parentID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}

	// Implicit author read-marker for the DM channel — saves a client round-trip.
	// Non-fatal: read-state is best-effort metadata, not a part of the send contract.
	if err := h.repo.UpsertReadMarker(r.Context(), currentUserID, db.ChannelKindDM, dmChannelID, msg.ID); err != nil {
		log.Printf("dm: failed to upsert author read marker: %v", err)
	}

	// Broadcast to all participants via the dm:{id} virtual channel.
	if h.hub != nil {
		msgJSON, err := json.Marshal(msg)
		if err != nil {
			log.Printf("SendDmMessage: failed to marshal message for broadcast: %v", err)
		} else {
			virtualChannelID := "dm:" + strconv.FormatInt(dmChannelID, 10)
			h.hub.BroadcastToChannel(virtualChannelID, ws.EventDmMessageCreate, msgJSON)
		}
	}

	// If this is the very first message in the channel, the recipient has never
	// seen this DM channel before and isn't subscribed to dm:{id}. Surface the
	// new channel to them via their per-user WS so their DM list updates live.
	// We detect "first message" via a post-insert count == 1 rather than wiring
	// a `created` flag through GetOrCreateDmChannel, because the channel is
	// typically created earlier by POST /dms (open-without-sending) — the true
	// "new surfacing" signal is the first inbound message, not channel creation.
	if h.hub != nil && !channel.IsGroup {
		// First-message surfacing for legacy 1:1 channels only — group DMs
		// already get DM_CHANNEL_CREATE fan-out at creation time in service.CreateChannel.
		count, cerr := h.repo.CountDmMessages(r.Context(), dmChannelID)
		if cerr == nil && count == 1 {
			recipientID := channel.User1ID
			if channel.User1ID == currentUserID {
				recipientID = channel.User2ID
			}
			recipientChannel, ferr := h.repo.GetDmChannelForUser(r.Context(), dmChannelID, recipientID)
			if ferr == nil {
				payload, perr := json.Marshal(map[string]interface{}{
					"channel": recipientChannel,
					"message": msg,
				})
				if perr == nil {
					h.hub.SendToUser(strconv.FormatInt(recipientID, 10), ws.EventDmChannelCreate, payload)
				}
			}
		}
	}

	// Notify recipients asynchronously. For 1:1 DMs notify the other user;
	// for group DMs fan out to all members minus the author.
	if h.dmNotify != nil {
		var senderUsername, senderAvatarURL string
		h.repo.DB().QueryRowContext(r.Context(),
			"SELECT username, COALESCE(avatar_url,'') FROM users WHERE id=$1", currentUserID,
		).Scan(&senderUsername, &senderAvatarURL)
		notifyFn := h.dmNotify

		if channel.IsGroup {
			// Fan out to every member except the author.
			members, merr := h.repo.GetDmMembers(r.Context(), dmChannelID)
			if merr != nil {
				log.Printf("SendDmMessage: failed to load group members for notify fan-out: %v", merr)
			} else {
				for _, m := range members {
					if m.UserID == currentUserID {
						continue
					}
					recipientID := m.UserID
					go notifyFn(context.Background(), recipientID, currentUserID, senderUsername, senderAvatarURL, dmChannelID)
				}
			}
		} else {
			recipientID := channel.User1ID
			if channel.User1ID == currentUserID {
				recipientID = channel.User2ID
			}
			go notifyFn(context.Background(), recipientID, currentUserID, senderUsername, senderAvatarURL, dmChannelID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(msg)
}

// ForwardDmRequest is the body for POST /dms/{id}/forward.
// Only MessageID and (for server-channel sources) ChannelID are honored —
// every other field is loaded from the source row by the server. Legacy
// fields are still parsed off the wire so older clients don't 400.
type ForwardDmRequest struct {
	MessageID string `json:"message_id"`
	ChannelID string `json:"channel_id,omitempty"`

	// Ignored — present for backward compat with clients that still send them.
	ChannelName       string `json:"channel_name,omitempty"`
	ServerID          string `json:"server_id,omitempty"`
	ServerName        string `json:"server_name,omitempty"`
	AuthorUsername    string `json:"author_username,omitempty"`
	AuthorDisplayName string `json:"author_display_name,omitempty"`
	AuthorAvatarURL   string `json:"author_avatar_url,omitempty"`
	Content           string `json:"content,omitempty"`
	AttachmentName    string `json:"attachment_name,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
}

// ForwardToDm handles POST /dms/{id}/forward
func (h *Handler) ForwardToDm(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	if userIDStr == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	currentUserID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user ID", http.StatusBadRequest)
		return
	}
	dmID := chi.URLParam(r, "id")
	dmChannelID, err := strconv.ParseInt(dmID, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid DM channel ID", http.StatusBadRequest)
		return
	}
	if _, err := h.repo.GetDmChannelByID(r.Context(), dmChannelID); err != nil {
		httputil.JSONError(w, "DM channel not found", http.StatusNotFound)
		return
	}
	isMember, err := h.svc.IsMember(r.Context(), dmChannelID, currentUserID)
	if err != nil || !isMember {
		httputil.JSONError(w, "not authorized", http.StatusForbidden)
		return
	}
	var req ForwardDmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.MessageID == "" {
		httputil.JSONError(w, "message_id is required", http.StatusBadRequest)
		return
	}
	if h.resolveForward == nil {
		// Wiring bug — refuse rather than fall back to client-supplied fields.
		httputil.JSONError(w, "forward not available", http.StatusServiceUnavailable)
		return
	}
	fwd, err := h.resolveForward(r.Context(), req.MessageID, req.ChannelID, userIDStr)
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			httputil.JSONError(w, "you cannot forward a message you cannot read", http.StatusForbidden)
		case errors.Is(err, ErrNotFound):
			httputil.JSONError(w, "source message not found", http.StatusNotFound)
		default:
			httputil.InternalError(w, err)
		}
		return
	}
	msg, err := h.repo.CreateForwardedDmMessage(r.Context(), dmChannelID, currentUserID, fwd)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	if h.hub != nil {
		msgJSON, _ := json.Marshal(msg)
		virtualChannelID := "dm:" + strconv.FormatInt(dmChannelID, 10)
		h.hub.BroadcastToChannel(virtualChannelID, ws.EventDmMessageCreate, msgJSON)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(msg)
}

// DeleteDmMessage handles DELETE /dms/{id}/messages/{messageId}
func (h *Handler) DeleteDmMessage(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	if userIDStr == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	currentUserID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user ID", http.StatusBadRequest)
		return
	}
	dmID := chi.URLParam(r, "id")
	messageID, err := strconv.ParseInt(chi.URLParam(r, "messageId"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid message id", http.StatusBadRequest)
		return
	}
	dmChannelID, err := strconv.ParseInt(dmID, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid DM channel id", http.StatusBadRequest)
		return
	}
	if _, err := h.repo.GetDmChannelByID(r.Context(), dmChannelID); err != nil {
		httputil.JSONError(w, "DM channel not found", http.StatusNotFound)
		return
	}
	isMember, err := h.svc.IsMember(r.Context(), dmChannelID, currentUserID)
	if err != nil || !isMember {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := h.repo.DeleteDmMessage(r.Context(), messageID, currentUserID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			httputil.JSONError(w, "message not found or not your message", http.StatusNotFound)
			return
		}
		httputil.InternalError(w, err)
		return
	}
	// Broadcast to both participants via dm virtual channel
	if h.hub != nil {
		payload, _ := json.Marshal(map[string]string{
			"message_id":    strconv.FormatInt(messageID, 10),
			"dm_channel_id": strconv.FormatInt(dmChannelID, 10),
		})
		h.hub.BroadcastToChannel("dm:"+strconv.FormatInt(dmChannelID, 10), ws.EventDmMessageDelete, payload)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ToggleDmReaction handles POST /dms/{id}/messages/{messageId}/reactions
func (h *Handler) ToggleDmReaction(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	if userIDStr == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	currentUserID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user ID", http.StatusBadRequest)
		return
	}
	dmID := chi.URLParam(r, "id")
	messageID, err := strconv.ParseInt(chi.URLParam(r, "messageId"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid message id", http.StatusBadRequest)
		return
	}
	dmChannelID, err := strconv.ParseInt(dmID, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid DM channel id", http.StatusBadRequest)
		return
	}
	if _, err := h.repo.GetDmChannelByID(r.Context(), dmChannelID); err != nil {
		httputil.JSONError(w, "DM channel not found", http.StatusNotFound)
		return
	}
	isMember, err := h.svc.IsMember(r.Context(), dmChannelID, currentUserID)
	if err != nil || !isMember {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Emoji string `json:"emoji"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Emoji == "" {
		httputil.JSONError(w, "emoji required", http.StatusBadRequest)
		return
	}
	added, err := h.repo.ToggleDmReaction(r.Context(), messageID, currentUserID, req.Emoji)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	// Broadcast to both participants via dm virtual channel
	if h.hub != nil {
		eventType := ws.EventDmReactionRemove
		if added {
			eventType = ws.EventDmReactionAdd
		}
		payload, _ := json.Marshal(map[string]string{
			"message_id":    strconv.FormatInt(messageID, 10),
			"dm_channel_id": strconv.FormatInt(dmChannelID, 10),
			"user_id":       userIDStr,
			"emoji":         req.Emoji,
		})
		h.hub.BroadcastToChannel("dm:"+strconv.FormatInt(dmChannelID, 10), eventType, payload)
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetMembers handles GET /dms/{id}/members — returns the member roster.
func (h *Handler) GetMembers(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	userID, _ := strconv.ParseInt(userIDStr, 10, 64)
	channelID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid channel id", http.StatusBadRequest)
		return
	}
	isMember, err := h.repo.IsDmMember(r.Context(), channelID, userID)
	if err != nil || !isMember {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	members, err := h.repo.GetDmMembers(r.Context(), channelID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	if members == nil {
		members = []db.DmChannelMember{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(members)
}

// AddMembersRequest is the body for POST /dms/{id}/members.
type AddMembersRequest struct {
	UserIDs []string `json:"user_ids"`
}

// AddMembers handles POST /dms/{id}/members — add users to a group DM.
func (h *Handler) AddMembers(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	actorID, _ := strconv.ParseInt(userIDStr, 10, 64)
	channelID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	var req AddMembersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	var newIDs []int64
	for _, sid := range req.UserIDs {
		n, perr := strconv.ParseInt(sid, 10, 64)
		if perr != nil {
			httputil.JSONError(w, "invalid user id", http.StatusBadRequest)
			return
		}
		newIDs = append(newIDs, n)
	}

	if err := h.svc.AddMembers(r.Context(), channelID, actorID, newIDs); err != nil {
		switch {
		case errors.Is(err, ErrNotFriend):
			httputil.JSONError(w, "can only add friends to a group", http.StatusForbidden)
			return
		case errors.Is(err, ErrBlocked):
			httputil.JSONError(w, "blocked", http.StatusForbidden)
			return
		}
		msg := err.Error()
		if strings.Contains(msg, "not a member") || strings.Contains(msg, "not a group") || strings.Contains(msg, "capacity") {
			httputil.JSONError(w, msg, http.StatusBadRequest)
			return
		}
		httputil.InternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LeaveRequest is the body for POST /dms/{id}/leave. Body is optional;
// when transfer_to is provided and the actor is the owner, ownership is
// passed to that user before leaving.
type LeaveRequest struct {
	TransferTo *string `json:"transfer_to,omitempty"`
}

// Leave handles POST /dms/{id}/leave.
func (h *Handler) Leave(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	actorID, _ := strconv.ParseInt(userIDStr, 10, 64)
	channelID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	var req LeaveRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // body optional

	var transfer *int64
	if req.TransferTo != nil && *req.TransferTo != "" {
		n, perr := strconv.ParseInt(*req.TransferTo, 10, 64)
		if perr != nil {
			httputil.JSONError(w, "invalid transfer_to", http.StatusBadRequest)
			return
		}
		transfer = &n
	}

	if err := h.svc.LeaveGroup(r.Context(), channelID, actorID, transfer); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "not a") || strings.Contains(msg, "only owner") || strings.Contains(msg, "transfer target") || strings.Contains(msg, "transfer to yourself") {
			httputil.JSONError(w, msg, http.StatusBadRequest)
			return
		}
		httputil.InternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveMember handles DELETE /dms/{id}/members/{userID} — owner-only kick.
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	actorID, _ := strconv.ParseInt(userIDStr, 10, 64)
	channelID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid channel id", http.StatusBadRequest)
		return
	}
	targetID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user id", http.StatusBadRequest)
		return
	}

	if err := h.svc.KickMember(r.Context(), channelID, actorID, targetID); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "not the owner") {
			httputil.JSONError(w, msg, http.StatusForbidden)
			return
		}
		if strings.Contains(msg, "yourself") || strings.Contains(msg, "not a member") || strings.Contains(msg, "not a group") {
			httputil.JSONError(w, msg, http.StatusBadRequest)
			return
		}
		httputil.InternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateGroupRequest is the body for PATCH /dms/{id}.
type UpdateGroupRequest struct {
	Name        *string `json:"name,omitempty"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	ClearAvatar bool    `json:"clear_avatar,omitempty"`
}

// UpdateGroup handles PATCH /dms/{id} — rename and/or update avatar.
func (h *Handler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	actorID, _ := strconv.ParseInt(userIDStr, 10, 64)
	channelID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	var req UpdateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name != nil {
		if err := h.svc.UpdateGroupName(r.Context(), channelID, actorID, *req.Name); err != nil {
			httputil.JSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if req.AvatarURL != nil || req.ClearAvatar {
		var avatar *string
		if !req.ClearAvatar {
			avatar = req.AvatarURL
		}
		if err := h.svc.UpdateGroupAvatar(r.Context(), channelID, actorID, avatar); err != nil {
			httputil.JSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// TransferOwnershipRequest is the body for POST /dms/{id}/transfer-ownership.
type TransferOwnershipRequest struct {
	NewOwnerID string `json:"new_owner_id"`
}

// TransferOwnership handles POST /dms/{id}/transfer-ownership.
func (h *Handler) TransferOwnership(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	actorID, _ := strconv.ParseInt(userIDStr, 10, 64)
	channelID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	var req TransferOwnershipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	newOwnerID, err := strconv.ParseInt(req.NewOwnerID, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid new_owner_id", http.StatusBadRequest)
		return
	}

	if err := h.svc.TransferOwnership(r.Context(), channelID, actorID, newOwnerID); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "not the owner") {
			httputil.JSONError(w, msg, http.StatusForbidden)
			return
		}
		httputil.JSONError(w, msg, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

