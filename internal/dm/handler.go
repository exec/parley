package dm

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"parley/internal/auth"
	"parley/internal/db"
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
		jsonError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		jsonError(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	channels, err := h.repo.GetUserDmChannels(r.Context(), userID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
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
		jsonError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	currentUserID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		jsonError(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	var req OpenDmChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.UserID == "" {
		jsonError(w, "user_id is required", http.StatusBadRequest)
		return
	}

	otherUserID, err := strconv.ParseInt(req.UserID, 10, 64)
	if err != nil {
		jsonError(w, "invalid user_id", http.StatusBadRequest)
		return
	}

	if currentUserID == otherUserID {
		jsonError(w, "cannot DM yourself", http.StatusBadRequest)
		return
	}

	channel, err := h.repo.GetOrCreateDmChannel(r.Context(), currentUserID, otherUserID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(channel)
}

// GetDmMessages handles GET /dms/{id}/messages
func (h *Handler) GetDmMessages(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	if userIDStr == "" {
		jsonError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		jsonError(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	dmID := chi.URLParam(r, "id")
	if dmID == "" {
		jsonError(w, "DM channel ID is required", http.StatusBadRequest)
		return
	}

	dmChannelID, err := strconv.ParseInt(dmID, 10, 64)
	if err != nil {
		jsonError(w, "invalid DM channel ID", http.StatusBadRequest)
		return
	}

	// Verify user is part of this DM channel
	channel, err := h.repo.GetDmChannelByID(r.Context(), dmChannelID)
	if err != nil {
		jsonError(w, "DM channel not found", http.StatusNotFound)
		return
	}

	if channel.User1ID != userID && channel.User2ID != userID {
		jsonError(w, "not authorized to view this DM channel", http.StatusForbidden)
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
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	messages, err := h.repo.GetDmMessages(r.Context(), dmChannelID, limit, offset)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
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
	Content string `json:"content"`
}

// SendDmMessage handles POST /dms/{id}/messages
func (h *Handler) SendDmMessage(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	if userIDStr == "" {
		jsonError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	currentUserID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		jsonError(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	dmID := chi.URLParam(r, "id")
	if dmID == "" {
		jsonError(w, "DM channel ID is required", http.StatusBadRequest)
		return
	}

	dmChannelID, err := strconv.ParseInt(dmID, 10, 64)
	if err != nil {
		jsonError(w, "invalid DM channel ID", http.StatusBadRequest)
		return
	}

	// Verify user is part of this DM channel
	channel, err := h.repo.GetDmChannelByID(r.Context(), dmChannelID)
	if err != nil {
		jsonError(w, "DM channel not found", http.StatusNotFound)
		return
	}

	if channel.User1ID != currentUserID && channel.User2ID != currentUserID {
		jsonError(w, "not authorized to send messages in this DM channel", http.StatusForbidden)
		return
	}

	var req SendDmMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		jsonError(w, "content is required", http.StatusBadRequest)
		return
	}

	msg, err := h.repo.CreateDmMessage(r.Context(), dmChannelID, currentUserID, req.Content)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Broadcast to the other user via WebSocket
	if h.hub != nil {
		// Determine the recipient
		var recipientID int64
		if channel.User1ID == currentUserID {
			recipientID = channel.User2ID
		} else {
			recipientID = channel.User1ID
		}

		// Send DM message event to recipient.
		// The hub wraps the payload in WSMessage{type, payload}, so we only
		// marshal the message itself — not an extra event envelope.
		msgJSON, err := json.Marshal(msg)
		if err != nil {
			log.Printf("SendDmMessage: failed to marshal message for broadcast: %v", err)
		} else {
			h.hub.SendToUser(strconv.FormatInt(recipientID, 10), "dm_message", msgJSON)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(msg)
}

func jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"message": message})
}