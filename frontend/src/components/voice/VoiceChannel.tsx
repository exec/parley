import React, { useEffect, useRef, useState, useCallback } from 'react';
import {
  Room,
  RoomEvent,
  LocalParticipant,
  RemoteParticipant,
  Track,
  createLocalTracks,
} from 'livekit-client';
import { Channel } from '../../api/types';
import { getVoiceToken, joinVoiceChannel, leaveVoiceChannel, VoiceParticipant } from '../../api/voice';
import './VoiceChannel.css';

interface VoiceChannelProps {
  channel: Channel;
  currentUserId: string;
  currentUsername: string;
  /** Initial presence list from Redux/AppContext (updated via WS) */
  participants: VoiceParticipant[];
  onLeave: () => void;
}

export const VoiceChannel: React.FC<VoiceChannelProps> = ({
  channel,
  currentUserId,
  participants,
  onLeave,
}) => {
  const roomRef = useRef<Room | null>(null);
  const [connected, setConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [muted, setMuted] = useState(false);
  const [deafened, setDeafened] = useState(false);
  const audioRefs = useRef<Map<string, HTMLAudioElement>>(new Map());

  // Attach a remote participant's audio track to an <audio> element
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

  const detachAudio = useCallback((participantIdentity: string) => {
    const el = audioRefs.current.get(participantIdentity);
    if (el) {
      el.remove();
      audioRefs.current.delete(participantIdentity);
    }
  }, []);

  const connect = useCallback(async () => {
    setConnecting(true);
    setError(null);
    try {
      const { token, url } = await getVoiceToken(channel.id);

      const room = new Room({ adaptiveStream: true, dynacast: true });
      roomRef.current = room;

      room.on(RoomEvent.TrackSubscribed, (_track, _pub, participant) => {
        attachAudio(participant as RemoteParticipant);
      });
      room.on(RoomEvent.TrackUnsubscribed, (_track, _pub, participant) => {
        detachAudio(participant.identity);
      });
      room.on(RoomEvent.ParticipantDisconnected, participant => {
        detachAudio(participant.identity);
      });
      room.on(RoomEvent.Disconnected, () => {
        setConnected(false);
        leaveVoiceChannel(channel.id).catch(() => {});
        onLeave();
      });

      await room.connect(url, token);

      // Publish microphone
      const tracks = await createLocalTracks({ audio: true, video: false });
      for (const track of tracks) {
        await room.localParticipant.publishTrack(track);
      }

      // Attach any already-present remote participants
      room.remoteParticipants.forEach(p => attachAudio(p));

      await joinVoiceChannel(channel.id);
      setConnected(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to connect');
      roomRef.current = null;
    } finally {
      setConnecting(false);
    }
  }, [channel.id, attachAudio, detachAudio, onLeave]);

  const disconnect = useCallback(async () => {
    await leaveVoiceChannel(channel.id).catch(() => {});
    roomRef.current?.disconnect();
    roomRef.current = null;
    audioRefs.current.forEach(el => el.remove());
    audioRefs.current.clear();
    setConnected(false);
    onLeave();
  }, [channel.id, onLeave]);

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

  // Connect on mount, disconnect on unmount
  useEffect(() => {
    connect();
    return () => {
      leaveVoiceChannel(channel.id).catch(() => {});
      roomRef.current?.disconnect();
      audioRefs.current.forEach(el => el.remove());
      audioRefs.current.clear();
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="voice-channel">
      <div className="voice-channel-header">
        <span className="voice-channel-icon">🔊</span>
        <span className="voice-channel-name">{channel.name}</span>
        <span className={`voice-status-badge ${connected ? 'connected' : connecting ? 'connecting' : 'error'}`}>
          {connected ? 'connected' : connecting ? 'connecting…' : 'disconnected'}
        </span>
      </div>

      {error && <div className="voice-error">{error} — <button onClick={connect}>retry</button></div>}

      <div className="voice-participants">
        {participants.length === 0 ? (
          <div className="voice-empty">No one else is here</div>
        ) : (
          participants.map(p => (
            <div key={p.user_id} className={`voice-participant ${p.user_id === currentUserId ? 'self' : ''}`}>
              <div className="voice-participant-avatar">{p.username.charAt(0).toUpperCase()}</div>
              <span className="voice-participant-name">{p.username}{p.user_id === currentUserId ? ' (you)' : ''}</span>
            </div>
          ))
        )}
      </div>

      <div className="voice-controls">
        <button
          className={`voice-ctrl-btn ${muted ? 'active' : ''}`}
          onClick={toggleMute}
          title={muted ? 'Unmute' : 'Mute'}
        >
          {muted ? '🔇' : '🎙'}
        </button>
        <button
          className={`voice-ctrl-btn ${deafened ? 'active' : ''}`}
          onClick={toggleDeafen}
          title={deafened ? 'Undeafen' : 'Deafen'}
        >
          {deafened ? '🔕' : '🔔'}
        </button>
        <button className="voice-ctrl-btn disconnect" onClick={disconnect} title="Disconnect">
          ✕ Leave
        </button>
      </div>
    </div>
  );
};
