package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"parley/internal/auth"
	"parley/internal/spaces"
)

const (
	uploadQuotaBytes    = 1 << 30        // 1 GB per user lifetime
	uploadRateWindow    = time.Hour      // sliding window for rate limit
	uploadRateLimit     = 30             // max uploads per user per hour
)

func handleUpload(spacesClient *spaces.Client, db *sql.DB) http.HandlerFunc {
	// Per-user rate limiter keyed by user ID string.
	userLimiter := newRateLimiter(uploadRateLimit, uploadRateWindow)

	return func(w http.ResponseWriter, r *http.Request) {
		if spacesClient == nil {
			http.Error(w, "file upload not configured", http.StatusServiceUnavailable)
			return
		}

		// Rate limit per authenticated user.
		userIDStr := auth.GetUserIDFromContext(r)
		if !userLimiter.Allow(userIDStr) {
			http.Error(w, "upload rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file field", http.StatusBadRequest)
			return
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "failed to read file", http.StatusInternalServerError)
			return
		}

		// Atomically reserve quota: increment only if it won't exceed the cap.
		userID, _ := strconv.ParseInt(userIDStr, 10, 64)
		var newTotal int64
		err = db.QueryRowContext(r.Context(),
			`UPDATE users
			 SET upload_bytes_used = upload_bytes_used + $1
			 WHERE id = $2 AND upload_bytes_used + $1 <= $3
			 RETURNING upload_bytes_used`,
			int64(len(data)), userID, int64(uploadQuotaBytes),
		).Scan(&newTotal)
		if err == sql.ErrNoRows {
			http.Error(w, "storage quota exceeded (1 GB limit)", http.StatusRequestEntityTooLarge)
			return
		}
		if err != nil {
			log.Printf("quota check error: %v", err)
			http.Error(w, "upload failed", http.StatusInternalServerError)
			return
		}

		// NSFW check disabled — sidecar moved to dedicated box (TODO)
		ext, ok := allowedFileExt(data)
		if !ok {
			// Refund the reserved quota on rejection.
			db.ExecContext(r.Context(),
				`UPDATE users SET upload_bytes_used = upload_bytes_used - $1 WHERE id = $2`,
				int64(len(data)), userID)
			http.Error(w, "only PNG, GIF, JPEG, WebM, OGG, and MP3 files are allowed", http.StatusBadRequest)
			return
		}
		key := fmt.Sprintf("uploads/%s%s", generateID(), ext)

		url, err := spacesClient.Upload(r.Context(), key, bytes.NewReader(data), int64(len(data)))
		if err != nil {
			// Refund quota on upload failure.
			db.ExecContext(r.Context(),
				`UPDATE users SET upload_bytes_used = upload_bytes_used - $1 WHERE id = $2`,
				int64(len(data)), userID)
			log.Printf("upload error: %v", err)
			http.Error(w, "upload failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"url": url})
	}
}
