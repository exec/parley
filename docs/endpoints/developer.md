# API Keys

Endpoints for managing your own API keys and bot users. All endpoints require authentication with a JWT or existing API key.

## List API Keys

<span class="method get">GET</span><span class="route">/api/developer/keys</span>

Returns all API keys owned by the authenticated account.

**Response** `200 OK`

```json
[
  {
    "id": 1,
    "key_prefix": "plk_3f9a2b1c",
    "user_id": 222,
    "owner_id": 111,
    "name": "My Bot Key",
    "is_bot": true,
    "bot_username": "MyBot",
    "created_at": "2026-03-14T12:00:00Z",
    "last_used_at": "2026-03-14T15:30:00Z"
  },
  {
    "id": 2,
    "key_prefix": "plk_9c8b7a6d",
    "user_id": 111,
    "owner_id": 111,
    "name": "Personal automation",
    "is_bot": false,
    "created_at": "2026-03-14T13:00:00Z",
    "last_used_at": null
  }
]
```

---

## Create API Key

<span class="method post">POST</span><span class="route">/api/developer/keys</span>

Creates a new API key. **The raw key is returned once and never stored — copy it immediately.**

**Request body**

```json
{
  "type": "bot",
  "bot_username": "MyBot",
  "name": "My Bot Key"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `type` | `"bot"` \| `"user"` | ✅ | `"bot"` creates a new bot user; `"user"` authenticates as you |
| `bot_username` | string | ✅ if `type=bot` | Username for the new bot account |
| `name` | string | — | Human-readable label for the key |

**Response** `200 OK`

```json
{
  "id": 1,
  "key": "plk_3f9a2b1c8d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a",
  "key_prefix": "plk_3f9a2b1c",
  "name": "My Bot Key",
  "type": "bot",
  "bot_username": "MyBot",
  "bot_user_id": 222
}
```

::: warning
The `key` field is only present in this response. It is hashed and not retrievable afterward. If you lose it, revoke and create a new key.
:::

::: details Bot vs. User key response differences

**Bot key** — `bot_username` and `bot_user_id` are included:
```json
{
  "type": "bot",
  "bot_username": "MyBot",
  "bot_user_id": 222
}
```

**User key** — no bot fields:
```json
{
  "type": "user"
}
```
:::

---

## Revoke API Key

<span class="method delete">DELETE</span><span class="route">/api/developer/keys/{keyId}</span>

Permanently revokes an API key. Any requests using the revoked key will immediately return `401`.

**Response** `204 No Content`

::: warning
Revoking a Bot API Key does **not** delete the bot user. The bot continues to exist as a Parley user but can no longer authenticate. You can create a new key for the same bot user by creating another bot key with the same username (the bot user must be deleted manually if desired).
:::

---

## Rename Bot User

<span class="method patch">PATCH</span><span class="route">/api/developer/bots/{botUserId}</span>

Renames a bot user you own. The `botUserId` is the `user_id` (not `id`) from the key list — i.e. the bot user's account ID.

**Request body**

```json
{ "username": "BetterBotName" }
```

**Response** `204 No Content`

---

## Key Object

```ts
interface APIKeyInfo {
  id: number              // key record ID
  key_prefix: string      // first 12 chars (e.g. "plk_3f9a2b1c")
  user_id: number         // the user this key authenticates as
  owner_id: number        // always the account that created the key
  name: string
  is_bot: boolean
  bot_username?: string   // only present when is_bot = true
  created_at: string      // ISO 8601
  last_used_at?: string   // null if never used
}
```
