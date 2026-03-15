# Voice

Voice channels use [LiveKit](https://livekit.io) as the underlying SFU. The REST endpoints manage presence; the actual audio connection uses the LiveKit client SDK.

## Get Voice Token

<span class="method get">GET</span><span class="route">/api/channels/{channelId}/voice/token</span>

Returns a LiveKit access token for the voice channel. Pass this token to the LiveKit client SDK to connect to the room.

**Response** `200 OK`

```json
{
  "token": "eyJ...",
  "url": "wss://vc.parley.x86-64.com"
}
```

---

## Join Voice Channel

<span class="method post">POST</span><span class="route">/api/channels/{channelId}/voice/join</span>

Registers the user as a present participant (updates the participants list broadcast via WebSocket). Call this **after** connecting to LiveKit.

**Response** `204 No Content`

---

## Leave Voice Channel

<span class="method post">POST</span><span class="route">/api/channels/{channelId}/voice/leave</span>

Removes the user from the participant list and disconnects from LiveKit. Call this **before** or **when** disconnecting.

**Response** `204 No Content`

---

## Get Participants

<span class="method get">GET</span><span class="route">/api/channels/{channelId}/voice/participants</span>

Returns current participants in the voice channel.

**Response** `200 OK`

```json
[
  { "user_id": "111", "username": "dylan" },
  { "user_id": "222", "username": "alice" }
]
```

---

## Full Connection Flow

```js
import { Room, createLocalTracks } from 'livekit-client';

// 1. Get a token
const { token, url } = await fetch('/api/channels/123/voice/token', {
  headers: { Authorization: `Bearer ${apiKey}` }
}).then(r => r.json());

// 2. Connect to LiveKit
const room = new Room();
await room.connect(url, token);

// 3. Publish audio
const tracks = await createLocalTracks({ audio: true, video: false });
for (const track of tracks) {
  await room.localParticipant.publishTrack(track);
}

// 4. Register presence
await fetch('/api/channels/123/voice/join', {
  method: 'POST',
  headers: { Authorization: `Bearer ${apiKey}` }
});

// 5. On disconnect
room.on(RoomEvent.Disconnected, async () => {
  await fetch('/api/channels/123/voice/leave', {
    method: 'POST',
    headers: { Authorization: `Bearer ${apiKey}` }
  });
});
```

::: info Bot API Keys and voice
Bot and selfbot API keys can join voice channels and publish audio tracks just like regular users.
:::
