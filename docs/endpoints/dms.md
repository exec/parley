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
    "other_user": {
      "id": "222",
      "username": "alice",
      "avatar_url": ""
    },
    "last_message_at": "2026-03-14T12:34:56Z"
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

**Response** `200 OK` — DM channel object.

---

## Get DM Messages

<span class="method get">GET</span><span class="route">/api/dms/{dmId}/messages?limit=50&offset=0</span>

Returns messages in a DM channel, newest first.

**Response** `200 OK` — array of message objects (same shape as channel messages).

---

## Send DM Message

<span class="method post">POST</span><span class="route">/api/dms/{dmId}/messages</span>

**Request body** — same as [Send Message](/endpoints/messages#send-a-message).

**Response** `201 Created` — message object.

::: warning Bot API Keys and DMs
Bot users can send and receive DMs just like regular users. Use `POST /api/dms` to open a DM between the bot and another user, then `POST /api/dms/{dmId}/messages` to send.
:::
