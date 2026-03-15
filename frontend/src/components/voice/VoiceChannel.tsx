import React, { useMemo } from 'react';
import { Channel } from '../../api/types';
import { VoiceParticipant } from '../../api/voice';
import './VoiceChannel.css';

interface VoiceChannelProps {
  channel: Channel;
  currentUserId: string;
  currentUsername: string;
  currentAvatarUrl?: string;
  participants: VoiceParticipant[];
  connected: boolean;
  connecting: boolean;
  error: string | null;
  muted: boolean;
  deafened: boolean;
  onToggleMute: () => void;
  onToggleDeafen: () => void;
  onLeave: () => void;
  onRetry: () => void;
}

export const VoiceChannel: React.FC<VoiceChannelProps> = ({
  channel,
  currentUserId,
  currentUsername,
  currentAvatarUrl,
  participants,
  connected,
  connecting,
  error,
  muted,
  deafened,
  onToggleMute,
  onToggleDeafen,
  onLeave,
  onRetry,
}) => {
  const allParticipants = useMemo(() => {
    const self = { user_id: currentUserId, username: currentUsername, avatar_url: currentAvatarUrl };
    const others = participants.filter(p => p.user_id !== currentUserId);
    return [self, ...others];
  }, [participants, currentUserId, currentUsername, currentAvatarUrl]);

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
          <button className={`voice-ctrl-btn ${muted ? 'active' : ''}`} onClick={onToggleMute} title={muted ? 'Unmute' : 'Mute'}>
            {muted ? '🔇' : '🎙'}
          </button>
          <button className={`voice-ctrl-btn ${deafened ? 'active' : ''}`} onClick={onToggleDeafen} title={deafened ? 'Undeafen' : 'Deafen'}>
            {deafened ? '🔕' : '🔔'}
          </button>
          <button className="voice-ctrl-btn disconnect" onClick={onLeave} title="Disconnect">
            ✕ Leave
          </button>
        </div>
      </div>

      {error && <div className="voice-error">{error} — <button onClick={onRetry}>retry</button></div>}

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
                {p.user_id === currentUserId && <span className="voice-participant-you">you</span>}
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
};
