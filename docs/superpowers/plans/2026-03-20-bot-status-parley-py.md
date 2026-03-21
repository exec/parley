# Bot Status, Presence & API Expansion — parley.py Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sync parley.py with all new API endpoints, add status lifecycle management across reconnects, fix the `Typing` context manager to use the REST path for Bot clients, add `status_type`/`status_text` to user models, and write the bots.md docs section.

**Architecture:** `ConnectionState` gets a `_client` back-reference so it can call `_reapply_status()` before firing `on_ready`. `HTTPClient` gets new methods for the new routes. `Client` tracks `_status_type`, `_status_text`, `_degraded`. The `Typing` class is refactored to accept `send_fn` + `interval` so `TextChannel.typing()` can branch on `Bot` vs non-`Bot`.

**Tech Stack:** Python 3.10+, asyncio, httpx. No new dependencies.

---

## File Map

| Action | File |
|--------|------|
| Modify | `extras/parley/state.py` — add `_client = None`, update `_on_gateway_connected`, fix `_handle_bot_status_update` |
| Modify | `extras/parley/client.py` — set `_state._client = self`, add status fields + methods |
| Modify | `extras/parley/http.py` — update `get_me`, add `edit_me`, `set_status`, `send_typing`, `update_bot_invite` |
| Modify | `extras/parley/models/user.py` — add `status_type`/`status_text` to `User` and `PublicUser` |
| Modify | `extras/parley/models/channel.py` — refactor `Typing`, update `TextChannel.typing()` |
| Modify | `docs/bots.md` — add "Bot Status & Presence" section |
| Already done | `extras/parley/models/member.py` — `bot_degraded` already present; no changes needed |
| Already done | `extras/parley/models/channel.py` — `channel_type` already in `Channel.__slots__`; `CHANNEL_TYPE_*` constants and `is_bin` intentionally omitted (ChannelType enum covers the same ground) |

---

## Task 1: `state.py` — `_client` back-reference + `_reapply_status` call

**Files:**
- Modify: `extras/parley/state.py`

**Context:** `ConnectionState.__init__` is at line 55. `_on_gateway_connected` is at line 107. It calls `self.http.get_me()` then `await self._dispatch_event("READY", {})` at line 120. We insert `_reapply_status` between the server population and the `READY` dispatch.

- [ ] **Step 1: Add `_client = None` to `ConnectionState.__init__`**

In `ConnectionState.__init__`, after the `self.gateway: Any = None` line (around line 73), add:

```python
# Back-reference to the owning Client, set by Client.__init__.
# Allows state to call _reapply_status before on_ready fires.
self._client: Any = None
```

- [ ] **Step 2: Insert `_reapply_status` call in `_on_gateway_connected`**

Find `_on_gateway_connected` (line 107). Current end of the method:
```python
        await self._populate_servers()
        await self._dispatch_event("READY", {})
```

Replace those two lines with:
```python
        await self._populate_servers()
        if self._client is not None:
            try:
                await self._client._reapply_status()
            except Exception:
                log.exception("Failed to re-apply status on reconnect")
        await self._dispatch_event("READY", {})
```

- [ ] **Step 3: Verify no import needed** (`Any` is already imported from `typing`)

Run: `cd /home/dylan/Developer/parley && python3 -c "import extras.parley.state"`
Expected: no error (may need `PYTHONPATH=.` prefix).

Run: `PYTHONPATH=/home/dylan/Developer/parley python3 -c "import extras.parley.state; print('ok')`"
Expected: `ok`

- [ ] **Step 4: Commit**

```bash
git add extras/parley/state.py
git commit -m "feat(parley.py): add _client back-reference to ConnectionState, call _reapply_status before on_ready"
```

---

## Task 2: `client.py` — back-reference, status state, and public methods

**Files:**
- Modify: `extras/parley/client.py`

**Context:** `Client.__init__` is at line 69. `self._state = ConnectionState(...)` is at line 81. `self._state.gateway = self._gateway` is at line 83. Add `self._state._client = self` immediately after line 83.

- [ ] **Step 1: Set back-reference in `Client.__init__`**

After line `self._state.gateway = self._gateway` (line 83), add:

```python
self._state._client = self
```

- [ ] **Step 2: Add status fields to `Client.__init__`**

After the `self._task: Optional[asyncio.Task] = None` line (line 87), add:

```python
# Status state — persisted across reconnects
self._status_type: str = "online"
self._status_text: str = ""
self._degraded: bool = False
```

- [ ] **Step 3: Add `_reapply_status`, `set_status`, `set_degraded` methods to `Client`**

Find a good location after `_raw_dispatch` (around line 173). Add these methods:

```python
async def _reapply_status(self) -> None:
    """Re-apply non-default status after reconnect. Called by ConnectionState before on_ready."""
    if self._status_type != "online":
        try:
            await self.http.set_status(self._status_type, self._status_text)
        except Exception:
            log.warning("Failed to re-apply status on reconnect")

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
    State persists across reconnects.
    """
    self._degraded = degraded
    if degraded:
        await self.set_status("dnd", reason)
    else:
        await self.set_status("online", "")
```

- [ ] **Step 4: Import check**

`log` is already defined at module level (`log = logging.getLogger("parley.client")`). No new imports needed.

- [ ] **Step 5: Run import check**

Run: `PYTHONPATH=/home/dylan/Developer/parley python3 -c "import extras.parley.client; print('ok')"`
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git add extras/parley/client.py
git commit -m "feat(parley.py): add status state + set_status/set_degraded/_reapply_status to Client"
```

---

## Task 3: `http.py` — update `get_me`, add new methods

**Files:**
- Modify: `extras/parley/http.py`

**Context:** `get_me` is at line 178, calls `/api/auth/me`. It must be updated in-place to `/api/users/me` — `_on_gateway_connected` in state.py calls `self.http.get_me()` on every connect.

- [ ] **Step 1: Update `get_me` in-place**

Find:
```python
async def get_me(self) -> dict:
    """``GET /api/auth/me`` — current user profile."""
    return await self.get("/api/auth/me")
```

Replace with:
```python
async def get_me(self) -> dict:
    """``GET /api/users/me`` — current user profile."""
    return await self.get("/api/users/me")
```

- [ ] **Step 2: Add new methods in the Users section (after `search_users`)**

After `search_users` (around line 201), add:

```python
async def edit_me(self, **fields) -> dict:
    """``PATCH /api/users/me`` — update username, display_name, or avatar_url."""
    allowed = {"username", "display_name", "avatar_url"}
    body = {k: v for k, v in fields.items() if k in allowed}
    return await self.request("PATCH", "/api/users/me", json=body)

async def set_status(self, status_type: str, text: str = "") -> None:
    """``PATCH /api/users/@me/status`` — set status_type and status_text."""
    await self.request("PATCH", "/api/users/@me/status", json={
        "status_type": status_type,
        "status_text": text,
    })

async def send_typing(self, channel_id: int, duration: int = 5) -> None:
    """``POST /api/channels/{id}/typing`` — notify typing for up to *duration* seconds."""
    await self.request("POST", f"/api/channels/{channel_id}/typing", json={
        "duration": max(1, min(60, duration)),
    })

async def update_bot_invite(self, bot_id: int, *, permissions: Optional[int] = None,
                            show_author: Optional[bool] = None) -> dict:
    """``PATCH /api/developer/bots/{id}/invite`` — update bot invite settings."""
    body: dict = {}
    if permissions is not None:
        body["permissions"] = permissions
    if show_author is not None:
        body["show_author"] = show_author
    return await self.request("PATCH", f"/api/developer/bots/{bot_id}/invite", json=body)
```

- [ ] **Step 3: Run import check**

Run: `PYTHONPATH=/home/dylan/Developer/parley python3 -c "import extras.parley.http; print('ok')"`
Expected: `ok`

- [ ] **Step 4: Commit**

```bash
git add extras/parley/http.py
git commit -m "feat(parley.py): update get_me to /users/me, add edit_me/set_status/send_typing/update_bot_invite"
```

---

## Task 4: `client.py` — `fetch_me`, `edit_profile`, `send_typing` wrappers

**Files:**
- Modify: `extras/parley/client.py`

**Context:** `fetch_me` may already exist. Search for it. If it exists, update it in-place. Otherwise add it. Add `edit_profile` and `send_typing` as new methods.

- [ ] **Step 1: Check for existing `fetch_me`**

Run: `grep -n "fetch_me" /home/dylan/Developer/parley/extras/parley/client.py`

- [ ] **Step 2a: If `fetch_me` exists — update it in-place**

Replace the existing `fetch_me` implementation with:
```python
async def fetch_me(self) -> "User":
    """Fetch the authenticated user's profile and update the cache."""
    data = await self.http.get_me()
    self._state.user = User._from_data(data, self._state)
    return self._state.user
```

- [ ] **Step 2b: If `fetch_me` does not exist — add it**

Add the same implementation near the `set_status`/`set_degraded` methods.

- [ ] **Step 3: Add `edit_profile` and `send_typing` wrappers**

```python
async def edit_profile(self, **fields) -> "User":
    """Update own profile. Allowed fields: username, display_name, avatar_url."""
    data = await self.http.edit_me(**fields)
    self._state.user = User._from_data(data, self._state)
    return self._state.user

async def send_typing(self, channel_id: int, duration: int = 5) -> None:
    """Send a typing indicator to *channel_id* for *duration* seconds (1-60)."""
    await self.http.send_typing(channel_id, duration)
```

- [ ] **Step 4: Verify `User` is imported**

`User` is imported at the top of client.py: `from .models.user import PublicUser, User`. ✓

Note: The spec uses `ClientUser` in some places. In this codebase `ClientUser = User` (an alias at `models/user.py:174`). They are the same type — no distinction exists.

- [ ] **Step 5: Run import check**

Run: `PYTHONPATH=/home/dylan/Developer/parley python3 -c "import extras.parley.client; print('ok')"`
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git add extras/parley/client.py
git commit -m "feat(parley.py): add fetch_me/edit_profile/send_typing wrappers to Client"
```

---

## Task 5: `models/user.py` — add `status_type` and `status_text`

**Files:**
- Modify: `extras/parley/models/user.py`

**Context:** `PublicUser.__slots__` is at line 46. `User.__slots__` extends it at line 125. Both `_from_data` class methods need updating.

- [ ] **Step 1: Add `status_type` and `status_text` to `PublicUser`**

In `PublicUser.__slots__` (line 46), add `"status_type"` and `"status_text"`:

```python
__slots__ = (
    "id",
    "username",
    "display_name",
    "avatar_url",
    "banner_url",
    "bio",
    "badges",
    "status_type",
    "status_text",
    "_state",
)
```

In `PublicUser.__init__`, add parameters and assignments:
```python
def __init__(
    self,
    *,
    id: int,
    username: str,
    display_name: str,
    avatar_url: str,
    banner_url: str,
    bio: str,
    badges: int,
    status_type: str = "offline",
    status_text: str = "",
    state: Optional[Any] = None,
) -> None:
    ...
    self.status_type = status_type
    self.status_text = status_text
```

In `PublicUser._from_data`, add:
```python
status_type=data.get("status_type", "offline") or "offline",
status_text=data.get("status_text", "") or "",
```

- [ ] **Step 2: Update `User.__init__` signature and `super().__init__()` call**

`User.__init__` uses explicit named parameters (not `**kwargs`), so `status_type`/`status_text` must be added explicitly.

In `User.__init__` (line ~127), add `status_type: str = "offline"` and `status_text: str = ""` to the parameter list:

```python
def __init__(
    self,
    *,
    id: int,
    username: str,
    display_name: str,
    avatar_url: str,
    banner_url: str,
    bio: str,
    badges: int,
    status_type: str = "offline",
    status_text: str = "",
    email: str,
    email_verified: bool,
    state: Optional[Any] = None,
) -> None:
    super().__init__(
        id=id,
        username=username,
        display_name=display_name,
        avatar_url=avatar_url,
        banner_url=banner_url,
        bio=bio,
        badges=badges,
        status_type=status_type,
        status_text=status_text,
        state=state,
    )
    self.email = email
    self.email_verified = email_verified
```

Update `User._from_data` to include:
```python
status_type=data.get("status_type", "offline") or "offline",
status_text=data.get("status_text", "") or "",
```

- [ ] **Step 3: Run import check**

Run: `PYTHONPATH=/home/dylan/Developer/parley python3 -c "import extras.parley.models.user; print('ok')"`
Expected: `ok`

- [ ] **Step 4: Commit**

```bash
git add extras/parley/models/user.py
git commit -m "feat(parley.py): add status_type/status_text fields to User and PublicUser"
```

---

## Task 6: `models/channel.py` — refactor `Typing` + `TextChannel.typing()`

**Files:**
- Modify: `extras/parley/models/channel.py`

**Context:** `Typing` class is at line 23. Current `__init__` takes `(channel_id, state)`. Current `_send` method calls `self._state.gateway.send(...)`. `TextChannel.typing()` at line 239 returns `Typing(self.id, self._state)`. We need to refactor `Typing` to take `send_fn` + `interval` instead, and update `typing()` to branch on `Bot`.

Circular import avoidance: `Bot` is defined in `client.py`. Use `TYPE_CHECKING` guard.

- [ ] **Step 1: Refactor `Typing` class**

Replace the current `Typing` class (lines 23-57) with:

```python
class Typing:
    """Async context manager that broadcasts a typing indicator while active.

    Calls *send_fn* immediately on enter, then repeatedly every *interval*
    seconds until the block exits.

    Bot clients use the REST path (longer intervals, server-managed expiry);
    non-Bot clients use the WS TYPING frame path (5-second intervals).
    """

    def __init__(
        self,
        channel_id: int,
        send_fn: "Callable[[], Awaitable[None]]",
        interval: float = 5.0,
    ) -> None:
        self._channel_id = channel_id
        self._send_fn = send_fn
        self._interval = interval
        self._task: Optional[asyncio.Task] = None

    async def _loop(self) -> None:
        try:
            while True:
                await self._send_fn()
                await asyncio.sleep(self._interval)
        except asyncio.CancelledError:
            pass

    async def __aenter__(self) -> "Typing":
        self._task = asyncio.create_task(self._loop())
        return self

    async def __aexit__(self, *_: Any) -> None:
        if self._task:
            self._task.cancel()
            self._task = None
```

- [ ] **Step 2: Add necessary imports**

At the top of `channel.py`, ensure these are present:

```python
from typing import TYPE_CHECKING, Any, Awaitable, Callable, Optional
```

`Awaitable` and `Callable` may not be there yet — add them.

Add a `TYPE_CHECKING` block at the end of the `if TYPE_CHECKING:` section (already present starting at line 59):

```python
if TYPE_CHECKING:
    from ..state import ConnectionState
    from ..client import Bot
```

- [ ] **Step 3: Update `TextChannel.typing()`**

Replace the current `typing()` method:

```python
def typing(self) -> Typing:
    return Typing(self.id, self._state)
```

with:

```python
def typing(self, duration: int = 5) -> Typing:
    """Return a context manager that shows a typing indicator.

    Bot clients use the REST endpoint with server-managed expiry (*duration*
    seconds, clamped to 1-60). Non-Bot clients use the WS TYPING frame path.

    Usage::

        async with channel.typing():
            reply = await slow_ai_call()
        await channel.send(reply)
    """
    client = self._state._client if self._state is not None else None

    # Use name-based check to avoid circular import at runtime.
    # (importing Bot from client.py here would create a cycle.)
    is_bot = (
        client is not None
        and type(client).__name__ in ("Bot", "CommandBot")
    )

    if is_bot:
        # REST path: send once per (duration - 2s buffer), minimum 3s interval.
        interval = float(max(3, duration - 2))
        send_fn = lambda: client.send_typing(self.id, duration)  # type: ignore[union-attr]
    else:
        # WS path: server expires typing in ~5s, resend every 5s.
        interval = 5.0
        state = self._state

        async def ws_send() -> None:
            if state is not None and state.gateway is not None:
                await state.gateway.send("TYPING", {"channel_id": str(self.id)})

        send_fn = ws_send

    return Typing(self.id, send_fn, interval)
```

Note: `type(client).__name__` is used instead of `isinstance(client, Bot)` to avoid a circular import at runtime. Do NOT add `if TYPE_CHECKING: from ..client import Bot` inside the function body — `TYPE_CHECKING` is always `False` at runtime, making such a block a no-op that only misleads readers. The module-level `if TYPE_CHECKING:` block (line 59) can optionally include `from ..client import Bot` for type-checker hints.

- [ ] **Step 4: Run import check**

Run: `PYTHONPATH=/home/dylan/Developer/parley python3 -c "import extras.parley.models.channel; print('ok')"`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add extras/parley/models/channel.py
git commit -m "feat(parley.py): refactor Typing to use send_fn+interval, Bot uses REST path"
```

---

## Task 7: `state.py` — fix `_handle_bot_status_update`

**Files:**
- Modify: `extras/parley/state.py`

**Context:** Find `_handle_bot_status_update` — it currently does nothing (pass-through). Update it to parse `server_id`, `bot_user_id`, `is_degraded`, update the member cache, and dispatch.

- [ ] **Step 1: Find `_handle_bot_status_update`**

Run: `grep -n "_handle_bot_status_update\|BOT_STATUS_UPDATE" /home/dylan/Developer/parley/extras/parley/state.py`

- [ ] **Step 2: Replace the handler body**

Find the handler. Replace its body with:

```python
async def _handle_bot_status_update(self, payload: dict) -> None:
    server_id = int(payload["server_id"])
    bot_user_id = int(payload["bot_user_id"])
    is_degraded = bool(payload.get("is_degraded", False))
    key = (server_id, bot_user_id)
    member = self.members.get(key)
    if member is not None:
        member.bot_degraded = is_degraded
    await self._dispatch_event("BOT_STATUS_UPDATE", payload)
```

Note: The existing handler may be `def` (sync) or `async def`. Check before replacing — if it's `def`, change to `async def`. If it's already async with a pass body, just replace the pass.

- [ ] **Step 3: Verify the handler is registered in `_EVENT_HANDLERS`**

Run: `grep -n "BOT_STATUS_UPDATE\|_handle_bot_status_update" /home/dylan/Developer/parley/extras/parley/state.py`

If it's not in `_EVENT_HANDLERS`, add it. Typically at the bottom of state.py:
```python
"BOT_STATUS_UPDATE": ConnectionState._handle_bot_status_update,
```

- [ ] **Step 4: Run import check**

Run: `PYTHONPATH=/home/dylan/Developer/parley python3 -c "import extras.parley.state; print('ok')"`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add extras/parley/state.py
git commit -m "feat(parley.py): implement _handle_bot_status_update — update member cache + dispatch"
```

---

## Task 8: `docs/bots.md` — add "Bot Status & Presence" section

**Files:**
- Modify: `docs/bots.md`

**Context:** Spec §Documentation Updates defines the full section to add after "Sending Messages".

- [ ] **Step 1: Find the "Sending Messages" section in `docs/bots.md`**

Run: `grep -n "Sending Messages\|## Sending" /home/dylan/Developer/parley/docs/bots.md`

- [ ] **Step 2: Insert the section immediately after "Sending Messages"**

Add the following after the Sending Messages section:

````markdown
### Bot Status & Presence

Bots appear **online** to server members when a WebSocket client (e.g. parley.py) is actively connected. When the client disconnects — including on crash — the bot shows as **offline**. This is enforced server-side; no client-side call can override it.

**Setting status**

```python
await bot.set_status("dnd", "Rate limit reached — retrying in 60s")
await bot.set_status("idle")
await bot.set_status("online")
```

Or directly via the API:
```http
PATCH /api/users/@me/status
Authorization: Bearer plk_your_api_key
{"status_type": "dnd", "status_text": "Service degraded"}
```

Allowed values: `online`, `idle`, `dnd`, `invisible`. Setting `offline` returns HTTP 400 — offline is managed by the server.

**Best practice: DND on degradation**

When your bot encounters errors (external API down, rate-limited, etc.), signal this to users by setting DND:

```python
async def on_error(self, error):
    await bot.set_degraded(True, "External API unavailable")
    # ... retry / backoff logic ...
    await bot.set_degraded(False)
```

`set_degraded(True)` sets status to DND with optional reason text. `set_degraded(False)` resets to online. **Status persists across reconnects** — if your bot crashes and reconnects, the hub will set status to `online` automatically (since it's now connected), but if you were DND before the crash, `_reapply_status` will restore DND before `on_ready` fires.

This is optional but encouraged: it trains users to check the bot's status indicator before filing issue reports.

**Typing indicators**

```python
async with channel.typing():
    reply = await slow_ai_call()
await channel.send(reply)
```

Bot clients automatically use the REST endpoint (`POST /api/channels/{id}/typing`) with server-managed expiry, rather than the WebSocket TYPING frame. This avoids the need to re-send every 5 seconds. The `typing()` context manager handles re-sending automatically. Default duration is 5 seconds; pass a custom value:

```python
async with channel.typing(duration=30):
    reply = await very_slow_operation()
```
````

- [ ] **Step 3: Verify the file builds**

Run: `cat /home/dylan/Developer/parley/docs/bots.md | head -5`
Expected: file is readable, no syntax errors.

- [ ] **Step 4: Commit**

```bash
git add docs/bots.md
git commit -m "docs: add Bot Status & Presence section to bots.md"
```

---

## Task 9: Final verification

- [ ] **Step 1: Full Python import check**

Run: `PYTHONPATH=/home/dylan/Developer/parley python3 -c "
import extras.parley.client
import extras.parley.http
import extras.parley.state
import extras.parley.models.channel
import extras.parley.models.user
import extras.parley.models.member
print('all imports ok')
"`
Expected: `all imports ok`

- [ ] **Step 2: Smoke test Typing branching logic**

Run: `PYTHONPATH=/home/dylan/Developer/parley python3 -c "
from extras.parley.models.channel import TextChannel, channel_from_data
print('Typing class ok')
"`
Expected: no error.

- [ ] **Step 3: Full Go build (cross-check backend not broken)**

Run: `cd /home/dylan/Developer/parley && go build ./...`
Expected: exits 0.
