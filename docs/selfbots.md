# Selfbots

A **Selfbot** (User API Key) lets you automate your own Parley account — the key authenticates as you directly.

::: info Selfbots are allowed on Parley
Unlike Discord, Parley explicitly permits selfbots. Use them however you'd like.
:::

## Creating a User API Key

1. Open **User Settings → Developer**
2. Click **+ Create Key**
3. Select **User (Selfbot)**
4. Optionally give the key a name (e.g. "Personal automation")
5. Copy the key — it is only shown once

## How selfbot messages appear in chat

Messages sent via a User API Key show a 🤖 robot icon next to your username. Hovering over it shows **"Selfbot"**. Your username and avatar are used normally — it's clearly still you.

```
dylan 🤖  12:34 PM
  Automated reply
```

The `via_api: true` field is set on every message sent through a User API Key.

## Sending a message as yourself

```http
POST /api/channels/{channelId}/messages
Authorization: Bearer plk_<your-user-key>
Content-Type: application/json

{
  "content": "This is an automated message."
}
```

The message is attributed to your account with `via_api: true`.

## Capabilities

A User API Key has the same permissions as your account. You can:

- Read and send messages in any channel you're a member of
- Edit and delete your own messages
- Manage servers and channels you own/admin
- Use DMs
- Subscribe to the WebSocket for real-time events

## Security

- User API Keys have full access to your account — treat them like passwords
- You can revoke a key at any time from **User Settings → Developer**
- Multiple keys can be active at once
- The raw key is never stored — only a SHA-256 hash

## Bot vs. Selfbot at a glance

| | Bot | Selfbot |
|---|---|---|
| Separate account | ✅ Yes | ❌ No — authenticates as you |
| Badge in chat | Purple **BOT** | 🤖 Selfbot icon |
| `author_is_bot` | `true` | `false` |
| `via_api` | `true` | `true` |
| Needs server membership | Must be added | Already a member (it's you) |
