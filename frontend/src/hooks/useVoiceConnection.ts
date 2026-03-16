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
        el.muted = deafenedRef.current;
        const savedSpeaker = settingsRef.current.speakerDeviceId;
        if (savedSpeaker && 'setSinkId' in el) (el as any).setSinkId(savedSpeaker).catch(() => {});
      }
    });
  }, []);

  const detachAudio = useCallback((identity: string) => {
    const el = audioRefsMap.current.get(identity);
    if (el) { el.remove(); audioRefsMap.current.delete(identity); }
  }, []);

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
    onDisconnectedRef.current();
  }, [cleanupAudio]);

  const doDisconnect = useCallback(() => {
    const cid = channelIdRef.current;
    channelIdRef.current = null; // null first so Disconnected handler is a no-op
    if (cid) leaveVoiceChannel(cid).catch(() => {});
    const room = roomRef.current;
    doCleanup();
    room?.disconnect(); // call disconnect AFTER cleanup (room ref already nulled in doCleanup)
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

      const room = new Room({ adaptiveStream: true, dynacast: true });
      roomRef.current = room;

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
        if (!channelIdRef.current) return; // user-initiated, already handled
        const serverCid = channelIdRef.current;
        leaveVoiceChannel(serverCid).catch(() => {});
        doCleanup();
      });

      await room.connect(url, token);

      const micTrack = new LocalAudioTrack(stream.getAudioTracks()[0]);
      audioTrackRef.current = micTrack;
      await room.localParticipant.publishTrack(micTrack);

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

      room.remoteParticipants.forEach(p => attachAudio(p));
      if (s.speakerDeviceId) applyOutputDevice(s.speakerDeviceId);

      await joinVoiceChannel(cid);
      updateParticipantList();
      setConnected(true);
    } catch (err) {
      stream?.getTracks().forEach(t => t.stop());
      setError(err instanceof Error ? err.message : 'Failed to connect');
      roomRef.current?.disconnect();
      roomRef.current = null;
      bcRef.current?.close();
      bcRef.current = null;
    } finally {
      setConnecting(false);
    }
  }, [attachAudio, detachAudio, doCleanup, doDisconnect, updateParticipantList, applyOutputDevice]);

  useEffect(() => {
    if (!channelId) return;
    connect(channelId);
    return () => { doDisconnect(); };
  }, [channelId]); // eslint-disable-line react-hooks/exhaustive-deps

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
    if (next) audioTrackRef.current.mute(); else audioTrackRef.current.unmute();
    setMuted(next);
    // DO NOT set speaking here — speaking is managed by VAD/PTT
  }, [muted]);

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
    if (channelId) connect(channelId);
  }, [channelId, connect]);

  return {
    connected, connecting, error,
    muted, deafened, videoEnabled, screenSharing, speaking,
    activeSpeakers, participants, localParticipant,
    settings,
    toggleMute, toggleDeafen, toggleVideo, toggleScreenShare,
    disconnect: doDisconnect,
    retry,
    updateSettings,
  };
}
