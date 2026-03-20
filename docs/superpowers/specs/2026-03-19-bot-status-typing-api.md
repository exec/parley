# Bot Status, Presence & API Expansion Design

## Goal

Make bot online indicators reflect ground-truth WebSocket connectivity, expose a suite of self-management API endpoints usable with bot API keys, add a timed typing REST endpoint, and sync parley.py with all of these additions plus recent service changes.

## Architecture

**Ground-truth presence via hub:** The WebSocket hub becomes the authority on whether a user (bot or human) is online. On first WS connection it writes `status_type = "online"` to the DB; on last WS disconnection (including crashes) it writes `status_type = "offline"`. No client can claim to be online without an active connection.

**Status expression via REST:** Bots (and users) can call `PATCH /api/users/me/status` to set `status_type` to `online`, `idle`, `dnd`, or `invisible`. Setting `offline` is rejected — only the hub can write that. This allows bots to signal degradation via `dnd` while remaining connected.

**parley.py:** Wraps all new endpoints, manages status state across reconnects, and is synced with endpoint changes made since the last library update.

**Tech Stack:** Go (backend), Python 3.10+ (parley.py), existing PostgreSQL `users` table (`status_type`, `status_text` columns already present).

---

## Backend Changes

### 1. DB Repository — new method

**File:** `internal/db/user_repository.go`

Add:
```go
func (r *Repository) SetUserStatusType(ctx context.Context, userID int64, statusType string) error {
    _, err := r.db.ExecContext(ctx,
        `UPDATE users SET status_type = $2, updated_at = NOW() WHERE id = $1`,
        userID, statusType)
    return err
}
```

This is a narrow write used only by the hub for the offline transition. For the online transition, the SQL is inlined in the hub (see §2) to use a conditional `WHERE` clause.

### 2. WebSocket Hub — server-enforced online/offline

**File:** `internal/websocket/hub.go`

Add a `StatusWriter` interface to the websocket package:
```go
type StatusWriter interface {
    SetUserStatusType(ctx context.Context, userID int64, statusType string) error
}
```

Add `statusWriter StatusWriter` field to `Hub`. Pass `repo` (the `*db.Repository`) at construction in `cmd/api/routes.go` — `db.Repository` satisfies `StatusWriter` via the method added in §1.

**`RegisterClient`** — after broadcasting `USER_ONLINE`, if this is the user's first connection, fire a goroutine:
```go
go func(uid int64) {
    _, err := h.db.ExecContext(context.Background(),
        `UPDATE users SET status_type = 'online', updated_at = NOW()
         WHERE id = $1 AND status_type != 'invisible'`, uid)
    if err != nil {
        log.Printf("hub: set online for user %d: %v", uid, err)
    }
}(numericUserID)
```
Using a raw DB exec here (rather than the `StatusWriter` method) because this is a conditional write that benefits from a single SQL statement. The `Hub` already holds a `*db.Repository` via `StatusWriter`; call `h.statusWriter` directly and expose the conditional write as a second method `SetUserStatusTypeIfNotInvisible` on the repository, or inline it. Prefer the second repository method for testability:

```go
// SetUserStatusTypeIfNotInvisible sets status_type only when the current value
// is not 'invisible'. Used by the hub on first WS connection.
func (r *Repository) SetUserStatusTypeIfNotInvisible(ctx context.Context, userID int64, statusType string) error {
    _, err := r.db.ExecContext(ctx,
        `UPDATE users SET status_type = $2, updated_at = NOW()
         WHERE id = $1 AND status_type != 'invisible'`, userID, statusType)
    return err
}
```

Update `StatusWriter` interface to include this method.

**`UnregisterClient`** — after broadcasting `USER_OFFLINE`, if this is the user's last connection:
```go
go func(uid int64) {
    if err := h.statusWriter.SetUserStatusType(context.Background(), uid, "offline"); err != nil {
        log.Printf("hub: set offline for user %d: %v", uid, err)
    }
}(numericUserID)
```
Unconditional — offline always wins on last disconnect.

### 3. `PATCH /api/users/me/status` — fix broadcast + accept bot API keys

**File:** `cmd/api/user_status_handler.go`

**Existing broadcast bug:** The current `handleUpdateStatus` broadcasts `EventUserUpdate` via `hub.BroadcastToChannel` per server. This must be replaced with `hub.BroadcastStatusUpdate(userIDStr, statusType, statusText)`, which broadcasts `USER_STATUS_UPDATE` to all nodes via Redis. This corrects the event type for all callers (users and bots), not just bots.

The corrected handler (simplified):
1. Parse and validate body (`status_type` ∈ {`online`, `idle`, `dnd`, `invisible`}; reject `offline` with HTTP 400 `{"error": "offline status is managed by the server"}`).
2. Trim `status_text` to 128 chars.
3. Update DB via `repo.UpdateUserStatus(ctx, userID, statusType, statusText)`.
4. Call `hub.BroadcastStatusUpdate(userIDStr, statusType, statusText)`.
5. Return HTTP 200 with the updated status.

**Bot-key auth:** Apply the same bot-key auth middleware used by message-sending routes so that `plk_...` tokens resolve to the bot's `userID` via context. The handler logic is identical for both JWT and bot-key callers.

### 4. New routes: `GET /api/users/me` and `PATCH /api/users/me`

**File:** `cmd/api/user_handlers.go` and `cmd/api/routes.go`

These are **new routes** (the existing self-endpoints are at `/api/auth/me` and `/api/auth/profile` — those are unchanged). Register the new routes in the section of `routes.go` that handles bot-key-aware endpoints, under a middleware that accepts both JWT and bot API keys.

`GET /api/users/me`:
- Return the full `User` object for the authenticated identity.
- Look up user by `userID` extracted from context.

`PATCH /api/users/me`:
- Accepted fields: `username`, `display_name`, `avatar_url`. All optional.
- `avatar_url`: apply the same CDN-host validation as `handleUpdateProfile` (`validateMediaURL` helper). Return HTTP 400 if invalid.
- `password`, `email`, and any other fields are silently ignored.
- On success: update DB, broadcast `USER_UPDATE` event per server (existing pattern from `handleUpdateProfile`).
- Return the updated `User` object.

### 5. `POST /api/channels/{channelId}/typing` — new REST endpoint

**File:** `cmd/api/message_handler.go`

Auth: JWT or bot API key (same dual-auth middleware).

Permission check: caller must be a member of the channel's server. Return HTTP 404 if channel not found. Return HTTP 403 if caller is not a member.

Request body:
```json
{ "duration": 10 }
```
- `duration`: integer seconds, optional, default `5`. Server clamps to `[1, 60]` — below 1 becomes 1, above 60 becomes 60. No error returned.

**Rate limit:** Use a package-level `typingRateLimiter` — a struct wrapping a `map[string]time.Time` with a `sync.RWMutex` (not `sync.Map`, which is untyped and requires type assertions — prefer the explicit mutex approach for clarity):

```go
type typingRateLimiter struct {
    mu      sync.Mutex
    lastAt  map[string]time.Time
}

func (t *typingRateLimiter) allow(key string, cooldown time.Duration) bool {
    t.mu.Lock()
    defer t.mu.Unlock()
    if last, ok := t.lastAt[key]; ok && time.Since(last) < cooldown {
        return false
    }
    t.lastAt[key] = time.Now()
    return true
}
```

Key: `fmt.Sprintf("%d:%d", userID, channelID)`. Cooldown: the clamped `duration` of the previous accepted request. The limiter is initialized once at startup and shared across all requests.

Response: HTTP 204 No Content on success.

Broadcast a `TYPING` event to channel subscribers:
```json
{
  "type": "TYPING",
  "payload": {
    "channel_id": "123",
    "user_id": "456",
    "username": "MyBot",
    "display_name": "My Bot",
    "expires_at": "2026-03-19T12:00:10Z"
  }
}
```
`expires_at` = `time.Now().UTC().Add(time.Duration(clampedDuration) * time.Second).Format(time.RFC3339)`.

**Frontend change** (`frontend/src/hooks/useWebSocket.ts` or wherever `TYPING` is handled): when a `TYPING` event arrives with `expires_at` present, clear the indicator at `expires_at` rather than a fixed 5-second timeout. Fall back to the 5-second timer when `expires_at` is absent (WS-sent frames, which remain unchanged).

**Existing WS typing path is unchanged.** The REST endpoint is additive.

---

## parley.py Changes

### ConnectionState Back-Reference

**`extras/parley/state.py`** — add `self._client = None` to `ConnectionState.__init__`. This is set from `client.py` immediately after constructing `ConnectionState`:

```python
# In Client.__init__, after: self._state = ConnectionState(...)
self._state._client = self
```

This back-reference allows `_on_gateway_connected` in `state.py` to call `await self._client._reapply_status()`.

### Status State Management

**`extras/parley/client.py`**

Add to `Client.__init__`:
```python
self._status_type: str = "online"
self._status_text: str = ""
self._degraded: bool = False
```

Add:
```python
async def _reapply_status(self) -> None:
    """Re-apply non-default status after reconnect. Called by state before on_ready."""
    if self._status_type != "online":
        try:
            await self.http.set_status(self._status_type, self._status_text)
        except Exception as e:
            log.warning(f"Failed to re-apply status on reconnect: {e}")

async def set_status(self, status_type: str, text: str = "") -> None:
    """Set status. status_type: online | idle | dnd | invisible"""
    await self.http.set_status(status_type, text)
    self._status_type = status_type
    self._status_text = text

async def set_degraded(self, degraded: bool, reason: str = "") -> None:
    """
    Convenience wrapper for signalling bot health to users.
    True  → DND with optional reason text.
    False → online, status text cleared.
    Intentionally resets to 'online' unconditionally (not idle/invisible).
    State persists across reconnects.
    """
    self._degraded = degraded
    if degraded:
        await self.set_status("dnd", reason)
    else:
        await self.set_status("online", "")
```

**`extras/parley/state.py`** — in `_on_gateway_connected` (existing signature: `async def _on_gateway_connected(self) -> None`, no payload param), insert the `_reapply_status` call before the existing `_dispatch_event("READY", {})` call. Do not change the method signature or the dispatch call:

```python
async def _on_gateway_connected(self) -> None:
    # ... existing cache population (unchanged) ...
    if self._client is not None:
        await self._client._reapply_status()
    await self._dispatch_event("READY", {})   # unchanged — fires user's on_ready
```

This ensures status is restored before user code runs in `on_ready`. If `_client` is `None` (e.g. in tests that construct `ConnectionState` directly), the call is safely skipped.

### New HTTP Methods

**`extras/parley/http.py`**

Update the existing `get_me` in-place (do not add a new method or alias — `_on_gateway_connected` in `state.py` calls `self.http.get_me()` on every connect and must transparently pick up the new route):

```python
async def get_me(self) -> dict:
    return await self.request("GET", "/users/me")   # was /auth/me — updated in-place
```

Add:
```python
async def edit_me(self, **fields) -> dict:
    allowed = {"username", "display_name", "avatar_url"}
    body = {k: v for k, v in fields.items() if k in allowed}
    return await self.request("PATCH", "/users/me", json=body)

async def set_status(self, status_type: str, text: str = "") -> None:
    await self.request("PATCH", "/users/me/status", json={
        "status_type": status_type,
        "status_text": text,
    })

async def send_typing(self, channel_id: int, duration: int = 5) -> None:
    await self.request("POST", f"/channels/{channel_id}/typing", json={
        "duration": max(1, min(60, duration)),
    })

async def update_bot_invite(self, bot_id: int, *, permissions: int = None,
                            show_author: bool = None) -> dict:
    body = {}
    if permissions is not None:
        body["permissions"] = permissions
    if show_author is not None:
        body["show_author"] = show_author
    return await self.request("PATCH", f"/developer/bots/{bot_id}/invite", json=body)
```

### Client-Level Wrappers

**`extras/parley/client.py`**

Update the existing `fetch_me()` in-place (it already exists; update it to use the new route via `http.get_me()` which is updated in-place above):
```python
async def fetch_me(self) -> "ClientUser":
    data = await self.http.get_me()
    self.user = ClientUser._from_data(data, self._state)
    return self.user
```

Add:
```python
async def edit_profile(self, **fields) -> "ClientUser":
    """Allowed fields: username, display_name, avatar_url"""
    data = await self.http.edit_me(**fields)
    self.user = ClientUser._from_data(data, self._state)
    return self.user

async def send_typing(self, channel_id: int, duration: int = 5) -> None:
    await self.http.send_typing(channel_id, duration)
```

`fetch_me()` is called automatically during `start()` before the gateway connects, so `bot.user` is populated before `on_ready` fires.

### Typing Context Manager — Bot vs WS Branch

**`extras/parley/models/channel.py`** — `Typing.__init__` gains `send_fn: Callable` and `interval: float` parameters:

```python
class Typing:
    def __init__(self, channel_id: int, send_fn: Callable[[], Awaitable[None]],
                 interval: float = 5.0):
        self._channel_id = channel_id
        self._send_fn = send_fn
        self._interval = interval
        self._task: Optional[asyncio.Task] = None

    async def _loop(self):
        while True:
            await self._send_fn()
            await asyncio.sleep(self._interval)
    ...
```

`TextChannel.typing(duration: int = 5)` constructs `send_fn` and `interval` depending on whether the client is a `Bot`:

```python
def typing(self, duration: int = 5) -> "Typing":
    client = self._state._client
    if client is not None and isinstance(client, Bot):
        # REST path: send once, then re-send just before expiry (duration - 2s buffer).
        # Minimum interval is 3s to avoid hammering the endpoint.
        interval = float(max(3, duration - 2))
        send_fn = lambda: client.send_typing(self.id, duration)
    else:
        # WS path: server expires typing in ~5s, so resend every 5s.
        interval = 5.0
        send_fn = lambda: self._state.gateway.send_json(
            {"type": "TYPING", "payload": {"channel_id": str(self.id)}}
        )
    return Typing(self.id, send_fn, interval)
```

`Bot` is imported at the call site using `TYPE_CHECKING` to avoid circular imports. Non-Bot clients continue using the WS TYPING frame path unchanged.

### State — BOT_STATUS_UPDATE

**`extras/parley/state.py`**

`BOT_STATUS_UPDATE` payload from server:
```json
{ "server_id": 123, "bot_user_id": 456, "is_degraded": true }
```

Update the existing `_handle_bot_status_update` (currently a pass-through) to:
```python
def _handle_bot_status_update(self, payload: dict) -> None:
    server_id = int(payload["server_id"])
    bot_user_id = int(payload["bot_user_id"])
    is_degraded = payload.get("is_degraded", False)
    key = (server_id, bot_user_id)
    member = self.members.get(key)
    if member:
        member.bot_degraded = is_degraded
    self.dispatch("bot_status_update", payload)
```

Note: `BOT_STATUS_UPDATE` carries `is_degraded` (the AI-bot-specific degraded flag from `server_bots`), not `status_type`. General bot `status_type` changes arrive via the existing `USER_STATUS_UPDATE` event and are handled by the existing `_handle_user_status_update`.

### Model Updates

**`extras/parley/models/member.py`**: add `bot_degraded: bool = False`, populated from `is_degraded` in member payloads.

**`extras/parley/models/channel.py`**:
- Add `channel_type: int = 0` to `Channel`.
- Add property `is_bin(self) -> bool: return self.channel_type == 2`.
- Module-level constants: `CHANNEL_TYPE_TEXT = 0`, `CHANNEL_TYPE_VOICE = 1`, `CHANNEL_TYPE_BIN = 2`.

**`extras/parley/models/user.py`**: add `status_type: str = "offline"` and `status_text: str = ""` to `User`/`ClientUser`, populated from user payload.

---

## Documentation Updates

**`docs/bots.md`** — add after the "Sending Messages" section:

### Bot Status & Presence

Bots appear **online** to server members when a WebSocket client (e.g. parley.py) is actively connected. When the client disconnects — including on crash — the bot shows as **offline**. This is enforced server-side.

**Setting status**

```python
await bot.set_status("dnd", "Rate limit reached — retrying in 60s")
await bot.set_status("idle")
await bot.set_status("online")
```

Or directly:
```http
PATCH /api/users/me/status
Authorization: Bearer plk_your_api_key
{"status_type": "dnd", "status_text": "Service degraded"}
```

Allowed values: `online`, `idle`, `dnd`, `invisible`. Setting `offline` returns HTTP 400.

**Best practice: DND on degradation**

```python
async def on_error(self, error):
    await bot.set_degraded(True, "External API unavailable")
    # ... retry logic ...
    await bot.set_degraded(False)
```

`set_degraded` state persists across reconnects. This is optional but encouraged — it trains users to check the bot's status indicator before filing reports.

---

## Error Handling Summary

| Scenario | Response |
|---|---|
| `status_type = "offline"` on `PATCH /api/users/me/status` | HTTP 400: `offline status is managed by the server` |
| `POST /api/channels/{id}/typing` — channel not found | HTTP 404 |
| `POST /api/channels/{id}/typing` — caller not a server member | HTTP 403 |
| `POST /api/channels/{id}/typing` — rate limited | HTTP 429 |
| `PATCH /api/users/me` with invalid `avatar_url` | HTTP 400 |
| Hub `SetUserStatusType` DB error | Logged, not surfaced to WS client |
| parley.py `_reapply_status` fails on reconnect | Logged as warning; `on_ready` still fires |

---

## What Is Not Changing

- Existing `/api/auth/me` (GET) and `/api/auth/profile` (PUT) routes — untouched.
- WS `TYPING` frame path — unchanged; REST endpoint is additive.
- `is_degraded` on `server_bots` (Polly AI bot flag) — separate from `status_type`, untouched.
- No frontend UI changes beyond `expires_at` handling in the TYPING event.
- Non-Bot parley.py clients (`Selfbot`, `CommandBot` on selfbot) continue using the WS typing frame.
