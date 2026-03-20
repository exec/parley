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
	// MP3: raw MPEG frame sync (no ID3) — sync word, valid MPEG version (not reserved 0x08), valid layer (not reserved 0x00)
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
