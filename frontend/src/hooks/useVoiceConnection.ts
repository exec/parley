import { useCallback, useRef, useState, useEffect } from 'react';
import type {
  Room,
  LocalParticipant,
  LocalVideoTrack,
  RemoteParticipant,
  RemoteAudioTrack,
  LocalAudioTrack,
  Participant,
} from 'livekit-client';
import type { MicVAD } from '@ricky0123/vad-web';
import { getVoiceToken, joinVoiceChannel, leaveVoiceChannel, refreshVoiceHeartbeat } from '../api/voice';
import { getActivity, type ActivityRecord } from '../api/activities';
import { useLocalVolumes } from './useLocalVolumes';
import { isTauri } from '../lib/tauri';

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

export function persistVoiceSettings(patch: Partial<VoiceSettings>) {
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
  activity: ActivityRecord | null;
  toggleMute: () => void;
  forceMute: () => void;
  toggleDeafen: () => void;
  toggleVideo: () => Promise<void>;
  toggleScreenShare: () => Promise<void>;
  disconnect: () => void;
  retry: () => void;
  updateSettings: (patch: Partial<VoiceSettings>) => void;
  receiveActivityStart: (ev: { vc: string; type: string; started_by: string; started_at_ms: number; params?: unknown }) => void;
  receiveActivityEnd: (ev: { vc: string }) => void;
}

// virtualChannelId must be a formatted VC string: "s:{channelId}" for server VCs, "dm:{channelId}" for DM/GC.
//
// remoteParticipantCount is the number of OTHER parley users currently in the
// voice channel (derived from VOICE_STATE_UPDATE events in App.tsx). When it
// reaches >=1 we attach to LiveKit and publish the mic; when it drops to 0 we
// detach so a user sitting alone never uploads audio or holds an SFU slot.
export function useVoiceConnection(
  virtualChannelId: string | null,
  onDisconnected: () => void,
  remoteParticipantCount: number = 0,
): VoiceConnectionReturn {
  const roomRef = useRef<Room | null>(null);
  const bcRef = useRef<BroadcastChannel | null>(null);
  const audioRefsMap = useRef<Map<string, HTMLAudioElement>>(new Map());
  const vadRef = useRef<MicVAD | null>(null);
  const audioTrackRef = useRef<LocalAudioTrack | null>(null);
  const videoTrackRef = useRef<LocalVideoTrack | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const channelIdRef = useRef<string | null>(null);
  // Cached livekit-client module, populated by attachLivekit. Helpers that only
  // run after attach (volume reapply, toggleVideo, attachAudio) read from here
  // so they don't have to await the dynamic import again.
  const livekitModRef = useRef<typeof import('livekit-client') | null>(null);
  const onDisconnectedRef = useRef(onDisconnected);
  onDisconnectedRef.current = onDisconnected;

  // LK lifecycle guards: attach/detach is async + count can flip mid-flight.
  const livekitAttachedRef = useRef(false);
  const attachInFlightRef = useRef(false);
  const remoteCountRef = useRef(0);
  remoteCountRef.current = remoteParticipantCount;

  const deafenedRef = useRef(false);

  const [settings, setSettings] = useState<VoiceSettings>(loadVoiceSettings);
  const settingsRef = useRef(settings);
  settingsRef.current = settings;

  const [connected, setConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [muted, setMuted] = useState(false);
  // Mirror muted in a ref so attachLivekit can apply current intent without
  // listing it as a dep (avoids re-creating the callback on every mute toggle).
  const mutedRef = useRef(false);
  mutedRef.current = muted;
  const [deafened, setDeafened] = useState(false);
  const [videoEnabled, setVideoEnabled] = useState(false);
  const [screenSharing, setScreenSharing] = useState(false);
  const [speaking, setSpeaking] = useState(false);
  const [activeSpeakers, setActiveSpeakers] = useState<Set<string>>(new Set());
  const [participants, setParticipants] = useState<RemoteParticipant[]>([]);
  const [localParticipant, setLocalParticipant] = useState<LocalParticipant | null>(null);
  const [activity, setActivity] = useState<ActivityRecord | null>(null);

  // Keep room in state so the volume-reapply effect can depend on it.
  const [room, setRoom] = useState<Room | null>(null);

  const { getVolume } = useLocalVolumes();

  const updateParticipantList = useCallback(() => {
    if (!roomRef.current) { setParticipants([]); setLocalParticipant(null); return; }
    setParticipants(Array.from(roomRef.current.remoteParticipants.values()));
    setLocalParticipant(roomRef.current.localParticipant);
  }, []);

  const applyOutputDevice = useCallback((deviceId: string) => {
    audioRefsMap.current.forEach(el => {
      if ('setSinkId' in el) (el as any).setSinkId(deviceId).catch(() => {});
    });
  }, []);

  const cleanupAudio = useCallback(() => {
    audioRefsMap.current.forEach(el => el.remove());
    audioRefsMap.current.clear();
  }, []);

  // One <audio> element per track SID so mic and screen-share audio are independent.
  const attachAudioTrack = useCallback((trackSid: string, track: any) => {
    if (audioRefsMap.current.has(trackSid)) return;
    const el = document.createElement('audio');
    el.autoplay = true;
    document.body.appendChild(el);
    audioRefsMap.current.set(trackSid, el);
    track.attach(el);
    el.muted = deafenedRef.current;
    const savedSpeaker = settingsRef.current.speakerDeviceId;
    if (savedSpeaker && 'setSinkId' in el) (el as any).setSinkId(savedSpeaker).catch(() => {});
  }, []);

  const detachAudioTrack = useCallback((trackSid: string) => {
    const el = audioRefsMap.current.get(trackSid);
    if (el) { el.remove(); audioRefsMap.current.delete(trackSid); }
  }, []);

  const attachAudio = useCallback((participant: RemoteParticipant) => {
    const lk = livekitModRef.current;
    if (!lk) return;
    participant.getTrackPublications().forEach(pub => {
      if (pub.kind === lk.Track.Kind.Audio && pub.track) {
        attachAudioTrack(pub.trackSid, pub.track);
      }
    });
  }, [attachAudioTrack]);

  const detachAudio = useCallback((participant: RemoteParticipant) => {
    const lk = livekitModRef.current;
    if (!lk) return;
    participant.getTrackPublications().forEach(pub => {
      if (pub.kind === lk.Track.Kind.Audio) detachAudioTrack(pub.trackSid);
    });
  }, [detachAudioTrack]);

  // detachLivekit tears down the LK room and unpublishes tracks but preserves
  // the user's parley presence (channelIdRef, streamRef, VAD, mute intent).
  // Called when the last remote participant leaves OR on full disconnect.
  const detachLivekit = useCallback(async () => {
    if (!livekitAttachedRef.current && !roomRef.current) return;
    livekitAttachedRef.current = false;

    if (videoTrackRef.current) {
      try { await roomRef.current?.localParticipant.unpublishTrack(videoTrackRef.current); } catch { /* noop */ }
      videoTrackRef.current.stop();
      videoTrackRef.current = null;
      setVideoEnabled(false);
    }
    if (audioTrackRef.current) {
      try { await roomRef.current?.localParticipant.unpublishTrack(audioTrackRef.current); } catch { /* noop */ }
      audioTrackRef.current = null;
    }

    const r = roomRef.current;
    roomRef.current = null;
    setRoom(null);
    cleanupAudio();
    setParticipants([]);
    setLocalParticipant(null);
    setActiveSpeakers(new Set());
    setScreenSharing(false);
    r?.disconnect();
  }, [cleanupAudio]);

  // attachLivekit connects the cached mic stream to a fresh LiveKit room and
  // publishes the audio track. Idempotent — safe to call repeatedly.
  // Pre-conditions: parley presence joined (connected=true), stream cached.
  const attachLivekit = useCallback(async () => {
    if (livekitAttachedRef.current || attachInFlightRef.current) return;
    if (!streamRef.current || !channelIdRef.current) return;
    attachInFlightRef.current = true;

    try {
      const cid = channelIdRef.current;
      const [{ token, url }, lk] = await Promise.all([
        getVoiceToken(cid),
        livekitModRef.current
          ? Promise.resolve(livekitModRef.current)
          : import('livekit-client').then(m => { livekitModRef.current = m; return m; }),
      ]);

      // Bail if the user disconnected or count dropped to 0 mid-fetch.
      if (channelIdRef.current !== cid || remoteCountRef.current < 1) return;

      const { Room, RoomEvent, Track, LocalAudioTrack } = lk;

      const r = new Room({ adaptiveStream: true, dynacast: true });
      roomRef.current = r;

      r.on(RoomEvent.ParticipantConnected, updateParticipantList);
      r.on(RoomEvent.ParticipantDisconnected, (p) => {
        detachAudio(p as RemoteParticipant);
        updateParticipantList();
      });
      r.on(RoomEvent.TrackSubscribed, (track, publication, participant) => {
        if (publication.kind === Track.Kind.Audio && track) {
          attachAudioTrack(publication.trackSid, track);
          const at = track as RemoteAudioTrack;
          if (typeof at.setVolume === 'function') {
            at.setVolume(getVolume(participant.identity) / 100);
          }
        }
        updateParticipantList();
      });
      r.on(RoomEvent.TrackUnsubscribed, (_track, publication) => {
        if (publication.kind === Track.Kind.Audio) {
          detachAudioTrack(publication.trackSid);
        }
        updateParticipantList();
      });
      r.on(RoomEvent.TrackPublished, updateParticipantList);
      r.on(RoomEvent.TrackUnpublished, updateParticipantList);
      r.on(RoomEvent.LocalTrackPublished, (pub) => {
        if (pub.source === Track.Source.ScreenShare) setScreenSharing(true);
        updateParticipantList();
      });
      r.on(RoomEvent.LocalTrackUnpublished, (pub) => {
        if (pub.source === Track.Source.ScreenShare) setScreenSharing(false);
        updateParticipantList();
      });
      r.on(RoomEvent.ActiveSpeakersChanged, (speakers: Participant[]) => {
        const ids = new Set(speakers.map(sp => sp.identity));
        setActiveSpeakers(ids);
      });
      r.on(RoomEvent.Disconnected, () => {
        // Unexpected disconnect (network, server). If the user is still in the
        // parley channel and others are present, the count-driven effect will
        // re-attach. If they're alone, this is the expected detach path.
        if (livekitAttachedRef.current) {
          livekitAttachedRef.current = false;
          roomRef.current = null;
          setRoom(null);
          cleanupAudio();
          setParticipants([]);
          setLocalParticipant(null);
          setActiveSpeakers(new Set());
        }
      });

      await r.connect(url, token);

      // Re-check after the handshake: user may have left or peer may have left.
      if (channelIdRef.current !== cid || remoteCountRef.current < 1) {
        r.disconnect();
        roomRef.current = null;
        return;
      }

      const audioTracks = streamRef.current.getAudioTracks();
      if (audioTracks.length === 0) {
        r.disconnect();
        roomRef.current = null;
        return;
      }
      const micTrack = new LocalAudioTrack(audioTracks[0]);
      audioTrackRef.current = micTrack;
      // Apply current mute intent before the first byte hits the wire.
      if (mutedRef.current) micTrack.mute();
      await r.localParticipant.publishTrack(micTrack, { audioPreset: { maxBitrate: 24_000 } });

      r.remoteParticipants.forEach(p => attachAudio(p));
      const s = settingsRef.current;
      if (s.speakerDeviceId) applyOutputDevice(s.speakerDeviceId);

      setRoom(r);
      updateParticipantList();
      livekitAttachedRef.current = true;

      // Count may have dropped during the handshake; tear down if so.
      if (remoteCountRef.current < 1) detachLivekit();
    } catch (err) {
      console.error('voice: LiveKit attach failed', err);
      try { roomRef.current?.disconnect(); } catch { /* noop */ }
      roomRef.current = null;
      setRoom(null);
    } finally {
      attachInFlightRef.current = false;
    }
  }, [attachAudio, attachAudioTrack, applyOutputDevice, cleanupAudio, detachAudio, detachAudioTrack, detachLivekit, getVolume, updateParticipantList]);

  // doCleanup tears down everything (LK + parley + stream + state). Called on
  // user-initiated disconnect AND on unexpected room disconnects we can't recover.
  const doCleanup = useCallback(() => {
    vadRef.current?.destroy();
    vadRef.current = null;
    bcRef.current?.close();
    bcRef.current = null;
    if (videoTrackRef.current) {
      videoTrackRef.current.stop();
      videoTrackRef.current = null;
    }
    audioTrackRef.current = null;
    streamRef.current?.getTracks().forEach(t => t.stop());
    streamRef.current = null;
    channelIdRef.current = null;
    livekitAttachedRef.current = false;
    roomRef.current = null;
    setRoom(null);
    cleanupAudio();
    setConnected(false);
    setMuted(false);
    setDeafened(false);
    deafenedRef.current = false;
    setVideoEnabled(false);
    setScreenSharing(false);
    setSpeaking(false);
    setActiveSpeakers(new Set());
    setParticipants([]);
    setLocalParticipant(null);
    setActivity(null);
    onDisconnectedRef.current();
  }, [cleanupAudio]);

  const doDisconnect = useCallback(() => {
    const cid = channelIdRef.current;
    channelIdRef.current = null; // null first so async paths bail early
    if (cid) leaveVoiceChannel(cid).catch(() => {});
    const r = roomRef.current;
    livekitAttachedRef.current = false;
    doCleanup();
    r?.disconnect();
  }, [doCleanup]);

  // connect (Phase 1): acquire mic, register parley presence, set up VAD.
  // Does NOT touch LiveKit — that happens later via attachLivekit when at
  // least one other parley user is present in the channel.
  const connect = useCallback(async (cid: string) => {
    if (channelIdRef.current === cid && (connecting || connected)) return;
    if (channelIdRef.current && channelIdRef.current !== cid) return; // already in another channel
    channelIdRef.current = cid;
    setConnecting(true);
    setError(null);

    let stream: MediaStream | null = null;
    try {
      const bc = new BroadcastChannel('parley_voice');
      bcRef.current = bc;
      bc.onmessage = (e) => { if (e.data?.action === 'claim') doDisconnect(); };
      bc.postMessage({ action: 'claim', channelId: cid });

      const s = settingsRef.current;
      stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          deviceId: s.micDeviceId ? { exact: s.micDeviceId } : undefined,
          noiseSuppression: s.noiseSuppression,
          echoCancellation: s.noiseSuppression,
          autoGainControl: true,
        },
      });
      streamRef.current = stream;

      // VAD operates on the mic stream and only flips local mute state +
      // optionally toggles audioTrackRef.mute() if a LiveKit track exists.
      // Safe to start before LiveKit is attached.
      if (s.vadMode === 'vad') {
        try {
          const { MicVAD } = await import('@ricky0123/vad-web');
          const vad = await MicVAD.new({
            baseAssetPath: '/',
            onnxWASMBasePath: '/',
            getStream: () => Promise.resolve(stream!),
            onSpeechStart: () => {
              if (settingsRef.current.vadMode !== 'vad') return;
              audioTrackRef.current?.unmute();
              setMuted(false);
              setSpeaking(true);
            },
            onSpeechEnd: (_audio: Float32Array) => {
              if (settingsRef.current.vadMode !== 'vad') return;
              audioTrackRef.current?.mute();
              setMuted(true);
              setSpeaking(false);
            },
          });
          vadRef.current = vad;
          vad.start();
          setMuted(true);
        } catch (vadErr) {
          console.warn('VAD initialization failed, falling back to always-on mode:', vadErr);
        }
      }

      await joinVoiceChannel(cid);
      setConnected(true);
    } catch (err) {
      stream?.getTracks().forEach(t => t.stop());
      streamRef.current = null;
      setError(err instanceof Error ? err.message : 'Failed to connect');
      bcRef.current?.close();
      bcRef.current = null;
      channelIdRef.current = null;
    } finally {
      setConnecting(false);
    }
  }, [connected, connecting, doDisconnect]);

  useEffect(() => {
    if (!virtualChannelId) return;
    connect(virtualChannelId);
    return () => { doDisconnect(); };
  }, [virtualChannelId]); // eslint-disable-line react-hooks/exhaustive-deps

  // Phase 2/3: react to remote-participant count changes.
  // 0 → 1+: attach LK and publish mic.
  // 1+ → 0: detach LK so we don't burn bandwidth or SFU slots when alone.
  useEffect(() => {
    if (!connected) return;
    if (remoteParticipantCount >= 1) {
      attachLivekit();
    } else {
      detachLivekit();
    }
  }, [connected, remoteParticipantCount, attachLivekit, detachLivekit]);

  // 15s heartbeat to keep the server-side presence alive.
  useEffect(() => {
    if (!connected || !virtualChannelId) return;
    const id = setInterval(() => {
      refreshVoiceHeartbeat(virtualChannelId).catch(() => { /* swallow */ });
    }, 15_000);
    return () => clearInterval(id);
  }, [connected, virtualChannelId]);

  // Fire a leave beacon on any path that's about to tear the renderer down.
  //   - Web: pagehide on tab/window close, browser quit, navigation.
  //   - Tauri: a "parley:quitting" event from the Rust side — fired when the
  //     user actually quits (tray Quit, cmd+Q, OS shutdown). The native
  //     handler delays exit by ~250ms so this beacon has time to flush.
  //     Clicking the X just hides the window, so no event fires there.
  // Non-graceful paths (process kill, power loss) fall back to the server-side
  // sweeper which evicts stale presence within ~30-45s.
  useEffect(() => {
    if (!connected || !virtualChannelId) return;
    const sendLeave = () => {
      try { navigator.sendBeacon(`/api/voice/${virtualChannelId}/leave`); } catch { /* noop */ }
    };
    window.addEventListener('pagehide', sendLeave);

    let unlistenTauri: (() => void) | null = null;
    let cancelled = false;
    if (isTauri()) {
      (async () => {
        const { listen } = await import('@tauri-apps/api/event');
        if (cancelled) return;
        unlistenTauri = await listen('parley:quitting', () => sendLeave());
      })();
    }

    return () => {
      window.removeEventListener('pagehide', sendLeave);
      cancelled = true;
      unlistenTauri?.();
    };
  }, [connected, virtualChannelId]);

  // Hydrate current activity on (re)connect.
  useEffect(() => {
    if (!connected || !virtualChannelId) return;
    let ignored = false;
    getActivity(virtualChannelId).then(a => { if (!ignored) setActivity(a); }).catch(() => {});
    return () => { ignored = true; };
  }, [connected, virtualChannelId]);

  // Re-apply stored per-user volumes whenever the volume map or room changes.
  useEffect(() => {
    if (!room) return;
    const lk = livekitModRef.current;
    if (!lk) return;
    room.remoteParticipants.forEach(p => {
      p.audioTrackPublications.forEach(pub => {
        const t = pub.track as RemoteAudioTrack | undefined;
        if (t && t.kind === lk.Track.Kind.Audio && typeof t.setVolume === 'function') {
          t.setVolume(getVolume(p.identity) / 100);
        }
      });
    });
  }, [getVolume, room]);

  useEffect(() => {
    if (!connected) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (settingsRef.current.vadMode !== 'ptt') return;
      if (e.code !== settingsRef.current.pttKey) return;
      const target = e.target as HTMLElement;
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) return;
      audioTrackRef.current?.unmute();
      setMuted(false);
      setSpeaking(true);
    };

    const handleKeyUp = (e: KeyboardEvent) => {
      if (settingsRef.current.vadMode !== 'ptt') return;
      if (e.code !== settingsRef.current.pttKey) return;
      const target = e.target as HTMLElement;
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) return;
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
    const next = !muted;
    if (next) {
      audioTrackRef.current?.mute();
    } else {
      // User explicitly unmuting — destroy VAD so it stops auto-muting
      if (vadRef.current) {
        vadRef.current.destroy();
        vadRef.current = null;
      }
      audioTrackRef.current?.unmute();
    }
    setMuted(next);
    // DO NOT set speaking here — speaking is managed by VAD/PTT
  }, [muted]);

  const forceMute = useCallback(() => {
    // Destroy VAD so it doesn't interfere
    if (vadRef.current) {
      vadRef.current.destroy();
      vadRef.current = null;
    }
    audioTrackRef.current?.mute();
    setMuted(true);
    setSpeaking(false);
  }, []);

  const toggleDeafen = useCallback(() => {
    const next = !deafened;
    deafenedRef.current = next;
    audioRefsMap.current.forEach(el => { el.muted = next; });
    setDeafened(next);
  }, [deafened]);

  const toggleVideo = useCallback(async () => {
    if (!roomRef.current) return; // requires LK attached (i.e., others present)
    if (videoEnabled) {
      if (videoTrackRef.current) {
        await roomRef.current.localParticipant.unpublishTrack(videoTrackRef.current);
        videoTrackRef.current.stop();
        videoTrackRef.current = null;
      }
      setVideoEnabled(false);
    } else {
      const s = settingsRef.current;
      // toggleVideo only fires while LK is attached, so livekitModRef is set —
      // but fall back to a fresh import for safety.
      const lk = livekitModRef.current ?? await import('livekit-client');
      const vt = await lk.createLocalVideoTrack({
        resolution: lk.VideoPresets.h720.resolution,
        deviceId: s.cameraDeviceId,
      });
      videoTrackRef.current = vt;
      await roomRef.current.localParticipant.publishTrack(vt);
      setVideoEnabled(true);
    }
    updateParticipantList();
  }, [videoEnabled, updateParticipantList]);

  const toggleScreenShare = useCallback(async () => {
    if (!roomRef.current) return; // requires LK attached
    if (screenSharing) {
      await roomRef.current.localParticipant.setScreenShareEnabled(false);
      setScreenSharing(false);
    } else {
      await roomRef.current.localParticipant.setScreenShareEnabled(true, { audio: true });
    }
    updateParticipantList();
  }, [screenSharing, updateParticipantList]);

  const updateSettings = useCallback((patch: Partial<VoiceSettings>) => {
    persistVoiceSettings(patch);
    setSettings(prev => ({ ...prev, ...patch }));
    if (patch.speakerDeviceId) applyOutputDevice(patch.speakerDeviceId);
  }, [applyOutputDevice]);

  const retry = useCallback(() => {
    if (virtualChannelId) connect(virtualChannelId);
  }, [virtualChannelId, connect]);

  // Called by App.tsx when a WS ACTIVITY_START event arrives for this VC.
  const receiveActivityStart = useCallback((ev: { vc: string; type: string; started_by: string; started_at_ms: number; params?: unknown }) => {
    if (ev.vc !== virtualChannelId) return;
    setActivity({ type: ev.type, started_by: ev.started_by, started_at_ms: ev.started_at_ms, params: ev.params });
  }, [virtualChannelId]);

  // Called by App.tsx when a WS ACTIVITY_END event arrives for this VC.
  const receiveActivityEnd = useCallback((ev: { vc: string }) => {
    if (ev.vc !== virtualChannelId) return;
    setActivity(null);
  }, [virtualChannelId]);

  return {
    connected, connecting, error,
    muted, deafened, videoEnabled, screenSharing, speaking,
    activeSpeakers, participants, localParticipant,
    settings,
    activity,
    toggleMute, forceMute, toggleDeafen, toggleVideo, toggleScreenShare,
    disconnect: doDisconnect,
    retry,
    updateSettings,
    receiveActivityStart,
    receiveActivityEnd,
  };
}
