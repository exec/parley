package dm

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/httputil"
	"parley/internal/validation"
	ws "parley/internal/websocket"
)

// Handler handles HTTP requests for DMs
type Handler struct {
	repo *db.Repository
	hub  *ws.Hub
}

// NewHandler creates a new DM handler
func NewHandler(repo *db.Repository, hub *ws.Hub) *Handler {
	return &Handler{repo: repo, hub: hub}
}

// OpenDmChannelRequest represents the request to open/start a DM
type OpenDmChannelRequest struct {
	UserID string `json:"user_id"`
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

	if req.UserID == "" {
		httputil.JSONError(w, "user_id is required", http.StatusBadRequest)
		return
	}

	otherUserID, err := strconv.ParseInt(req.UserID, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user_id", http.StatusBadRequest)
		return
	}

	if currentUserID == otherUserID {
		httputil.JSONError(w, "cannot DM yourself", http.StatusBadRequest)
		return
	}

	channel, err := h.repo.GetOrCreateDmChannel(r.Context(), currentUserID, otherUserID)
	if err != nil {
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
	channel, err := h.repo.GetDmChannelByID(r.Context(), dmChannelID)
	if err != nil {
		httputil.JSONError(w, "DM channel not found", http.StatusNotFound)
		return
	}

	if channel.User1ID != userID && channel.User2ID != userID {
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

	if channel.User1ID != currentUserID && channel.User2ID != currentUserID {
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

	// Broadcast to all participants via the dm:{id} virtual channel.
	if h.hub != nil {
		msgJSON, err := json.Marshal(msg)
		if err != nil {
			log.Printf("SendDmMessage: failed to marshal message for broadcast: %v", err)
		} else {
			virtualChannelID := "dm:" + strconv.FormatInt(dmChannelID, 10)
			h.hub.BroadcastToChannel(virtualChannelID, "dm_message", msgJSON)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(msg)
}

// ForwardDmRequest is the body for POST /dms/{id}/forward
type ForwardDmRequest struct {
	MessageID        string `json:"message_id"`
	ChannelID        string `json:"channel_id,omitempty"`
	ChannelName      string `json:"channel_name,omitempty"`
	ServerID         string `json:"server_id,omitempty"`
	ServerName       string `json:"server_name,omitempty"`
	AuthorUsername   string `json:"author_username"`
	AuthorDisplayName string `json:"author_display_name,omitempty"`
	AuthorAvatarURL  string `json:"author_avatar_url,omitempty"`
	Content          string `json:"content,omitempty"`
	AttachmentName   string `json:"attachment_name,omitempty"`
	CreatedAt        string `json:"created_at"`
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
	channel, err := h.repo.GetDmChannelByID(r.Context(), dmChannelID)
	if err != nil {
		httputil.JSONError(w, "DM channel not found", http.StatusNotFound)
		return
	}
	if channel.User1ID != currentUserID && channel.User2ID != currentUserID {
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
	createdAt, err := time.Parse(time.RFC3339Nano, req.CreatedAt)
	if err != nil {
		createdAt, _ = time.Parse(time.RFC3339, req.CreatedAt)
	}
	fwd := &db.ForwardedMessageData{
		MessageID:        req.MessageID,
		ChannelID:        req.ChannelID,
		ChannelName:      req.ChannelName,
		ServerID:         req.ServerID,
		ServerName:       req.ServerName,
		AuthorUsername:   req.AuthorUsername,
		AuthorDisplayName: req.AuthorDisplayName,
		AuthorAvatarURL:  req.AuthorAvatarURL,
		Content:          req.Content,
		AttachmentName:   req.AttachmentName,
		CreatedAt:        createdAt,
	}
	msg, err := h.repo.CreateForwardedDmMessage(r.Context(), dmChannelID, currentUserID, fwd)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	if h.hub != nil {
		msgJSON, _ := json.Marshal(msg)
		virtualChannelID := "dm:" + strconv.FormatInt(dmChannelID, 10)
		h.hub.BroadcastToChannel(virtualChannelID, "dm_message", msgJSON)
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
	channel, err := h.repo.GetDmChannelByID(r.Context(), dmChannelID)
	if err != nil {
		httputil.JSONError(w, "DM channel not found", http.StatusNotFound)
		return
	}
	if channel.User1ID != currentUserID && channel.User2ID != currentUserID {
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
		h.hub.BroadcastToChannel("dm:"+strconv.FormatInt(dmChannelID, 10), "dm_message_delete", payload)
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
	channel, err := h.repo.GetDmChannelByID(r.Context(), dmChannelID)
	if err != nil {
		httputil.JSONError(w, "DM channel not found", http.StatusNotFound)
		return
	}
	if channel.User1ID != currentUserID && channel.User2ID != currentUserID {
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
		eventType := "dm_reaction_remove"
		if added {
			eventType = "dm_reaction_add"
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

