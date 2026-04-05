package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"parley/internal/auth"
	"parley/internal/spaces"
)

const (
	uploadQuotaBytes = 1 << 30   // 1 GB per user (rolling — oldest files evicted to make room)
	uploadRateWindow = time.Hour // sliding window for rate limit
	uploadRateLimit  = 30        // max uploads per user per hour
)

func handleUpload(spacesClient *spaces.Client, db *sql.DB) http.HandlerFunc {
	userLimiter := newRateLimiter(uploadRateLimit, uploadRateWindow)

	return func(w http.ResponseWriter, r *http.Request) {
		if spacesClient == nil {
			http.Error(w, "file upload not configured", http.StatusServiceUnavailable)
			return
		}

		userIDStr := auth.GetUserIDFromContext(r)
		if !userLimiter.Allow(userIDStr) {
			http.Error(w, "upload rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		if err := r.ParseMultipartForm(50 << 20); err != nil {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file field", http.StatusBadRequest)
			return
		}
		defer file.Close()

		data, err := io.ReadAll(io.LimitReader(file, 50<<20+1))
		if err != nil {
			http.Error(w, "failed to read file", http.StatusInternalServerError)
			return
		}
		if int64(len(data)) > 50<<20 {
			http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
			return
		}

		ext, ok := allowedFileExt(data)
		if !ok {
			http.Error(w, "only PNG, GIF, JPEG, WebM, OGG, MP3, and WAV files are allowed", http.StatusBadRequest)
			return
		}

		// Optimize images before quota accounting so we charge the compressed size.
		data = optimizeImage(data, ext)

		userID, _ := strconv.ParseInt(userIDStr, 10, 64)
		fileSize := int64(len(data))

		// Ensure quota: evict oldest uploads if needed, then reserve space.
		if err := ensureQuota(r.Context(), db, spacesClient, userID, fileSize); err != nil {
			log.Printf("quota eviction error for user %d: %v", userID, err)
			http.Error(w, "storage quota exceeded", http.StatusRequestEntityTooLarge)
			return
		}

		key := fmt.Sprintf("uploads/%s%s", generateID(), ext)
		url, err := spacesClient.Upload(r.Context(), key, bytes.NewReader(data), fileSize)
		if err != nil {
			// Refund quota on upload failure.
			db.ExecContext(r.Context(),
				`UPDATE users SET upload_bytes_used = upload_bytes_used - $1 WHERE id = $2`,
				fileSize, userID)
			log.Printf("upload error: %v", err)
			http.Error(w, "upload failed", http.StatusInternalServerError)
			return
		}

		// Record the upload for future eviction ordering.
		// If this fails, clean up the Spaces object and refund the quota so the
		// reserved bytes are not permanently lost.
		if _, err := db.ExecContext(r.Context(),
			`INSERT INTO user_uploads (user_id, spaces_key, file_size) VALUES ($1, $2, $3)`,
			userID, key, fileSize); err != nil {
			if delErr := spacesClient.Delete(r.Context(), key); delErr != nil {
				log.Printf("cleanup: delete %s after DB failure: %v", key, delErr)
			}
			db.ExecContext(r.Context(),
				`UPDATE users SET upload_bytes_used = GREATEST(0, upload_bytes_used - $1) WHERE id = $2`,
				fileSize, userID)
			log.Printf("upload record error for user %d: %v", userID, err)
			http.Error(w, "upload failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"url": url})
	}
}

// ensureQuota evicts the user's oldest files until fileSize bytes can fit within
// uploadQuotaBytes, then atomically reserves the space. Returns an error only if
// quota cannot be freed (e.g. all files deleted and still not enough room).
func ensureQuota(ctx context.Context, db *sql.DB, sc *spaces.Client, userID, fileSize int64) error {
	for {
		// Try atomic reservation.
		var newTotal int64
		err := db.QueryRowContext(ctx,
			`UPDATE users
			 SET upload_bytes_used = upload_bytes_used + $1
			 WHERE id = $2 AND upload_bytes_used + $1 <= $3
			 RETURNING upload_bytes_used`,
			fileSize, userID, int64(uploadQuotaBytes),
		).Scan(&newTotal)
		if err == nil {
			return nil // reserved successfully
		}
		if err != sql.ErrNoRows {
			return err
		}

		// Quota full — evict the oldest upload.
		var oldKey string
		var oldSize int64
		var oldID int64
		err = db.QueryRowContext(ctx,
			`SELECT id, spaces_key, file_size FROM user_uploads
			 WHERE user_id = $1 ORDER BY created_at ASC LIMIT 1`, userID,
		).Scan(&oldID, &oldKey, &oldSize)
		if err == sql.ErrNoRows {
			return fmt.Errorf("no evictable uploads remain") // hard limit
		}
		if err != nil {
			return err
		}

		// Delete from Spaces (best-effort — don't fail if the object is already gone).
		if delErr := sc.Delete(ctx, oldKey); delErr != nil {
			log.Printf("eviction: delete %s: %v", oldKey, delErr)
		}

		// Remove from tracking and reclaim bytes atomically.
		db.ExecContext(ctx, `DELETE FROM user_uploads WHERE id = $1`, oldID)
		db.ExecContext(ctx,
			`UPDATE users SET upload_bytes_used = GREATEST(0, upload_bytes_used - $1) WHERE id = $2`,
			oldSize, userID)
	}
}

// optimizeImage recompresses the image data if possible without quality loss.
// Currently optimizes JPEGs by re-encoding at quality 82.
// Returns the original data unchanged for all other formats.
func optimizeImage(data []byte, ext string) []byte {
	if !strings.EqualFold(ext, ".jpg") && !strings.EqualFold(filepath.Ext(ext), ".jpeg") {
		return data
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 82}); err != nil || buf.Len() >= len(data) {
		return data
	}
	return buf.Bytes()
}
