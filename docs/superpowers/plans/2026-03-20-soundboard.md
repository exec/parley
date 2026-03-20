# Soundboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-server soundboards — privileged users upload audio files, any member can play them into a voice channel (heard by everyone via Web Audio API mixing into their LiveKit mic track), with cross-server access grouped by server in the VC panel.

**Architecture:** New `internal/soundboard` Go package with handler/service/repository backed by a `soundboard_sounds` DB table. The upload handler does magic-byte validation and calls `spaces.Client.Upload()` directly. Playback is client-side: Web Audio API merges the sound with the mic stream and calls `LocalAudioTrack.replaceTrack()` to swap the published mic track. A `SOUNDBOARD_PLAY` WebSocket event carries `duration_ms` so all clients know when to clear the emoji indicator on the playing user's tile.

**Tech Stack:** Go + Chi, PostgreSQL, DigitalOcean Spaces, React/TypeScript, Web Audio API, LiveKit client SDK 2.17.x, existing WebSocket hub.

**Spec:** `docs/superpowers/specs/2026-03-20-soundboard-design.md`

---

## File Map

**Create:**
- `internal/soundboard/models.go` — `Sound` and `SoundWithServer` structs
- `internal/soundboard/repository.go` — DB queries
- `internal/soundboard/service.go` — validation, magic bytes, Spaces upload/delete, count enforcement
- `internal/soundboard/handler.go` — 6 HTTP endpoints
- `frontend/src/api/soundboard.ts` — API client functions
- `frontend/src/components/settings/SoundboardTab.tsx` — server settings tab
- `frontend/src/components/settings/SoundboardTab.css` — styles
- `frontend/src/components/voice/SoundboardPanel.tsx` — VC popover + Web Audio playback

**Modify:**
- `internal/db/migrations.go` — add `soundboard_sounds` migration
- `internal/voice/service.go` — add `IsParticipant` method
- `internal/websocket/events.go` — add `EventSoundboardPlay` constant
- `cmd/api/routes.go` — exclude soundboard POST from 64KB body limit, register soundboard routes
- `frontend/src/components/settings/ServerSettings.tsx` — add `soundboard` tab
- `frontend/src/components/voice/VoiceChannel.tsx` — add soundboard button + panel, pass `activeSoundEmojis`
- `frontend/src/components/voice/ParticipantTile.tsx` — add `activeSoundEmoji` prop + indicator
- `frontend/src/components/voice/ParticipantTile.css` — emoji indicator styles

---

## Task 1: DB Migration

**Files:**
- Modify: `internal/db/migrations.go`

- [ ] **Step 1: Open `internal/db/migrations.go` and append a new migration string to the `Migrations` slice.**

The last entry in the slice ends with a closing backtick. Append after it:

```go
,
`-- Soundboard sounds
CREATE TABLE IF NOT EXISTS soundboard_sounds (
    id          BIGSERIAL PRIMARY KEY,
    server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    uploader_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    name        VARCHAR(32) NOT NULL,
    emoji       VARCHAR(64),
    file_url    TEXT NOT NULL,
    file_key    TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_soundboard_sounds_server ON soundboard_sounds(server_id);`,
```

- [ ] **Step 2: Verify the Go file compiles.**

```bash
cd /home/dylan/Developer/parley && go build ./internal/db/...
```
Expected: no output (success).

- [ ] **Step 3: Commit.**

```bash
git add internal/db/migrations.go
git commit -m "feat: add soundboard_sounds migration"
```

---

## Task 2: Go Models and Repository

**Files:**
- Create: `internal/soundboard/models.go`
- Create: `internal/soundboard/repository.go`

- [ ] **Step 1: Create `internal/soundboard/models.go`.**

```go
package soundboard

import "time"

// Sound is a single soundboard entry.
type Sound struct {
	ID         int64     `json:"id,string"`
	ServerID   int64     `json:"server_id,string"`
	UploaderID int64     `json:"uploader_id,string"`
	Name       string    `json:"name"`
	Emoji      string    `json:"emoji,omitempty"`
	FileURL    string    `json:"file_url"`
	FileKey    string    `json:"-"` // never expose the storage key to clients
	CreatedAt  time.Time `json:"created_at"`
}

// SoundWithServer adds the server name for cross-server listing.
type SoundWithServer struct {
	Sound
	ServerName string `json:"server_name"`
}
```

- [ ] **Step 2: Create `internal/soundboard/repository.go`.**

```go
package soundboard

import (
	"context"
	"database/sql"
)

// Repository handles all soundboard DB queries.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new Repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// CountByServer returns the number of sounds for a server.
func (r *Repository) CountByServer(ctx context.Context, serverID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM soundboard_sounds WHERE server_id = $1`, serverID,
	).Scan(&n)
	return n, err
}

// Create inserts a new sound and returns it with its generated ID and created_at.
func (r *Repository) Create(ctx context.Context, s *Sound) (*Sound, error) {
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO soundboard_sounds (server_id, uploader_id, name, emoji, file_url, file_key)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at`,
		s.ServerID, s.UploaderID, s.Name, s.Emoji, s.FileURL, s.FileKey,
	).Scan(&s.ID, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// ListByServer returns all sounds for a server ordered by name.
func (r *Repository) ListByServer(ctx context.Context, serverID int64) ([]Sound, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, server_id, uploader_id, name, emoji, file_url, file_key, created_at
		 FROM soundboard_sounds WHERE server_id = $1 ORDER BY name`,
		serverID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Sound
	for rows.Next() {
		var s Sound
		if err := rows.Scan(&s.ID, &s.ServerID, &s.UploaderID, &s.Name, &s.Emoji,
			&s.FileURL, &s.FileKey, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetByID returns a single sound by ID.
func (r *Repository) GetByID(ctx context.Context, id int64) (*Sound, error) {
	var s Sound
	err := r.db.QueryRowContext(ctx,
		`SELECT id, server_id, uploader_id, name, emoji, file_url, file_key, created_at
		 FROM soundboard_sounds WHERE id = $1`, id,
	).Scan(&s.ID, &s.ServerID, &s.UploaderID, &s.Name, &s.Emoji,
		&s.FileURL, &s.FileKey, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Update updates a sound's name and/or emoji.
func (r *Repository) Update(ctx context.Context, id int64, name, emoji string) (*Sound, error) {
	var s Sound
	err := r.db.QueryRowContext(ctx,
		`UPDATE soundboard_sounds SET name=$1, emoji=$2
		 WHERE id=$3
		 RETURNING id, server_id, uploader_id, name, emoji, file_url, file_key, created_at`,
		name, emoji, id,
	).Scan(&s.ID, &s.ServerID, &s.UploaderID, &s.Name, &s.Emoji,
		&s.FileURL, &s.FileKey, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Delete removes a sound by ID and returns the file_key for Spaces cleanup.
func (r *Repository) Delete(ctx context.Context, id int64) (fileKey string, err error) {
	err = r.db.QueryRowContext(ctx,
		`DELETE FROM soundboard_sounds WHERE id=$1 RETURNING file_key`, id,
	).Scan(&fileKey)
	return fileKey, err
}

// ListForUser returns all sounds from servers the user is a member of,
// joined with server name, ordered by server name then sound name.
func (r *Repository) ListForUser(ctx context.Context, userID int64) ([]SoundWithServer, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT ss.id, ss.server_id, ss.uploader_id, ss.name, ss.emoji,
		        ss.file_url, ss.file_key, ss.created_at, s.name AS server_name
		 FROM soundboard_sounds ss
		 JOIN server_members sm ON sm.server_id = ss.server_id AND sm.user_id = $1
		 JOIN servers s ON s.id = ss.server_id
		 ORDER BY s.name, ss.name`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SoundWithServer
	for rows.Next() {
		var s SoundWithServer
		if err := rows.Scan(&s.ID, &s.ServerID, &s.UploaderID, &s.Name, &s.Emoji,
			&s.FileURL, &s.FileKey, &s.CreatedAt, &s.ServerName); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
```

- [ ] **Step 3: Build to verify.**

```bash
cd /home/dylan/Developer/parley && go build ./internal/soundboard/...
```
Expected: no output.

- [ ] **Step 4: Commit.**

```bash
git add internal/soundboard/
git commit -m "feat: soundboard models and repository"
```

---

## Task 3: Go Service

**Files:**
- Create: `internal/soundboard/service.go`

The service owns: magic-byte validation (audio only), size limit, count limit, Spaces upload/delete with cleanup-on-failure, ID generation.

- [ ] **Step 1: Create `internal/soundboard/service.go`.**

```go
package soundboard

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"parley/internal/spaces"
)

const (
	MaxSoundsPerServer = 48
	MaxFileSizeBytes   = 1 << 20 // 1 MB
)

// audioExt inspects the first 12 bytes and returns the file extension if the
// data is an audio format accepted by the soundboard (mp3, ogg, wav).
// Returns ("", false) for all other types.
func audioExt(data []byte) (string, bool) {
	if len(data) < 12 {
		return "", false
	}
	switch {
	// OGG (ogg/vorbis, ogg/opus)
	case data[0] == 0x4F && data[1] == 0x67 && data[2] == 0x67 && data[3] == 0x53:
		return ".ogg", true
	// MP3: ID3v2 tag
	case data[0] == 0x49 && data[1] == 0x44 && data[2] == 0x33:
		return ".mp3", true
	// MP3: raw MPEG frame sync (no ID3)
	case data[0] == 0xFF && (data[1]&0xE0 == 0xE0) && (data[1]&0x18 != 0x08) && (data[1]&0x06 != 0x00):
		return ".mp3", true
	// WAV: RIFF....WAVE
	case data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x41 && data[10] == 0x56 && data[11] == 0x45:
		return ".wav", true
	}
	return "", false
}

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("soundboard: generateID: crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}

// Service handles soundboard business logic.
type Service struct {
	repo   *Repository
	spaces *spaces.Client
}

// NewService creates a new Service.
func NewService(repo *Repository, sc *spaces.Client) *Service {
	return &Service{repo: repo, spaces: sc}
}

// UploadResult is returned from UploadSound on success.
type UploadResult struct {
	FileURL string
	FileKey string
	Ext     string
}

// ValidateAndUpload validates the audio bytes (size + magic bytes), checks the
// per-server count limit, uploads to Spaces, and returns the CDN URL and key.
// On DB insert failure after upload, callers must delete the Spaces object using
// the returned FileKey — this function does not do that cleanup itself.
func (s *Service) ValidateAndUpload(ctx context.Context, serverID int64, data []byte) (*UploadResult, error) {
	if int64(len(data)) > MaxFileSizeBytes {
		return nil, errors.New("file exceeds 1 MB limit")
	}
	ext, ok := audioExt(data)
	if !ok {
		return nil, errors.New("only MP3, OGG, and WAV files are accepted")
	}

	count, err := s.repo.CountByServer(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("count check: %w", err)
	}
	if count >= MaxSoundsPerServer {
		return nil, fmt.Errorf("server has reached the %d sound limit", MaxSoundsPerServer)
	}

	key := fmt.Sprintf("soundboard/%d/%s%s", serverID, generateID(), ext)
	url, err := s.spaces.Upload(ctx, key, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}
	return &UploadResult{FileURL: url, FileKey: key, Ext: ext}, nil
}

// DeleteSpacesObject removes a file from Spaces. Used for cleanup on DB failure.
func (s *Service) DeleteSpacesObject(ctx context.Context, key string) error {
	return s.spaces.Delete(ctx, key)
}

// ValidateName returns an error if name is empty or longer than 32 chars.
func ValidateName(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if len([]rune(name)) > 32 {
		return errors.New("name must be 32 characters or fewer")
	}
	return nil
}

// ValidateEmoji returns an error if emoji exceeds 64 chars.
func ValidateEmoji(emoji string) error {
	if len([]rune(emoji)) > 64 {
		return errors.New("emoji must be 64 characters or fewer")
	}
	return nil
}

// ReadAll reads up to limit+1 bytes from r. Returns the data and whether it
// exceeded the limit. Used to enforce file size before loading into memory.
func ReadAll(r io.Reader, limit int64) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > limit {
		return nil, true, nil
	}
	return data, false, nil
}
```

- [ ] **Step 2: Build.**

```bash
cd /home/dylan/Developer/parley && go build ./internal/soundboard/...
```
Expected: no output.

- [ ] **Step 3: Write tests for `audioExt` and `ValidateName`.**

Create `internal/soundboard/service_test.go`:

```go
package soundboard

import (
	"strings"
	"testing"
)

func TestAudioExt(t *testing.T) {
	// OGG magic bytes
	ogg := []byte{0x4F, 0x67, 0x67, 0x53, 0, 0, 0, 0, 0, 0, 0, 0}
	if ext, ok := audioExt(ogg); !ok || ext != ".ogg" {
		t.Errorf("ogg: got (%q, %v)", ext, ok)
	}

	// MP3 ID3v2
	mp3 := []byte{0x49, 0x44, 0x33, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if ext, ok := audioExt(mp3); !ok || ext != ".mp3" {
		t.Errorf("mp3 id3: got (%q, %v)", ext, ok)
	}

	// WAV RIFF....WAVE
	wav := []byte{0x52, 0x49, 0x46, 0x46, 0, 0, 0, 0, 0x57, 0x41, 0x56, 0x45}
	if ext, ok := audioExt(wav); !ok || ext != ".wav" {
		t.Errorf("wav: got (%q, %v)", ext, ok)
	}

	// PNG (not an audio file)
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}
	if _, ok := audioExt(png); ok {
		t.Error("png should not be accepted")
	}

	// Too short
	if _, ok := audioExt([]byte{0x4F, 0x67}); ok {
		t.Error("too-short slice should not be accepted")
	}
}

func TestValidateName(t *testing.T) {
	if err := ValidateName(""); err == nil {
		t.Error("empty name should fail")
	}
	if err := ValidateName("airhorn"); err != nil {
		t.Errorf("valid name: %v", err)
	}
	if err := ValidateName(strings.Repeat("x", 33)); err == nil {
		t.Error("33-char name should fail")
	}
	if err := ValidateName(strings.Repeat("x", 32)); err != nil {
		t.Errorf("32-char name should pass: %v", err)
	}
}
```

- [ ] **Step 4: Run tests.**

```bash
cd /home/dylan/Developer/parley && go test ./internal/soundboard/... -v
```
Expected: `PASS` for both tests.

- [ ] **Step 5: Commit.**

```bash
git add internal/soundboard/
git commit -m "feat: soundboard service with audio validation"
```

---

## Task 4: Go Handler

**Files:**
- Create: `internal/soundboard/handler.go`

The handler wires the repository + service into HTTP endpoints. Pattern: parse auth → validate permissions → call service/repo → respond.

- [ ] **Step 1: Create `internal/soundboard/handler.go`.**

```go
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
	ws "parley/internal/websocket"
	"parley/internal/voice"
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
```

- [ ] **Step 2: Add `IsParticipant` to `internal/voice/service.go`.**

The handler calls `h.voiceSvc.IsParticipant(...)` — add this method now so the package compiles. Add after the `Participants` method:

```go
// IsParticipant returns true if the user is currently in the voice channel.
func (s *Service) IsParticipant(ctx context.Context, channelID, userID string) (bool, error) {
	if s.rdb == nil {
		return false, nil
	}
	exists, err := s.rdb.HExists(ctx, presenceKey(channelID), userID).Result()
	return exists, err
}
```

- [ ] **Step 3: Add the WS event constant to `internal/websocket/events.go`.**

Add after the Bin events block:

```go
// Soundboard events
EventSoundboardPlay = "SOUNDBOARD_PLAY"
```

- [ ] **Step 4: Build and commit.**

```bash
cd /home/dylan/Developer/parley && go build ./internal/...
git add internal/soundboard/ internal/voice/service.go internal/websocket/events.go
git commit -m "feat: soundboard handler, IsParticipant, WS event constant"
```
Expected: build produces no output; commit succeeds.

---

## Task 5: Wire Routes in `cmd/api/routes.go`

**Files:**
- Modify: `cmd/api/routes.go`

- [ ] **Step 1: Update the 64KB body limit middleware in `cmd/api/routes.go`.**

Find this block near the top of `registerRoutes`:

```go
router.Use(func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/api/upload" && r.URL.Path != "/api/me/themes/generate" {
            r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
        }
        next.ServeHTTP(w, r)
    })
})
```

Replace with:

```go
router.Use(func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        isSoundboardUpload := r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/soundboard")
        if r.URL.Path != "/api/upload" && r.URL.Path != "/api/me/themes/generate" && !isSoundboardUpload {
            r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
        }
        next.ServeHTTP(w, r)
    })
})
```

Also add `"strings"` to the import block at the top of `routes.go` if it's not already there.

- [ ] **Step 2: Register soundboard routes in `cmd/api/routes.go`.**

Add the import at the top of `routes.go`:

```go
"parley/internal/soundboard"
```

Inside the `registerRoutes` function, add to the `registerRoutes` signature — no, actually construct the handler inline inside the protected routes group. Find the section where other handlers are constructed and add after the voice handler lines (around where `voiceHandler := voice.NewHandler(...)` is):

```go
// Soundboard handler
soundboardRepo := soundboard.NewRepository(repo.DB())
soundboardSvc := soundboard.NewService(soundboardRepo, spacesClient)
soundboardHandler := soundboard.NewHandler(soundboardRepo, soundboardSvc, repo, hub, voiceSvc)
```

Then add routes inside the authenticated `r.Group` block, near the voice routes:

```go
// Soundboard routes
r.Get("/soundboard", soundboardHandler.ListAll)
r.Get("/servers/{serverId}/soundboard", soundboardHandler.List)
r.Post("/servers/{serverId}/soundboard", soundboardHandler.Upload)
r.Patch("/servers/{serverId}/soundboard/{soundId}", soundboardHandler.UpdateSound)
r.Delete("/servers/{serverId}/soundboard/{soundId}", soundboardHandler.DeleteSound)
r.Post("/channels/{channelId}/soundboard/play", soundboardHandler.Play)
```

Note: you need to find out where `repo.DB()` is called or available. The `db.Repository` has a `DB()` method — check `internal/db/repository.go` for it. If it doesn't exist, add it: `func (r *Repository) DB() *sql.DB { return r.db }`.

- [ ] **Step 3: Build the full binary.**

```bash
cd /home/dylan/Developer/parley && go build ./cmd/api/...
```
Expected: no output.

- [ ] **Step 4: Commit.**

```bash
git add cmd/api/routes.go internal/db/repository.go
git commit -m "feat: wire soundboard routes"
```

---

## Task 6: Frontend API Client

**Files:**
- Create: `frontend/src/api/soundboard.ts`

- [ ] **Step 1: Create `frontend/src/api/soundboard.ts`.**

```typescript
import { apiClient } from './client';

export interface Sound {
  id: string;
  server_id: string;
  uploader_id: string;
  name: string;
  emoji?: string;
  file_url: string;
  created_at: string;
}

export interface SoundWithServer extends Sound {
  server_name: string;
}

export async function listServerSounds(serverId: string): Promise<Sound[]> {
  return apiClient.get<Sound[]>(`/servers/${serverId}/soundboard`);
}

export async function listMySounds(): Promise<SoundWithServer[]> {
  return apiClient.get<SoundWithServer[]>('/soundboard');
}

export async function uploadSound(
  serverId: string,
  file: File,
  name: string,
  emoji: string,
): Promise<Sound> {
  const formData = new FormData();
  formData.append('file', file);
  formData.append('name', name);
  formData.append('emoji', emoji);

  const token = localStorage.getItem('token');
  const response = await fetch(`/api/servers/${serverId}/soundboard`, {
    method: 'POST',
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    body: formData,
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Upload failed: ${response.statusText}`);
  }
  return response.json();
}

export async function updateSound(
  serverId: string,
  soundId: string,
  name: string,
  emoji: string,
): Promise<Sound> {
  return apiClient.patch<Sound>(`/servers/${serverId}/soundboard/${soundId}`, { name, emoji });
}

export async function deleteSound(serverId: string, soundId: string): Promise<void> {
  return apiClient.delete<void>(`/servers/${serverId}/soundboard/${soundId}`);
}

export async function playSoundEvent(
  channelId: string,
  soundId: string,
  soundName: string,
  emoji: string,
  durationMs: number,
): Promise<void> {
  return apiClient.post<void>(`/channels/${channelId}/soundboard/play`, {
    sound_id: soundId,
    sound_name: soundName,
    emoji,
    duration_ms: durationMs,
  });
}
```

- [ ] **Step 2: Check what methods `apiClient` exposes.**

Open `frontend/src/api/client.ts` and verify it has `get`, `post`, `patch`, `delete` methods. If `patch` or `delete` are missing, add them following the same pattern as `post`.

- [ ] **Step 3: Build the frontend to check for TypeScript errors.**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | head -30
```
Expected: no TypeScript errors in `soundboard.ts`.

- [ ] **Step 4: Commit.**

```bash
git add frontend/src/api/soundboard.ts frontend/src/api/client.ts
git commit -m "feat: soundboard API client"
```

---

## Task 7: SoundboardTab Component (Server Settings)

**Files:**
- Create: `frontend/src/components/settings/SoundboardTab.tsx`
- Create: `frontend/src/components/settings/SoundboardTab.css`

- [ ] **Step 1: Create `frontend/src/components/settings/SoundboardTab.css`.**

```css
.soundboard-tab {}

.soundboard-count {
  font-size: 12px;
  color: var(--parley-text-muted);
  margin-bottom: 16px;
}

.soundboard-upload-form {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px;
  background: var(--parley-bg-secondary);
  border: 1px solid var(--parley-border);
  border-radius: 6px;
  margin-bottom: 20px;
}

.soundboard-upload-row {
  display: flex;
  gap: 8px;
  align-items: flex-end;
}

.soundboard-upload-row .settings-form-input {
  flex: 1;
}

.soundboard-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 8px;
}

.soundboard-card {
  background: var(--parley-bg-secondary);
  border: 1px solid var(--parley-border);
  border-radius: 6px;
  padding: 10px 12px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.soundboard-card-emoji {
  font-size: 20px;
  width: 28px;
  text-align: center;
  flex-shrink: 0;
}

.soundboard-card-info {
  flex: 1;
  min-width: 0;
}

.soundboard-card-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--parley-text-normal);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.soundboard-card-uploader {
  font-size: 11px;
  color: var(--parley-text-muted);
}

.soundboard-card-delete {
  background: none;
  border: none;
  color: var(--parley-text-muted);
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 3px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
}

.soundboard-card-delete:hover {
  color: var(--parley-danger);
  background: rgba(var(--parley-danger-rgb), 0.1);
}

.soundboard-empty {
  color: var(--parley-text-muted);
  font-size: 13px;
  padding: 24px 0;
  text-align: center;
}
```

- [ ] **Step 2: Create `frontend/src/components/settings/SoundboardTab.tsx`.**

```tsx
import React, { useEffect, useRef, useState } from 'react';
import { Trash2 } from 'lucide-react';
import { Sound, listServerSounds, uploadSound, deleteSound } from '../../api/soundboard';
import './SoundboardTab.css';

interface Props {
  serverId: string;
}

const MAX_SOUNDS = 48;
const MAX_FILE_BYTES = 1 * 1024 * 1024;

export const SoundboardTab: React.FC<Props> = ({ serverId }) => {
  const [sounds, setSounds] = useState<Sound[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  // Upload form state
  const [uploadName, setUploadName] = useState('');
  const [uploadEmoji, setUploadEmoji] = useState('');
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState('');
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    listServerSounds(serverId)
      .then(setSounds)
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, [serverId]);

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0] ?? null;
    if (f && f.size > MAX_FILE_BYTES) {
      setUploadError('File exceeds 1 MB limit');
      setUploadFile(null);
      return;
    }
    setUploadFile(f);
    setUploadError('');
    if (f && !uploadName) {
      // Auto-fill name from filename (strip extension)
      setUploadName(f.name.replace(/\.[^.]+$/, '').slice(0, 32));
    }
  };

  const handleUpload = async () => {
    if (!uploadFile || !uploadName.trim()) return;
    setUploading(true);
    setUploadError('');
    try {
      const sound = await uploadSound(serverId, uploadFile, uploadName.trim(), uploadEmoji.trim());
      setSounds(prev => [...prev, sound]);
      setUploadName('');
      setUploadEmoji('');
      setUploadFile(null);
      if (fileInputRef.current) fileInputRef.current.value = '';
    } catch (e) {
      setUploadError(e instanceof Error ? e.message : 'Upload failed');
    } finally {
      setUploading(false);
    }
  };

  const handleDelete = async (sound: Sound) => {
    if (!confirm(`Delete "${sound.name}"?`)) return;
    try {
      await deleteSound(serverId, sound.id);
      setSounds(prev => prev.filter(s => s.id !== sound.id));
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Delete failed');
    }
  };

  if (loading) return <div style={{ color: 'var(--parley-text-muted)', fontSize: 13 }}>Loading...</div>;

  return (
    <div className="soundboard-tab">
      <h2 className="settings-page-title">Soundboard</h2>

      {error && <div className="settings-error">{error}</div>}

      <div className="soundboard-count">{sounds.length} / {MAX_SOUNDS} sounds</div>

      {/* Upload form */}
      {sounds.length < MAX_SOUNDS && (
        <div className="soundboard-upload-form">
          <div className="settings-section-title">Add Sound</div>
          {uploadError && <div className="settings-error" style={{ marginBottom: 0 }}>{uploadError}</div>}
          <div className="soundboard-upload-row">
            <input
              className="settings-form-input"
              type="text"
              placeholder="Name (required)"
              value={uploadName}
              maxLength={32}
              onChange={e => setUploadName(e.target.value)}
            />
            <input
              className="settings-form-input"
              type="text"
              placeholder="Emoji (optional)"
              value={uploadEmoji}
              style={{ width: 100 }}
              onChange={e => setUploadEmoji(e.target.value)}
            />
          </div>
          <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <input
              ref={fileInputRef}
              type="file"
              accept=".mp3,.ogg,.wav,audio/mpeg,audio/ogg,audio/wav"
              style={{ flex: 1, fontSize: 13 }}
              onChange={handleFileChange}
            />
            <button
              className="settings-btn settings-btn-primary"
              onClick={handleUpload}
              disabled={uploading || !uploadFile || !uploadName.trim()}
            >
              {uploading ? 'Uploading...' : 'Upload'}
            </button>
          </div>
        </div>
      )}

      {/* Sound list */}
      {sounds.length === 0 ? (
        <div className="soundboard-empty">No sounds yet — upload one above.</div>
      ) : (
        <div className="soundboard-grid">
          {sounds.map(sound => (
            <div key={sound.id} className="soundboard-card">
              <div className="soundboard-card-emoji">{sound.emoji || '🔊'}</div>
              <div className="soundboard-card-info">
                <div className="soundboard-card-name">{sound.name}</div>
              </div>
              <button
                className="soundboard-card-delete"
                title="Delete sound"
                onClick={() => handleDelete(sound)}
              >
                <Trash2 size={14} />
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
```

- [ ] **Step 3: Build frontend to verify no TypeScript errors.**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | head -30
```
Expected: no errors in the new files.

- [ ] **Step 4: Commit.**

```bash
git add frontend/src/components/settings/SoundboardTab.tsx frontend/src/components/settings/SoundboardTab.css
git commit -m "feat: SoundboardTab component for server settings"
```

---

## Task 8: Wire SoundboardTab into ServerSettings

**Files:**
- Modify: `frontend/src/components/settings/ServerSettings.tsx`

- [ ] **Step 1: Add the import at the top of `ServerSettings.tsx`.**

After the existing imports, add:

```tsx
import { SoundboardTab } from './SoundboardTab';
```

- [ ] **Step 2: Add `'soundboard'` to the `Tab` type.**

Find:

```tsx
type Tab = 'overview' | 'roles' | 'invites' | 'members' | 'bots' | 'danger';
```

Replace with:

```tsx
type Tab = 'overview' | 'roles' | 'invites' | 'members' | 'bots' | 'soundboard' | 'danger';
```

- [ ] **Step 3: Add the Soundboard nav item in the sidebar.**

In the sidebar JSX, the `hasPerm(myPerms, PERM_MANAGE_SERVER)` check needs to be added. Find this block in the JSX (near the bots nav item):

```tsx
<button className={`settings-nav-item${activeTab === 'bots' ? ' active' : ''}`} onClick={() => setActiveTab('bots')}>
  Bots
</button>
```

Add after it (the tab is only shown to users with Manage Server):

```tsx
{hasPerm(myPerms, PERM_MANAGE_SERVER) && (
  <button className={`settings-nav-item${activeTab === 'soundboard' ? ' active' : ''}`} onClick={() => setActiveTab('soundboard')}>
    Soundboard
  </button>
)}
```

- [ ] **Step 4: Load `myPerms` when the soundboard tab opens.**

The `myPerms` state is loaded via `loadMyPerms()` when the roles or bots tab opens. Add soundboard to that effect:

Find:

```tsx
useEffect(() => {
  if (isOpen && activeTab === 'bots' && server) {
    loadMyPerms();
  }
}, [isOpen, activeTab, server]); // eslint-disable-line react-hooks/exhaustive-deps
```

Replace with:

```tsx
useEffect(() => {
  if (isOpen && (activeTab === 'bots' || activeTab === 'soundboard') && server) {
    loadMyPerms();
  }
}, [isOpen, activeTab, server]); // eslint-disable-line react-hooks/exhaustive-deps
```

- [ ] **Step 5: Render the SoundboardTab in the content area.**

Find the bots tab render:

```tsx
{activeTab === 'bots' && (
  <BotsTab
    ...
  />
)}
```

Add after it:

```tsx
{activeTab === 'soundboard' && server && (
  <SoundboardTab serverId={server.id} />
)}
```

- [ ] **Step 6: Add `PERM_MANAGE_SERVER` to the permissions import if not already there.**

Look at the top of `ServerSettings.tsx` for the permissions import:

```tsx
import {
  PERMISSION_CATEGORIES,
  PERM_ALL,
  PERM_ADMINISTRATOR,
  hasPerm,
  permFromNumber,
  permToNumber,
} from '../../lib/permissions';
```

Check `frontend/src/lib/permissions.ts` — `PERM_MANAGE_SERVER` should be defined there as `1n << 1n`. If it exists, add it to the import. If it doesn't exist yet, add it to `permissions.ts` following the existing pattern.

- [ ] **Step 7: Build and verify.**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | head -30
```
Expected: no errors.

- [ ] **Step 8: Commit.**

```bash
git add frontend/src/components/settings/ServerSettings.tsx frontend/src/lib/permissions.ts
git commit -m "feat: add Soundboard tab to server settings"
```

---

## Task 9: SoundboardPanel Component (VC Popover + Web Audio Playback)

**Files:**
- Create: `frontend/src/components/voice/SoundboardPanel.tsx`

This is the most complex component. It:
1. Fetches sounds from all user's servers
2. Groups them by server
3. Previews sounds locally via `AudioContext.destination`
4. Broadcasts sounds via Web Audio merge → LiveKit `LocalAudioTrack.replaceTrack()`

- [ ] **Step 1: Create `frontend/src/components/voice/SoundboardPanel.tsx`.**

```tsx
import React, { useEffect, useRef, useState } from 'react';
import { Headphones, Volume2, Square } from 'lucide-react';
import { LocalParticipant, Track } from 'livekit-client';
import { SoundWithServer, listMySounds, playSoundEvent } from '../../api/soundboard';
import './SoundboardPanel.css';

interface Props {
  channelId: string;
  localParticipant: LocalParticipant | null;
  muted: boolean;
  onClose: () => void;
}

interface PlayingState {
  soundId: string;
  mode: 'preview' | 'broadcast';
  stop: () => void;
}

export const SoundboardPanel: React.FC<Props> = ({ channelId, localParticipant, muted, onClose }) => {
  const [sounds, setSounds] = useState<SoundWithServer[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [playing, setPlaying] = useState<PlayingState | null>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    listMySounds()
      .then(setSounds)
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  // Close on outside click
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        onClose();
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [onClose]);

  // Close on Escape
  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [onClose]);

  const stopCurrent = () => {
    if (playing) {
      playing.stop();
      setPlaying(null);
    }
  };

  const decodeSound = async (url: string): Promise<{ buffer: AudioBuffer; ctx: AudioContext }> => {
    const ctx = new AudioContext();
    const response = await fetch(url);
    const arrayBuffer = await response.arrayBuffer();
    const buffer = await ctx.decodeAudioData(arrayBuffer);
    return { buffer, ctx };
  };

  const handlePreview = async (sound: SoundWithServer) => {
    if (playing?.soundId === sound.id) { stopCurrent(); return; }
    stopCurrent();

    try {
      const { buffer, ctx } = await decodeSound(sound.file_url);
      const src = ctx.createBufferSource();
      src.buffer = buffer;
      src.connect(ctx.destination);

      const stop = () => {
        try { src.stop(); } catch { /* already stopped */ }
        ctx.close();
        setPlaying(null);
      };

      src.onended = () => { ctx.close(); setPlaying(null); };
      src.start();

      setPlaying({ soundId: sound.id, mode: 'preview', stop });
    } catch (e) {
      setError('Preview failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  };

  const handleBroadcast = async (sound: SoundWithServer) => {
    if (!localParticipant) return;
    if (playing?.soundId === sound.id && playing.mode === 'broadcast') { stopCurrent(); return; }
    stopCurrent();

    // Find the published mic track publication
    const micPub = Array.from(localParticipant.trackPublications.values())
      .find(p => p.source === Track.Source.Microphone);

    if (!micPub?.track) {
      setError('No microphone track found — are you connected to voice?');
      return;
    }

    const localAudioTrack = micPub.track as any; // LocalAudioTrack
    const originalMediaTrack: MediaStreamTrack = localAudioTrack.mediaStreamTrack;
    const wasMuted = micPub.isMuted;

    try {
      const { buffer, ctx } = await decodeSound(sound.file_url);
      const dest = ctx.createMediaStreamDestination();

      // Sound path
      const src = ctx.createBufferSource();
      src.buffer = buffer;
      const soundGain = ctx.createGain();
      soundGain.gain.value = 1.0;
      src.connect(soundGain);
      soundGain.connect(dest);

      // Mic path (gain = 0 if user is muted — voice won't leak)
      const micStream = new MediaStream([originalMediaTrack]);
      const micSource = ctx.createMediaStreamSource(micStream);
      const micGain = ctx.createGain();
      micGain.gain.value = muted ? 0 : 1.0;
      micSource.connect(micGain);
      micGain.connect(dest);

      const mergedTrack = dest.stream.getAudioTracks()[0];

      // If muted, temporarily unmute the LiveKit track so audio goes through
      if (wasMuted) {
        await localAudioTrack.unmute();
      }

      // Replace the underlying MediaStreamTrack with the merged one
      await localAudioTrack.replaceTrack(mergedTrack, false);

      // Fire SOUNDBOARD_PLAY WebSocket event
      const durationMs = Math.round(buffer.duration * 1000);
      playSoundEvent(channelId, sound.id, sound.name, sound.emoji ?? '', durationMs).catch(() => {});

      const restore = async () => {
        try { src.stop(); } catch { /* already stopped */ }
        // Restore original mic track
        await localAudioTrack.replaceTrack(originalMediaTrack, false);
        ctx.close();
        // Re-mute if the user was muted before
        if (wasMuted) {
          await localAudioTrack.mute();
        }
        setPlaying(null);
      };

      src.onended = restore;
      src.start();

      setPlaying({ soundId: sound.id, mode: 'broadcast', stop: restore });
    } catch (e) {
      setError('Playback failed: ' + (e instanceof Error ? e.message : String(e)));
      // Best-effort restore
      try {
        await localAudioTrack.replaceTrack(originalMediaTrack, false);
        if (wasMuted) await localAudioTrack.mute();
      } catch { /* ignore restore errors */ }
    }
  };

  // Group sounds by server_id (preserve sorted order from API)
  const grouped: { serverId: string; serverName: string; sounds: SoundWithServer[] }[] = [];
  for (const sound of sounds) {
    const last = grouped[grouped.length - 1];
    if (last && last.serverId === sound.server_id) {
      last.sounds.push(sound);
    } else {
      grouped.push({ serverId: sound.server_id, serverName: sound.server_name, sounds: [sound] });
    }
  }

  return (
    <div className="soundboard-panel" ref={panelRef}>
      <div className="soundboard-panel-header">
        <span className="soundboard-panel-title">Soundboard</span>
      </div>
      {error && <div className="soundboard-panel-error">{error}</div>}
      {loading ? (
        <div className="soundboard-panel-empty">Loading...</div>
      ) : sounds.length === 0 ? (
        <div className="soundboard-panel-empty">No sounds available. Upload sounds in server settings.</div>
      ) : (
        <div className="soundboard-panel-list">
          {grouped.map(group => (
            <div key={group.serverId} className="soundboard-panel-group">
              <div className="soundboard-panel-group-label">{group.serverName}</div>
              {group.sounds.map(sound => {
                const isPlaying = playing?.soundId === sound.id;
                return (
                  <div key={sound.id} className="soundboard-panel-item">
                    <span className="soundboard-panel-emoji">{sound.emoji || '🔊'}</span>
                    <span className="soundboard-panel-name">{sound.name}</span>
                    <button
                      className="soundboard-panel-btn"
                      title="Preview (only you hear this)"
                      onClick={() => handlePreview(sound)}
                      disabled={!!(playing && playing.soundId !== sound.id)}
                    >
                      <Headphones size={14} />
                    </button>
                    <button
                      className={`soundboard-panel-btn soundboard-panel-btn--play${isPlaying && playing?.mode === 'broadcast' ? ' active' : ''}`}
                      title={isPlaying && playing?.mode === 'broadcast' ? 'Stop' : 'Play for everyone'}
                      onClick={() => handleBroadcast(sound)}
                      disabled={!localParticipant || !!(playing && playing.soundId !== sound.id)}
                    >
                      {isPlaying && playing?.mode === 'broadcast' ? <Square size={14} /> : <Volume2 size={14} />}
                    </button>
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
```

- [ ] **Step 2: Create `frontend/src/components/voice/SoundboardPanel.css`.**

```css
.soundboard-panel {
  position: absolute;
  bottom: calc(100% + 8px);
  left: 50%;
  transform: translateX(-50%);
  width: 300px;
  max-height: 400px;
  background: var(--parley-panel-bg, var(--parley-input));
  border: 1px solid var(--parley-border);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
  display: flex;
  flex-direction: column;
  z-index: 100;
  overflow: hidden;
}

.soundboard-panel-header {
  padding: 10px 12px 8px;
  border-bottom: 1px solid var(--parley-border);
  flex-shrink: 0;
}

.soundboard-panel-title {
  font-size: 12px;
  font-weight: 700;
  color: var(--parley-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.6px;
}

.soundboard-panel-error {
  padding: 6px 12px;
  font-size: 12px;
  color: var(--parley-danger);
  flex-shrink: 0;
}

.soundboard-panel-empty {
  padding: 20px 12px;
  font-size: 12px;
  color: var(--parley-text-muted);
  text-align: center;
}

.soundboard-panel-list {
  overflow-y: auto;
  flex: 1;
  padding: 6px 0;
}

.soundboard-panel-group {
  margin-bottom: 4px;
}

.soundboard-panel-group-label {
  font-size: 10px;
  font-weight: 700;
  color: var(--parley-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.6px;
  padding: 4px 12px 2px;
}

.soundboard-panel-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 12px;
}

.soundboard-panel-item:hover {
  background: var(--parley-hover);
}

.soundboard-panel-emoji {
  font-size: 16px;
  width: 22px;
  text-align: center;
  flex-shrink: 0;
}

.soundboard-panel-name {
  flex: 1;
  font-size: 13px;
  color: var(--parley-text-normal);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.soundboard-panel-btn {
  background: none;
  border: 1px solid var(--parley-border-light);
  border-radius: 4px;
  color: var(--parley-text-muted);
  padding: 3px 5px;
  cursor: pointer;
  display: flex;
  align-items: center;
  flex-shrink: 0;
  transition: background 0.1s, color 0.1s;
}

.soundboard-panel-btn:hover:not(:disabled) {
  background: rgba(var(--accent-rgb), 0.1);
  color: var(--parley-accent);
  border-color: rgba(var(--accent-rgb), 0.3);
}

.soundboard-panel-btn.active {
  background: rgba(var(--parley-danger-rgb), 0.1);
  color: var(--parley-danger);
  border-color: rgba(var(--parley-danger-rgb), 0.3);
}

.soundboard-panel-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
```

- [ ] **Step 3: Build frontend.**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | head -30
```
Expected: no errors.

- [ ] **Step 4: Commit.**

```bash
git add frontend/src/components/voice/SoundboardPanel.tsx frontend/src/components/voice/SoundboardPanel.css
git commit -m "feat: SoundboardPanel with Web Audio playback"
```

---

## Task 10: ParticipantTile Emoji Indicator

**Files:**
- Modify: `frontend/src/components/voice/ParticipantTile.tsx`
- Modify: `frontend/src/components/voice/ParticipantTile.css`

- [ ] **Step 1: Add `activeSoundEmoji` prop to `ParticipantTile`.**

In `ParticipantTile.tsx`, find the interface:

```tsx
interface ParticipantTileProps {
  participant: Participant;
  isLocal?: boolean;
  isSpeaking?: boolean;
  isScreenShare?: boolean;
  displayName?: string;
  avatarUrl?: string;
  onContextMenu?: (e: React.MouseEvent) => void;
  onClick?: (e: React.MouseEvent) => void;
}
```

Add `activeSoundEmoji?: string;` to the interface and destructure it in the component function signature:

```tsx
export const ParticipantTile: React.FC<ParticipantTileProps> = ({
  participant,
  isLocal,
  isSpeaking,
  isScreenShare,
  displayName,
  avatarUrl,
  onContextMenu,
  onClick,
  activeSoundEmoji,  // <-- add this
}) => {
```

- [ ] **Step 2: Add the emoji badge to the footer JSX.**

Find the footer:

```tsx
<div className="participant-tile-footer">
  <span className="participant-tile-name">
    {name}
    {isLocal && <span className="participant-tile-you">You</span>}
  </span>
  {isMuted && !isScreenShare && (
    <span className="participant-tile-muted">
      <MicOff size={12} color="var(--parley-danger)" />
    </span>
  )}
</div>
```

Replace with:

```tsx
<div className="participant-tile-footer">
  <span className="participant-tile-name">
    {name}
    {isLocal && <span className="participant-tile-you">You</span>}
  </span>
  {activeSoundEmoji && !isScreenShare && (
    <span className="participant-tile-sound-emoji" title="Playing sound">
      {activeSoundEmoji}
    </span>
  )}
  {isMuted && !isScreenShare && (
    <span className="participant-tile-muted">
      <MicOff size={12} color="var(--parley-danger)" />
    </span>
  )}
</div>
```

- [ ] **Step 3: Add the CSS for the emoji badge to `ParticipantTile.css`.**

Append to the end of `ParticipantTile.css`:

```css
.participant-tile-sound-emoji {
  font-size: 14px;
  line-height: 1;
  flex-shrink: 0;
  animation: soundboard-pop 0.2s ease;
}

@keyframes soundboard-pop {
  0% { transform: scale(0.5); opacity: 0; }
  60% { transform: scale(1.2); }
  100% { transform: scale(1); opacity: 1; }
}
```

- [ ] **Step 4: Build frontend.**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | head -30
```
Expected: no errors.

- [ ] **Step 5: Commit.**

```bash
git add frontend/src/components/voice/ParticipantTile.tsx frontend/src/components/voice/ParticipantTile.css
git commit -m "feat: participant tile soundboard emoji indicator"
```

---

## Task 11: Wire VoiceChannel — Soundboard Button, Panel, WS Event Handling

**Files:**
- Modify: `frontend/src/components/voice/VoiceChannel.tsx`

This task wires everything together: the soundboard button in the controls bar, the panel popover, handling `SOUNDBOARD_PLAY` WS events, and passing `activeSoundEmoji` to each `ParticipantTile`.

The `SOUNDBOARD_PLAY` WS event is received in the app's WebSocket handler (likely `App.tsx` or a WebSocket hook). Look at how `VOICE_STATE_UPDATE` is handled to find where to add the new handler — then thread the `activeSoundEmojis` state down to `VoiceChannel` via props, or handle it directly inside `VoiceChannel` if the WS event is already threaded there.

**Step-by-step:**

- [ ] **Step 1: Find where WebSocket events are dispatched.**

Search for `VOICE_STATE_UPDATE` handling in the frontend:

```bash
cd /home/dylan/Developer/parley/frontend && grep -r "VOICE_STATE_UPDATE\|voiceStateUpdate" src/ --include="*.tsx" --include="*.ts" -l
```

This will tell you which file handles WS events. It's likely `App.tsx` or a custom hook.

- [ ] **Step 2: Add `SOUNDBOARD_PLAY` handler.**

**Note on broadcast routing:** The backend broadcasts `SOUNDBOARD_PLAY` on the server's virtual WebSocket channel (`"server:" + serverID`) — the same channel used for `VOICE_STATE_UPDATE`. This means all members of the server receive the event, not just the VC participants. The frontend filters by `channel_id` in the payload to only process events for the voice channel the user is currently in. This is a deliberate re-use of the existing server-level fan-out pattern.

In the file that handles WS events (found above), add a case for `SOUNDBOARD_PLAY`:

```typescript
// In the WS message handler switch/if-else:
case 'SOUNDBOARD_PLAY': {
  const { channel_id, user_id, emoji, duration_ms } = data;
  // Only process if the user is in this voice channel
  // (event is broadcast server-wide; filter by channel_id)
  if (channel_id === activeVoiceChannelId) {
    setActiveSoundEmojis(prev => {
      const next = new Map(prev);
      // Clear any existing timeout for this user
      const existing = next.get(user_id);
      if (existing?.timeoutId) clearTimeout(existing.timeoutId);
      const ms = (duration_ms && duration_ms > 0) ? Math.min(duration_ms, 60_000) : 30_000;
      const timeoutId = window.setTimeout(() => {
        setActiveSoundEmojis(m => {
          const n = new Map(m);
          n.delete(user_id);
          return n;
        });
      }, ms);
      next.set(user_id, { emoji, timeoutId });
      return next;
    });
  }
  break;
}
```

Add the `activeSoundEmojis` state near other voice state:

```typescript
const [activeSoundEmojis, setActiveSoundEmojis] = useState<Map<string, { emoji: string; timeoutId: number }>>(new Map());
```

Clean it up when leaving voice:

```typescript
// When voice channel changes or unmounts, clear all emoji timeouts
useEffect(() => {
  return () => {
    activeSoundEmojis.forEach(v => clearTimeout(v.timeoutId));
  };
}, [activeSoundEmojis]);
```

Pass `activeSoundEmojis` to `VoiceChannel` as a new prop:

```tsx
<VoiceChannel
  ...
  activeSoundEmojis={activeSoundEmojis}
/>
```

- [ ] **Step 3: Add `activeSoundEmojis` prop to `VoiceChannel`.**

In `VoiceChannel.tsx`, add to the props interface:

```tsx
activeSoundEmojis?: Map<string, { emoji: string; timeoutId: number }>;
```

Destructure it in the component function.

- [ ] **Step 4: Pass `activeSoundEmoji` to each `ParticipantTile` in `VoiceChannel.tsx`.**

Find where `ParticipantTile` is rendered in the grid/speaker views. For each tile, add:

```tsx
activeSoundEmoji={activeSoundEmojis?.get(participant.identity)?.emoji}
```

- [ ] **Step 5: Add the soundboard button and panel to `VoiceChannel.tsx`.**

Add state at the top of the component:

```tsx
const [soundboardOpen, setSoundboardOpen] = useState(false);
```

Add the import at the top:

```tsx
import { SoundboardPanel } from './SoundboardPanel';
import { Music2 } from 'lucide-react';
```

In the controls bar JSX (near the other `vc-ctrl` buttons), add before the leave button:

```tsx
{connected && (
  <div style={{ position: 'relative' }}>
    <button
      className={`vc-ctrl${soundboardOpen ? ' active' : ''}`}
      onClick={() => setSoundboardOpen(o => !o)}
      title="Soundboard"
    >
      <Music2 size={18} color={soundboardOpen ? 'var(--parley-accent)' : 'var(--parley-text-muted)'} />
    </button>
    {soundboardOpen && (
      <SoundboardPanel
        channelId={channel.id}
        localParticipant={localParticipant}
        muted={muted}
        onClose={() => setSoundboardOpen(false)}
      />
    )}
  </div>
)}
```

- [ ] **Step 6: Build frontend.**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | head -50
```
Expected: no errors. Fix any type errors (common: `Map` not assignable — use `Map<string, {emoji:string; timeoutId:number}>` consistently).

- [ ] **Step 7: Commit.**

```bash
git add frontend/src/components/voice/VoiceChannel.tsx
git commit -m "feat: wire soundboard button, panel, and SOUNDBOARD_PLAY event"
```

---

## Task 12: End-to-End Build Verification

- [ ] **Step 1: Run all Go tests.**

```bash
cd /home/dylan/Developer/parley && go test ./... 2>&1
```
Expected: all tests pass, no failures.

- [ ] **Step 2: Full Go build.**

```bash
cd /home/dylan/Developer/parley && go build ./...
```
Expected: no output.

- [ ] **Step 3: Full frontend build.**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build
```
Expected: build succeeds with no TypeScript errors.

- [ ] **Step 4: Final commit if any loose files remain.**

```bash
cd /home/dylan/Developer/parley && git status
```
Stage and commit any uncommitted changes.

- [ ] **Step 5: Push.**

```bash
git push origin main
```
