package soundboard

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/httputil"
	"parley/internal/permissions"
	"parley/internal/voice"
	ws "parley/internal/websocket"
)

// Handler handles soundboard HTTP endpoints.
type Handler struct {
	repo     *Repository
	svc      *Service
	dbRepo   *db.Repository
	hub      *ws.Hub
	voiceSvc *voice.Service
}

// NewHandler creates a new Handler.
func NewHandler(repo *Repository, svc *Service, dbRepo *db.Repository, hub *ws.Hub, voiceSvc *voice.Service) *Handler {
	return &Handler{repo: repo, svc: svc, dbRepo: dbRepo, hub: hub, voiceSvc: voiceSvc}
}

// List returns all sounds for a server.
// GET /api/servers/{serverId}/soundboard
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, err := strconv.ParseInt(r.PathValue("serverId"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid server id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	member, err := h.dbRepo.GetMember(ctx, serverID, userID)
	if err != nil || member == nil {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}

	sounds, err := h.repo.ListByServer(ctx, serverID)
	if err != nil {
		log.Printf("soundboard list: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if sounds == nil {
		sounds = []Sound{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sounds)
}

// Upload handles multipart audio upload + metadata.
// POST /api/servers/{serverId}/soundboard
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, err := strconv.ParseInt(r.PathValue("serverId"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid server id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	srv, err := h.dbRepo.GetServerByID(ctx, serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	ok, err := permissions.HasPermission(ctx, h.dbRepo, serverID, userID, srv.OwnerID, permissions.PermManageServer)
	if err != nil || !ok {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}

	// Parse multipart form — limit enforced by http.MaxBytesReader in routes.go.
	if err := r.ParseMultipartForm(MaxFileSizeBytes + 4096); err != nil {
		httputil.JSONError(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}

	name := r.FormValue("name")
	if err := ValidateName(name); err != nil {
		httputil.JSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	emoji := r.FormValue("emoji")
	if err := ValidateEmoji(emoji); err != nil {
		httputil.JSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		httputil.JSONError(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, exceeded, err := ReadAll(file, MaxFileSizeBytes)
	if err != nil {
		httputil.JSONError(w, "failed to read file", http.StatusInternalServerError)
		return
	}
	if exceeded {
		httputil.JSONError(w, "file exceeds 1 MB limit", http.StatusRequestEntityTooLarge)
		return
	}

	result, err := h.svc.ValidateAndUpload(ctx, serverID, data)
	if err != nil {
		httputil.JSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	sound := &Sound{
		ServerID:   serverID,
		UploaderID: userID,
		Name:       name,
		Emoji:      emoji,
		FileURL:    result.FileURL,
		FileKey:    result.FileKey,
	}

	created, err := h.repo.Create(ctx, sound)
	if err != nil {
		// Clean up Spaces object on DB failure (I-9 pattern).
		if delErr := h.svc.DeleteSpacesObject(ctx, result.FileKey); delErr != nil {
			log.Printf("soundboard: cleanup %s after DB failure: %v", result.FileKey, delErr)
		}
		log.Printf("soundboard create: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// UpdateSound updates a sound's name and/or emoji.
// PATCH /api/servers/{serverId}/soundboard/{soundId}
func (h *Handler) UpdateSound(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, err := strconv.ParseInt(r.PathValue("serverId"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid server id", http.StatusBadRequest)
		return
	}
	soundID, err := strconv.ParseInt(r.PathValue("soundId"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid sound id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	srv, err := h.dbRepo.GetServerByID(ctx, serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}
	ok, err := permissions.HasPermission(ctx, h.dbRepo, serverID, userID, srv.OwnerID, permissions.PermManageServer)
	if err != nil || !ok {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}

	// Ensure the sound belongs to this server.
	existing, err := h.repo.GetByID(ctx, soundID)
	if err != nil || existing.ServerID != serverID {
		httputil.JSONError(w, "sound not found", http.StatusNotFound)
		return
	}

	var body struct {
		Name  string `json:"name"`
		Emoji string `json:"emoji"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.JSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	name := body.Name
	if name == "" {
		name = existing.Name // keep existing if not provided
	}
	if err := ValidateName(name); err != nil {
		httputil.JSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	emoji := body.Emoji
	if err := ValidateEmoji(emoji); err != nil {
		httputil.JSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	updated, err := h.repo.Update(ctx, soundID, name, emoji)
	if err != nil {
		log.Printf("soundboard update: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// DeleteSound deletes a sound and its Spaces object.
// DELETE /api/servers/{serverId}/soundboard/{soundId}
func (h *Handler) DeleteSound(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, err := strconv.ParseInt(r.PathValue("serverId"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid server id", http.StatusBadRequest)
		return
	}
	soundID, err := strconv.ParseInt(r.PathValue("soundId"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid sound id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	srv, err := h.dbRepo.GetServerByID(ctx, serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}
	ok, err := permissions.HasPermission(ctx, h.dbRepo, serverID, userID, srv.OwnerID, permissions.PermManageServer)
	if err != nil || !ok {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}

	// Ensure the sound belongs to this server before deleting.
	existing, err := h.repo.GetByID(ctx, soundID)
	if err != nil || existing.ServerID != serverID {
		httputil.JSONError(w, "sound not found", http.StatusNotFound)
		return
	}

	fileKey, err := h.repo.Delete(ctx, soundID)
	if err != nil {
		log.Printf("soundboard delete: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if err := h.svc.DeleteSpacesObject(ctx, fileKey); err != nil {
		log.Printf("soundboard: delete spaces object %s: %v", fileKey, err)
		// Non-fatal: DB record is gone, CDN object will be orphaned but harmless.
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListAll returns sounds from all servers the authenticated user is in.
// GET /api/soundboard
func (h *Handler) ListAll(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sounds, err := h.repo.ListForUser(r.Context(), userID)
	if err != nil {
		log.Printf("soundboard list-all: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if sounds == nil {
		sounds = []SoundWithServer{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sounds)
}

// Play fires a SOUNDBOARD_PLAY WebSocket event into the voice channel.
// POST /api/channels/{channelId}/soundboard/play
func (h *Handler) Play(w http.ResponseWriter, r *http.Request) {
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
	ch, err := h.dbRepo.GetChannelByID(ctx, channelID)
	if err != nil {
		httputil.JSONError(w, "channel not found", http.StatusNotFound)
		return
	}

	srv, err := h.dbRepo.GetServerByID(ctx, ch.ServerID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}

	ok, err := permissions.HasChannelPermission(ctx, h.dbRepo, ch.ServerID, userID, srv.OwnerID, channelID, permissions.PermUseSoundboard)
	if err != nil || !ok {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}

	// Verify the user is actually in the voice channel.
	inChannel, err := h.voiceSvc.IsParticipant(ctx, channelIDStr, userIDStr)
	if err != nil || !inChannel {
		httputil.JSONError(w, "you must be in the voice channel to play sounds", http.StatusForbidden)
		return
	}

	var body struct {
		SoundID    string `json:"sound_id"`
		SoundName  string `json:"sound_name"`
		Emoji      string `json:"emoji"`
		DurationMS int64  `json:"duration_ms"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		httputil.JSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Cap duration_ms at 60s.
	if body.DurationMS > 60_000 {
		body.DurationMS = 60_000
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"channel_id":  channelIDStr,
		"user_id":     userIDStr,
		"sound_id":    body.SoundID,
		"sound_name":  body.SoundName,
		"emoji":       body.Emoji,
		"duration_ms": body.DurationMS,
	})

	serverVirtualChannelID := "server:" + strconv.FormatInt(ch.ServerID, 10)
	h.hub.BroadcastToChannel(serverVirtualChannelID, ws.EventSoundboardPlay, payload)

	w.WriteHeader(http.StatusNoContent)
}
