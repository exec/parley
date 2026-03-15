import { useCallback, useRef, useState, useEffect } from 'react';
import {
  Room,
  RoomEvent,
  LocalParticipant,
  RemoteParticipant,
  Track,
  createLocalTracks,
} from 'livekit-client';
import { getVoiceToken, joinVoiceChannel, leaveVoiceChannel } from '../api/voice';

export interface VoiceConnectionState {
  connected: boolean;
  connecting: boolean;
  error: string | null;
  muted: boolean;
  deafened: boolean;
}

export function useVoiceConnection(
  channelId: string | null,
  onDisconnected: () => void,
): VoiceConnectionState & {
  toggleMute: () => void;
  toggleDeafen: () => void;
  disconnect: () => void;
  retry: () => void;
} {
  const roomRef = useRef<Room | null>(null);
  const bcRef = useRef<BroadcastChannel | null>(null);
  const audioRefs = useRef<Map<string, HTMLAudioElement>>(new Map());
  const channelIdRef = useRef<string | null>(null);
  const onDisconnectedRef = useRef(onDisconnected);
  onDisconnectedRef.current = onDisconnected;

  const [connected, setConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [muted, setMuted] = useState(false);
  const [deafened, setDeafened] = useState(false);

  const cleanupAudio = useCallback(() => {
    audioRefs.current.forEach(el => el.remove());
    audioRefs.current.clear();
  }, []);

  const attachAudio = useCallback((participant: RemoteParticipant) => {
    participant.getTrackPublications().forEach(pub => {
      if (pub.kind === Track.Kind.Audio && pub.track) {
        let el = audioRefs.current.get(participant.identity);
        if (!el) {
          el = document.createElement('audio');
          el.autoplay = true;
          document.body.appendChild(el);
          audioRefs.current.set(participant.identity, el);
        }
        pub.track.attach(el);
      }
    });
  }, []);

  const detachAudio = useCallback((identity: string) => {
    const el = audioRefs.current.get(identity);
    if (el) { el.remove(); audioRefs.current.delete(identity); }
  }, []);

  const doDisconnect = useCallback(() => {
    bcRef.current?.close();
    bcRef.current = null;
    const cid = channelIdRef.current;
    if (cid) leaveVoiceChannel(cid).catch(() => {});
    roomRef.current?.disconnect();
    roomRef.current = null;
    channelIdRef.current = null;
    cleanupAudio();
    setConnected(false);
    setMuted(false);
    setDeafened(false);
    onDisconnectedRef.current();
  }, [cleanupAudio]);

  const connect = useCallback(async (cid: string) => {
    if (roomRef.current) return;
    channelIdRef.current = cid;
    setConnecting(true);
    setError(null);
    try {
      const { token, url } = await getVoiceToken(cid);

      const bc = new BroadcastChannel('parley_voice');
      bcRef.current = bc;
      bc.onmessage = (e) => { if (e.data?.action === 'claim') doDisconnect(); };
      bc.postMessage({ action: 'claim', channelId: cid });

      const room = new Room({ adaptiveStream: true, dynacast: true });
      roomRef.current = room;

      room.on(RoomEvent.TrackSubscribed, (_t, _p, participant) => attachAudio(participant as RemoteParticipant));
      room.on(RoomEvent.TrackUnsubscribed, (_t, _p, participant) => detachAudio(participant.identity));
      room.on(RoomEvent.ParticipantDisconnected, p => detachAudio(p.identity));
      room.on(RoomEvent.Disconnected, () => {
        setConnected(false);
        leaveVoiceChannel(cid).catch(() => {});
        onDisconnectedRef.current();
      });

      await room.connect(url, token);
      const tracks = await createLocalTracks({ audio: true, video: false });
      for (const track of tracks) await room.localParticipant.publishTrack(track);
      room.remoteParticipants.forEach(p => attachAudio(p));
      await joinVoiceChannel(cid);
      setConnected(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to connect');
      roomRef.current = null;
      bcRef.current?.close();
      bcRef.current = null;
    } finally {
      setConnecting(false);
    }
  }, [attachAudio, detachAudio, doDisconnect]);

  // Connect when channelId is set, cleanup on unmount
  useEffect(() => {
    if (!channelId) return;
    connect(channelId);
    return () => {
      bcRef.current?.close();
      bcRef.current = null;
      if (channelIdRef.current) leaveVoiceChannel(channelIdRef.current).catch(() => {});
      roomRef.current?.disconnect();
      roomRef.current = null;
      cleanupAudio();
    };
  }, [channelId]); // eslint-disable-line react-hooks/exhaustive-deps

  const toggleMute = useCallback(() => {
    const local: LocalParticipant | undefined = roomRef.current?.localParticipant;
    if (!local) return;
    const next = !muted;
    local.setMicrophoneEnabled(!next);
    setMuted(next);
  }, [muted]);

  const toggleDeafen = useCallback(() => {
    const next = !deafened;
    audioRefs.current.forEach(el => { el.muted = next; });
    setDeafened(next);
  }, [deafened]);

  return {
    connected, connecting, error, muted, deafened,
    toggleMute, toggleDeafen,
    disconnect: doDisconnect,
    retry: () => { if (channelId) connect(channelId); },
  };
}
