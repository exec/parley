package message

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
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
	Content string `json:"content"`
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

	if req.Content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	msg, err := h.service.SendMessage(r.Context(), channelID, authorID.(string), req.Content)
	if err != nil {
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

	// Parse query parameters
	limit := 50
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			if l > 200 {
				l = 200
			}
			limit = l
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 && o <= 10000 {
			offset = o
		}
	}

	messages, err := h.service.GetChannelMessages(r.Context(), channelID, limit, offset)
	if err != nil {
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

	// Check if the message exists and belongs to the user
	msg, err := h.service.GetMessage(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if msg.AuthorID != authorID.(string) {
		http.Error(w, "you can only delete your own messages", http.StatusForbidden)
		return
	}

	if err := h.service.DeleteMessage(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

