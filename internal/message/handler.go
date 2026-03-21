package message

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/httputil"
)

// Handler handles HTTP requests for messages
type Handler struct {
	service *MessageService
	cdnHost string
}

// NewHandler creates a new message handler
func NewHandler(service *MessageService, cdnHost string) *Handler {
	return &Handler{service: service, cdnHost: cdnHost}
}

// validateAttachmentURL ensures an attachment URL is either empty or points to
// the configured CDN host over HTTPS, preventing SSRF via arbitrary URLs.
func (h *Handler) validateAttachmentURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid attachment URL")
	}
	if u.Scheme != "https" {
		return errors.New("invalid attachment URL")
	}
	if h.cdnHost != "" && !strings.EqualFold(u.Host, h.cdnHost) {
		return errors.New("invalid attachment URL")
	}
	return nil
}

// Routes returns the chi router with message routes
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	// POST /channels/:channelID/messages - send message
	r.Post("/", h.SendMessage)

	// GET /channels/:channelID/messages - get messages
	r.Get("/", h.GetChannelMessages)

	// PUT /messages/:id - edit message
	r.Put("/", h.EditMessage)

	// DELETE /messages/:id - delete message
	r.Delete("/", h.DeleteMessage)

	return r
}

// SendMessageRequest represents the request body for sending a message
type SendMessageRequest struct {
	Content        string `json:"content"`
	Nonce          string `json:"nonce"`
	ParentID       string `json:"parent_id"`
	AttachmentURL  string `json:"attachment_url"`
	AttachmentName string `json:"attachment_name"`
	AttachmentType string `json:"attachment_type"`
}

// SendMessage handles POST /channels/:channelID/messages
func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		httputil.JSONError(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	// Get author ID from context (set by auth middleware)
	authorID := auth.GetUserIDFromContext(r)
	if authorID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content == "" && req.AttachmentURL == "" {
		httputil.JSONError(w, "content or attachment is required", http.StatusBadRequest)
		return
	}

	if err := h.validateAttachmentURL(req.AttachmentURL); err != nil {
		httputil.JSONError(w, "invalid attachment URL", http.StatusBadRequest)
		return
	}

	msg, err := h.service.SendMessage(r.Context(), channelID, authorID, req.Content, req.Nonce, req.AttachmentURL, req.AttachmentName, req.AttachmentType, req.ParentID)
	if err != nil {
		if err.Error() == "forbidden" {
			httputil.JSONError(w, "you do not have permission to send messages in this channel", http.StatusForbidden)
			return
		}
		httputil.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(msg)
}

// GetChannelMessages handles GET /channels/:channelID/messages
func (h *Handler) GetChannelMessages(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		httputil.JSONError(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	// Get user ID from context for ViewChannel check.
	userID := auth.GetUserIDFromContext(r)

	// Parse query parameters
	limit := 50
	var beforeID int64

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			if l > 50 {
				l = 50
			}
			limit = l
		}
	}

	if beforeStr := r.URL.Query().Get("before"); beforeStr != "" {
		if b, err := strconv.ParseInt(beforeStr, 10, 64); err == nil && b > 0 {
			beforeID = b
		}
	}

	messages, err := h.service.GetChannelMessages(r.Context(), channelID, userID, limit, beforeID)
	if err != nil {
		if err.Error() == "channel not found" {
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
			return
		}
		httputil.InternalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// EditMessageRequest represents the request body for editing a message
type EditMessageRequest struct {
	Content string `json:"content"`
}

// EditMessage handles PUT /messages/:id
func (h *Handler) EditMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.JSONError(w, "message ID is required", http.StatusBadRequest)
		return
	}

	// Get author ID from context (set by auth middleware)
	authorID := auth.GetUserIDFromContext(r)
	if authorID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	var req EditMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Check if the message exists and belongs to the user
	msg, err := h.service.GetMessage(r.Context(), id)
	if err != nil {
		httputil.JSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	if msg.AuthorID != authorID {
		httputil.JSONError(w, "you can only edit your own messages", http.StatusForbidden)
		return
	}

	updatedMsg, err := h.service.EditMessage(r.Context(), id, req.Content)
	if err != nil {
		httputil.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedMsg)
}

// DeleteMessage handles DELETE /messages/:id
func (h *Handler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.JSONError(w, "message ID is required", http.StatusBadRequest)
		return
	}

	// Get author ID from context (set by auth middleware)
	authorID := auth.GetUserIDFromContext(r)
	if authorID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	// Check if the user has permission to delete the message (author or MANAGE_MESSAGES)
	canManage, err := h.service.CanManageMessage(r.Context(), id, authorID)
	if err != nil {
		if err.Error() == "message not found" {
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
		} else {
			httputil.InternalError(w, err)
		}
		return
	}
	if !canManage {
		httputil.JSONError(w, "you can only delete your own messages", http.StatusForbidden)
		return
	}

	if err := h.service.DeleteMessage(r.Context(), id); err != nil {
		httputil.InternalError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetMessageVersions handles GET /messages/:id/versions
func (h *Handler) GetMessageVersions(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "id")
	versions, err := h.service.GetMessageVersions(r.Context(), messageID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	if versions == nil {
		versions = []db.MessageVersion{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(versions)
}

// SearchMessages handles GET /servers/{serverID}/messages/search
func (h *Handler) SearchMessages(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		httputil.JSONError(w, "server ID is required", http.StatusBadRequest)
		return
	}
	userID := auth.GetUserIDFromContext(r)

	q := r.URL.Query().Get("q")
	fromUserID := r.URL.Query().Get("from")
	inChannelID := r.URL.Query().Get("in")

	limit := 25
	if ls := r.URL.Query().Get("limit"); ls != "" {
		if v, err := strconv.Atoi(ls); err == nil && v > 0 && v <= 50 {
			limit = v
		}
	}
	var beforeID int64
	if bs := r.URL.Query().Get("before"); bs != "" {
		if v, err := strconv.ParseInt(bs, 10, 64); err == nil && v > 0 {
			beforeID = v
		}
	}

	messages, err := h.service.SearchMessages(r.Context(), serverID, userID, q, fromUserID, inChannelID, limit, beforeID)
	if err != nil {
		if err.Error() == "forbidden" {
			httputil.JSONError(w, "forbidden", http.StatusForbidden)
			return
		}
		httputil.InternalError(w, err)
		return
	}
	if messages == nil {
		messages = []*Message{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// ToggleReactionRequest is the body for POST /messages/:id/reactions
type ToggleReactionRequest struct {
	Emoji string `json:"emoji"`
}

// ToggleReaction handles POST /messages/:id/reactions — adds or removes a reaction.
func (h *Handler) ToggleReaction(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "id")
	if messageID == "" {
		httputil.JSONError(w, "message ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	var req ToggleReactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Emoji == "" {
		httputil.JSONError(w, "emoji is required", http.StatusBadRequest)
		return
	}

	if err := h.service.ToggleReaction(r.Context(), messageID, userID, req.Emoji); err != nil {
		if err.Error() == "message not found" {
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
			return
		}
		httputil.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PinMessage handles POST /channels/{channelID}/pins/{messageID}
func (h *Handler) PinMessage(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	if err := h.service.PinMessage(r.Context(), channelID, messageID, userID); err != nil {
		switch err.Error() {
		case "forbidden":
			httputil.JSONError(w, "you do not have permission to pin messages", http.StatusForbidden)
		case "message not found", "channel not found":
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
		default:
			httputil.InternalError(w, err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UnpinMessage handles DELETE /channels/{channelID}/pins/{messageID}
func (h *Handler) UnpinMessage(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	if err := h.service.UnpinMessage(r.Context(), channelID, messageID, userID); err != nil {
		switch err.Error() {
		case "forbidden":
			httputil.JSONError(w, "you do not have permission to unpin messages", http.StatusForbidden)
		case "channel not found":
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
		default:
			httputil.InternalError(w, err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetChannelPins handles GET /channels/{channelID}/pins
func (h *Handler) GetChannelPins(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	msgs, err := h.service.GetChannelPins(r.Context(), channelID, userID)
	if err != nil {
		switch err.Error() {
		case "forbidden", "channel not found":
			httputil.JSONError(w, err.Error(), http.StatusForbidden)
		default:
			httputil.InternalError(w, err)
		}
		return
	}
	if msgs == nil {
		msgs = []*Message{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs)
}

