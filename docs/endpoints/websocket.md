# WebSocket

The WebSocket connection provides real-time events. Connect once and receive all events for channels you're subscribed to.

## Connecting

```
wss://parley.x86-64.com/ws?token=<your-token>
```

The token can be a JWT **or** a `plk_` API key.

```js
const ws = new WebSocket(
  `wss://parley.x86-64.com/ws?token=${apiKey}`
);
```

::: tip Authorization header alternative
If your WebSocket client supports custom headers (non-browser environments), you can also pass the token as `Authorization: Bearer <token>` instead of a query parameter.
:::

---

## Message Format

All WebSocket messages are JSON with the shape:

```json
{
  "event": "EVENT_NAME",
  "data": { ... }
}
```

---

## Subscribing to Channels

After connecting, subscribe to a channel to receive its events:

```json
{ "event": "subscribe", "channel_id": "123456789" }
```

Unsubscribe:

```json
{ "event": "unsubscribe", "channel_id": "123456789" }
```

The `channel_id` can also be a pseudo-channel for server-level events:

| Pseudo-channel | Events received |
|---|---|
| `"server:{serverId}"` | Member joins/leaves, server updates, user presence |
| `"123456789"` (channel ID) | Messages, edits, deletes, reactions, typing |

---

## Events

### `MESSAGE_CREATE`

Fired when a new message is sent.

```json
{
  "event": "MESSAGE_CREATE",
  "data": {
    "id": "123",
    "channel_id": "456",
    "author_id": "111",
    "author": "dylan",
    "content": "Hello!",
    "created_at": "2026-03-14T12:34:56Z",
    "reactions": [],
    "author_is_bot": false,
    "via_api": true
  }
}
```

### `MESSAGE_UPDATE`

Fired when a message is edited.

```json
{
  "event": "MESSAGE_UPDATE",
  "data": { "id": "123", "content": "Edited", "updated_at": "..." }
}
```

### `MESSAGE_DELETE`

```json
{
  "event": "MESSAGE_DELETE",
  "data": { "id": "123", "channel_id": "456" }
}
```

### `REACTION_UPDATE`

Fired when a reaction is added or removed.

```json
{
  "event": "REACTION_UPDATE",
  "data": {
    "message_id": "123",
    "emoji": "👍",
    "count": 3,
    "user_ids": ["111", "222", "333"]
  }
}
```

### `TYPING_START`

Fired when a user starts typing. The client sends this; you'll receive it from other users.

```json
{
  "event": "TYPING_START",
  "data": { "channel_id": "456", "user_id": "111", "username": "dylan" }
}
```

### `VOICE_STATE_UPDATE`

Fired when a user joins or leaves a voice channel.

```json
{
  "event": "VOICE_STATE_UPDATE",
  "data": {
    "channel_id": "789",
    "user_id": "111",
    "username": "dylan",
    "action": "join"
  }
}
```

`action` is either `"join"` or `"leave"`.

### `MEMBER_JOIN` / `MEMBER_LEAVE`

Fired on the server pseudo-channel when a member joins or leaves.

```json
{
  "event": "MEMBER_JOIN",
  "data": { "server_id": "987", "user_id": "222", "username": "alice" }
}
```

### `USER_UPDATE`

Fired on server pseudo-channels when a member updates their profile.

```json
{
  "event": "USER_UPDATE",
  "data": { "user_id": "111", "username": "new-name", "avatar_url": "..." }
}
```

### `CHANNEL_CREATE` / `CHANNEL_DELETE`

Fired when a channel is created or deleted in a server.

### `DM_MESSAGE_CREATE`

Same shape as `MESSAGE_CREATE`, delivered to DM participants.

---

## Sending Typing Indicator

```json
{ "event": "typing", "channel_id": "456" }
```

---

## Example: Bot listening for messages

```js
const ws = new WebSocket(`wss://parley.x86-64.com/ws?token=${botApiKey}`);

ws.onopen = () => {
  // Subscribe to a channel
  ws.send(JSON.stringify({ event: 'subscribe', channel_id: '123' }));
};

ws.onmessage = ({ data }) => {
  const msg = JSON.parse(data);
  if (msg.event === 'MESSAGE_CREATE' && !msg.data.author_is_bot) {
    if (msg.data.content === '!ping') {
      fetch(`/api/channels/${msg.data.channel_id}/messages`, {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${botApiKey}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ content: 'Pong!' }),
      });
    }
  }
};
```
