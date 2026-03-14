# Parley TODO

This is a living task list for Parley - a Discord clone.

## Bugs

### Critical
- [x] **Double-close panic on WebSocket client** — `websocket/hub.go:101,188,241` — `UnregisterClient` closes `client.send`, but `SendToUser`/`BroadcastToChannel` also close it when the send buffer is full, and `WritePump` has its own defer close. Race between these paths will panic. Fix: use `sync.Once` to guard the close.
- [x] **Any server member can delete any channel** — `channel/handler.go` — `DeleteChannel` handler never checks that the requesting user is the server owner before deleting. Any member can nuke channels.
- [x] **JWT falls back to known weak secret** — `internal/auth/config.go:18` — If `JWT_SECRET` env var is unset the code falls back to the string `"parley-secret-key-change-in-production"`. Should panic/fatal on startup instead.
- [x] **WebSocket CheckOrigin disabled** — `cmd/api/routes.go:203` — `CheckOrigin` always returns `true`, allowing cross-site WebSocket hijacking (CSWSH). Should validate `Origin` against an allowlist matching CORS config.

### High
- [x] **SendMessage silently drops author username on DB error** — `internal/message/service.go:86` — `Scan(&authorUsername)` error is ignored with `_`. If the query fails the message broadcasts with an empty username and no error is surfaced.
- [x] **DM message broadcast payload format wrong** — `internal/dm/handler.go` — DM message broadcast marshals an intermediate `event` map rather than the `DmMessage` struct directly. Client-side handler may not receive sender info correctly.
- [x] **No rate limiting on auth endpoints** — `cmd/api/routes.go:40-43` — `/api/auth/register` and `/api/auth/login` have no rate limit, enabling brute-force and account enumeration.
- [x] **No request body size limit** — `cmd/api/main.go` — No `MaxHeaderBytes` or `http.MaxBytesReader` set; large payloads can DOS the API.
- [x] **LIKE metachar injection in user search** — `internal/db/repository.go` — Search uses `ILIKE $1` with raw user input (parameterized, so not SQL-injectable, but `%` and `_` in the input bypass the intended prefix-search semantics).

### Medium
- [ ] **Missing null check on `activeChannel` in AppContext** — `frontend/src/context/AppContext.tsx:257` — `receiveMessage` accesses `activeChannel.id` inside a condition that already checks `activeChannel`, but the channel can become `null` between re-renders and the state setter. Add defensive check.
- [x] **Missing React error boundary** — `frontend/src/App.tsx` — A runtime rendering error crashes the entire app with a blank screen. Wrap major sections in an error boundary.
- [x] **No message channel-name length validation** — `internal/channel/service.go` — Only checks for empty name; no max length. Very long names cause DB truncation or UI overflow.
- [ ] **Large message offset queries unvalidated** — `internal/message/handler.go:94-107` — Limit is capped at 200 but offset has no upper bound, allowing arbitrarily expensive DB seeks. Use keyset/cursor pagination.
- [x] **Hardcoded Redis fallback is silent** — `internal/websocket/redis.go:41` — Falls back to `redis://localhost:6379` with no warning log; in a misconfigured deploy this means cross-node broadcasts silently fail.

### Low
- [ ] **`UpdatedAt` not tracked for channels** — `internal/channel/service.go:172` — Channel struct populates `UpdatedAt` from `CreatedAt`. Add `updated_at` column.
- [ ] **No exponential backoff on WebSocket reconnect** — `frontend/src/hooks/useWebSocket.ts:84` — Always reconnects after exactly 3 s. Should use exponential backoff with jitter. (Also listed under Infrastructure below.)
- [ ] **Username length not validated on profile update** — `internal/auth/service.go:152-159` — No max-length check on username update; signup has a limit but profile update does not.
- [ ] **No logging on WebSocket subscribe/unsubscribe failures** — `internal/websocket/client.go` — Silent failure makes subscription bugs hard to trace in logs.

---

## High Priority

### Completed
- [x] Redis pub/sub for cross-node WebSocket broadcasting (3 droplets: 138.197.83.70, 138.197.97.235, 165.227.121.15)
- [x] User settings context menu (click username at bottom-left shows menu)
- [x] User settings modal (change username/password)
- [x] Server member join events broadcast via WebSocket

### Pending

- [ ] Right-click context menu on usernames in chat (channel messages)
- [ ] Right-click context menu on usernames in sidebar (user sidebar)
- [ ] DM from search doesn't show for the sender
- [ ] User joining server doesn't refresh server sidebar for others immediately
- [ ] Message editing in voice channels
- [ ] Delete/edit messages in VC

---

## Medium Priority

### Features
- [ ] Typing indicators
- [ ] Unread message badges on servers and DM channels
- [ ] Real-time online status indicators
- [ ] Channel topics and descriptions
- [ ] Message reactions
- [ ] Emoji picker
- [ ] Image/file upload in messages
- [ ] Server pictures and user profile pictures (DigitalOcean Spaces)
- [ ] User banners (PNG/JPG/animated GIF support with ideal size recommendation)

### Infrastructure
- [ ] CI/CD deploy script (auto-deploy on push to main)
- [ ] WebSocket reconnection with exponential backoff

---

## Lower Priority

- [ ] Voice channels/voice chat (high priority but complex - need voice server architecture decision)
- [ ] Server discovery / public servers list
- [ ] Message search
- [ ] Notification system (browser push + in-app)
- [ ] User profile page with custom display name

---

## Future / Large Projects

- [ ] Admin panel for service administration
  - Full observability and administrative capabilities
  - Ban users (dissolve accounts with funny error message)
  - View logs/metrics

- [ ] Server-wide permissions/privileges system
  - Tab in server settings to control these
  - Multiple roles per user
  - Custom role interface (not browser dropdown)

- [ ] Passkey authentication
  - Configurable in user settings
  - Logic to determine passkey vs password before prompting

- [ ] 2FA (Google Authenticator first)
  - Large undertaking

- [ ] Message search in messages

---

## Already Implemented but Need Verification/Adjustment

- [ ] User-profile-username CSS (top margin issue - user fixed, need verification)
- [ ] Server settings modal (delete button working?)
- [ ] Channel URLs (URL-based navigation - refresh preserves position)
