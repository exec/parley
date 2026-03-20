# Direct Messages

## List DM Channels

<span class="method get">GET</span><span class="route">/api/dms</span>

Returns all open DM channels for the authenticated user.

**Response** `200 OK`

```json
[
  {
    "id": "123456789",
    "user1_id": "111",
    "user2_id": "222",
    "created_at": "2026-03-14T00:00:00Z",
    "other_user_id": "222",
    "other_username": "alice",
    "other_display_name": "Alice",
    "other_avatar_url": "https://cdn.parley.x86-64.com/uploads/avatar.png"
  }
]
```

---

## Open DM

<span class="method post">POST</span><span class="route">/api/dms</span>

Opens (or returns an existing) DM channel with another user.

**Request body**

```json
{ "user_id": "222" }
```

**Response** `200 OK` — DM channel object (same shape as list items above).

---

## Get DM Messages

<span class="method get">GET</span><span class="route">/api/dms/{dmId}/messages?limit=50&before=</span>

Returns messages in a DM channel, newest first. Uses cursor-based pagination — pass the oldest message ID as `before` to load earlier messages. Max `limit` is 200.

**Response** `200 OK` — array of message objects (same shape as channel messages).

---

## Send DM Message

<span class="method post">POST</span><span class="route">/api/dms/{dmId}/messages</span>

**Request body** — same as [Send Message](/endpoints/messages#send-a-message).

**Response** `201 Created` — message object.

::: warning Bot API Keys and DMs
Bot users can send and receive DMs just like regular users. Use `POST /api/dms` to open a DM between the bot and another user, then `POST /api/dms/{dmId}/messages` to send.
:::
