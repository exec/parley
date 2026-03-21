package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/validation"
)

func handleListAPIKeys(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerIDStr := auth.GetUserIDFromContext(r)
		ownerID, err := strconv.ParseInt(ownerIDStr, 10, 64)
		if err != nil {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		keys, err := repo.GetAPIKeysByOwner(r.Context(), ownerID)
		if err != nil {
			jsonError(w, "failed to list keys", http.StatusInternalServerError)
			return
		}
		if keys == nil {
			keys = []db.APIKeyInfo{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(keys)
	}
}

func handleCreateAPIKey(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerIDStr := auth.GetUserIDFromContext(r)
		ownerID, err := strconv.ParseInt(ownerIDStr, 10, 64)
		if err != nil {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Type        string `json:"type"`
			BotUsername string `json:"bot_username"`
			Name        string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request", http.StatusBadRequest)
			return
		}
		if req.Type != "bot" && req.Type != "user" {
			jsonError(w, "type must be 'bot' or 'user'", http.StatusBadRequest)
			return
		}
		if req.Type == "bot" && strings.TrimSpace(req.BotUsername) == "" {
			jsonError(w, "bot_username is required for bot type", http.StatusBadRequest)
			return
		}

		// Generate key: plk_ + 40 hex chars (20 random bytes)
		keyBytes := make([]byte, 20)
		if _, randErr := rand.Read(keyBytes); randErr != nil {
			jsonError(w, "failed to generate key", http.StatusInternalServerError)
			return
		}
		fullKey := "plk_" + hex.EncodeToString(keyBytes)
		keyHash := auth.SHA256Hex(fullKey)
		keyPrefix := fullKey[:12] // "plk_" + first 8 hex chars

		var targetUserID int64
		var botUsername string
		var botUserID int64
		var keyID int64

		name := strings.TrimSpace(req.Name)

		if req.Type == "bot" {
			botUsername = strings.TrimSpace(req.BotUsername)
			if !validation.ValidUsername(botUsername) {
				jsonError(w, "bot username may only contain letters, numbers, underscores, hyphens and dots", http.StatusBadRequest)
				return
			}
			if name == "" {
				name = botUsername
			}
			const maxBotsPerUser = 10
			botCount, countErr := repo.CountBotsByOwner(r.Context(), ownerID)
			if countErr != nil {
				jsonError(w, "failed to check bot limit", http.StatusInternalServerError)
				return
			}
			if botCount >= maxBotsPerUser {
				jsonError(w, "bot limit reached: maximum 10 bots per user", http.StatusForbidden)
				return
			}
			botUserID, keyID, err = repo.CreateBotWithKey(r.Context(), botUsername, keyHash, keyPrefix, name, ownerID)
			if err != nil {
				if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
					jsonError(w, "bot username already taken", http.StatusConflict)
					return
				}
				jsonError(w, "failed to create bot", http.StatusInternalServerError)
				return
			}
			targetUserID = botUserID
		} else {
			targetUserID = ownerID
			if name == "" {
				name = "User API Key"
			}
			keyID, err = repo.CreateAPIKey(r.Context(), keyHash, keyPrefix, name, targetUserID, ownerID)
			if err != nil {
				jsonError(w, "failed to create key", http.StatusInternalServerError)
				return
			}
		}

		resp := map[string]interface{}{
			"id":         keyID,
			"key":        fullKey,
			"key_prefix": keyPrefix,
			"name":       name,
			"type":       req.Type,
		}
		if req.Type == "bot" {
			resp["bot_username"] = botUsername
			resp["bot_user_id"] = botUserID
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func handleRevokeAPIKey(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerIDStr := auth.GetUserIDFromContext(r)
		ownerID, err := strconv.ParseInt(ownerIDStr, 10, 64)
		if err != nil {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		keyIDStr := chi.URLParam(r, "id")
		keyID, err := strconv.ParseInt(keyIDStr, 10, 64)
		if err != nil {
			jsonError(w, "invalid key id", http.StatusBadRequest)
			return
		}
		if err := repo.RevokeAPIKey(r.Context(), keyID, ownerID); err != nil {
			if err == db.ErrNotFound {
				jsonError(w, "key not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to revoke key", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleRenameBotUser(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerIDStr := auth.GetUserIDFromContext(r)
		ownerID, err := strconv.ParseInt(ownerIDStr, 10, 64)
		if err != nil {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		botIDStr := chi.URLParam(r, "botId")
		botID, err := strconv.ParseInt(botIDStr, 10, 64)
		if err != nil {
			jsonError(w, "invalid bot id", http.StatusBadRequest)
			return
		}
		var req struct {
			Username string `json:"username"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request", http.StatusBadRequest)
			return
		}
		newUsername := strings.TrimSpace(req.Username)
		if newUsername == "" {
			jsonError(w, "username is required", http.StatusBadRequest)
			return
		}
		if !validation.ValidUsername(newUsername) {
			jsonError(w, "bot username may only contain letters, numbers, underscores, hyphens and dots", http.StatusBadRequest)
			return
		}
		if err := repo.RenameBotUser(r.Context(), botID, ownerID, newUsername); err != nil {
			if err == db.ErrNotFound {
				jsonError(w, "bot not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to rename bot", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
