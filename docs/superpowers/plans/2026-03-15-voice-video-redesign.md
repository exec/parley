# Voice & Video Redesign Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade Parley's audio-only voice channels to full video/screen-share with VAD, PTT, noise suppression, a Discord-style persistent controls widget, and replace all emoji icons across the app with Lucide React icons.

**Architecture:** The existing `useVoiceConnection` hook stays in `MainApp` — `MainApp` wraps the entire router and never unmounts, so it is effectively "above the router" without touching `AppContext`. Voice state is passed via props through `MainLayout` to `VoiceControls` (new fixed-position widget) and `VoiceChannel` (rewritten grid/speaker UI). No new context is needed — prop drilling from `MainApp` is sufficient and matches the existing pattern. `ParticipantTile` handles per-participant rendering (video, avatar, speaking glow). `VoiceSettings` is a new tab in `UserSettings`. **Note:** `vadMode` changes while connected take effect on next connect (reconnect required) — this is acceptable scope for this iteration.

**Tech Stack:** LiveKit Cloud, `livekit-client@2.17.3` (already installed), `@ricky0123/vad-web`, `lucide-react`, browser-native noise suppression constraints

---

## File Map

**Modified:**
- `internal/voice/service.go` — add `canPublishSources` to JWT claim
- `frontend/vite.config.ts` — WASM support for `@ricky0123/vad-web`
- `frontend/package.json` — add `@ricky0123/vad-web`, `lucide-react`
- `frontend/src/index.css` — add `--sidebar-width: 232px` CSS variable
- `frontend/src/hooks/useVoiceConnection.ts` — full rewrite
- `frontend/src/App.tsx` — wire new hook return values, render `VoiceControls`
- `frontend/src/components/layout/MainLayout.tsx` — accept `voiceControlsSlot` prop
- `frontend/src/components/layout/MainLayout.css` — add `.main-content--voice-connected` padding rule
- `frontend/src/components/voice/VoiceChannel.tsx` — full rewrite (grid/speaker)
- `frontend/src/components/voice/VoiceChannel.css` — full rewrite
- `frontend/src/components/settings/UserSettings.tsx` — add `'voice'` tab
- All icon-bearing components — replace emoji/unicode with Lucide

**Created:**
- `frontend/src/components/voice/ParticipantTile.tsx`
- `frontend/src/components/voice/ParticipantTile.css`
- `frontend/src/components/voice/VoiceControls.tsx`
- `frontend/src/components/voice/VoiceControls.css`
- `frontend/src/components/settings/VoiceSettings.tsx`
- `frontend/src/components/settings/VoiceSettings.css`

---

## Chunk 1: Backend + Build Foundation

### Task 1: Add video/screen share permissions to LiveKit token

**Files:**
- Modify: `internal/voice/service.go:57-64`

- [ ] **Open `internal/voice/service.go` and find the `"video"` claim map (line ~57)**

- [ ] **Add `canPublishSources` to the map:**

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

- [ ] **Build to verify no errors:**

```bash
cd /home/dylan/Developer/parley && go build ./...
```

Expected: no output (success)

- [ ] **Commit:**

```bash
git add internal/voice/service.go
git commit -m "feat: expand LiveKit token to allow video and screen share sources"
```

---

### Task 2: Configure Vite for WASM + add npm packages

**Files:**
- Modify: `frontend/vite.config.ts`
- Modify: `frontend/package.json`

- [ ] **Update `frontend/vite.config.ts` to support `@ricky0123/vad-web` WASM:**

Replace the entire file with:

```typescript
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  optimizeDeps: {
    exclude: ['@ricky0123/vad-web'],
  },
  assetsInclude: ['**/*.wasm'],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8081',
        changeOrigin: true,
      },
      '/ws': {
        target: 'ws://localhost:8081',
        changeOrigin: true,
        ws: true,
      },
    },
  },
});
```

- [ ] **Install new packages:**

```bash
cd /home/dylan/Developer/parley/frontend && npm install @ricky0123/vad-web lucide-react
```

Expected: packages added to `node_modules`, `package.json` updated with both packages

- [ ] **Verify install:**

```bash
ls node_modules/@ricky0123/vad-web && ls node_modules/lucide-react
```

Expected: both directories exist

- [ ] **Add `--sidebar-width` CSS variable to `frontend/src/index.css`.** Find the `:root {` block and add this line inside it (after the existing variables):

```css
--sidebar-width: 232px;
```

- [ ] **Commit:**

```bash
cd /home/dylan/Developer/parley/frontend && git add vite.config.ts package.json package-lock.json src/index.css
git commit -m "chore: add @ricky0123/vad-web, lucide-react, WASM vite config, sidebar-width var"
```

---

## Chunk 2: Hook Rewrite

### Task 3: Rewrite `useVoiceConnection`

**Files:**
- Modify: `frontend/src/hooks/useVoiceConnection.ts` (full rewrite)

This is the largest single task. Read the existing file first (`frontend/src/hooks/useVoiceConnection.ts`) to understand the patterns being replaced, then replace the entire file.

Key differences from the old hook:
- Uses `getUserMedia` with noise suppression constraints instead of `createLocalTracks`
- Same stream shared with VAD (no second `getUserMedia` call)
- New state: `videoEnabled`, `screenSharing`, `speaking`, `activeSpeakers`, `participants`, `localParticipant`, `settings`
- PTT global key listener with input element guard
- `updateSettings` persists to `localStorage` under `parley_voice_settings`
- BroadcastChannel pattern is retained

- [ ] **Replace `frontend/src/hooks/useVoiceConnection.ts` entirely:**

```typescript
import { useCallback, useRef, useState, useEffect } from 'react';
import {
  Room,
  RoomEvent,
  LocalParticipant,
  LocalVideoTrack,
  RemoteParticipant,
  Track,
  LocalAudioTrack,
  createLocalVideoTrack,
  VideoPresets,
  Participant,
} from 'livekit-client';
import { MicVAD } from '@ricky0123/vad-web';
import { getVoiceToken, joinVoiceChannel, leaveVoiceChannel } from '../api/voice';

export interface VoiceSettings {
  micDeviceId?: string;
  speakerDeviceId?: string;
  cameraDeviceId?: string;
  noiseSuppression: boolean;
  vadMode: 'vad' | 'ptt' | 'always';
  pttKey: string; // KeyboardEvent.code e.g. 'Space', 'KeyV'
}

const DEFAULT_SETTINGS: VoiceSettings = {
  noiseSuppression: true,
  vadMode: 'vad',
  pttKey: 'Space',
};

export function loadVoiceSettings(): VoiceSettings {
  try {
    const raw = localStorage.getItem('parley_voice_settings');
    return raw ? { ...DEFAULT_SETTINGS, ...JSON.parse(raw) } : DEFAULT_SETTINGS;
  } catch {
    return DEFAULT_SETTINGS;
  }
}

function persistVoiceSettings(patch: Partial<VoiceSettings>) {
  const current = loadVoiceSettings();
  localStorage.setItem('parley_voice_settings', JSON.stringify({ ...current, ...patch }));
}

export interface VoiceConnectionReturn {
  connected: boolean;
  connecting: boolean;
  error: string | null;
  muted: boolean;
  deafened: boolean;
  videoEnabled: boolean;
  screenSharing: boolean;
  speaking: boolean;
  activeSpeakers: Set<string>;
  participants: RemoteParticipant[];
  localParticipant: LocalParticipant | null;
  settings: VoiceSettings;
  toggleMute: () => void;
  toggleDeafen: () => void;
  toggleVideo: () => Promise<void>;
  toggleScreenShare: () => Promise<void>;
  disconnect: () => void;
  retry: () => void;
  updateSettings: (patch: Partial<VoiceSettings>) => void;
}

export function useVoiceConnection(
  channelId: string | null,
  onDisconnected: () => void,
): VoiceConnectionReturn {
  const roomRef = useRef<Room | null>(null);
  const bcRef = useRef<BroadcastChannel | null>(null);
  const audioRefsMap = useRef<Map<string, HTMLAudioElement>>(new Map());
  const vadRef = useRef<MicVAD | null>(null);
  const audioTrackRef = useRef<LocalAudioTrack | null>(null);
  const videoTrackRef = useRef<LocalVideoTrack | null>(null);
  const channelIdRef = useRef<string | null>(null);
  const onDisconnectedRef = useRef(onDisconnected);
  onDisconnectedRef.current = onDisconnected;

  const [settings, setSettings] = useState<VoiceSettings>(loadVoiceSettings);
  const settingsRef = useRef(settings);
  settingsRef.current = settings;

  const [connected, setConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [muted, setMuted] = useState(false);
  const [deafened, setDeafened] = useState(false);
  const [videoEnabled, setVideoEnabled] = useState(false);
  const [screenSharing, setScreenSharing] = useState(false);
  const [speaking, setSpeaking] = useState(false);
  const [activeSpeakers, setActiveSpeakers] = useState<Set<string>>(new Set());
  const [participants, setParticipants] = useState<RemoteParticipant[]>([]);
  const [localParticipant, setLocalParticipant] = useState<LocalParticipant | null>(null);

  const updateParticipantList = useCallback(() => {
    if (!roomRef.current) { setParticipants([]); setLocalParticipant(null); return; }
    setParticipants(Array.from(roomRef.current.remoteParticipants.values()));
    setLocalParticipant(roomRef.current.localParticipant);
  }, []);

  const applyOutputDevice = useCallback((deviceId: string) => {
    document.querySelectorAll('audio').forEach(el => {
      if ('setSinkId' in el) (el as any).setSinkId(deviceId).catch(() => {});
    });
  }, []);

  const cleanupAudio = useCallback(() => {
    audioRefsMap.current.forEach(el => el.remove());
    audioRefsMap.current.clear();
  }, []);

  const attachAudio = useCallback((participant: RemoteParticipant) => {
    participant.getTrackPublications().forEach(pub => {
      if (pub.kind === Track.Kind.Audio && pub.track) {
        let el = audioRefsMap.current.get(participant.identity);
        if (!el) {
          el = document.createElement('audio');
          el.autoplay = true;
          document.body.appendChild(el);
          audioRefsMap.current.set(participant.identity, el);
        }
        pub.track.attach(el);
        el.muted = deafened;
        const savedSpeaker = settingsRef.current.speakerDeviceId;
        if (savedSpeaker && 'setSinkId' in el) (el as any).setSinkId(savedSpeaker).catch(() => {});
      }
    });
  }, [deafened]);

  const detachAudio = useCallback((identity: string) => {
    const el = audioRefsMap.current.get(identity);
    if (el) { el.remove(); audioRefsMap.current.delete(identity); }
  }, []);

  const doDisconnect = useCallback(() => {
    // VAD cleanup
    vadRef.current?.destroy();
    vadRef.current = null;
    // BroadcastChannel
    bcRef.current?.close();
    bcRef.current = null;
    // Video track
    if (videoTrackRef.current) {
      videoTrackRef.current.stop();
      videoTrackRef.current = null;
    }
    // Audio track
    audioTrackRef.current = null;
    // Leave room
    const cid = channelIdRef.current;
    if (cid) leaveVoiceChannel(cid).catch(() => {});
    roomRef.current?.disconnect();
    roomRef.current = null;
    channelIdRef.current = null;
    cleanupAudio();
    setConnected(false);
    setMuted(false);
    setDeafened(false);
    setVideoEnabled(false);
    setScreenSharing(false);
    setSpeaking(false);
    setActiveSpeakers(new Set());
    setParticipants([]);
    setLocalParticipant(null);
    onDisconnectedRef.current();
  }, [cleanupAudio]);

  const connect = useCallback(async (cid: string) => {
    if (roomRef.current) return;
    channelIdRef.current = cid;
    setConnecting(true);
    setError(null);

    try {
      const { token, url } = await getVoiceToken(cid);

      // Multi-tab claim
      const bc = new BroadcastChannel('parley_voice');
      bcRef.current = bc;
      bc.onmessage = (e) => { if (e.data?.action === 'claim') doDisconnect(); };
      bc.postMessage({ action: 'claim', channelId: cid });

      // Acquire mic with noise suppression constraints
      const s = settingsRef.current;
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          deviceId: s.micDeviceId ? { exact: s.micDeviceId } : undefined,
          noiseSuppression: s.noiseSuppression,
          echoCancellation: s.noiseSuppression,
          autoGainControl: true,
        },
      });

      const room = new Room({ adaptiveStream: true, dynacast: true });
      roomRef.current = room;

      // Participant list events
      room.on(RoomEvent.ParticipantConnected, updateParticipantList);
      room.on(RoomEvent.ParticipantDisconnected, (p) => {
        detachAudio(p.identity);
        updateParticipantList();
      });
      room.on(RoomEvent.TrackSubscribed, (_t, _p, participant) => {
        attachAudio(participant as RemoteParticipant);
        updateParticipantList();
      });
      room.on(RoomEvent.TrackUnsubscribed, (_t, _p, participant) => {
        detachAudio(participant.identity);
        updateParticipantList();
      });
      room.on(RoomEvent.TrackPublished, updateParticipantList);
      room.on(RoomEvent.TrackUnpublished, updateParticipantList);
      room.on(RoomEvent.LocalTrackPublished, (pub) => {
        if (pub.source === Track.Source.ScreenShare) setScreenSharing(true);
        updateParticipantList();
      });
      room.on(RoomEvent.LocalTrackUnpublished, (pub) => {
        if (pub.source === Track.Source.ScreenShare) setScreenSharing(false);
        updateParticipantList();
      });
      room.on(RoomEvent.ActiveSpeakersChanged, (speakers: Participant[]) => {
        const ids = new Set(speakers.map(sp => sp.identity));
        setActiveSpeakers(ids);
      });
      room.on(RoomEvent.Disconnected, () => {
        setConnected(false);
        leaveVoiceChannel(cid).catch(() => {});
        onDisconnectedRef.current();
      });

      await room.connect(url, token);

      // Publish mic from shared stream (same stream used by VAD below)
      const micTrack = new LocalAudioTrack(stream.getAudioTracks()[0]);
      audioTrackRef.current = micTrack;
      await room.localParticipant.publishTrack(micTrack);

      // VAD (uses same stream — no second getUserMedia call)
      if (s.vadMode === 'vad') {
        const vad = await MicVAD.new({
          stream,
          onSpeechStart: () => {
            if (settingsRef.current.vadMode !== 'vad') return;
            audioTrackRef.current?.unmute();
            setMuted(false);
            setSpeaking(true);
          },
          onSpeechEnd: () => {
            if (settingsRef.current.vadMode !== 'vad') return;
            audioTrackRef.current?.mute();
            setMuted(true);
            setSpeaking(false);
          },
        });
        vadRef.current = vad;
        vad.start();
        // Start muted; VAD will unmute on speech
        micTrack.mute();
        setMuted(true);
      }

      // Existing remote participants
      room.remoteParticipants.forEach(p => attachAudio(p));

      // Apply saved output device
      if (s.speakerDeviceId) applyOutputDevice(s.speakerDeviceId);

      await joinVoiceChannel(cid);
      updateParticipantList();
      setConnected(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to connect');
      roomRef.current?.disconnect();
      roomRef.current = null;
      bcRef.current?.close();
      bcRef.current = null;
    } finally {
      setConnecting(false);
    }
  }, [attachAudio, detachAudio, doDisconnect, updateParticipantList, applyOutputDevice]);

  // Connect when channelId is set
  useEffect(() => {
    if (!channelId) return;
    connect(channelId);
    return () => {
      vadRef.current?.destroy();
      vadRef.current = null;
      bcRef.current?.close();
      bcRef.current = null;
      if (channelIdRef.current) leaveVoiceChannel(channelIdRef.current).catch(() => {});
      roomRef.current?.disconnect();
      roomRef.current = null;
      videoTrackRef.current?.stop();
      videoTrackRef.current = null;
      cleanupAudio();
    };
  }, [channelId]); // eslint-disable-line react-hooks/exhaustive-deps

  // PTT global listeners
  useEffect(() => {
    if (!connected) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (settingsRef.current.vadMode !== 'ptt') return;
      if (e.code !== settingsRef.current.pttKey) return;
      // Guard: skip when user is typing
      const target = e.target as HTMLElement;
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) return;
      audioTrackRef.current?.unmute();
      setMuted(false);
      setSpeaking(true);
    };

    const handleKeyUp = (e: KeyboardEvent) => {
      if (settingsRef.current.vadMode !== 'ptt') return;
      if (e.code !== settingsRef.current.pttKey) return;
      audioTrackRef.current?.mute();
      setMuted(true);
      setSpeaking(false);
    };

    window.addEventListener('keydown', handleKeyDown);
    window.addEventListener('keyup', handleKeyUp);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
      window.removeEventListener('keyup', handleKeyUp);
    };
  }, [connected]);

  const toggleMute = useCallback(() => {
    if (!audioTrackRef.current) return;
    // PTT/VAD modes: manual mute toggle overrides
    const next = !muted;
    if (next) audioTrackRef.current.mute(); else audioTrackRef.current.unmute();
    setMuted(next);
    if (!next) setSpeaking(true); else setSpeaking(false);
  }, [muted]);

  const toggleDeafen = useCallback(() => {
    const next = !deafened;
    audioRefsMap.current.forEach(el => { el.muted = next; });
    setDeafened(next);
  }, [deafened]);

  const toggleVideo = useCallback(async () => {
    if (!roomRef.current) return;
    if (videoEnabled) {
      // Disable
      await roomRef.current.localParticipant.setCameraEnabled(false);
      videoTrackRef.current?.stop();
      videoTrackRef.current = null;
      setVideoEnabled(false);
    } else {
      // Enable
      const s = settingsRef.current;
      const vt = await createLocalVideoTrack({
        resolution: VideoPresets.h720.resolution, // VideoPresets.h720 is a VideoPreset, not VideoResolution — use .resolution
        deviceId: s.cameraDeviceId,
      });
      videoTrackRef.current = vt as any;
      await roomRef.current.localParticipant.publishTrack(vt);
      setVideoEnabled(true);
    }
    updateParticipantList();
  }, [videoEnabled, updateParticipantList]);

  const toggleScreenShare = useCallback(async () => {
    if (!roomRef.current) return;
    if (screenSharing) {
      await roomRef.current.localParticipant.setScreenShareEnabled(false);
      setScreenSharing(false);
    } else {
      await roomRef.current.localParticipant.setScreenShareEnabled(true, { audio: true });
      // screenSharing state is set via RoomEvent.LocalTrackPublished
    }
    updateParticipantList();
  }, [screenSharing, updateParticipantList]);

  const updateSettings = useCallback((patch: Partial<VoiceSettings>) => {
    persistVoiceSettings(patch);
    setSettings(prev => ({ ...prev, ...patch }));
    // Apply output device immediately if changed
    if (patch.speakerDeviceId) applyOutputDevice(patch.speakerDeviceId);
  }, [applyOutputDevice]);

  return {
    connected, connecting, error,
    muted, deafened, videoEnabled, screenSharing, speaking,
    activeSpeakers, participants, localParticipant,
    settings,
    toggleMute, toggleDeafen, toggleVideo, toggleScreenShare,
    disconnect: doDisconnect,
    retry: () => { if (channelId) connect(channelId); },
    updateSettings,
  };
}
```

- [ ] **Verify TypeScript compiles (requires Task 2 `npm install` to have completed in this environment first):**

```bash
cd /home/dylan/Developer/parley/frontend && npx tsc --noEmit 2>&1 | head -30
```

Expected: few or no errors. The `vadMode` setting takes effect on next connect — changing it while connected requires reconnecting.

**Note on VAD mode switching:** The VAD instance is created at connect time based on the `vadMode` setting. If the user changes `vadMode` via `VoiceSettings` while connected, the change persists to `localStorage` and takes effect on the next connection. In-flight mode switching is out of scope for this iteration.

- [ ] **Commit:**

```bash
git add frontend/src/hooks/useVoiceConnection.ts
git commit -m "feat: rewrite useVoiceConnection with video, VAD, PTT, screen share"
```

---

## Chunk 3: Voice UI Components

### Task 4: Create `ParticipantTile`

**Files:**
- Create: `frontend/src/components/voice/ParticipantTile.tsx`
- Create: `frontend/src/components/voice/ParticipantTile.css`

`ParticipantTile` renders one participant slot. It gets a LiveKit `Participant` for track management and explicit `displayName`/`avatarUrl` props for metadata (since LiveKit tokens don't carry avatar URLs).

- [ ] **Create `frontend/src/components/voice/ParticipantTile.tsx`:**

```typescript
import React, { useRef, useEffect, useMemo } from 'react';
import { Participant, RemoteParticipant, LocalParticipant, Track } from 'livekit-client';
import { MicOff, Monitor } from 'lucide-react';
import './ParticipantTile.css';

interface ParticipantTileProps {
  participant: Participant;
  isLocal?: boolean;
  isSpeaking?: boolean;
  isScreenShare?: boolean;
  displayName?: string;
  avatarUrl?: string;
}

export const ParticipantTile: React.FC<ParticipantTileProps> = ({
  participant,
  isLocal,
  isSpeaking,
  isScreenShare,
  displayName,
  avatarUrl,
}) => {
  const videoRef = useRef<HTMLVideoElement>(null);

  const videoPublication = useMemo(() => {
    const source = isScreenShare ? Track.Source.ScreenShare : Track.Source.Camera;
    return Array.from(participant.trackPublications.values()).find(
      p => p.source === source
    );
  }, [participant, participant.trackPublications.size, isScreenShare]); // eslint-disable-line react-hooks/exhaustive-deps

  const hasVideo = !!(videoPublication?.track && !videoPublication.isMuted);

  useEffect(() => {
    if (!videoRef.current || !videoPublication?.track) return;
    videoPublication.track.attach(videoRef.current);
    return () => {
      videoPublication.track?.detach(videoRef.current!);
    };
  }, [videoPublication?.track]);

  const micPublication = useMemo(() => {
    return Array.from(participant.trackPublications.values()).find(
      p => p.source === Track.Source.Microphone
    );
  }, [participant, participant.trackPublications.size]); // eslint-disable-line react-hooks/exhaustive-deps

  const isMuted = micPublication ? micPublication.isMuted : true;
  const name = displayName ?? participant.name ?? participant.identity;
  const initial = name.charAt(0).toUpperCase();

  return (
    <div className={`participant-tile ${isSpeaking ? 'participant-tile--speaking' : ''} ${isScreenShare ? 'participant-tile--screen' : ''}`}>
      <div className="participant-tile-media">
        {hasVideo ? (
          <video
            ref={videoRef}
            autoPlay
            playsInline
            muted={isLocal}
            className="participant-tile-video"
          />
        ) : (
          <div className="participant-tile-avatar">
            {isScreenShare ? (
              <Monitor size={40} color="#32CD32" />
            ) : avatarUrl ? (
              <img src={avatarUrl} alt={name} />
            ) : (
              <span className="participant-tile-initial">{initial}</span>
            )}
          </div>
        )}
      </div>

      <div className="participant-tile-footer">
        <span className="participant-tile-name">
          {name}
          {isLocal && <span className="participant-tile-you">You</span>}
        </span>
        {isMuted && !isScreenShare && (
          <span className="participant-tile-muted">
            <MicOff size={12} color="#cc4444" />
          </span>
        )}
      </div>
    </div>
  );
};
```

- [ ] **Create `frontend/src/components/voice/ParticipantTile.css`:**

```css
.participant-tile {
  position: relative;
  background: #0a0c0f;
  border: 1px solid #1e2228;
  border-radius: 8px;
  overflow: hidden;
  aspect-ratio: 16 / 9;
  display: flex;
  flex-direction: column;
  transition: border-color 0.15s;
}

.participant-tile--speaking {
  border-color: #32CD32;
  box-shadow: 0 0 0 2px #32CD32, 0 0 8px rgba(50, 205, 50, 0.4);
}

.participant-tile--screen {
  aspect-ratio: 16 / 9;
  border-color: #2255cc;
}

.participant-tile-media {
  flex: 1;
  position: relative;
  background: #060809;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

.participant-tile-video {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.participant-tile-avatar {
  width: 80px;
  height: 80px;
  border-radius: 50%;
  background: #1a1d22;
  border: 2px solid #2a2d32;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

.participant-tile-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  border-radius: 50%;
}

.participant-tile-initial {
  font-size: 28px;
  font-weight: 700;
  color: #32CD32;
  font-family: inherit;
}

.participant-tile-footer {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  background: rgba(0, 0, 0, 0.6);
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
}

.participant-tile-name {
  flex: 1;
  font-size: 12px;
  font-weight: 600;
  color: #ddd;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: flex;
  align-items: center;
  gap: 6px;
}

.participant-tile-you {
  font-size: 10px;
  font-weight: 700;
  color: #32CD32;
  background: rgba(50, 205, 50, 0.15);
  border: 1px solid rgba(50, 205, 50, 0.3);
  border-radius: 3px;
  padding: 1px 5px;
}

.participant-tile-muted {
  flex-shrink: 0;
  display: flex;
  align-items: center;
}
```

- [ ] **Commit:**

```bash
git add frontend/src/components/voice/ParticipantTile.tsx frontend/src/components/voice/ParticipantTile.css
git commit -m "feat: add ParticipantTile component with video, avatar fallback, speaking glow"
```

---

### Task 5: Rewrite `VoiceChannel`

**Files:**
- Modify: `frontend/src/components/voice/VoiceChannel.tsx` (full rewrite)
- Modify: `frontend/src/components/voice/VoiceChannel.css` (full rewrite)

The new `VoiceChannel` reads voice state from props (passed from `App.tsx`). It owns the grid/speaker toggle. It does NOT own the connection — it just displays what the hook gives it.

The chat panel is rendered alongside this by `App.tsx` in the existing `.vc-layout` wrapper.

- [ ] **Replace `frontend/src/components/voice/VoiceChannel.tsx`:**

```typescript
import React, { useState, useMemo } from 'react';
import { RemoteParticipant, LocalParticipant } from 'livekit-client';
import { LayoutGrid, Maximize2, Mic, MicOff, Headphones, HeadphonesOff, Video, VideoOff, Monitor, MonitorOff, PhoneOff, Volume2 } from 'lucide-react';
import { Channel } from '../../api/types';
import { VoiceParticipant } from '../../api/voice';
import { ParticipantTile } from './ParticipantTile';
import './VoiceChannel.css';

interface VoiceChannelProps {
  channel: Channel;
  currentUser: { id: string; username: string; avatar_url?: string };
  participants: RemoteParticipant[];
  localParticipant: LocalParticipant | null;
  voiceParticipants: Record<string, VoiceParticipant>; // userID → metadata
  activeSpeakers: Set<string>;
  connected: boolean;
  connecting: boolean;
  error: string | null;
  muted: boolean;
  deafened: boolean;
  videoEnabled: boolean;
  screenSharing: boolean;
  onToggleMute: () => void;
  onToggleDeafen: () => void;
  onToggleVideo: () => Promise<void>;
  onToggleScreenShare: () => Promise<void>;
  onLeave: () => void;
  onRetry: () => void;
}

type ViewMode = 'grid' | 'speaker';

export const VoiceChannel: React.FC<VoiceChannelProps> = ({
  channel,
  currentUser, // currentUser.id is used via localMeta below
  participants,
  localParticipant,
  voiceParticipants,
  activeSpeakers,
  connected,
  connecting,
  error,
  muted,
  deafened,
  videoEnabled,
  screenSharing,
  onToggleMute,
  onToggleDeafen,
  onToggleVideo,
  onToggleScreenShare,
  onLeave,
  onRetry,
}) => {
  const [viewMode, setViewMode] = useState<ViewMode>('grid');
  const [pinnedIdentity, setPinnedIdentity] = useState<string | null>(null);

  // Build tile list: local first, then remotes
  const allParticipants = useMemo(() => {
    const list: Array<{ participant: LocalParticipant | RemoteParticipant; isLocal: boolean }> = [];
    if (localParticipant) list.push({ participant: localParticipant, isLocal: true });
    participants.forEach(p => list.push({ participant: p, isLocal: false }));
    return list;
  }, [localParticipant, participants]);

  // Screen share tiles
  const screenShares = useMemo(() => {
    return allParticipants.filter(({ participant }) => {
      return Array.from(participant.trackPublications.values()).some(
        pub => pub.source === Track.Source.ScreenShare && pub.track && !pub.isMuted
      );
    });
  }, [allParticipants]);

  // In speaker view: find spotlight participant
  const spotlightIdentity = pinnedIdentity ?? (activeSpeakers.size > 0 ? Array.from(activeSpeakers)[0] : null);
  const spotlightParticipant = allParticipants.find(({ participant }) => participant.identity === spotlightIdentity) ?? allParticipants[0];
  const filmstripParticipants = allParticipants.filter(({ participant }) => participant !== spotlightParticipant?.participant);

  const getMeta = (identity: string) => {
    const meta = voiceParticipants[identity];
    return { displayName: meta?.username, avatarUrl: meta?.avatar_url };
  };
  const localMeta = { displayName: currentUser.username, avatarUrl: currentUser.avatar_url };

  const statusLabel = connected ? 'Connected' : connecting ? 'Connecting…' : 'Disconnected';

  return (
    <div className="vc-view">
      {/* Header */}
      <div className="vc-header">
        <div className="vc-header-left">
          <Volume2 size={16} color="#32CD32" />
          <span className="vc-channel-name">{channel.name}</span>
          <span className={`vc-status ${connected ? 'connected' : connecting ? 'connecting' : 'error'}`}>
            {statusLabel}
          </span>
        </div>
        <div className="vc-header-controls">
          <button
            className={`vc-hdr-btn ${viewMode === 'grid' ? 'active' : ''}`}
            onClick={() => setViewMode('grid')}
            title="Grid view"
          >
            <LayoutGrid size={16} />
          </button>
          <button
            className={`vc-hdr-btn ${viewMode === 'speaker' ? 'active' : ''}`}
            onClick={() => setViewMode('speaker')}
            title="Speaker view"
          >
            <Maximize2 size={16} />
          </button>
        </div>
      </div>

      {error && (
        <div className="vc-error">
          {error} — <button onClick={onRetry} className="vc-retry-btn">Retry</button>
        </div>
      )}

      {/* Main area */}
      {viewMode === 'grid' ? (
        <div className="vc-grid">
          {/* Screen share tiles */}
          {screenShares.map(({ participant, isLocal }) => {
            const meta = isLocal ? localMeta : getMeta(participant.identity);
            return (
              <ParticipantTile
                key={`screen-${participant.identity}`}
                participant={participant}
                isLocal={isLocal}
                isSpeaking={activeSpeakers.has(participant.identity)}
                isScreenShare
                displayName={meta.displayName}
                avatarUrl={meta.avatarUrl}
              />
            );
          })}
          {/* Participant tiles */}
          {allParticipants.map(({ participant, isLocal }) => {
            const meta = isLocal ? localMeta : getMeta(participant.identity);
            return (
              <ParticipantTile
                key={participant.identity}
                participant={participant}
                isLocal={isLocal}
                isSpeaking={isLocal ? false : activeSpeakers.has(participant.identity)}
                displayName={meta.displayName}
                avatarUrl={meta.avatarUrl}
              />
            );
          })}
          {allParticipants.length === 0 && (
            <div className="vc-empty">No one else here yet…</div>
          )}
        </div>
      ) : (
        <div className="vc-speaker">
          {spotlightParticipant ? (
            <div
              className="vc-spotlight"
              onClick={() => setPinnedIdentity(
                pinnedIdentity === spotlightParticipant.participant.identity
                  ? null
                  : spotlightParticipant.participant.identity
              )}
            >
              <ParticipantTile
                participant={spotlightParticipant.participant}
                isLocal={spotlightParticipant.isLocal}
                isSpeaking={activeSpeakers.has(spotlightParticipant.participant.identity)}
                displayName={spotlightParticipant.isLocal ? localMeta.displayName : getMeta(spotlightParticipant.participant.identity).displayName}
                avatarUrl={spotlightParticipant.isLocal ? localMeta.avatarUrl : getMeta(spotlightParticipant.participant.identity).avatarUrl}
              />
            </div>
          ) : (
            <div className="vc-empty">No one here yet…</div>
          )}
          {filmstripParticipants.length > 0 && (
            <div className="vc-filmstrip">
              {filmstripParticipants.map(({ participant, isLocal }) => {
                const meta = isLocal ? localMeta : getMeta(participant.identity);
                return (
                  <div
                    key={participant.identity}
                    className="vc-filmstrip-tile"
                    onClick={() => setPinnedIdentity(participant.identity)}
                  >
                    <ParticipantTile
                      participant={participant}
                      isLocal={isLocal}
                      isSpeaking={activeSpeakers.has(participant.identity)}
                      displayName={meta.displayName}
                      avatarUrl={meta.avatarUrl}
                    />
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}

      {/* In-channel controls (secondary; main controls are in VoiceControls widget) */}
      <div className="vc-controls">
        <button className={`vc-ctrl ${muted ? 'vc-ctrl--off' : ''}`} onClick={onToggleMute} title={muted ? 'Unmute' : 'Mute'}>
          {muted ? <MicOff size={18} color="#cc4444" /> : <Mic size={18} color="#32CD32" />}
        </button>
        <button className={`vc-ctrl ${deafened ? 'vc-ctrl--off' : ''}`} onClick={onToggleDeafen} title={deafened ? 'Undeafen' : 'Deafen'}>
          {deafened ? <HeadphonesOff size={18} color="#cc4444" /> : <Headphones size={18} color="#32CD32" />}
        </button>
        <button className={`vc-ctrl ${videoEnabled ? '' : 'vc-ctrl--off'}`} onClick={onToggleVideo} title={videoEnabled ? 'Turn off camera' : 'Turn on camera'}>
          {videoEnabled ? <Video size={18} color="#32CD32" /> : <VideoOff size={18} color="#555" />}
        </button>
        <button className={`vc-ctrl ${screenSharing ? '' : 'vc-ctrl--off'}`} onClick={onToggleScreenShare} title={screenSharing ? 'Stop sharing' : 'Share screen'}>
          {screenSharing ? <Monitor size={18} color="#32CD32" /> : <MonitorOff size={18} color="#555" />}
        </button>
        <button className="vc-ctrl vc-ctrl--leave" onClick={onLeave} title="Leave channel">
          <PhoneOff size={18} color="#cc4444" />
        </button>
      </div>
    </div>
  );
};
```

- [ ] **Replace `frontend/src/components/voice/VoiceChannel.css`:**

```css
.vc-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: #060809;
  overflow: hidden;
}

/* Header */
.vc-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  background: #0a0c0f;
  border-bottom: 1px solid #1e2228;
  flex-shrink: 0;
}

.vc-header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.vc-channel-name {
  font-size: 15px;
  font-weight: 700;
  color: #ddd;
}

.vc-status {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 10px;
  font-weight: 600;
}

.vc-status.connected { background: rgba(50,205,50,0.15); color: #32CD32; }
.vc-status.connecting { background: rgba(240,178,50,0.15); color: #f0b232; }
.vc-status.error { background: rgba(204,68,68,0.15); color: #cc4444; }

.vc-header-controls {
  display: flex;
  gap: 4px;
}

.vc-hdr-btn {
  background: none;
  border: 1px solid #2a2d32;
  border-radius: 4px;
  color: #555;
  padding: 6px 8px;
  cursor: pointer;
  display: flex;
  align-items: center;
  transition: background 0.1s, color 0.1s;
}

.vc-hdr-btn:hover, .vc-hdr-btn.active {
  background: rgba(50, 205, 50, 0.1);
  color: #32CD32;
  border-color: rgba(50, 205, 50, 0.3);
}

/* Error bar */
.vc-error {
  background: #2a0f0f;
  border-bottom: 1px solid #5a1a1a;
  color: #ff7777;
  padding: 8px 16px;
  font-size: 13px;
  flex-shrink: 0;
}

.vc-retry-btn {
  background: none;
  border: 1px solid #cc4444;
  color: #cc4444;
  border-radius: 3px;
  padding: 1px 8px;
  cursor: pointer;
  font-family: inherit;
  font-size: 12px;
  margin-left: 8px;
}

/* Grid view */
.vc-grid {
  flex: 1;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 8px;
  padding: 16px;
  overflow-y: auto;
  align-content: start;
}

.vc-empty {
  color: #444;
  font-size: 13px;
  text-align: center;
  padding: 40px;
  grid-column: 1 / -1;
}

/* Speaker view */
.vc-speaker {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.vc-spotlight {
  flex: 1;
  padding: 12px;
  min-height: 0;
  cursor: pointer;
}

.vc-spotlight .participant-tile {
  height: 100%;
  aspect-ratio: unset;
}

.vc-filmstrip {
  display: flex;
  gap: 8px;
  padding: 8px 12px;
  overflow-x: auto;
  background: #0a0c0f;
  border-top: 1px solid #1e2228;
  flex-shrink: 0;
}

.vc-filmstrip-tile {
  width: 160px;
  flex-shrink: 0;
  cursor: pointer;
}

/* In-channel bottom controls */
.vc-controls {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  background: #0a0c0f;
  border-top: 1px solid #1e2228;
  flex-shrink: 0;
  justify-content: center;
}

.vc-ctrl {
  background: #16191d;
  border: 1px solid #2a2d32;
  border-radius: 8px;
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background 0.1s, border-color 0.1s;
}

.vc-ctrl:hover {
  background: #1e2228;
}

.vc-ctrl--off {
  border-color: #3a1a1a;
}

.vc-ctrl--leave {
  border-color: #3a1a1a;
}

.vc-ctrl--leave:hover {
  background: #2a1010;
  border-color: #cc4444;
}
```

- [ ] **Commit:**

```bash
git add frontend/src/components/voice/VoiceChannel.tsx frontend/src/components/voice/VoiceChannel.css
git commit -m "feat: rewrite VoiceChannel with grid/speaker view, video tiles, screen share"
```

---

### Task 6: Create `VoiceControls` (persistent bottom-left widget)

**Files:**
- Create: `frontend/src/components/voice/VoiceControls.tsx`
- Create: `frontend/src/components/voice/VoiceControls.css`

This widget is always visible when connected. It is rendered by `App.tsx` and positioned fixed.

- [ ] **Create `frontend/src/components/voice/VoiceControls.tsx`:**

```typescript
import React from 'react';
import { Mic, MicOff, Headphones, HeadphonesOff, Video, VideoOff, Monitor, MonitorOff, PhoneOff } from 'lucide-react';
import './VoiceControls.css';

interface VoiceControlsProps {
  channelName: string;
  muted: boolean;
  deafened: boolean;
  videoEnabled: boolean;
  screenSharing: boolean;
  vadMode: 'vad' | 'ptt' | 'always';
  pttKey: string;
  onNavigate: () => void;
  onToggleMute: () => void;
  onToggleDeafen: () => void;
  onToggleVideo: () => void;
  onToggleScreenShare: () => void;
  onDisconnect: () => void;
}

export const VoiceControls: React.FC<VoiceControlsProps> = ({
  channelName,
  muted,
  deafened,
  videoEnabled,
  screenSharing,
  vadMode,
  pttKey,
  onNavigate,
  onToggleMute,
  onToggleDeafen,
  onToggleVideo,
  onToggleScreenShare,
  onDisconnect,
}) => {
  const pttLabel = pttKey.replace('Key', '').replace('Digit', '').replace('Space', 'SPACE');

  return (
    <div className="voice-widget">
      <div className="voice-widget-status" onClick={onNavigate}>
        <span className="voice-widget-dot" />
        <div className="voice-widget-info">
          <span className="voice-widget-label">Voice Connected</span>
          <span className="voice-widget-channel">#{channelName}</span>
        </div>
      </div>
      <div className="voice-widget-controls">
        <button
          className={`vw-btn ${muted ? 'vw-btn--off' : ''}`}
          onClick={onToggleMute}
          title={muted ? 'Unmute' : 'Mute'}
        >
          {muted ? <MicOff size={16} color="#cc4444" /> : <Mic size={16} color="#32CD32" />}
        </button>
        <button
          className={`vw-btn ${deafened ? 'vw-btn--off' : ''}`}
          onClick={onToggleDeafen}
          title={deafened ? 'Undeafen' : 'Deafen'}
        >
          {deafened ? <HeadphonesOff size={16} color="#cc4444" /> : <Headphones size={16} color="#32CD32" />}
        </button>
        <button
          className={`vw-btn ${!videoEnabled ? 'vw-btn--off' : ''}`}
          onClick={onToggleVideo}
          title={videoEnabled ? 'Camera off' : 'Camera on'}
        >
          {videoEnabled ? <Video size={16} color="#32CD32" /> : <VideoOff size={16} color="#555" />}
        </button>
        <button
          className={`vw-btn ${!screenSharing ? 'vw-btn--off' : ''}`}
          onClick={onToggleScreenShare}
          title={screenSharing ? 'Stop sharing' : 'Share screen'}
        >
          {screenSharing ? <Monitor size={16} color="#32CD32" /> : <MonitorOff size={16} color="#555" />}
        </button>
        <button className="vw-btn vw-btn--leave" onClick={onDisconnect} title="Disconnect">
          <PhoneOff size={16} color="#cc4444" />
        </button>
      </div>
      {vadMode === 'ptt' && (
        <div className="voice-widget-ptt">
          Hold <kbd>{pttLabel}</kbd> to talk
        </div>
      )}
      {vadMode === 'vad' && (
        <div className="voice-widget-mode">Voice Activity</div>
      )}
      {vadMode === 'always' && (
        <div className="voice-widget-mode">Always transmitting</div>
      )}
    </div>
  );
};
```

- [ ] **Create `frontend/src/components/voice/VoiceControls.css`:**

```css
.voice-widget {
  position: fixed;
  bottom: 0;
  left: var(--sidebar-width, 232px);
  width: 230px;
  background: #0d0f12;
  border: 1px solid #1e2228;
  border-bottom: none;
  border-radius: 8px 8px 0 0;
  z-index: 200;
  font-family: inherit;
  box-shadow: 0 -4px 20px rgba(0, 0, 0, 0.5);
}

.voice-widget-status {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px 6px;
  cursor: pointer;
}

.voice-widget-status:hover .voice-widget-channel {
  color: #aaa;
}

.voice-widget-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #32CD32;
  flex-shrink: 0;
  box-shadow: 0 0 6px #32CD32;
}

.voice-widget-info {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.voice-widget-label {
  font-size: 11px;
  font-weight: 700;
  color: #32CD32;
  letter-spacing: 0.5px;
}

.voice-widget-channel {
  font-size: 11px;
  color: #777;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: color 0.1s;
}

.voice-widget-controls {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 10px 8px;
}

.vw-btn {
  background: none;
  border: 1px solid #2a2d32;
  border-radius: 6px;
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background 0.1s, border-color 0.1s;
  flex-shrink: 0;
}

.vw-btn:hover {
  background: #1a1d22;
}

.vw-btn--off {
  border-color: #2a2020;
}

.vw-btn--leave {
  border-color: #3a1a1a;
  margin-left: auto;
}

.vw-btn--leave:hover {
  background: #2a1010;
  border-color: #cc4444;
}

.voice-widget-ptt {
  padding: 0 12px 8px;
  font-size: 11px;
  color: #555;
}

.voice-widget-ptt kbd {
  background: #1a1d22;
  border: 1px solid #2a2d32;
  border-radius: 3px;
  padding: 1px 5px;
  font-size: 10px;
  font-family: inherit;
  color: #aaa;
}

.voice-widget-mode {
  padding: 0 12px 8px;
  font-size: 11px;
  color: #444;
}
```

- [ ] **Commit:**

```bash
git add frontend/src/components/voice/VoiceControls.tsx frontend/src/components/voice/VoiceControls.css
git commit -m "feat: add VoiceControls persistent widget with mute/deafen/video/screen/PTT"
```

---

## Chunk 4: App Integration + Settings

### Task 7: Wire new hook into `App.tsx` + render `VoiceControls`

**Files:**
- Modify: `frontend/src/App.tsx`

This task wires the new hook return values into the existing `App.tsx` render tree and renders `VoiceControls`.

- [ ] **In `App.tsx`, update the `useVoiceConnection` destructuring (around line 136).** The hook now returns more values. Replace:

```typescript
const {
  connected: vcConnected,
  connecting: vcConnecting,
  error: vcError,
  muted: vcMuted,
  deafened: vcDeafened,
  toggleMute: vcToggleMute,
  toggleDeafen: vcToggleDeafen,
  disconnect: vcDisconnect,
  retry: vcRetry,
} = useVoiceConnection(activeVoiceChannel, handleVcLeave);
```

With:

```typescript
const {
  connected: vcConnected,
  connecting: vcConnecting,
  error: vcError,
  muted: vcMuted,
  deafened: vcDeafened,
  videoEnabled: vcVideoEnabled,
  screenSharing: vcScreenSharing,
  activeSpeakers: vcActiveSpeakers,
  participants: vcParticipants,
  localParticipant: vcLocalParticipant,
  settings: vcSettings,
  toggleMute: vcToggleMute,
  toggleDeafen: vcToggleDeafen,
  toggleVideo: vcToggleVideo,
  toggleScreenShare: vcToggleScreenShare,
  disconnect: vcDisconnect,
  retry: vcRetry,
} = useVoiceConnection(activeVoiceChannel, handleVcLeave);
```

- [ ] **Add the `VoiceControls` import at the top of `App.tsx`:**

```typescript
import { VoiceControls } from './components/voice/VoiceControls';
```

- [ ] **Find the `VoiceChannel` render in `App.tsx` (around line 633) and update its props to match the new interface.**

**Important:** The existing `voiceParticipants` state in `App.tsx` is shaped `Record<channelId, VoiceParticipant[]>`. The new `VoiceChannel` component expects `Record<userId, VoiceParticipant>`. You must transform it at the call site:

```typescript
<VoiceChannel
  channel={vcChannel}
  currentUser={{ id: currentUser.id, username: currentUser.username, avatar_url: currentUser.avatar_url }}
  participants={vcParticipants}
  localParticipant={vcLocalParticipant}
  voiceParticipants={Object.fromEntries(
    (voiceParticipants[activeVoiceChannel!] ?? []).map(p => [p.user_id, p])
  )}
  activeSpeakers={vcActiveSpeakers}
  connected={vcConnected}
  connecting={vcConnecting}
  error={vcError}
  muted={vcMuted}
  deafened={vcDeafened}
  videoEnabled={vcVideoEnabled}
  screenSharing={vcScreenSharing}
  onToggleMute={vcToggleMute}
  onToggleDeafen={vcToggleDeafen}
  onToggleVideo={vcToggleVideo}
  onToggleScreenShare={vcToggleScreenShare}
  onLeave={vcDisconnect}
  onRetry={vcRetry}
/>
```

- [ ] **Render `VoiceControls` in the JSX return of `MainApp`. Find where `MainLayout` is rendered and add `VoiceControls` just before or after it (it's `position: fixed` so DOM position doesn't matter):**

```typescript
{vcConnected && vcChannel && (
  <VoiceControls
    channelName={vcChannel.name}
    muted={vcMuted}
    deafened={vcDeafened}
    videoEnabled={vcVideoEnabled}
    screenSharing={vcScreenSharing}
    vadMode={vcSettings.vadMode}
    pttKey={vcSettings.pttKey}
    onNavigate={() => selectChannel(vcChannel.id)}
    onToggleMute={vcToggleMute}
    onToggleDeafen={vcToggleDeafen}
    onToggleVideo={() => vcToggleVideo()}
    onToggleScreenShare={() => vcToggleScreenShare()}
    onDisconnect={vcDisconnect}
  />
)}
```

- [ ] **Add `.main-content--voice-connected` to `frontend/src/components/layout/MainLayout.css`:**

```css
.main-content--voice-connected {
  padding-bottom: 96px;
}
```

- [ ] **Update `frontend/src/components/layout/MainLayout.tsx`**: Add `voiceConnected?: boolean` to `MainLayoutProps`, then apply the class on the content div:

```tsx
interface MainLayoutProps {
  // ... existing props ...
  voiceConnected?: boolean;
}

// In the JSX:
<div className={`main-content${voiceConnected ? ' main-content--voice-connected' : ''}`}>
  {children}
</div>
```

- [ ] **Pass `voiceConnected={vcConnected}` to `<MainLayout>` in `App.tsx`** — find the `<MainLayout` JSX and add this prop.

- [ ] **Verify TypeScript compiles:**

```bash
cd /home/dylan/Developer/parley/frontend && npx tsc --noEmit 2>&1 | head -40
```

Fix any type errors reported before continuing.

- [ ] **Start dev server and test:**

```bash
cd /home/dylan/Developer/parley/frontend && npm run dev
```

Open the app, join a voice channel. Verify:
- VoiceControls widget appears at bottom-left
- VoiceChannel renders in the main area with header, empty grid, controls bar
- Navigating to a text channel while connected keeps the widget visible
- Widget channel name click navigates back to voice channel

- [ ] **Commit:**

```bash
git add frontend/src/App.tsx frontend/src/components/layout/MainLayout.tsx frontend/src/components/layout/MainLayout.css
git commit -m "feat: wire video/screen share into App.tsx, render persistent VoiceControls widget"
```

---

### Task 8: Create `VoiceSettings` + add tab to `UserSettings`

**Files:**
- Create: `frontend/src/components/settings/VoiceSettings.tsx`
- Create: `frontend/src/components/settings/VoiceSettings.css`
- Modify: `frontend/src/components/settings/UserSettings.tsx`

- [ ] **Create `frontend/src/components/settings/VoiceSettings.tsx`:**

```typescript
import React, { useState, useEffect, useRef } from 'react';
import { Mic, Volume2, Video, Settings } from 'lucide-react';
import { loadVoiceSettings, VoiceSettings, persistVoiceSettings } from '../../hooks/useVoiceConnection';
// Note: import persistVoiceSettings — we need to export it from the hook file
import './VoiceSettings.css';

// Export persistVoiceSettings from useVoiceConnection.ts if not already done:
// export function persistVoiceSettings(patch: Partial<VoiceSettings>) { ... }

export const VoiceSettingsTab: React.FC = () => {
  const [settings, setSettings] = useState<VoiceSettings>(loadVoiceSettings);
  const [inputDevices, setInputDevices] = useState<MediaDeviceInfo[]>([]);
  const [outputDevices, setOutputDevices] = useState<MediaDeviceInfo[]>([]);
  const [videoDevices, setVideoDevices] = useState<MediaDeviceInfo[]>([]);
  const [bindingPtt, setBindingPtt] = useState(false);
  const [testRecording, setTestRecording] = useState<'idle' | 'recording' | 'playing'>('idle');
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);

  useEffect(() => {
    navigator.mediaDevices.enumerateDevices().then(devices => {
      setInputDevices(devices.filter(d => d.kind === 'audioinput'));
      setOutputDevices(devices.filter(d => d.kind === 'audiooutput'));
      setVideoDevices(devices.filter(d => d.kind === 'videoinput'));
    }).catch(() => {});
  }, []);

  const update = (patch: Partial<VoiceSettings>) => {
    const next = { ...settings, ...patch };
    setSettings(next);
    persistVoiceSettings(patch);
    // Apply output device immediately if changed
    if (patch.speakerDeviceId) {
      document.querySelectorAll('audio').forEach(el => {
        if ('setSinkId' in el) (el as any).setSinkId(patch.speakerDeviceId).catch(() => {});
      });
    }
  };

  const startPttBind = () => {
    setBindingPtt(true);
    const handler = (e: KeyboardEvent) => {
      e.preventDefault();
      update({ pttKey: e.code });
      setBindingPtt(false);
      window.removeEventListener('keydown', handler, true);
    };
    window.addEventListener('keydown', handler, true);
  };

  const testMic = async () => {
    if (testRecording !== 'idle') return;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: { deviceId: settings.micDeviceId ? { exact: settings.micDeviceId } : undefined },
      });
      const recorder = new MediaRecorder(stream);
      chunksRef.current = [];
      recorder.ondataavailable = e => chunksRef.current.push(e.data);
      recorder.onstop = () => {
        stream.getTracks().forEach(t => t.stop());
        const blob = new Blob(chunksRef.current, { type: 'audio/webm' });
        const url = URL.createObjectURL(blob);
        const audio = new Audio(url);
        setTestRecording('playing');
        audio.onended = () => { setTestRecording('idle'); URL.revokeObjectURL(url); };
        audio.play();
      };
      mediaRecorderRef.current = recorder;
      recorder.start();
      setTestRecording('recording');
      setTimeout(() => { recorder.stop(); }, 3000);
    } catch {
      setTestRecording('idle');
    }
  };

  const pttLabel = settings.pttKey.replace('Key', '').replace('Digit', '').replace('Space', 'SPACE');

  return (
    <div className="vs-container">
      <h2 className="vs-heading">Voice & Video</h2>

      {/* Input Device */}
      <div className="vs-section">
        <label className="vs-label"><Mic size={13} /> Input Device</label>
        <select
          className="vs-select"
          value={settings.micDeviceId ?? ''}
          onChange={e => update({ micDeviceId: e.target.value || undefined })}
        >
          <option value="">Default</option>
          {inputDevices.map(d => <option key={d.deviceId} value={d.deviceId}>{d.label || `Microphone ${d.deviceId.slice(0, 8)}`}</option>)}
        </select>
        <button className="vs-test-btn" onClick={testMic} disabled={testRecording !== 'idle'}>
          {testRecording === 'recording' ? 'Recording… (3s)' : testRecording === 'playing' ? 'Playing back…' : 'Test Microphone'}
        </button>
      </div>

      {/* Output Device */}
      <div className="vs-section">
        <label className="vs-label"><Volume2 size={13} /> Output Device</label>
        <select
          className="vs-select"
          value={settings.speakerDeviceId ?? ''}
          onChange={e => update({ speakerDeviceId: e.target.value || undefined })}
        >
          <option value="">Default</option>
          {outputDevices.map(d => <option key={d.deviceId} value={d.deviceId}>{d.label || `Speaker ${d.deviceId.slice(0, 8)}`}</option>)}
        </select>
      </div>

      {/* Camera */}
      <div className="vs-section">
        <label className="vs-label"><Video size={13} /> Camera</label>
        <select
          className="vs-select"
          value={settings.cameraDeviceId ?? ''}
          onChange={e => update({ cameraDeviceId: e.target.value || undefined })}
        >
          <option value="">Default</option>
          {videoDevices.map(d => <option key={d.deviceId} value={d.deviceId}>{d.label || `Camera ${d.deviceId.slice(0, 8)}`}</option>)}
        </select>
      </div>

      {/* Noise Suppression */}
      <div className="vs-section vs-section--row">
        <div className="vs-section-info">
          <label className="vs-label">Noise Suppression</label>
          <span className="vs-hint">Reduces background noise using browser audio processing</span>
        </div>
        <button
          className={`vs-toggle ${settings.noiseSuppression ? 'vs-toggle--on' : ''}`}
          onClick={() => update({ noiseSuppression: !settings.noiseSuppression })}
        >
          {settings.noiseSuppression ? 'On' : 'Off'}
        </button>
      </div>

      {/* Voice Mode */}
      <div className="vs-section">
        <label className="vs-label"><Settings size={13} /> Voice Mode</label>
        <div className="vs-radio-group">
          {(['vad', 'ptt', 'always'] as const).map(mode => (
            <label key={mode} className={`vs-radio-option ${settings.vadMode === mode ? 'active' : ''}`}>
              <input
                type="radio"
                name="vadMode"
                value={mode}
                checked={settings.vadMode === mode}
                onChange={() => update({ vadMode: mode })}
              />
              <span className="vs-radio-label">
                {mode === 'vad' ? 'Voice Activity' : mode === 'ptt' ? 'Push to Talk' : 'Always On'}
              </span>
              <span className="vs-radio-desc">
                {mode === 'vad' ? 'Auto-detects speech' : mode === 'ptt' ? 'Hold key to transmit' : 'Microphone always open'}
              </span>
            </label>
          ))}
        </div>

        {settings.vadMode === 'ptt' && (
          <div className="vs-ptt-bind">
            <span className="vs-hint">Push to Talk key:</span>
            <button
              className={`vs-keybind-btn ${bindingPtt ? 'listening' : ''}`}
              onClick={startPttBind}
            >
              {bindingPtt ? 'Press any key…' : <kbd>{pttLabel}</kbd>}
            </button>
          </div>
        )}
      </div>
    </div>
  );
};
```

**Important:** Before this file compiles, export `persistVoiceSettings` from `useVoiceConnection.ts`. Open that file and change the function declaration from:

```typescript
function persistVoiceSettings(patch: Partial<VoiceSettings>) {
```

to:

```typescript
export function persistVoiceSettings(patch: Partial<VoiceSettings>) {
```

- [ ] **Create `frontend/src/components/settings/VoiceSettings.css`:**

```css
.vs-container {
  padding: 0 4px;
}

.vs-heading {
  font-size: 18px;
  font-weight: 700;
  color: #32CD32;
  margin-bottom: 24px;
  padding-bottom: 12px;
  border-bottom: 1px solid #1e2228;
}

.vs-section {
  margin-bottom: 24px;
}

.vs-section--row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.vs-section-info {
  flex: 1;
}

.vs-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: #555;
  margin-bottom: 8px;
}

.vs-hint {
  font-size: 12px;
  color: #444;
  display: block;
  margin-top: 4px;
}

.vs-select {
  width: 100%;
  background: #0a0c0f;
  border: 1px solid #2a2d32;
  border-radius: 4px;
  color: #ccc;
  padding: 8px 12px;
  font-size: 13px;
  font-family: inherit;
  outline: none;
  cursor: pointer;
  margin-bottom: 8px;
}

.vs-select:focus { border-color: #32CD32; }

.vs-test-btn {
  background: #16191d;
  border: 1px solid #2a2d32;
  color: #888;
  border-radius: 4px;
  padding: 6px 14px;
  font-size: 12px;
  cursor: pointer;
  font-family: inherit;
  transition: background 0.1s;
}

.vs-test-btn:hover:not(:disabled) { background: #1e2228; color: #ccc; }
.vs-test-btn:disabled { opacity: 0.5; cursor: default; }

.vs-toggle {
  background: #2a2020;
  border: 1px solid #3a1a1a;
  color: #888;
  border-radius: 4px;
  padding: 6px 16px;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
  font-family: inherit;
  transition: all 0.1s;
  flex-shrink: 0;
}

.vs-toggle--on {
  background: rgba(50, 205, 50, 0.15);
  border-color: rgba(50, 205, 50, 0.4);
  color: #32CD32;
}

.vs-radio-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.vs-radio-option {
  display: flex;
  flex-direction: column;
  padding: 10px 12px;
  background: #0a0c0f;
  border: 1px solid #1e2228;
  border-radius: 6px;
  cursor: pointer;
  transition: border-color 0.1s;
}

.vs-radio-option input { display: none; }

.vs-radio-option.active {
  border-color: #32CD32;
  background: rgba(50, 205, 50, 0.05);
}

.vs-radio-label {
  font-size: 13px;
  font-weight: 600;
  color: #ccc;
}

.vs-radio-option.active .vs-radio-label { color: #32CD32; }

.vs-radio-desc {
  font-size: 11px;
  color: #555;
  margin-top: 2px;
}

.vs-ptt-bind {
  margin-top: 12px;
  display: flex;
  align-items: center;
  gap: 10px;
}

.vs-keybind-btn {
  background: #16191d;
  border: 1px solid #2a2d32;
  border-radius: 4px;
  color: #ccc;
  padding: 6px 16px;
  cursor: pointer;
  font-family: inherit;
  font-size: 13px;
  min-width: 100px;
  transition: border-color 0.1s;
}

.vs-keybind-btn.listening {
  border-color: #32CD32;
  color: #32CD32;
  animation: pulse-border 1s infinite;
}

.vs-keybind-btn kbd {
  font-family: inherit;
  font-weight: 700;
}

@keyframes pulse-border {
  0%, 100% { border-color: #32CD32; }
  50% { border-color: rgba(50, 205, 50, 0.4); }
}
```

- [ ] **Add `'voice'` tab to `UserSettings.tsx`.** Open `frontend/src/components/settings/UserSettings.tsx` and:

1. Change `type Tab = 'account' | 'profile' | 'developer';` to:

```typescript
type Tab = 'account' | 'profile' | 'developer' | 'voice';
```

2. Import `VoiceSettingsTab`:

```typescript
import { VoiceSettingsTab } from './VoiceSettings';
```

3. In the tab button list (wherever `account`, `profile`, `developer` tabs are rendered), add:

```tsx
<button
  className={`settings-tab ${activeTab === 'voice' ? 'active' : ''}`}
  onClick={() => setActiveTab('voice')}
>
  Voice & Video
</button>
```

4. In the tab content render (the conditional that shows different content per tab), add:

```tsx
{activeTab === 'voice' && <VoiceSettingsTab />}
```

- [ ] **Verify TypeScript compiles and dev server starts:**

```bash
cd /home/dylan/Developer/parley/frontend && npx tsc --noEmit 2>&1 | head -20 && npm run dev
```

Open User Settings, verify the Voice & Video tab appears and renders device selectors.

- [ ] **Commit:**

```bash
git add frontend/src/components/settings/VoiceSettings.tsx frontend/src/components/settings/VoiceSettings.css frontend/src/components/settings/UserSettings.tsx frontend/src/hooks/useVoiceConnection.ts
git commit -m "feat: add Voice & Video settings tab with device pickers, VAD mode, PTT bind"
```

---

### Task 9: Update env vars on API servers

- [ ] **SSH to each API server and find where LiveKit env vars are set:**

```bash
ssh root@174.138.51.177 "grep -r LIVEKIT /etc/parley.env /etc/environment /etc/systemd/system/ 2>/dev/null | head -5"
```

Note: if the env file uses `export LIVEKIT_URL=...` prefix, adjust the `sed` patterns below to match.

- [ ] **Update vars on all 3 servers** (credentials are in `memory/project_infra.md` and `terraform/terraform.tfvars`):

```bash
# Run for each server: 174.138.51.177, 159.203.111.52, 167.71.186.109
# Replace <KEY> and <SECRET> with values from project_infra.md
ssh root@174.138.51.177 "sed -i 's|LIVEKIT_URL=.*|LIVEKIT_URL=wss://parley-6jjbl5wy.livekit.cloud|' /etc/parley.env && sed -i 's|LIVEKIT_API_KEY=.*|LIVEKIT_API_KEY=<KEY>|' /etc/parley.env && sed -i 's|LIVEKIT_API_SECRET=.*|LIVEKIT_API_SECRET=<SECRET>|' /etc/parley.env && systemctl restart parley-api"
```

(Repeat for `159.203.111.52` and `167.71.186.109`)

**Note:** The exact env file path may differ — check the systemd service file at `/etc/systemd/system/parley-api.service` to find where `EnvironmentFile=` points.

- [ ] **Verify each server is running:**

```bash
ssh root@174.138.51.177 "systemctl status parley-api | head -5"
```

Expected: `active (running)`

---

## Chunk 5: Global Icon Pass

### Task 10: Replace all emoji/unicode icons with Lucide React

**Files:** All frontend components listed below. Do this as one commit.

This is a find-and-replace audit. For each file, search for emoji characters and unicode icon strings, replace with the appropriate Lucide component.

**Import to add to every file that uses icons:**

```typescript
import { /* relevant icons */ } from 'lucide-react';
```

**Standard icon usage pattern:**

```tsx
<Hash size={14} color="#32CD32" />          // inline in text
<Settings size={16} color="currentColor" /> // button icon (inherits CSS color)
<Trash2 size={16} color="#cc4444" />        // destructive action
```

- [ ] **Audit and update `frontend/src/components/layout/ChannelList.tsx`**

Look for: `#`, `🔊`, `</>` or similar for channel types. Replace with:
- Text channel `#` → `<Hash size={13} />`
- Voice channel → `<Volume2 size={13} />`
- Bin channel → `<Code2 size={13} />`
- Category expand/collapse → `<ChevronRight size={12} />` / `<ChevronDown size={12} />`
- Add channel `+` → `<Plus size={14} />`
- Settings gear → `<Settings size={14} />`

- [ ] **Audit and update `frontend/src/components/layout/UserSidebar.tsx`**

Look for: settings cog emoji, mic/deafen icons, headphones. Replace with `Settings`, `Mic`/`MicOff`, `Headphones`/`HeadphonesOff`.

- [ ] **Audit and update `frontend/src/components/chat/MessageInput.tsx`**

Look for: emoji picker trigger, file attach, GIF, send arrow. Replace with `Smile`, `Paperclip`, `Film`, `Send`.

- [ ] **Audit and update `frontend/src/components/chat/Message.tsx`**

Look for: edit pencil, delete trash, pin, reaction add, reply. Replace with `Pencil`, `Trash2`, `Pin`, `SmilePlus`, `Reply`.

- [ ] **Audit and update `frontend/src/components/modals/CreateChannelModal.tsx`**

Look for: channel type icons (hash, volume, code). Replace with `Hash`, `Volume2`, `Code2`.

- [ ] **Audit and update any remaining components** in `components/modals/`, `components/settings/`, `components/bin/` that contain emoji or unicode icon characters. Quick search:

```bash
grep -rn '🔊\|🎙\|🔇\|🔕\|🔔\|📌\|📎\|🗑\|✏\|🔍\|⚙\|👤\|😀\|✕\|→\|←\|⬡' frontend/src/components/ --include="*.tsx"
```

Replace each hit with the appropriate Lucide icon from the mapping in the spec (`docs/superpowers/specs/2026-03-15-voice-video-redesign.md`).

- [ ] **Verify the app builds without errors:**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | tail -20
```

Expected: build completes, no TypeScript errors

- [ ] **Visual check in dev server:** Run `npm run dev`, click through the app. Verify no missing icons (broken components), no emoji remaining in buttons/labels.

- [ ] **Commit:**

```bash
git add frontend/src/components/
git commit -m "feat: replace all emoji/unicode icons with Lucide React across app"
```

---

## Final: Build + Deploy

- [ ] **Build frontend:**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build
```

- [ ] **Push all commits:**

```bash
git push origin main
```

- [ ] **Deploy backend (Go binary) to all 3 servers:**

```bash
ssh root@174.138.51.177 "cd /opt/parley && git pull && /usr/bin/go build -o parley-api ./cmd/api/ && systemctl restart parley-api"
ssh root@159.203.111.52 "cd /opt/parley && git pull && /usr/bin/go build -o parley-api ./cmd/api/ && systemctl restart parley-api"
ssh root@167.71.186.109 "cd /opt/parley && git pull && /usr/bin/go build -o parley-api ./cmd/api/ && systemctl restart parley-api"
```

- [ ] **Deploy frontend using deploy script:**

```bash
cd /home/dylan/Developer/parley && bash deploy-frontend.sh
```

- [ ] **Smoke test on production:**
  - Join a voice channel — verify LiveKit Cloud connection (no self-hosted VC server)
  - Toggle camera on — verify video appears in grid
  - Toggle screen share — verify screen appears as extra tile
  - Navigate to a text channel while connected — VoiceControls widget stays visible
  - Test VAD mode (speak — mic should auto-open)
  - Test PTT mode (hold Space — mic opens; release — mic closes)
  - Open User Settings → Voice & Video tab — verify device dropdowns populate
