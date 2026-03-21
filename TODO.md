# Parley TODO

This is a living task list for Parley - a Discord clone.

## Infra — next actions

- [ ] **Dedicated Redis node** — move Redis off the DB droplet to reduce resource contention at high load. Redis is the cross-node WS fan-out bottleneck before Postgres is. Add `s-1vcpu-1gb` ($6/mo).
- [ ] **Downsize DB droplet** `s-2vcpu-4gb` → `s-1vcpu-2gb` ($24 → $12/mo) to pay for the Redis node. Adjust `userdata-db.sh`: `shared_buffers = 512MB`, `effective_cache_size = 1500MB`. Safe because Redis moving off frees RAM and PgBouncer session pooling caps server connections at 75.
- [ ] **Wire Redis node into Terraform** — add `redis_ip` var to `variables.tf` and `proxmox/variables.tf`, pass `REDIS_URL=redis://<redis_ip>:6379` to API userdata, move Redis install/config out of `userdata-db.sh` into a new `userdata-redis.sh`.
- [ ] **Update live servers** once Proxmox bench confirms 25k target — resize DB droplet, provision Redis droplet, update env on all 3 API nodes.

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
- [x] **Missing null check on `activeChannel` in AppContext** — `frontend/src/context/AppContext.tsx:257` — `receiveMessage` accesses `activeChannel.id` inside a condition that already checks `activeChannel`, but the channel can become `null` between re-renders and the state setter. Add defensive check.
- [x] **Missing React error boundary** — `frontend/src/App.tsx` — A runtime rendering error crashes the entire app with a blank screen. Wrap major sections in an error boundary.
- [x] **No message channel-name length validation** — `internal/channel/service.go` — Only checks for empty name; no max length. Very long names cause DB truncation or UI overflow.
- [x] **Large message offset queries unvalidated** — `internal/message/handler.go:94-107` — Limit is capped at 200 but offset has no upper bound, allowing arbitrarily expensive DB seeks. Use keyset/cursor pagination.
- [x] **Hardcoded Redis fallback is silent** — `internal/websocket/redis.go:41` — Falls back to `redis://localhost:6379` with no warning log; in a misconfigured deploy this means cross-node broadcasts silently fail.

### Low
- [x] **`UpdatedAt` not tracked for channels** — `internal/channel/service.go:172` — Channel struct populates `UpdatedAt` from `CreatedAt`. Add `updated_at` column.
- [x] **No exponential backoff on WebSocket reconnect** — `frontend/src/hooks/useWebSocket.ts:84` — Always reconnects after exactly 3 s. Should use exponential backoff with jitter. (Also listed under Infrastructure below.)
- [x] **Username length not validated on profile update** — `internal/auth/service.go:152-159` — No max-length check on username update; signup has a limit but profile update does not.
- [x] **No logging on WebSocket subscribe/unsubscribe failures** — `internal/websocket/client.go` — Silent failure makes subscription bugs hard to trace in logs.

---

## High Priority

### Completed
- [x] Redis pub/sub for cross-node WebSocket broadcasting (3 droplets: 138.197.83.70, 138.197.97.235, 165.227.121.15)
- [x] User settings context menu (click username at bottom-left shows menu)
- [x] User settings modal (change username/password)
- [x] Server member join events broadcast via WebSocket

### Pending

- [x] Right-click context menu on usernames in sidebar (user sidebar) — "Manage Roles" context menu added
- [x] Right-click context menu on usernames in chat (channel messages) — View Profile / Send Message popup on avatar/username right-click
- [x] DM from search doesn't show for the sender
- [x] User joining server doesn't refresh server sidebar for others immediately — fixed: event type casing mismatch corrected, handler now calls reloadMembers for active server
- [x] Message editing in voice channels
- [x] Delete/edit messages in VC

---

## Bots

- [x] **Hard limit: 10 active bots per user** — enforce in `POST /developer/keys` (bot creation path) by counting existing bots owned by the requesting user before inserting. Return 403 with a clear error if at limit.

---

## Medium Priority

### Features
- [ ] **LaTeX math rendering not working in production** — `remark-math` + `rehype-katex` are installed and the pipeline works in Node/SSR tests, but `$$x^2$$` shows as raw text in the live app. Plugins are in the bundle. Likely a runtime issue with how Vite bundles the unified ecosystem in production. Use `$$...$$` syntax (single dollar disabled to avoid false positives on `$5`).
- [x] Typing indicators
- [x] Unread message badges on servers and DM channels
- [x] Real-time online status indicators
- [x] Channel topics and descriptions
- [x] Message reactions
- [x] Emoji picker (full categorized picker with search, 8 categories)
- [x] Image/file upload in messages
- [x] Server pictures and user profile pictures (DigitalOcean Spaces)
- [x] User banners (banner now displays in profile modal)
- [x] Email verification (Brevo HTTP API)

### Infrastructure
- [x] CI/CD deploy script (auto-deploy on push to main)
- [x] WebSocket reconnection with exponential backoff

---

## Lower Priority

- [x] Voice channels/voice chat
- [x] Message search
- [x] User profile page with custom display name

---

## Feature Backlog

Features ordered roughly by complexity. Grouped by what they touch.

### Chat UX — frontend only (MessageList, MessageInput, Message.tsx)

- [x] **Scroll-to-bottom button** — floating "↓" button in MessageList when scrolled up; shows unread count badge when new messages arrive while scrolled up.
- [x] **Large emoji rendering** — messages containing only 1–5 emoji (no other text) render at 2× font size. Detection logic fixed in Message.tsx.
- [x] **Character counter in MessageInput** — show remaining chars when content exceeds 80% of the 4000-char limit. Red when at limit.
- [x] **Collapse long messages** — messages longer than ~20 lines are capped with a "See more" expand link. State per-message in component only.
- [x] **Copy message content** — "Copy Text" option in message right-click context menu; writes raw content to clipboard.

### Fixes

- [ ] **Fix LaTeX math rendering in production** — `remark-math` + `rehype-katex` are installed but `$$x^2$$` shows as raw text in the Vite build. Likely ESM/bundling issue — debug and fix. Touch: MarkdownRenderer.tsx, vite.config.ts.

### Bots

- [x] **Enforce 10-bot hard limit per user** — count existing bots owned by the user before creating a new one; return 403 if already at 10. Touch: internal/bots/handler.go or cmd/api/developer_handlers.go.
- [ ] **Bot API: soundboard triggering** — expose `POST /api/channels/{channelId}/soundboard/play` for bot auth; broadcasts `SOUNDBOARD_PLAY` WS event server-side without requiring a LiveKit connection. Touch: internal/soundboard/handler.go, cmd/api/routes.go.

### Message features — new backend table + endpoints + frontend (internal/message/, Message.tsx)

- [ ] **User and message reporting** — "Report" option in user right-click context menu and message context menu. Submits a report with a category (from existing `report_categories`) and optional description. `POST /api/reports`. Admin panel already has report viewing; this adds the frontend submission flow and backend endpoint.
- [ ] **Pinned messages** — `PermManageMessages` can pin/unpin. New `pinned_messages` table. `POST/DELETE /channels/{id}/pins/{messageId}`, `GET /channels/{id}/pins`. Pin indicator on messages; "📌 N pinned" button in channel header opens a panel.
- [x] **Forward message** — "Forward" in context menu opens a modal to pick a channel/DM and sends the content there (quoted or with a forward embed). No new backend endpoints needed.

### Moderation — touches server + permissions + message/voice middleware

- [ ] **Member timeout** — `PermModerateMember` can time out a user for a duration (1m/5m/10m/1h/1d). `timed_out_until` column on `server_members`; middleware blocks messages and voice while active. Duration picker in member context menu. Timeout badge in sidebar.
- [ ] **Server audit log** — structured log of privileged actions (kick/ban/timeout, role changes, channel changes, invite changes). New `server_audit_log` table. `GET /servers/{id}/audit-log` (paginated, filterable). Requires `PermViewAuditLog`. Audit Log tab in ServerSettings.

### Discovery

- [ ] **Server discovery / public servers list** — `is_public` + `description` on servers, admin-managed `server_categories`, junction table (max 3 per server). `GET /api/discover` (paginated, searchable, filterable by category). Globe icon in sidebar → DiscoveryPage. Server settings Overview tab gets description + is_public toggle + category picker. Spec: `docs/superpowers/specs/2026-03-21-server-discovery-design.md`.

### Large — significant new systems

- [x] **In-app notification center** — new `notifications` table. Notify on: @mention, reaction to your message, friend request, DM while away. `GET /notifications`, `PATCH /notifications/read-all`. Bell icon in sidebar with unread badge; WS push via `NOTIFICATION_CREATE`.
- [ ] **2FA / TOTP (Google Authenticator)** — `totp_secret` (encrypted), `totp_enabled`, `totp_backup_codes` on users. Setup flow: generate secret → show QR → confirm code → save backup codes. Login flow: detect `requires_2fa`, show TOTP challenge modal. Touch: internal/auth/, AccountTab.tsx.
- [ ] **Custom server emoji** — `server_emoji` table; `PermManageExpressions` to manage. Upload PNG/GIF ≤ 256 KB, 2–32 char name. Emoji picker shows custom emoji first. Messages resolve `:custom:` codes to inline `<img>`. Soundboard gets optional `custom_emoji_id` FK. Custom Emoji tab in ServerSettings.
- [ ] **Screen sharing / Go Live** — `getDisplayMedia()` → publish `LocalVideoTrack` via livekit-client. `PermStream` check on join. `SCREEN_SHARE_STARTED/STOPPED` WS events. ParticipantTile gains video element; layout shifts to large center tile when someone is sharing. "Go Live" button in VoiceControls.

---

## Code Health / Refactoring

These are growing-pains issues identified from a structural inspection — not bugs or features, just tech debt.

### Backend

- [x] **`cmd/api/routes.go` (929 lines)** — Split into `routes.go` (pure registration, ~140 lines) + `auth_handlers.go`, `user_handlers.go`, `developer_handlers.go`, `websocket_handler.go`, `upload_handler.go`, `helpers.go`.
- [x] **`internal/server/service.go` (957 lines)** — Split into `service.go` (types + helpers), `server_crud.go`, `server_members.go`, `server_invites.go`, `server_roles.go`.
- [x] **`cmd/admin/main.go` (765 lines)** — Split into `main.go` (CLI + globals), `server.go` (HTTP bootstrap + routes), `middleware.go` (JWT auth), `handlers.go` (all HTTP handlers), `helpers.go` (jsonOK/jsonError/queryInt).

### Frontend

- [ ] **`App.tsx` (854 lines, 50+ state vars)** — God component handling routing, WebSocket setup, all modal state, voice state, DM state, and server navigation. Should delegate to context/hooks more aggressively.
- [ ] **`AppContext.tsx` (630 lines)** — Global context doing everything: servers, channels, DMs, messages, voice, presence. Consider splitting into domain-specific contexts or a proper state management approach.
- [ ] **`UserSettings.tsx` (732 lines)** — Three tabs (Account, Profile, Developer) inlined into one file. Each tab should be its own component: `AccountTab.tsx`, `ProfileTab.tsx`, `DeveloperTab.tsx`.
- [ ] **`ServerSettings.tsx` (704 lines)** — Same problem as UserSettings — multiple settings sections should be extracted into sub-components.
- [ ] **`ChannelList.tsx` (479 lines)** — Contains 3 separate context menus, server list, channel hierarchy, and voice bar. Should be broken into focused sub-components.

### CSS

- [ ] **`ui/styles.css` (1,794 lines)** — Monolithic file mixing global resets, utility classes, and dozens of component styles. Should be split per component or migrated to CSS Modules.
- [ ] **`chat/Chat.css` (1,056 lines)** — Imported by 4 separate components (ChatWindow, DmChat, MessageInput, MessageList). Consider splitting into per-component files.
- [ ] **`settings/Settings.css` (1,092 lines)** — Shared by UserSettings and ServerSettings. Side-effect coupling: changing one component's styles risks breaking the other.
- [ ] **`layout/ChannelList.css` (719 lines)** — Excessive for a single component; inline context menu styles and voice bar styles should be co-located with those sub-components when extracted.

---

## Security Suggestions (from 2026-03-16 audit)

Lower-priority hardening items. Criticals and Importants have been resolved.

- [x] **S1 — Dependency CVE scanning** — Add `govulncheck ./...` to CI. Watch: `gorilla/websocket`, `golang-jwt/jwt`, `lib/pq`, `aws-sdk-go-v2`.
- [x] **S2 — bcrypt 72-byte truncation** — bcrypt silently truncates at 72 bytes. Document and enforce max password length of 72 chars at registration and password-change (`internal/auth/service.go`).
- [x] **S3 — Invite code entropy** — Codes are 32-bit (4 bytes). Increase to 6+ bytes in `internal/server/service.go:generateInviteCode`. Also rate-limit `GET /invites/{code}`.
- [x] **S4 — JWT in WebSocket URL query param is logged** — `wss://...?token=<JWT>` appears in Chi/nginx logs. Mitigation: exchange JWT for a short-lived (60s) single-use WS ticket via `POST /api/ws-ticket`.
- [x] **S5 — Upload keys use nanosecond timestamps (guessable)** — Replace `time.Now().UnixNano()` in `cmd/api/helpers.go:generateID` with `crypto/rand`.
- [x] **S6 — Phone number in localStorage** — `GET /auth/me` returns phone_number which lands in localStorage. Fetch on-demand in settings page instead.
- [x] **S7 — Security response headers** — Verify nginx sets `Content-Security-Policy`, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`.
- [x] **S8 — No minimum password length** — Add 8-char minimum at registration in `internal/auth/service.go`.
- [x] **S9 — ADMIN_IMPERSONATE_SECRET has no startup warning** — Unlike JWT_SECRET there's no log warning when unset. Add one so operators know the feature is inactive.
- [x] **S10 — Message content max length** — Only the 64KB global body cap applies. Add explicit max (e.g. 4000 chars) in `internal/message/service.go` and `internal/dm/handler.go`.

---

## Already Implemented but Need Verification/Adjustment

- [x] User-profile-username CSS (duplicate rule fixed — `margin: 25px 0 0` now applies correctly)
- [x] Server settings modal (delete button works with confirmation step)
- [x] Channel URLs (URL-based navigation — refresh preserves server/channel/DM position)
