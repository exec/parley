package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"parley/internal/spaces"
)

func handleUpload(spacesClient *spaces.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if spacesClient == nil {
			http.Error(w, "file upload not configured", http.StatusServiceUnavailable)
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

		// Buffer file so we can both NSFW-check and upload it
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "failed to read file", http.StatusInternalServerError)
			return
		}

		// NSFW check disabled — sidecar moved to dedicated box (TODO)
		// contentType := header.Header.Get("Content-Type")
		// if strings.HasPrefix(contentType, "image/") { checkNSFW(...) }

		ext, ok := allowedFileExt(data)
		if !ok {
			http.Error(w, "only PNG, GIF, JPEG, WebM, OGG, and MP3 files are allowed", http.StatusBadRequest)
			return
		}
		key := fmt.Sprintf("uploads/%s%s", generateID(), ext)

		url, err := spacesClient.Upload(r.Context(), key, bytes.NewReader(data), int64(len(data)))
		if err != nil {
			log.Printf("upload error: %v", err)
			http.Error(w, "upload failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"url": url})
	}
}
