# Messages

## Send a Message

<span class="method post">POST</span><span class="route">/api/channels/{channelId}/messages</span>

Sends a message to a text channel. The authenticated user (or bot) must be a member of the server that owns the channel.

**Request body**

```json
{
  "content": "Hello, world!",
  "nonce": "optional-client-dedup-id",
  "attachment_url": "https://cdn.example.com/file.png",
  "attachment_name": "file.png",
  "attachment_type": "image/png"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `content` | string | ✅ (if no attachment) | Message text |
| `nonce` | string | — | Client-generated dedup ID; echoed back in the response |
| `attachment_url` | string | ✅ (if no content) | URL of an attached file |
| `attachment_name` | string | — | Display filename for the attachment |
| `attachment_type` | string | — | MIME type of the attachment |

**Response** `201 Created`

```json
{
  "id": "123456789",
  "channel_id": "987654321",
  "author_id": "111",
  "author_username": "dylan",
  "author_display_name": "Dylan",
  "author_avatar_url": "https://cdn.parley.x86-64.com/uploads/avatar.png",
  "content": "Hello, world!",
  "created_at": "2026-03-14T12:34:56Z",
  "nonce": "optional-client-dedup-id",
  "author_is_bot": false,
  "via_api": true
}
```

::: details Bot vs. User API Key differences

`via_api` is `true` for **both** bot and user API keys — it just means the message came through the API rather than the web client.

**Bot Key** — message is from the bot user:
```json
{ "author_username": "MyBot", "author_is_bot": true, "via_api": true }
```
Shows a purple **BOT** badge in chat (`author_is_bot` takes precedence).

**User Key (Selfbot)** — message is from you:
```json
{ "author_username": "dylan", "author_is_bot": false, "via_api": true }
```
Shows a 🤖 selfbot icon in chat.

:::

---

## Get Messages

<span class="method get">GET</span><span class="route">/api/channels/{channelId}/messages?limit=50&before=</span>

Returns messages in a channel, newest first. Uses cursor-based pagination.

**Query parameters**

| Parameter | Default | Description |
|---|---|---|
| `limit` | `50` | Number of messages (max 50) |
| `before` | — | Message ID cursor — returns messages older than this ID |

**Response** `200 OK`

```json
[
  {
    "id": "123456789",
    "channel_id": "987654321",
    "author_id": "111",
    "author_username": "dylan",
    "author_display_name": "Dylan",
    "author_avatar_url": "https://cdn.parley.x86-64.com/uploads/avatar.png",
    "content": "Hello!",
    "created_at": "2026-03-14T12:34:56Z",
    "reactions": [],
    "author_is_bot": false,
    "via_api": false
  }
]
```

---

## Edit a Message

<span class="method put">PUT</span><span class="route">/api/messages/{messageId}</span>

Edits the content of a message. Only the original author can edit.

**Request body**

```json
{ "content": "Edited content" }
```

**Response** `200 OK` — returns the updated message object.

---

## Delete a Message

<span class="method delete">DELETE</span><span class="route">/api/messages/{messageId}</span>

Deletes a message. Only the original author (or a server admin) can delete.

**Response** `204 No Content`

---

## Toggle Reaction

<span class="method post">POST</span><span class="route">/api/messages/{messageId}/reactions</span>

Adds or removes an emoji reaction. If you already reacted with this emoji, the reaction is removed.

**Request body**

```json
{ "emoji": "👍" }
```

**Response** `204 No Content`

---

## Message Object

```ts
interface Message {
  id: string
  channel_id: string
  author_id: string
  author_username: string        // login username
  author_display_name?: string   // display name (may differ from username)
  author_avatar_url?: string
  author_is_bot: boolean         // true if sent by a bot user
  via_api: boolean               // true if sent via an API key (bot or selfbot)
  content: string
  created_at: string             // ISO 8601
  updated_at: string             // set on edits
  nonce?: string
  parent_id?: string             // reply-to message ID
  parent_author_username?: string
  parent_author_display_name?: string
  attachment_url?: string
  attachment_name?: string
  attachment_type?: string
  reactions: Reaction[]
}

interface Reaction {
  emoji: string
  count: number
  user_ids: string[]
}
```
