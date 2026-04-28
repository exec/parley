package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/validation"
)

// ============ Request / response types ============

type RegisterRequest struct {
	Username   string `json:"username"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	Password   string `json:"password"`
	InviteCode string `json:"invite_code"`
	// IsAdult is asserted by the client after a local DOB check. The DOB
	// itself is never sent or stored — see PRIVACY.md §11.
	IsAdult bool `json:"is_adult"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

type AuthResponse struct {
	User  auth.User `json:"user"`
	Token string    `json:"token"`
}

// publicUserResponse is a version of PublicUser with string IDs for frontend compatibility.
type publicUserResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url"`
	BannerURL   string `json:"banner_url,omitempty"`
	Bio         string `json:"bio,omitempty"`
	Badges      int    `json:"badges"`
	CreatedAt   string `json:"created_at"`
}

func toPublicUserResponse(u db.PublicUser) publicUserResponse {
	return publicUserResponse{
		ID:          strconv.FormatInt(u.ID, 10),
		Username:    u.Username,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
		BannerURL:   u.BannerURL,
		Bio:         u.Bio,
		Badges:      u.Badges,
		CreatedAt:   u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ============ Utility functions ============

func jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"message": message})
}

// generateID returns a cryptographically random 16-byte hex string for use as upload keys.
func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Extremely unlikely; rand.Read only fails if the OS entropy pool is broken.
		panic(fmt.Sprintf("generateID: crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}

// validateMediaURL ensures a URL is either empty or points to the CDN host.
// This prevents SSRF via arbitrary avatar/banner URLs.
func validateMediaURL(rawURL, cdnHost string) error {
	return validation.MediaURL(rawURL, cdnHost)
}

// allowedFileExt inspects the magic bytes of data and returns the file extension
// for allowed upload types (PNG, GIF, JPEG, WebM, OGG, MP3, WAV). Returns ("", false) for anything else.
func allowedFileExt(data []byte) (string, bool) {
	if len(data) < 12 {
		return "", false
	}
	switch {
	// Images
	case data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return ".jpg", true
	case data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 &&
		data[4] == 0x0D && data[5] == 0x0A && data[6] == 0x1A && data[7] == 0x0A:
		return ".png", true
	case data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38:
		return ".gif", true
	// Video / audio containers
	case data[0] == 0x1A && data[1] == 0x45 && data[2] == 0xDF && data[3] == 0xA3:
		return ".webm", true
	// OGG (covers ogg/opus and ogg/vorbis audio)
	case data[0] == 0x4F && data[1] == 0x67 && data[2] == 0x67 && data[3] == 0x53:
		return ".ogg", true
	// MP3: ID3v2 tag header
	case data[0] == 0x49 && data[1] == 0x44 && data[2] == 0x33:
		return ".mp3", true
	// MP3: raw MPEG frame sync (no ID3 tag)
	case data[0] == 0xFF && (data[1]&0xE0 == 0xE0) && (data[1]&0x18 != 0x08) && (data[1]&0x06 != 0x00):
		return ".mp3", true
	// WAV: RIFF....WAVE
	case data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x41 && data[10] == 0x56 && data[11] == 0x45:
		return ".wav", true
	}
	return "", false
}

