package message

import (
	"context"
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

	// Apply auth middleware to all routes
	r.Use(AuthMiddleware)

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
			limit = l
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
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

// AuthMiddleware is an authentication middleware that extracts user ID from the request
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		// In a real implementation, you would validate the token and extract user ID
		// For now, we'll look for a X-User-ID header that would be set after token validation
		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			http.Error(w, "user ID not found", http.StatusUnauthorized)
			return
		}

		// Add user ID to context
		ctx := r.Context()
		ctx = context.WithValue(ctx, "userID", userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}