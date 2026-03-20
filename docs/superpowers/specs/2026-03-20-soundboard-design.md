# Soundboard Implementation Design

> **For agentic workers:** Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task-by-task.

**Goal:** Add per-server soundboards that privileged users can manage, and that any member can play into a voice channel — audible to everyone via Web Audio API mixing into the LiveKit microphone track — with cross-server access from the VC panel.

**Architecture:** New `internal/soundboard` Go package (handler/service/repository) backed by a single `soundboard_sounds` table. File storage reuses the existing Spaces CDN via the existing `/api/upload` endpoint. Playback is entirely client-side: Web Audio API decodes and mixes the audio into the LiveKit track. A WebSocket event notifies channel members so they can show the sound's emoji on the playing user's participant tile.

**Tech Stack:** Go (existing patterns), PostgreSQL, DigitalOcean Spaces (existing), React/TypeScript, Web Audio API, LiveKit client SDK, existing WebSocket hub.

---

## Data Model

### `soundboard_sounds` table

```sql
CREATE TABLE soundboard_sounds (
    id          BIGSERIAL PRIMARY KEY,
    server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    uploader_id BIGINT NOT NULL REFERENCES users(id) ON DELETE SET NULL,
    name        VARCHAR(32) NOT NULL,
    emoji       VARCHAR(64),           -- unicode emoji string, nullable
    file_url    TEXT NOT NULL,          -- public CDN URL
    file_key    TEXT NOT NULL,          -- Spaces object key (for deletion)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_soundboard_sounds_server ON soundboard_sounds(server_id);
```

**Future extension:** When server custom emoji land, add `custom_emoji_id BIGINT REFERENCES custom_emojis(id)`. The `emoji` (unicode) field stays; `custom_emoji_id` is an alternative. The frontend renders whichever is set.

---

## Backend

### Package: `internal/soundboard/`

Files:
- `models.go` — `Sound` struct
- `repository.go` — DB queries
- `service.go` — business logic, validation, Spaces cleanup
- `handler.go` — HTTP handlers

### REST Endpoints

| Method | Path | Permission | Notes |
|--------|------|------------|-------|
| `GET` | `/api/servers/{serverId}/soundboard` | member | List server's sounds |
| `POST` | `/api/servers/{serverId}/soundboard` | `PermManageServer` | Upload sound + metadata (multipart) |
| `PATCH` | `/api/servers/{serverId}/soundboard/{soundId}` | `PermManageServer` | Update name and/or emoji |
| `DELETE` | `/api/servers/{serverId}/soundboard/{soundId}` | `PermManageServer` | Delete record + Spaces object |
| `GET` | `/api/soundboard` | authenticated | All sounds from all user's servers, grouped by server |
| `POST` | `/api/channels/{channelId}/soundboard/play` | `PermUseSoundboard` | Fire `SOUNDBOARD_PLAY` WS event into the channel |

### Upload validation (POST `/api/servers/{serverId}/soundboard`)

- Accepted MIME types: `audio/mpeg`, `audio/ogg`, `audio/wav`, `audio/wave`
- Max file size: 1 MB (enforced server-side via `http.MaxBytesReader`)
- Max sounds per server: 48 (count check before insert; return 422 with clear message if at limit)
- If DB insert fails after Spaces upload, delete the Spaces object (pattern from existing `I-9` fix)
- File stored at key: `soundboard/{serverId}/{randomID}{ext}`

### WebSocket event: `SOUNDBOARD_PLAY`

Broadcast to all members of the voice channel when `POST /api/channels/{channelId}/soundboard/play` is called.

```json
{
  "type": "SOUNDBOARD_PLAY",
  "channel_id": "123",
  "user_id": "456",
  "sound_id": "789",
  "sound_name": "airhorn",
  "emoji": "📯"
}
```

Sent to the channel's WebSocket room (same routing as existing channel events).

---

## Frontend

### New files

- `frontend/src/components/settings/SoundboardTab.tsx` — server settings tab
- `frontend/src/components/voice/SoundboardPanel.tsx` — VC popover panel
- `frontend/src/api/soundboard.ts` — API client functions

### Modified files

- `frontend/src/components/settings/ServerSettings.tsx` — add `soundboard` tab (visible only to users with `PermManageServer`)
- `frontend/src/components/voice/VoiceChannel.tsx` — add `Music` button to controls bar, wire `SoundboardPanel`
- `frontend/src/components/voice/ParticipantTile.tsx` — show emoji indicator below name in footer
- `frontend/src/components/voice/ParticipantTile.css` — style for the emoji indicator
- `frontend/src/hooks/useVoiceConnection.ts` (or `App.tsx`) — handle `SOUNDBOARD_PLAY` WS event

### SoundboardTab (server settings)

- Only rendered when `hasPerm(myPerms, PERM_MANAGE_SERVER)`
- Sound count badge: "12 / 48 sounds"
- Add Sound form at top: file input (`.mp3,.ogg,.wav`), name field (max 32 chars), emoji text input (optional), Upload button
- Sound list: grid of cards, each showing emoji + name + uploader username + delete button
- Inline error display for upload failures (file too large, wrong type, at limit)

### SoundboardPanel (voice channel)

- Opened by a `Music2` button in the bottom VC controls bar
- Renders as a popover above the controls bar
- Fetches from `GET /api/soundboard` on open (all servers the user is in)
- Groups sounds by server name with a section header per server
- Each sound card: emoji (if set) + name, two buttons:
  - **Preview** (headphones icon) — plays locally only, no WS event
  - **Play** (speaker icon) — mixes into LiveKit mic track + fires WS event
- While a sound is playing: play button shows stop icon; preview button disabled
- Panel closes on outside click or Escape

### Participant tile emoji indicator

- `ParticipantTile` accepts a new optional prop: `activeSoundEmoji?: string`
- Rendered as a small badge below the name in `.participant-tile-footer`
- Parent (`VoiceChannel`) maintains `Map<userId, emoji>` state, populated by `SOUNDBOARD_PLAY` events
- Cleared when the audio ends (frontend tracks duration from the audio buffer; fallback: 30s timeout)

---

## Playback Architecture (Web Audio API)

### Broadcast play

```
CDN URL
  → fetch() → ArrayBuffer
  → AudioContext.decodeAudioData()
  → AudioBufferSourceNode
  → GainNode (sound gain)
  ↘
    ChannelMergerNode → MediaStreamDestination → MediaStream
  ↗
  GainNode (mic gain) ← getUserMedia stream
```

1. Merge mic stream + decoded sound buffer via `ChannelMergerNode`
2. Route merged stream to `MediaStreamDestination`
3. Call `localParticipant.publishTrack(mergedTrack, { source: Track.Source.Microphone })` replacing the existing mic track
4. On `AudioBufferSourceNode.onended`: restore original mic-only track, clear emoji indicator

### Local preview

1. Same decode steps
2. Route `AudioBufferSourceNode` → `AudioContext.destination` (speakers) only
3. No LiveKit track replacement, no WS event

---

## Permission Summary

| Action | Required permission |
|--------|-------------------|
| View server soundboard list | Server member |
| Upload / rename / delete sound | `PermManageServer` |
| Play sound (broadcast) | `PermUseSoundboard` (already exists at bit 40) |
| Fetch cross-server sounds | Authenticated + server membership |
| Trigger play via API (bot future) | `PermUseSoundboard` |

`PermUseSoundboard` is already defined in `internal/permissions/permissions.go` at bit 40 and is already included in the implicit-deny chain under `PermConnect`.

---

## Constraints & Limits

- 48 sounds per server (enforced at upload time)
- 1 MB max per file
- Accepted formats: `.mp3`, `.ogg`, `.wav`
- Sound name: 1–32 characters
- Emoji: optional, max 64 chars (unicode string; future: `custom_emoji_id` FK)
- File key pattern: `soundboard/{serverId}/{randomID}{ext}`
