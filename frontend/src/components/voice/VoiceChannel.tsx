import React, { useState, useMemo, useEffect } from 'react';
import { RemoteParticipant, LocalParticipant, Track, TrackPublication } from 'livekit-client';
import { LayoutGrid, Maximize2, MessageSquare, Mic, MicOff, Headphones, HeadphoneOff, Video, VideoOff, Monitor, MonitorOff, PhoneOff, Volume2 } from 'lucide-react';
import { Channel } from '../../api/types';
import { VoiceParticipant, kickVoiceParticipant } from '../../api/voice';
import { ParticipantTile } from './ParticipantTile';
import { VoiceContextMenu } from './VoiceContextMenu';
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
  canMuteMembers?: boolean;
  onMuteParticipant?: (userId: string) => void;
  canKickFromVoice?: boolean;
  vcChatOpen?: boolean;
  onToggleVcChat?: () => void;
}

type ViewMode = 'grid' | 'speaker';

export const VoiceChannel: React.FC<VoiceChannelProps> = ({
  channel,
  currentUser,
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
  canMuteMembers,
  onMuteParticipant,
  canKickFromVoice,
  vcChatOpen,
  onToggleVcChat,
}) => {
  const [viewMode, setViewMode] = useState<ViewMode>('grid');
  const [pinnedIdentity, setPinnedIdentity] = useState<string | null>(null);
  const [contextMenu, setContextMenu] = useState<{ participantId: string; x: number; y: number } | null>(null);

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
      return Array.from(participant.trackPublications.values() as Iterable<TrackPublication>).some(
        pub => pub.source === Track.Source.ScreenShare && pub.track && !pub.isMuted
      );
    });
  }, [allParticipants]);

  useEffect(() => {
    if (pinnedIdentity && !allParticipants.some(({ participant }) => participant.identity === pinnedIdentity)) {
      setPinnedIdentity(null);
    }
  }, [allParticipants, pinnedIdentity]);

  useEffect(() => {
    if (!contextMenu) return;
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setContextMenu(null);
    };
    window.addEventListener('keydown', handleKey);
    return () => window.removeEventListener('keydown', handleKey);
  }, [contextMenu]);

  // In speaker view: find spotlight participant
  const spotlightIdentity = pinnedIdentity ?? (activeSpeakers.size > 0 ? Array.from(activeSpeakers)[0] : null);
  const spotlightParticipant = allParticipants.find(({ participant }) => participant.identity === spotlightIdentity) ?? allParticipants[0];
  const filmstripParticipants = allParticipants.filter(({ participant }) => participant !== spotlightParticipant?.participant);

  const getMeta = (identity: string, participant?: RemoteParticipant | LocalParticipant) => {
    const meta = voiceParticipants[identity];
    const displayName = meta?.username || participant?.name || undefined;
    return { displayName, avatarUrl: meta?.avatar_url };
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
          {onToggleVcChat && (
            <button
              className={`vc-hdr-btn ${vcChatOpen ? 'active' : ''}`}
              onClick={onToggleVcChat}
              title={vcChatOpen ? 'Hide chat' : 'Show chat'}
            >
              <MessageSquare size={16} />
            </button>
          )}
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
            const meta = isLocal ? localMeta : getMeta(participant.identity, participant as RemoteParticipant);
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
            const meta = isLocal ? localMeta : getMeta(participant.identity, participant as RemoteParticipant);
            return (
              <div key={participant.identity} style={{ position: 'relative' }}>
                <ParticipantTile
                  participant={participant}
                  isLocal={isLocal}
                  isSpeaking={isLocal ? false : activeSpeakers.has(participant.identity)}
                  displayName={meta.displayName}
                  avatarUrl={meta.avatarUrl}
                  onContextMenu={!isLocal && (canMuteMembers || canKickFromVoice) ? (e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    setContextMenu({ participantId: participant.identity, x: e.clientX, y: e.clientY });
                  } : undefined}
                />
                {canMuteMembers && !isLocal && onMuteParticipant && (
                  <button
                    style={{ position: 'absolute', top: 4, right: 4, background: 'rgba(0,0,0,0.6)', border: 'none', borderRadius: 4, padding: '2px 4px', cursor: 'pointer' }}
                    title="Force mute"
                    onClick={() => onMuteParticipant(participant.identity)}
                  >
                    <MicOff size={14} color="#cc4444" />
                  </button>
                )}
              </div>
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
                displayName={spotlightParticipant.isLocal ? localMeta.displayName : getMeta(spotlightParticipant.participant.identity, spotlightParticipant.participant as RemoteParticipant).displayName}
                avatarUrl={spotlightParticipant.isLocal ? localMeta.avatarUrl : getMeta(spotlightParticipant.participant.identity, spotlightParticipant.participant as RemoteParticipant).avatarUrl}
              />
            </div>
          ) : (
            <div className="vc-empty">No one here yet…</div>
          )}
          {filmstripParticipants.length > 0 && (
            <div className="vc-filmstrip">
              {filmstripParticipants.map(({ participant, isLocal }) => {
                const meta = isLocal ? localMeta : getMeta(participant.identity, participant as RemoteParticipant);
                return (
                  <div
                    key={participant.identity}
                    className="vc-filmstrip-tile"
                    style={{ position: 'relative' }}
                    onClick={() => setPinnedIdentity(participant.identity)}
                  >
                    <ParticipantTile
                      participant={participant}
                      isLocal={isLocal}
                      isSpeaking={activeSpeakers.has(participant.identity)}
                      displayName={meta.displayName}
                      avatarUrl={meta.avatarUrl}
                      onContextMenu={!isLocal && (canMuteMembers || canKickFromVoice) ? (e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        setContextMenu({ participantId: participant.identity, x: e.clientX, y: e.clientY });
                      } : undefined}
                    />
                    {canMuteMembers && !isLocal && onMuteParticipant && (
                      <button
                        style={{ position: 'absolute', top: 4, right: 4, background: 'rgba(0,0,0,0.6)', border: 'none', borderRadius: 4, padding: '2px 4px', cursor: 'pointer' }}
                        title="Force mute"
                        onClick={(e) => { e.stopPropagation(); onMuteParticipant(participant.identity); }}
                      >
                        <MicOff size={14} color="#cc4444" />
                      </button>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}

      {contextMenu && (
        <VoiceContextMenu
          position={{ x: contextMenu.x, y: contextMenu.y }}
          participantId={contextMenu.participantId}
          canMute={!!canMuteMembers}
          canKick={!!canKickFromVoice}
          onMute={() => { onMuteParticipant?.(contextMenu.participantId); setContextMenu(null); }}
          onKick={async () => {
            const id = contextMenu.participantId;
            try { await kickVoiceParticipant(channel.id, id); } catch (e) { console.error(e); }
            setContextMenu(null);
          }}
          onClose={() => setContextMenu(null)}
        />
      )}

      {/* In-channel controls (secondary; main controls are in VoiceControls widget) */}
      <div className="vc-controls">
        <button className={`vc-ctrl ${muted ? 'vc-ctrl--off' : ''}`} onClick={onToggleMute} title={muted ? 'Unmute' : 'Mute'}>
          {muted ? <MicOff size={18} color="#cc4444" /> : <Mic size={18} color="#32CD32" />}
        </button>
        <button className={`vc-ctrl ${deafened ? 'vc-ctrl--off' : ''}`} onClick={onToggleDeafen} title={deafened ? 'Undeafen' : 'Deafen'}>
          {deafened ? <HeadphoneOff size={18} color="#cc4444" /> : <Headphones size={18} color="#32CD32" />}
        </button>
        <button className={`vc-ctrl ${videoEnabled ? '' : 'vc-ctrl--off'}`} onClick={() => onToggleVideo().catch(console.error)} title={videoEnabled ? 'Turn off camera' : 'Turn on camera'}>
          {videoEnabled ? <Video size={18} color="#32CD32" /> : <VideoOff size={18} color="#555" />}
        </button>
        <button className={`vc-ctrl ${screenSharing ? '' : 'vc-ctrl--off'}`} onClick={() => onToggleScreenShare().catch(console.error)} title={screenSharing ? 'Stop sharing' : 'Share screen'}>
          {screenSharing ? <Monitor size={18} color="#32CD32" /> : <MonitorOff size={18} color="#555" />}
        </button>
        <button className="vc-ctrl vc-ctrl--leave" onClick={onLeave} title="Leave channel">
          <PhoneOff size={18} color="#cc4444" />
        </button>
      </div>
    </div>
  );
};
