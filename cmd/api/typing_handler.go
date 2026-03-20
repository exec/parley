package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"parley/internal/auth"
	"parley/internal/db"
	ws "parley/internal/websocket"
)

// typingRateLimiter tracks the last-accepted typing request per (userID:channelID) key.
// The cooldown for each key equals the clamped duration of the previous accepted request.
type typingRateLimiter struct {
	mu      sync.Mutex
	lastAt  map[string]time.Time
	lastDur map[string]time.Duration
}

func newTypingRateLimiter() *typingRateLimiter {
	return &typingRateLimiter{
		lastAt:  make(map[string]time.Time),
		lastDur: make(map[string]time.Duration),
	}
}

// allow returns true if the request is outside the cooldown window for the given key.
// If allowed, it records the new timestamp and sets the cooldown to `newCooldown`.
func (t *typingRateLimiter) allow(key string, newCooldown time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if last, ok := t.lastAt[key]; ok {
		if cooldown, hasCooldown := t.lastDur[key]; hasCooldown && time.Since(last) < cooldown {
			return false
		}
	}
	t.lastAt[key] = time.Now()
	t.lastDur[key] = newCooldown
	return true
}

// Package-level singleton — initialized once at startup in handleChannelTyping.
var globalTypingLimiter = newTypingRateLimiter()

func handleChannelTyping(repo *db.Repository, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		channelIDStr := chi.URLParam(r, "channelId")
		channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
		if err != nil {
			jsonError(w, "invalid channel id", http.StatusBadRequest)
			return
		}

		var req struct {
			Duration int `json:"duration"`
		}
		req.Duration = 5 // default
		// Decode is best-effort; ignore body errors (no required fields).
		json.NewDecoder(r.Body).Decode(&req)

		// Clamp duration to [1, 60].
		if req.Duration < 1 {
			req.Duration = 1
		}
		if req.Duration > 60 {
			req.Duration = 60
		}

		// Verify channel exists.
		ch, err := repo.GetChannelByID(r.Context(), channelID)
		if err != nil {
			jsonError(w, "channel not found", http.StatusNotFound)
			return
		}

		// Verify caller is a member of the channel's server.
		userID, _ := strconv.ParseInt(userIDStr, 10, 64)
		if _, err := repo.GetMember(r.Context(), ch.ServerID, userID); err != nil {
			if err == db.ErrNotFound {
				jsonError(w, "forbidden", http.StatusForbidden)
			} else {
				jsonError(w, "internal error", http.StatusInternalServerError)
			}
			return
		}

		// Rate limit: key = "userID:channelID", cooldown = previous clamped duration.
		key := fmt.Sprintf("%s:%d", userIDStr, channelID)
		cooldown := time.Duration(req.Duration) * time.Second
		if !globalTypingLimiter.allow(key, cooldown) {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		// Look up username and display_name for the broadcast payload.
		u, err := repo.GetUserByID(r.Context(), userID)
		if err != nil {
			jsonError(w, "user not found", http.StatusInternalServerError)
			return
		}

		expiresAt := time.Now().UTC().Add(time.Duration(req.Duration) * time.Second).Format(time.RFC3339)

		payload, _ := json.Marshal(map[string]string{
			"channel_id":   channelIDStr,
			"user_id":      userIDStr,
			"username":     u.Username,
			"display_name": u.DisplayName,
			"expires_at":   expiresAt,
		})
		hub.BroadcastToChannel(fmt.Sprintf("%d", channelID), ws.EventUserTyping, payload)

		w.WriteHeader(http.StatusNoContent)
	}
}
