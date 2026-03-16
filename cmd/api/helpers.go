package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"parley/internal/auth"
	"parley/internal/db"
)

// ============ Request / response types ============

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
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

// generateID returns a unique string ID based on the current time in nanoseconds.
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// allowedFileExt inspects the magic bytes of data and returns the file extension
// for allowed upload types (PNG, GIF, JPEG, WebM, OGG, MP3). Returns ("", false) for anything else.
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
	}
	return "", false
}

// checkNSFW sends an image to the local NSFW sidecar and returns true if it should be blocked.
// Fails open (returns false) if the sidecar is unavailable, so uploads are never hard-blocked by infra issues.
func checkNSFW(ctx context.Context, data []byte, _ string) (bool, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "upload")
	if err != nil {
		return false, err
	}
	if _, err := part.Write(data); err != nil {
		return false, err
	}
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:8081/check", body)
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err // sidecar down — fail open
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("nsfw sidecar returned %d", resp.StatusCode)
	}

	var result struct {
		Predictions []struct {
			ClassName   string  `json:"className"`
			Probability float64 `json:"probability"`
		} `json:"predictions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	for _, p := range result.Predictions {
		if (p.ClassName == "Porn" || p.ClassName == "Hentai") && p.Probability > 0.6 {
			return true, nil
		}
	}
	return false, nil
}
