package message

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"parley/internal/db"
)

// Handler handles HTTP requests for messages
type Handler struct {
	service *MessageService
}

// NewHandler creates a new message handler
func NewHandler(service *MessageService) *Handler {
	return &Handler{service: service}
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
		http.Error(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	// Get author ID from context (set by auth middleware)
	authorID := r.Context().Value("userID")
	if authorID == nil {
		http.Error(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content == "" && req.AttachmentURL == "" {
		http.Error(w, "content or attachment is required", http.StatusBadRequest)
		return
	}

	msg, err := h.service.SendMessage(r.Context(), channelID, authorID.(string), req.Content, req.Nonce, req.AttachmentURL, req.AttachmentName, req.AttachmentType, req.ParentID)
	if err != nil {
		if err.Error() == "forbidden" {
			http.Error(w, "you do not have permission to send messages in this channel", http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
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
		http.Error(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	// Get user ID from context for ViewChannel check.
	userID, _ := r.Context().Value("userID").(string)

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
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "message ID is required", http.StatusBadRequest)
		return
	}

	// Get author ID from context (set by auth middleware)
	authorID := r.Context().Value("userID")
	if authorID == nil {
		http.Error(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	var req EditMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Check if the message exists and belongs to the user
	msg, err := h.service.GetMessage(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if msg.AuthorID != authorID.(string) {
		http.Error(w, "you can only edit your own messages", http.StatusForbidden)
		return
	}

	updatedMsg, err := h.service.EditMessage(r.Context(), id, req.Content)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedMsg)
}

// DeleteMessage handles DELETE /messages/:id
func (h *Handler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "message ID is required", http.StatusBadRequest)
		return
	}

	// Get author ID from context (set by auth middleware)
	authorID := r.Context().Value("userID")
	if authorID == nil {
		http.Error(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	// Check if the user has permission to delete the message (author or MANAGE_MESSAGES)
	canManage, err := h.service.CanManageMessage(r.Context(), id, authorID.(string))
	if err != nil {
		if err.Error() == "message not found" {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if !canManage {
		http.Error(w, "you can only delete your own messages", http.StatusForbidden)
		return
	}

	if err := h.service.DeleteMessage(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetMessageVersions handles GET /messages/:id/versions
func (h *Handler) GetMessageVersions(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "id")
	versions, err := h.service.GetMessageVersions(r.Context(), messageID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "server ID is required", http.StatusBadRequest)
		return
	}
	userID, _ := r.Context().Value("userID").(string)

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
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "message ID is required", http.StatusBadRequest)
		return
	}

	userID := r.Context().Value("userID")
	if userID == nil {
		http.Error(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	var req ToggleReactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Emoji == "" {
		http.Error(w, "emoji is required", http.StatusBadRequest)
		return
	}

	if err := h.service.ToggleReaction(r.Context(), messageID, userID.(string), req.Emoji); err != nil {
		if err.Error() == "message not found" {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

