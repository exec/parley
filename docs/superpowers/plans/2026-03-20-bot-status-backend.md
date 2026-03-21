# Bot Status, Presence & API Expansion — Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Server-enforced bot/user presence via hub, new self-management API endpoints, timed typing REST endpoint, and frontend `expires_at` support.

**Architecture:** Hub writes `status_type` to DB on first WS connect (conditional on not-invisible) and on last WS disconnect (unconditional). New routes `GET/PATCH /api/users/me` and `POST /api/channels/{id}/typing` are added alongside the existing auth routes. The typing endpoint uses a per-(user,channel) rate limiter keyed on the previous request's clamped duration.

**Tech Stack:** Go 1.21+, Chi router, PostgreSQL, existing `internal/db.Repository`, `internal/websocket.Hub`.

---

## File Map

| Action | File |
|--------|------|
| Modify | `internal/db/user_repository.go` — add two status-write methods |
| Modify | `internal/websocket/hub.go` — `StatusWriter` interface, field, DB calls in Register/Unregister |
| Modify | `cmd/api/main.go` — wire `repo` as StatusWriter into hub (NOT routes.go) |
| Modify | `cmd/api/routes.go` — register new routes |
| Modify | `cmd/api/user_status_handler.go` — fix broadcast, do own validation (bypass authService.UpdateStatus) |
| Modify | `cmd/api/user_handlers.go` — add `handleGetMe2` and `handlePatchMe` handlers |
| Create | `cmd/api/typing_handler.go` — typing rate limiter + `handleChannelTyping` |
| Modify | `frontend/src/hooks/useWebSocket.ts` — handle `expires_at` in `USER_TYPING` |
| Modify | `frontend/src/App.tsx` — update `handleTyping` callback to accept `expires_at` |

---

## Task 1: DB — add `SetUserStatusType` and `SetUserStatusTypeIfNotInvisible`

**Files:**
- Modify: `internal/db/user_repository.go`

- [ ] **Step 1: Add the two methods at the bottom of user_repository.go**

Append after the existing `UpdateUserStatus` function (around line 640):

```go
// SetUserStatusType sets status_type unconditionally for the given user.
// Used by the hub on last WS disconnect.
func (r *Repository) SetUserStatusType(ctx context.Context, userID int64, statusType string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET status_type = $2, updated_at = NOW() WHERE id = $1`,
		userID, statusType)
	return err
}

// SetUserStatusTypeIfNotInvisible sets status_type only when the current value
// is not 'invisible'. Used by the hub on first WS connection.
func (r *Repository) SetUserStatusTypeIfNotInvisible(ctx context.Context, userID int64, statusType string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET status_type = $2, updated_at = NOW()
		 WHERE id = $1 AND status_type != 'invisible'`,
		userID, statusType)
	return err
}
```

- [ ] **Step 2: Build to verify no errors**

Run: `cd /home/dylan/Developer/parley && go build ./internal/db/...`
Expected: exits 0, no output.

- [ ] **Step 3: Commit**

```bash
git add internal/db/user_repository.go
git commit -m "feat: add SetUserStatusType and SetUserStatusTypeIfNotInvisible to repo"
```

---

## Task 2: Hub — `StatusWriter` interface, field, and DB calls on connect/disconnect

**Files:**
- Modify: `internal/websocket/hub.go`

**Context:** `Hub.RegisterClient` is at line ~151, `Hub.UnregisterClient` at line ~223. `NewHub()` is at line ~93. No `statusWriter` field exists yet. `BroadcastStatusUpdate` already exists (line 426) — do not change it.

- [ ] **Step 1: Add `StatusWriter` interface and `statusWriter` field**

After the `Publisher` interface block (around line 33), add:

```go
// StatusWriter is implemented by *db.Repository. Hub uses it to persist
// online/offline status to the database on WS connect and disconnect.
type StatusWriter interface {
	SetUserStatusType(ctx context.Context, userID int64, statusType string) error
	SetUserStatusTypeIfNotInvisible(ctx context.Context, userID int64, statusType string) error
}
```

In the `Hub` struct (around line 59), add the field after `publisher Publisher`:

```go
// statusWriter persists online/offline status to the DB on connect/disconnect.
statusWriter StatusWriter
```

- [ ] **Step 2: Add `SetStatusWriter` setter (mirrors `SetPublisher` pattern)**

After the `SetPublisher` method (around line 107):

```go
// SetStatusWriter sets the StatusWriter used to persist online/offline status.
// Call this before starting the hub's Run loop.
func (h *Hub) SetStatusWriter(sw StatusWriter) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.statusWriter = sw
}
```

- [ ] **Step 3: DB write on first connection in `RegisterClient`**

In `RegisterClient`, after the `USER_ONLINE` broadcast block (after line ~217, before the closing `}`), add:

```go
// Persist 'online' status to DB on first connection (skip if invisible).
if isFirstConnection {
	uid, parseErr := strconv.ParseInt(client.userID, 10, 64)
	h.mu.RLock()
	sw := h.statusWriter
	h.mu.RUnlock()
	if parseErr == nil && sw != nil {
		go func(id int64) {
			if err := sw.SetUserStatusTypeIfNotInvisible(context.Background(), id, "online"); err != nil {
				log.Printf("hub: set online for user %d: %v", id, err)
			}
		}(uid)
	}
}
```

Add `"strconv"` and `"context"` to the hub.go import block if not already present.

- [ ] **Step 4: DB write on last disconnect in `UnregisterClient`**

In `UnregisterClient`, after the `USER_OFFLINE` broadcast block (after line ~274, before the closing `}`), add:

```go
// Persist 'offline' status to DB unconditionally on last disconnect.
if userFullyOffline {
	uid, parseErr := strconv.ParseInt(client.userID, 10, 64)
	h.mu.RLock()
	sw := h.statusWriter
	h.mu.RUnlock()
	if parseErr == nil && sw != nil {
		go func(id int64) {
			if err := sw.SetUserStatusType(context.Background(), id, "offline"); err != nil {
				log.Printf("hub: set offline for user %d: %v", id, err)
			}
		}(uid)
	}
}
```

- [ ] **Step 5: Build**

Run: `cd /home/dylan/Developer/parley && go build ./internal/websocket/...`
Expected: exits 0.

- [ ] **Step 6: Commit**

```bash
git add internal/websocket/hub.go
git commit -m "feat: hub writes online/offline status to DB on WS connect/disconnect"
```

---

## Task 3: Wire `statusWriter` into hub in `cmd/api/main.go`

**Files:**
- Modify: `cmd/api/main.go`

**Context:** `hub` is created in `cmd/api/main.go`, not in `routes.go`. The hub has `SetPublisher` called on it before `Run()`. We need to call `hub.SetStatusWriter(repo)` in the same place.

- [ ] **Step 1: Find where `SetPublisher` is called in main.go**

Run: `grep -n SetPublisher /home/dylan/Developer/parley/cmd/api/main.go`

- [ ] **Step 2: Add `hub.SetStatusWriter(repo)` immediately after `hub.SetPublisher`**

In `cmd/api/main.go`, after the line `hub.SetPublisher(redisHub)` (or after the conditional that calls it), add:

```go
hub.SetStatusWriter(repo)
```

This must be called before `go hub.Run()`.

- [ ] **Step 3: Build**

Run: `cd /home/dylan/Developer/parley && go build ./cmd/api/...`
Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add cmd/api/main.go
git commit -m "feat: wire repo as StatusWriter into hub"
```

---

## Task 4: Fix `handleUpdateStatus` — use `BroadcastStatusUpdate`, add `idle`, remove dead `repo` param

**Files:**
- Modify: `cmd/api/user_status_handler.go`
- Modify: `cmd/api/routes.go` (update call site to remove `repo` argument)

**Context:** Three things to fix: (1) broadcast must use `hub.BroadcastStatusUpdate` not per-server `BroadcastToChannel`; (2) `authService.UpdateStatus` only accepts `online/dnd/afk/invisible` — not `idle`. The handler must do its own validation and call `repo.UpdateUserStatus` directly to correctly support all spec-required values; (3) remove the now-unused `authService` and `repo` params (handler uses `repo.UpdateUserStatus` directly — keep `repo`, drop `authService`).

The `PATCH /users/@me/status` route already uses `auth.AuthMiddlewareWith(authService)` which accepts both JWT and `plk_...` bot keys (see `middleware.go:65`). No route auth change needed.

- [ ] **Step 1: Rewrite `handleUpdateStatus`**

Replace the entire content of `cmd/api/user_status_handler.go` with:

```go
package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"parley/internal/auth"
	"parley/internal/db"
	ws "parley/internal/websocket"
)

func handleUpdateStatus(hub *ws.Hub, repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			StatusType string `json:"status_type"`
			StatusText string `json:"status_text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		// Reject offline — only the hub can write that.
		if req.StatusType == "offline" {
			jsonError(w, "offline status is managed by the server", http.StatusBadRequest)
			return
		}

		// Validate allowed values. "idle" is included (authService.UpdateStatus
		// uses an older allowlist that omits it, so we call repo directly).
		switch req.StatusType {
		case "online", "idle", "dnd", "invisible":
			// valid
		default:
			jsonError(w, "invalid status type", http.StatusBadRequest)
			return
		}

		// Trim status_text to 128 chars.
		if len(req.StatusText) > 128 {
			req.StatusText = req.StatusText[:128]
		}
		// Trim any trailing multi-byte boundary issues from the 128-char cut.
		req.StatusText = strings.TrimSpace(req.StatusText)

		userID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if err := repo.UpdateUserStatus(r.Context(), userID, req.StatusType, req.StatusText); err != nil {
			jsonError(w, "failed to update status", http.StatusInternalServerError)
			return
		}

		// Broadcast USER_STATUS_UPDATE cross-node via BroadcastStatusUpdate.
		if hub != nil {
			hub.BroadcastStatusUpdate(userIDStr, req.StatusType, req.StatusText)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status_type": req.StatusType,
			"status_text": req.StatusText,
		})
	}
}
```

- [ ] **Step 2: Update call site in `routes.go`**

Find `handleUpdateStatus(authService, hub, repo)` at `routes.go:277`.

Change to:
```go
r.Patch("/users/@me/status", handleUpdateStatus(hub, repo))
```

- [ ] **Step 3: Build**

Run: `cd /home/dylan/Developer/parley && go build ./cmd/api/...`
Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add cmd/api/user_status_handler.go cmd/api/routes.go
git commit -m "fix: handleUpdateStatus supports idle, broadcasts USER_STATUS_UPDATE cross-node, rejects offline"
```

---

## Task 5: Add `GET /api/users/me` and `PATCH /api/users/me` handlers

**Files:**
- Modify: `cmd/api/user_handlers.go`
- Modify: `cmd/api/routes.go`

**Context:** The existing `/api/auth/me` (line 118 in routes.go) returns `auth.User`. The new `/api/users/me` returns a richer response including `status_type`/`status_text`. `handleUpdateProfile` at line 126 shows the broadcast pattern for `USER_UPDATE`.

- [ ] **Step 1: Add `userMeResponse` helper and two handlers in `user_handlers.go`**

Append at end of `cmd/api/user_handlers.go`:

```go
// userMeResponse builds the JSON body for GET/PATCH /api/users/me.
func userMeResponse(u *db.User) map[string]interface{} {
	return map[string]interface{}{
		"id":             fmt.Sprintf("%d", u.ID),
		"username":       u.Username,
		"display_name":   u.DisplayName,
		"avatar_url":     u.AvatarURL,
		"banner_url":     u.BannerURL,
		"bio":            u.Bio,
		"badges":         u.Badges,
		"email_verified": u.EmailVerified,
		"status_type":    u.StatusType,
		"status_text":    u.StatusText,
	}
}

// handleGetMeSelf handles GET /api/users/me — returns the full profile for the
// authenticated identity (JWT or bot API key).
func handleGetMeSelf(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var id int64
		fmt.Sscan(userIDStr, &id)
		user, err := repo.GetUserByID(r.Context(), id)
		if err != nil {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(userMeResponse(user))
	}
}

// handlePatchMe handles PATCH /api/users/me — updates username, display_name,
// and/or avatar_url. Password and email changes are ignored.
func handlePatchMe(repo *db.Repository, hub *ws.Hub, cdnHost string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Username    *string `json:"username"`
			DisplayName *string `json:"display_name"`
			AvatarURL   *string `json:"avatar_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.AvatarURL != nil {
			if err := validateMediaURL(*req.AvatarURL, cdnHost); err != nil {
				jsonError(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		var userID int64
		fmt.Sscan(userIDStr, &userID)

		user, err := repo.GetUserByID(r.Context(), userID)
		if err != nil {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}

		if req.Username != nil && *req.Username != "" {
			user.Username = *req.Username
		}
		if req.DisplayName != nil {
			user.DisplayName = *req.DisplayName
		}
		if req.AvatarURL != nil {
			user.AvatarURL = *req.AvatarURL
		}

		if err := repo.UpdateUserFields(r.Context(), userID, user.Username, user.DisplayName, user.AvatarURL); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Broadcast USER_UPDATE to all servers the user belongs to.
		if hub != nil {
			servers, serversErr := repo.GetServersByUserID(r.Context(), userID)
			if serversErr == nil {
				payload, marshalErr := json.Marshal(map[string]string{
					"user_id":      userIDStr,
					"username":     user.Username,
					"display_name": user.DisplayName,
					"avatar_url":   user.AvatarURL,
				})
				if marshalErr == nil {
					for _, srv := range servers {
						hub.BroadcastToChannel(fmt.Sprintf("server:%d", srv.ID), ws.EventUserUpdate, payload)
					}
				}
			}
		}

		updated, _ := repo.GetUserByID(r.Context(), userID)
		if updated == nil {
			updated = user
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(userMeResponse(updated))
	}
}
```

- [ ] **Step 2: Add `UpdateUserFields` to `internal/db/user_repository.go`**

Append:

```go
// UpdateUserFields updates username, display_name, and avatar_url for the given user.
// Used by PATCH /api/users/me.
func (r *Repository) UpdateUserFields(ctx context.Context, userID int64, username, displayName, avatarURL string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET username = $2, display_name = $3, avatar_url = $4, updated_at = NOW()
		 WHERE id = $1`,
		userID, username, displayName, avatarURL)
	return err
}
```

- [ ] **Step 3: Register the new routes in `routes.go`**

In the protected routes group (around line 274, near the existing `/users/@me/status` route), add:

```go
r.Get("/users/me", handleGetMeSelf(repo))
r.Patch("/users/me", handlePatchMe(repo, hub, cdnHost))
```

- [ ] **Step 4: Build**

Run: `cd /home/dylan/Developer/parley && go build ./cmd/api/...`
Expected: exits 0.

- [ ] **Step 5: Commit**

```bash
git add cmd/api/user_handlers.go internal/db/user_repository.go cmd/api/routes.go
git commit -m "feat: add GET/PATCH /api/users/me endpoints"
```

---

## Task 6: Typing endpoint — `POST /api/channels/{channelId}/typing`

**Files:**
- Create: `cmd/api/typing_handler.go` (spec named `message_handler.go` but that file doesn't exist; a dedicated file is cleaner)
- Modify: `cmd/api/routes.go`

**Context:** `ws.EventUserTyping = "USER_TYPING"`. The existing WS path broadcasts `{channel_id, user_id, username}`. The REST endpoint adds `expires_at` and `display_name`. `repo.GetChannelByID` returns a `*db.Channel` with a `.ServerID` field. `repo.GetMember(ctx, serverID, userID)` returns `*db.ServerMember` — if err is `db.ErrNotFound`, user is not a member.

User lookup for username/display_name: use `repo.GetUserByID`.

- [ ] **Step 1: Create `cmd/api/typing_handler.go`**

```go
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
	mu     sync.Mutex
	lastAt map[string]time.Time
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
```

- [ ] **Step 2: Register the route in `routes.go`**

In the protected routes group, near the channel routes (around line 197), add:

```go
r.Post("/channels/{channelId}/typing", handleChannelTyping(repo, hub))
```

- [ ] **Step 3: Build**

Run: `cd /home/dylan/Developer/parley && go build ./cmd/api/...`
Expected: exits 0.

- [ ] **Step 4: Run tests**

Run: `cd /home/dylan/Developer/parley && JWT_SECRET=test-secret go test ./...`
Expected: PASS (no existing typing tests will break).

- [ ] **Step 5: Commit**

```bash
git add cmd/api/typing_handler.go cmd/api/routes.go
git commit -m "feat: POST /api/channels/{id}/typing REST endpoint with duration + rate limiter"
```

---

## Task 7: Frontend — handle `expires_at` in `USER_TYPING` events

**Files:**
- Modify: `frontend/src/hooks/useWebSocket.ts`
- Modify: `frontend/src/App.tsx`

**Context:** `USER_TYPING` handler is at `useWebSocket.ts:241-247`. It calls `onTypingRef.current(payload.user_id, payload.username, payload.channel_id)`. The App.tsx `handleTyping` callback is at line 469, uses a hardcoded 3000ms timeout. We need to pass `expires_at` through and use it when present.

- [ ] **Step 1: Update `USER_TYPING` handler in `useWebSocket.ts`**

Find the block (lines 241-247):
```typescript
} else if (wsMsg.type === 'USER_TYPING' && onTypingRef.current) {
  if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
    const payload = wsMsg.payload as { user_id: string; username: string; channel_id: string };
    if (payload.user_id && payload.channel_id) {
      onTypingRef.current(payload.user_id, payload.username ?? '', payload.channel_id);
    }
  }
}
```

Replace with:
```typescript
} else if (wsMsg.type === 'USER_TYPING' && onTypingRef.current) {
  if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
    const payload = wsMsg.payload as { user_id: string; username: string; channel_id: string; expires_at?: string };
    if (payload.user_id && payload.channel_id) {
      onTypingRef.current(payload.user_id, payload.username ?? '', payload.channel_id, payload.expires_at);
    }
  }
}
```

Also update the `onTyping` prop type in the `UseWebSocketOptions` interface from:
```typescript
onTyping?: (userId: string, username: string, channelId: string) => void;
```
to:
```typescript
onTyping?: (userId: string, username: string, channelId: string, expiresAt?: string) => void;
```

- [ ] **Step 2: Update `handleTyping` in `App.tsx` to use `expires_at`**

Find `handleTyping` (around line 469):
```typescript
const handleTyping = useCallback((userId: string, username: string, channelId: string) => {
  if (userId === currentUser?.id) return;
  const key = `${channelId}:${userId}`;

  const existing = typingTimeoutsRef.current.get(key);
  if (existing) clearTimeout(existing);

  setTypingUsers(prev => {
    ...
  });

  const timeout = setTimeout(() => {
    ...
  }, 3000);

  typingTimeoutsRef.current.set(key, timeout);
}, [currentUser?.id]);
```

Replace with:
```typescript
const handleTyping = useCallback((userId: string, username: string, channelId: string, expiresAt?: string) => {
  if (userId === currentUser?.id) return;
  const key = `${channelId}:${userId}`;

  const existing = typingTimeoutsRef.current.get(key);
  if (existing) clearTimeout(existing);

  setTypingUsers(prev => {
    const list = prev[channelId] ?? [];
    if (list.some(t => t.userId === userId)) return prev;
    return { ...prev, [channelId]: [...list, { userId, username }] };
  });

  // Use expires_at when present (REST path), fall back to 3s (WS path).
  let delay = 3000;
  if (expiresAt) {
    const ms = new Date(expiresAt).getTime() - Date.now();
    if (ms > 0) delay = ms;
  }

  const timeout = setTimeout(() => {
    setTypingUsers(prev => {
      const list = prev[channelId] ?? [];
      const filtered = list.filter(t => t.userId !== userId);
      if (filtered.length === 0) {
        const { [channelId]: _removed, ...rest } = prev;
        return rest;
      }
      return { ...prev, [channelId]: filtered };
    });
    typingTimeoutsRef.current.delete(key);
  }, delay);

  typingTimeoutsRef.current.set(key, timeout);
}, [currentUser?.id]);
```

- [ ] **Step 3: Build frontend**

Run: `cd /home/dylan/Developer/parley/frontend && npm run build`
Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/hooks/useWebSocket.ts frontend/src/App.tsx
git commit -m "feat: frontend handles expires_at in USER_TYPING for REST-based typing duration"
```

---

## Task 8: End-to-end build verification

- [ ] **Step 1: Full Go build**

Run: `cd /home/dylan/Developer/parley && go build ./...`
Expected: exits 0.

- [ ] **Step 2: Full tests**

Run: `cd /home/dylan/Developer/parley && JWT_SECRET=test-secret go test ./...`
Expected: all PASS.

- [ ] **Step 3: Frontend build**

Run: `cd /home/dylan/Developer/parley/frontend && npm run build`
Expected: exits 0.
