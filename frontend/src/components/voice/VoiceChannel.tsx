import React, { useEffect, useRef, useState, useCallback, useMemo } from 'react';
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
  currentAvatarUrl?: string;
  /** Participant list from Redux/AppContext (updated via WS) */
  participants: VoiceParticipant[];
  onLeave: () => void;
}

export const VoiceChannel: React.FC<VoiceChannelProps> = ({
  channel,
  currentUserId,
  currentUsername,
  currentAvatarUrl,
  participants,
  onLeave,
}) => {
  const roomRef = useRef<Room | null>(null);
  const bcRef = useRef<BroadcastChannel | null>(null);
  const [connected, setConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [muted, setMuted] = useState(false);
  const [deafened, setDeafened] = useState(false);
  const audioRefs = useRef<Map<string, HTMLAudioElement>>(new Map());

  // Always show self in the participant list
  const allParticipants = useMemo(() => {
    // Always show self; if the server list includes us, patch in local avatar/name in case it's missing
    const self = { user_id: currentUserId, username: currentUsername, avatar_url: currentAvatarUrl };
    const others = participants.filter(p => p.user_id !== currentUserId);
    return [self, ...others];
  }, [participants, currentUserId, currentUsername, currentAvatarUrl]);

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

  const disconnect = useCallback(async () => {
    bcRef.current?.close();
    bcRef.current = null;
    await leaveVoiceChannel(channel.id).catch(() => {});
    roomRef.current?.disconnect();
    roomRef.current = null;
    audioRefs.current.forEach(el => el.remove());
    audioRefs.current.clear();
    setConnected(false);
    onLeave();
  }, [channel.id, onLeave]);

  const connect = useCallback(async () => {
    setConnecting(true);
    setError(null);
    try {
      const { token, url } = await getVoiceToken(channel.id);

      // Cross-tab takeover: claim this voice channel, kick any other tab already connected
      const bc = new BroadcastChannel('parley_voice');
      bcRef.current = bc;
      bc.onmessage = (e) => {
        if (e.data?.action === 'claim') {
          // Another tab is taking over voice — disconnect this one
          disconnect();
        }
      };
      bc.postMessage({ action: 'claim', channelId: channel.id });

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

      const tracks = await createLocalTracks({ audio: true, video: false });
      for (const track of tracks) {
        await room.localParticipant.publishTrack(track);
      }

      room.remoteParticipants.forEach(p => attachAudio(p));

      await joinVoiceChannel(channel.id);
      setConnected(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to connect');
      roomRef.current = null;
      bcRef.current?.close();
      bcRef.current = null;
    } finally {
      setConnecting(false);
    }
  }, [channel.id, attachAudio, detachAudio, onLeave, disconnect]);

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

  useEffect(() => {
    connect();
    return () => {
      bcRef.current?.close();
      bcRef.current = null;
      leaveVoiceChannel(channel.id).catch(() => {});
      roomRef.current?.disconnect();
      audioRefs.current.forEach(el => el.remove());
      audioRefs.current.clear();
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const statusLabel = connected ? 'connected' : connecting ? 'connecting…' : 'disconnected';
  const statusClass = connected ? 'connected' : connecting ? 'connecting' : 'error';

  return (
    <div className="voice-channel">
      <div className="voice-channel-header">
        <span className="voice-channel-icon">🔊</span>
        <div className="voice-channel-header-info">
          <span className="voice-channel-name">{channel.name}</span>
          <span className={`voice-status-badge ${statusClass}`}>{statusLabel}</span>
        </div>
        <div className="voice-header-controls">
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

      {error && <div className="voice-error">{error} — <button onClick={connect}>retry</button></div>}

      <div className="voice-participants">
        {allParticipants.length === 0 ? (
          <div className="voice-empty">No one here yet…</div>
        ) : (
          allParticipants.map(p => (
            <div key={p.user_id} className={`voice-participant ${p.user_id === currentUserId ? 'self' : ''}`}>
              <div className="voice-participant-avatar">
                {p.avatar_url
                  ? <img src={p.avatar_url} alt={p.username} style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: '50%' }} />
                  : p.username.charAt(0).toUpperCase()
                }
              </div>
              <div className="voice-participant-info">
                <span className="voice-participant-name">{p.username}</span>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
};
