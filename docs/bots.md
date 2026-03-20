# Bots

A **Bot** is a separate Parley user account that you own and control via a Bot API Key.

## Creating a bot

1. Open **User Settings → Developer**
2. Click **+ Create Key**
3. Select **Bot** and enter a username
4. Copy the key — it is only shown once

This creates a new user with `is_bot: true` owned by your account. The bot's username appears in chat and can be changed at any time.

## How bots appear in chat

Messages sent by a bot user show a purple **BOT** badge after the username:

```
MyBot BOT  12:34 PM
  Hello from the API!
```

The `author_is_bot: true` field is included on every message object from a bot.

## Sending a message as a bot

```http
POST /api/channels/{channelId}/messages
Authorization: Bearer plk_<your-bot-key>
Content-Type: application/json

{
  "content": "Hello from my bot!"
}
```

The message is attributed to the bot user, not to you.

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

## Adding a Bot to a Server

A bot needs to be a **member of a server** before it can post in that server's channels. Only the server **owner** or a user with the **Administrator** permission can add a bot:

```http
POST /api/servers/{serverId}/members
Authorization: Bearer <your-user-jwt-or-key>
Content-Type: application/json

{
  "user_id": "<bot-user-id>"
}
```

The bot's user ID is returned when you create it (`bot_user_id` field) and is also visible in the Developer tab.

::: info Administrator permission
The Administrator role permission grants all other permissions, including the ability to add bots to servers. Assign it to a role in **Server Settings → Roles** to delegate bot management without transferring server ownership.
:::

## Renaming a bot

```http
PATCH /api/developer/bots/{botUserId}
Authorization: Bearer <your-key-or-jwt>
Content-Type: application/json

{
  "username": "NewBotName"
}
```

## Bot vs. Selfbot at a glance

| | Bot | Selfbot |
|---|---|---|
| Separate account | ✅ Yes | ❌ No — authenticates as you |
| Badge in chat | Purple **BOT** | 🤖 Selfbot icon |
| Can be added to servers independently | ✅ | N/A — already a member |
| `author_is_bot` | `true` | `false` |
| `via_api` | `true` | `true` |
