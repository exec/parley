# Parley Voice & Video Redesign — Spec

**Date:** 2026-03-15
**Status:** Approved

---

## Overview

Add full-featured voice and video calling to Parley's existing voice channel infrastructure. Migrate from a self-hosted LiveKit droplet to LiveKit Cloud, enable video and screen sharing, add VAD/PTT/noise suppression, redesign the voice/video UI end-to-end in Discord's style, and replace all emoji/unicode icons across the app with Lucide React icons.

Almost entirely frontend work. Backend changes are limited to environment variable updates and a small token permission expansion.

---

## Goals

- Video and screen sharing in voice channels via LiveKit Cloud
- Automatic voice activation (VAD) and push-to-talk (PTT) modes
- Noise suppression via browser-native constraints + optional WASM enhancement
- Discord-style UI: main area call view + persistent bottom-left controls widget
- Grid view and speaker view, toggleable
- Mic, camera, and speaker device selection in User Settings
- Lucide React icons across the entire app, styled terminal green (#32CD32)

---

## Non-Goals

- Separate "video channel" type — video is enabled within existing voice channels
- Recording or live streaming
- End-to-end encryption (LiveKit Cloud handles transport security)
- Mobile native apps

---

## Backend Changes

### 1. LiveKit Cloud Migration

Update environment variables on all 3 API servers (`parley-api-1`, `parley-api-2`, `parley-api-3`):

```
LIVEKIT_URL=wss://your-project.livekit.cloud
LIVEKIT_API_KEY=your-livekit-api-key
LIVEKIT_API_SECRET=your-livekit-api-secret
```

`terraform.tfvars` has been updated with these values. The `parley-vc` self-hosted droplet can be decommissioned after confirming LiveKit Cloud is working.

### 2. Token Permissions (`internal/voice/service.go`)

The existing service uses a manual `jwt.MapClaims` approach (no LiveKit Go SDK). Extend the `"video"` claim map to include `canPublishSources` with the camelCase key LiveKit expects:

```go
"video": map[string]interface{}{
    "room":           channelID,
    "roomJoin":       true,
    "canPublish":     true,
    "canSubscribe":   true,
    "canPublishData": true,
    "canPublishSources": []string{
        "camera",
        "microphone",
        "screen_share",
        "screen_share_audio",
    },
},
```

No new Go dependencies. No new API endpoints. No database migrations.

---

## Frontend Changes

### New Dependencies

```json
"@ricky0123/vad-web": "latest",
"lucide-react": "latest"
```

**Note on noise suppression:** Use browser-native constraints (`noiseSuppression: true`, `echoCancellation: true`) passed via `getUserMedia`. This is zero-dependency and works in all modern browsers. Do NOT add `@livekit/noise-suppression` or `@livekit/krisp-noise-filter` — the correct package name is unclear and Krisp requires a commercial license. The noise suppression toggle in settings enables/disables these constraints when acquiring the mic stream.

### Vite Configuration (`vite.config.ts`)

`@ricky0123/vad-web` ships a WASM binary. Add the following to `vite.config.ts` or the build will fail with a MIME type error in dev and silently omit the WASM in production:

```typescript
export default defineConfig({
  // ...existing config...
  optimizeDeps: {
    exclude: ['@ricky0123/vad-web'],
  },
  assetsInclude: ['**/*.wasm'],
})
```

---

### `AppContext.tsx` — Voice State

Add global voice state so `VoiceControls` can render from anywhere in the layout:

```typescript
interface VoiceState {
  channelId: string | null;
  channelName: string | null;
  serverId: string | null;
  connected: boolean;
  connecting: boolean;
  muted: boolean;
  deafened: boolean;
  videoEnabled: boolean;
  screenSharing: boolean;
  speaking: boolean;                    // local VAD output
  activeSpeakers: Set<string>;          // remote participant identities (from LiveKit ActiveSpeakersChanged)
  vadMode: 'vad' | 'ptt' | 'always';
  pttKey: string;                       // KeyboardEvent.code, e.g. 'Space', 'KeyV'
  noiseSuppressionEnabled: boolean;
}
```

`AppContext` exposes `voiceState` and `setVoiceState`. Default: all nulls/false, `vadMode: 'vad'`, `pttKey: 'Space'`, `noiseSuppressionEnabled: true`.

### Dual-Panel Layout: Voice + Chat

When a voice channel is selected, the main content area shows the video grid. The current implementation also shows a chat panel alongside. **Keep this behavior:** voice channel = video grid on the left/center, text chat on the right in a sidebar panel. The `VoiceChannel.tsx` rewrite should render the grid filling available space, and `App.tsx` continues to render the chat panel when `activeChannel` is a voice channel. The spec's layout diagram shows the grid area only — the chat panel is rendered by the existing routing logic outside `VoiceChannel`.

### Multi-Tab Safety

Retain the existing `BroadcastChannel('parley_voice')` pattern from the current hook. When the hook mounts in `AppProvider`, it claims the voice slot via `BroadcastChannel` exactly as before, ensuring only one tab can hold an active voice connection. The implementation agent should carry this logic forward into the rewrite unchanged.

### Hook Mounting — Critical Architecture Note

`useVoiceConnection` must be mounted **above the router**, in `AppProvider` or a dedicated `VoiceProvider` wrapping the entire app. It must NOT live inside `VoiceChannel.tsx` — that component unmounts when the user navigates to a text channel, which would disconnect the room and destroy the persistent connection the bottom-left widget depends on.

**Recommended structure:**
```
<AppProvider>          ← mounts useVoiceConnection, owns voiceState
  <Router>
    <MainLayout>       ← renders <VoiceControls /> when voiceState.connected
      <Routes>
        <VoiceChannel> ← reads voiceState from context, does NOT own the connection
        <ChatWindow>
      </Routes>
    </MainLayout>
  </Router>
</AppProvider>
```

---

### `useVoiceConnection.ts` — Full Rewrite

**Mic acquisition (shared stream):**

```typescript
const stream = await navigator.mediaDevices.getUserMedia({
  audio: {
    deviceId: selectedMicId,
    noiseSuppression: noiseSuppressionEnabled,
    echoCancellation: noiseSuppressionEnabled,
    autoGainControl: true,
  }
});
```

The same `stream` is passed to both LiveKit and VAD — no duplicate `getUserMedia` calls:

```typescript
// LiveKit audio track from existing stream
const audioTrack = new LocalAudioTrack(stream.getAudioTracks()[0]);
await room.localParticipant.publishTrack(audioTrack);

// VAD from same stream (avoids second getUserMedia acquisition)
const vad = await MicVAD.new({
  stream,
  onSpeechStart: () => { if (vadMode === 'vad') audioTrack.unmute(); },
  onSpeechEnd:   () => { if (vadMode === 'vad') audioTrack.mute(); },
});
```

**PTT — global key listener with input guard:**

```typescript
const handleKeyDown = (e: KeyboardEvent) => {
  if (vadMode !== 'ptt') return;
  if (e.code !== pttKey) return;
  // Guard: don't fire PTT when user is typing
  const tag = (e.target as HTMLElement).tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA' || (e.target as HTMLElement).isContentEditable) return;
  audioTrack.unmute();
  setVoiceState(s => ({ ...s, muted: false, speaking: true }));
};
const handleKeyUp = (e: KeyboardEvent) => {
  if (vadMode !== 'ptt' || e.code !== pttKey) return;
  audioTrack.mute();
  setVoiceState(s => ({ ...s, muted: true, speaking: false }));
};
window.addEventListener('keydown', handleKeyDown);
window.addEventListener('keyup', handleKeyUp);
// cleanup on unmount
```

**Video:**
```typescript
const videoTrack = await createLocalVideoTrack({ resolution: VideoPresets.h720 });
await room.localParticipant.publishTrack(videoTrack);
```

**Screen share:**
```typescript
await room.localParticipant.setScreenShareEnabled(true, { audio: true });
```

**Active speakers (remote):**
```typescript
room.on(RoomEvent.ActiveSpeakersChanged, (speakers: Participant[]) => {
  const ids = new Set(speakers.map(s => s.identity));
  setVoiceState(s => ({ ...s, activeSpeakers: ids }));
});
```

**Output device:**
```typescript
// After remote tracks are subscribed, apply selected sink to all audio elements
const applyOutputDevice = (deviceId: string) => {
  document.querySelectorAll('audio').forEach(el => {
    if ('setSinkId' in el) (el as any).setSinkId(deviceId);
  });
};
```
This must be called once on connect (using the saved `selectedSpeakerId` from localStorage) and again whenever the user changes the output device.

**Hook return:**
```typescript
{
  connected, connecting, error,
  muted, deafened, videoEnabled, screenSharing, speaking,
  activeSpeakers,    // Set<string> — participant identities currently speaking
  vadMode, pttKey,
  participants,      // RemoteParticipant[]
  localParticipant,  // LocalParticipant | null
  toggleMute, toggleDeafen, toggleVideo,
  startScreenShare, stopScreenShare,
  disconnect,
  setVadMode, setPttKey,
}
```

---

### `ParticipantTile.tsx` — New Component

Renders a single participant in the video grid.

**Props:** `{ participant, isLocal, isSpeaking, isScreenShare }`

- `isSpeaking` is derived in `VoiceChannel`: `activeSpeakers.has(participant.identity)` for remotes; `voiceState.speaking` for local
- Camera on: renders LiveKit `<VideoTrack>` element
- Camera off: centered avatar circle (initials/avatar image)
- Speaking: `box-shadow: 0 0 0 2px #32CD32, 0 0 8px rgba(50, 205, 50, 0.4)`
- Muted: `MicOff` (Lucide) badge bottom-right
- Screen share tile: separate instance with `Monitor` icon header

---

### `VoiceChannel.tsx` — Full Rewrite

Renders in the main content area when a voice channel is selected. Reads all state from `AppContext` — does NOT own the connection.

**Layout:**

```
┌─────────────────────────────────────────────────────┐
│ #channel-name        [LayoutGrid] [Maximize2]  [Settings] │  ← header bar
├─────────────────────────────────────────────────────┤
│                                                      │
│              ParticipantTile grid                    │
│         (grid or speaker layout)                     │
│                                                      │
│   Screen share appears as extra ParticipantTile      │
│                                                      │
└─────────────────────────────────────────────────────┘
```

**Grid view:** CSS grid, `grid-template-columns: repeat(auto-fit, minmax(280px, 1fr))`. Tiles fill space evenly.

**Speaker view:** One large `ParticipantTile` (active speaker = first in `activeSpeakers`, or pinned). Horizontal filmstrip at bottom: `display: flex; overflow-x: auto; height: 110px`.

**Active speaker auto-spotlight:** In speaker view, if `activeSpeakers` changes and the spotlighted participant is not speaking, switch spotlight to the first active speaker (unless user has manually pinned someone).

---

### `VoiceControls.tsx` — New Component

Persistent widget. Rendered by `MainLayout` when `voiceState.connected`.

```
┌──────────────────────────────────┐
│  Voice Connected                 │
│  #general-voice                  │
├──────────────────────────────────┤
│  [Mic] [Headphones] [Video] [Monitor] [PhoneOff]  │
│  Voice Activity / [SPACE to talk]                 │
└──────────────────────────────────┘
```

**Positioning:** Use a CSS custom property `--sidebar-width` defined at `:root` (set to `232px`). Widget uses `left: var(--sidebar-width)`. This ensures if sidebar width changes, only one variable needs updating.

```css
.voice-controls {
  position: fixed;
  bottom: 0;
  left: var(--sidebar-width);
  width: 220px;
  z-index: 100;
}
```

Icons use Lucide (`Mic`/`MicOff`, `Headphones`/`HeadphonesOff`, `Video`/`VideoOff`, `Monitor`/`MonitorOff`, `PhoneOff`). Green when active, `#cc4444` when off/muted.

PTT mode shows: `[SPACE to talk]` label beneath buttons.

Widget height is approximately 88px. `MainLayout` adds `padding-bottom: 96px` to the main content area when `voiceState.connected` to prevent content being hidden behind it.

---

### `VoiceSettings.tsx` — New Component

New tab in User Settings: **"Voice & Video"** (tab literal: `'voice'`).

Add `'voice'` to the `Tab` type union in `UserSettings.tsx`:
```typescript
type Tab = 'account' | 'profile' | 'developer' | 'voice';
```

Sections:

1. **Input Device** — `<select>` from `enumerateDevices()` filtered to `audioinput`
2. **Output Device** — `audiooutput` devices; selecting one calls `applyOutputDevice(deviceId)` immediately
3. **Camera** — `videoinput` devices
4. **Noise Suppression** — toggle; updates mic stream constraints on next connect
5. **Voice Mode** — radio: Voice Activity / Push to Talk / Always On
   - PTT: key bind button — click, listen for `keydown`, save `event.code` to localStorage
6. **Test Microphone** — record 3 seconds, play back

All settings persisted to `localStorage` under `parley_voice_settings`.

---

### `MainLayout.tsx` — Minor Changes

- Define `--sidebar-width: 232px` on `:root` (or in global CSS)
- Import and render `<VoiceControls />` when `voiceState.connected`
- Add `paddingBottom: voiceState.connected ? 96 : 0` to main content area style

---

### Global Icon Pass — Lucide React

Replace all emoji and unicode icon characters across the entire frontend with Lucide React components. All icons default to `color: #32CD32` (terminal green) with `size={16}` for inline use, `size={20}` for control buttons.

**Files to audit (likely contain emoji/unicode icons):**

- `components/layout/ChannelList.tsx` — # hash, volume, code channel icons; category chevrons
- `components/layout/MainLayout.tsx` — any nav icons
- `components/layout/UserSidebar.tsx` — settings gear, mic/deafen status
- `components/chat/MessageInput.tsx` — emoji picker, attach, send, gif
- `components/chat/Message.tsx` — edit pencil, delete trash, pin, reactions
- `components/chat/TypingIndicator.tsx` — any icons
- `components/modals/CreateChannelModal.tsx` — channel type icons (hash, volume, code)
- `components/modals/CreateServerModal.tsx` — any icons
- `components/modals/UserProfileModal.tsx` — close X, badges
- `components/modals/AssignRolesModal.tsx` — any icons
- `components/modals/ChannelSettingsModal.tsx` — any icons
- `components/settings/ServerSettings.tsx` — settings, members, roles, delete icons
- `components/settings/UserSettings.tsx` — tab icons if any
- `components/settings/ChannelPermissions.tsx` — permission state icons
- `components/voice/VoiceChannel.tsx` — all (full rewrite, use Lucide throughout)
- `components/ui/*` — any shared UI with icons

**Icon key mappings:**

| Context | New Lucide Icon |
|---------|-----------------|
| Text channel | `Hash` |
| Voice channel | `Volume2` |
| Bin channel | `Code2` |
| Category (collapsed/expanded) | `ChevronRight` / `ChevronDown` |
| Settings gear | `Settings` |
| Close / X | `X` |
| Add / Plus | `Plus` |
| Members | `Users` |
| Invite | `UserPlus` |
| Notification | `Bell` / `BellOff` |
| Search | `Search` |
| Edit | `Pencil` |
| Delete | `Trash2` |
| Pin | `Pin` |
| Emoji picker | `Smile` |
| Attach file | `Paperclip` |
| Send | `Send` |
| GIF | `Film` |
| Mic | `Mic` / `MicOff` |
| Deafen | `Headphones` / `HeadphonesOff` |
| Camera | `Video` / `VideoOff` |
| Screen share | `Monitor` / `MonitorOff` |
| Leave call | `PhoneOff` |
| Grid view | `LayoutGrid` |
| Speaker view | `Maximize2` |
| Speaking | `Radio` |
| Online dot | `Circle` (filled, small) |
| PTT key | `Mic` with key badge overlay |

**Do this as a single dedicated commit** after voice/video features are working, to keep diffs reviewable.

---

## Data Flow

```
User clicks voice channel
    → GET /channels/{id}/voice/token
    → AppContext: useVoiceConnection.connect(token, url)  [mounted above router]
    → Room.connect() to LiveKit Cloud
    → POST /channels/{id}/voice/join  (Redis presence)
    → WebSocket VOICE_STATE_UPDATE broadcast
    → voiceState.connected = true
    → VoiceChannel renders participants in main area
    → VoiceControls widget appears bottom-left

User toggles camera
    → createLocalVideoTrack()
    → room.localParticipant.publishTrack()
    → remote participants receive TrackSubscribed
    → ParticipantTile switches from avatar to video

User shares screen
    → room.localParticipant.setScreenShareEnabled(true, { audio: true })
    → extra ParticipantTile appears for screen share track

Someone speaks
    → LiveKit emits ActiveSpeakersChanged
    → voiceState.activeSpeakers updated
    → VoiceChannel passes isSpeaking to each ParticipantTile
    → Green glow border on speaking tile

User leaves
    → room.disconnect()
    → POST /channels/{id}/voice/leave
    → WebSocket VOICE_STATE_UPDATE broadcast
    → voiceState reset to defaults
    → VoiceControls unmounts
```

---

## File Change Summary

| File | Change |
|------|--------|
| `internal/voice/service.go` | Add `canPublishSources` to JWT map claim |
| `terraform/terraform.tfvars` | Updated LiveKit Cloud credentials (done) |
| `vite.config.ts` | Add `optimizeDeps.exclude` + `assetsInclude` for WASM |
| `frontend/package.json` | Add `@ricky0123/vad-web`, `lucide-react` |
| `frontend/src/hooks/useVoiceConnection.ts` | Full rewrite — shared stream, VAD, PTT with input guard, video, screen share, output device |
| `frontend/src/context/AppContext.tsx` | Mount `useVoiceConnection`, add `VoiceState` with `activeSpeakers` + `noiseSuppressionEnabled` |
| `frontend/src/App.tsx` | Remove `useVoiceConnection` call from `MainApp` (line ~136) and remove `activeVoiceChannel` local state — these move to `AppContext` |
| `frontend/src/components/voice/VoiceChannel.tsx` | Full rewrite — reads from context, grid/speaker UI |
| `frontend/src/components/voice/ParticipantTile.tsx` | New — video tile, speaking glow, mute badge |
| `frontend/src/components/voice/VoiceControls.tsx` | New — persistent bottom-left widget |
| `frontend/src/components/settings/VoiceSettings.tsx` | New — device pickers, VAD mode, PTT bind, noise toggle |
| `frontend/src/components/settings/UserSettings.tsx` | Add `'voice'` tab literal + render VoiceSettings |
| `frontend/src/components/layout/MainLayout.tsx` | Render VoiceControls, bottom padding, define `--sidebar-width` |
| `frontend/src/index.css` (or global CSS) | `:root { --sidebar-width: 232px }` |
| All frontend components (see list above) | Replace emoji/unicode with Lucide icons (separate commit) |
| All 3 API servers | Update env vars via SSH or next Terraform apply |

---

## CSS Conventions

Follow existing terminal green theme:
- Active/enabled: `#32CD32`
- Muted/disabled icons: `#cc4444`
- Background tiles: `#0a0c0f`
- Borders: `#1e2228`
- Speaking glow: `box-shadow: 0 0 0 2px #32CD32, 0 0 8px rgba(50, 205, 50, 0.4)`
- Icon size: `size={16}` inline, `size={20}` control buttons
- Control widget background: `#0d0f12; border: 1px solid #1e2228`
- `--sidebar-width: 232px` defined at `:root`
