import { useCallback, useRef, useState, useEffect } from 'react';
import {
  Room,
  RoomEvent,
  LocalParticipant,
  LocalVideoTrack,
  RemoteParticipant,
  RemoteAudioTrack,
  Track,
  LocalAudioTrack,
  createLocalVideoTrack,
  VideoPresets,
  Participant,
} from 'livekit-client';
import { MicVAD } from '@ricky0123/vad-web';
import { getVoiceToken, joinVoiceChannel, leaveVoiceChannel, refreshVoiceHeartbeat } from '../api/voice';
import { getActivity, type ActivityRecord } from '../api/activities';
import { useLocalVolumes } from './useLocalVolumes';

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
export function useVoiceConnection(
  virtualChannelId: string | null,
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

  const deafenedRef = useRef(false);

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
    participant.getTrackPublications().forEach(pub => {
      if (pub.kind === Track.Kind.Audio && pub.track) {
        attachAudioTrack(pub.trackSid, pub.track);
      }
    });
  }, [attachAudioTrack]);

  const detachAudio = useCallback((participant: RemoteParticipant) => {
    participant.getTrackPublications().forEach(pub => {
      if (pub.kind === Track.Kind.Audio) detachAudioTrack(pub.trackSid);
    });
  }, [detachAudioTrack]);

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
    channelIdRef.current = null;
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
    channelIdRef.current = null; // null first so Disconnected handler is a no-op
    if (cid) leaveVoiceChannel(cid).catch(() => {});
    const r = roomRef.current;
    doCleanup();
    r?.disconnect(); // call disconnect AFTER cleanup (room ref already nulled in doCleanup)
  }, [doCleanup]);

  const connect = useCallback(async (cid: string) => {
    if (roomRef.current) return;
    channelIdRef.current = cid;
    setConnecting(true);
    setError(null);

    let stream: MediaStream | null = null;
    try {
      const { token, url } = await getVoiceToken(cid);

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
          (track as RemoteAudioTrack).setVolume(getVolume(participant.identity) / 100);
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
        if (!channelIdRef.current) return; // user-initiated, already handled
        const vcId = channelIdRef.current;
        leaveVoiceChannel(vcId).catch(() => {});
        doCleanup();
      });

      await r.connect(url, token);

      const micTrack = new LocalAudioTrack(stream.getAudioTracks()[0]);
      audioTrackRef.current = micTrack;
      await r.localParticipant.publishTrack(micTrack, { audioPreset: { maxBitrate: 24_000 } });

      if (s.vadMode === 'vad') {
        try {
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
          micTrack.mute();
          setMuted(true);
        } catch (vadErr) {
          console.warn('VAD initialization failed, falling back to always-on mode:', vadErr);
          // mic stays unmuted — voice still works, just without VAD
        }
      }

      r.remoteParticipants.forEach(p => attachAudio(p));
      if (s.speakerDeviceId) applyOutputDevice(s.speakerDeviceId);

      await joinVoiceChannel(cid);
      setRoom(r);
      updateParticipantList();
      setConnected(true);
    } catch (err) {
      stream?.getTracks().forEach(t => t.stop());
      setError(err instanceof Error ? err.message : 'Failed to connect');
      roomRef.current?.disconnect();
      roomRef.current = null;
      setRoom(null);
      bcRef.current?.close();
      bcRef.current = null;
    } finally {
      setConnecting(false);
    }
  }, [attachAudio, detachAudio, doCleanup, doDisconnect, updateParticipantList, applyOutputDevice, getVolume]);

  useEffect(() => {
    if (!virtualChannelId) return;
    connect(virtualChannelId);
    return () => { doDisconnect(); };
  }, [virtualChannelId]); // eslint-disable-line react-hooks/exhaustive-deps

  // 15s heartbeat to keep the server-side presence alive.
  useEffect(() => {
    if (!connected || !virtualChannelId) return;
    const id = setInterval(() => {
      refreshVoiceHeartbeat(virtualChannelId).catch(() => { /* swallow */ });
    }, 15_000);
    return () => clearInterval(id);
  }, [connected, virtualChannelId]);

  // Hydrate current activity on (re)connect.
  useEffect(() => {
    if (!connected || !virtualChannelId) return;
    getActivity(virtualChannelId).then(setActivity).catch(() => {});
  }, [connected, virtualChannelId]);

  // Re-apply stored per-user volumes whenever the volume map or room changes.
  useEffect(() => {
    if (!room) return;
    room.remoteParticipants.forEach(p => {
      p.audioTrackPublications.forEach(pub => {
        if (pub.track && pub.track.kind === Track.Kind.Audio) {
          (pub.track as RemoteAudioTrack).setVolume(getVolume(p.identity) / 100);
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
    if (!audioTrackRef.current) return;
    const next = !muted;
    if (next) {
      audioTrackRef.current.mute();
    } else {
      // User explicitly unmuting — destroy VAD so it stops auto-muting
      if (vadRef.current) {
        vadRef.current.destroy();
        vadRef.current = null;
      }
      audioTrackRef.current.unmute();
    }
    setMuted(next);
    // DO NOT set speaking here — speaking is managed by VAD/PTT
  }, [muted]);

  const forceMute = useCallback(() => {
    if (!audioTrackRef.current) return;
    // Destroy VAD so it doesn't interfere
    if (vadRef.current) {
      vadRef.current.destroy();
      vadRef.current = null;
    }
    audioTrackRef.current.mute();
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
    if (!roomRef.current) return;
    if (videoEnabled) {
      if (videoTrackRef.current) {
        await roomRef.current.localParticipant.unpublishTrack(videoTrackRef.current);
        videoTrackRef.current.stop();
        videoTrackRef.current = null;
      }
      setVideoEnabled(false);
    } else {
      const s = settingsRef.current;
      const vt = await createLocalVideoTrack({
        resolution: VideoPresets.h720.resolution,
        deviceId: s.cameraDeviceId,
      });
      videoTrackRef.current = vt;
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
