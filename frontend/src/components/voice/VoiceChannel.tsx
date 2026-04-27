import React, { useState, useMemo, useEffect, useRef, useCallback } from 'react';
import { Track } from 'livekit-client';
import type { RemoteParticipant, LocalParticipant, TrackPublication } from 'livekit-client';
import { LayoutGrid, Maximize2, MessageSquare, Mic, MicOff, Headphones, HeadphoneOff, Video, VideoOff, Monitor, MonitorOff, PhoneOff, Volume2, X, Expand, Music2, Users } from 'lucide-react';
import { Channel } from '../../api/types';
import { VoiceParticipant, kickVoiceParticipant, serverVc } from '../../api/voice';
import { ParticipantTile } from './ParticipantTile';
import { VoiceContextMenu } from './VoiceContextMenu';
import { SoundboardPanel } from './SoundboardPanel';
import { ActivitySlot } from './ActivitySlot';
import type { ActivityRecord } from '../../api/activities';
import './VoiceChannel.css';

interface VoiceChannelProps {
  channel: Channel;
  vc?: string;
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
  activity?: ActivityRecord | null;
  layout?: 'full' | 'compact';
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
  onParticipantClick?: (userId: string, e: React.MouseEvent) => void;
  activeSoundEmojis?: Map<string, string>;
  /** When wired, renders a members-toggle button in the header (used by DM split view). */
  showMembers?: boolean;
  onToggleMembers?: () => void;
}

type ViewMode = 'grid' | 'speaker';

export const VoiceChannel: React.FC<VoiceChannelProps> = ({
  channel,
  vc,
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
  activity = null,
  layout = 'full',
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
  onParticipantClick,
  activeSoundEmojis,
  showMembers,
  onToggleMembers,
}) => {
  const [viewMode, setViewMode] = useState<ViewMode>('grid');
  const [pinnedIdentity, setPinnedIdentity] = useState<string | null>(null);
  const [soundboardOpen, setSoundboardOpen] = useState(false);
  const [contextMenu, setContextMenu] = useState<{ participantId: string; x: number; y: number } | null>(null);
  const [enlargedShare, setEnlargedShare] = useState<{ participant: LocalParticipant | RemoteParticipant; isLocal: boolean } | null>(null);
  const [shareCtxMenu, setShareCtxMenu] = useState<{ participant: LocalParticipant | RemoteParticipant; isLocal: boolean; x: number; y: number } | null>(null);
  const overlayRef = useRef<HTMLDivElement>(null);

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
    if (!contextMenu && !enlargedShare && !shareCtxMenu) return;
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { setContextMenu(null); setEnlargedShare(null); setShareCtxMenu(null); }
    };
    const handleMouse = () => setShareCtxMenu(null);
    window.addEventListener('keydown', handleKey);
    if (shareCtxMenu) window.addEventListener('mousedown', handleMouse);
    return () => {
      window.removeEventListener('keydown', handleKey);
      window.removeEventListener('mousedown', handleMouse);
    };
  }, [contextMenu, enlargedShare, shareCtxMenu]);

  // In speaker view: screenshares are first-class spotlight candidates.
  // Build a combined list: screen-share entries first, then plain participant entries.
  type SpeakerItem = { participant: LocalParticipant | RemoteParticipant; isLocal: boolean; isScreenShare?: boolean };
  const allSpeakerItems = useMemo<SpeakerItem[]>(() => {
    const shares: SpeakerItem[] = screenShares.map(s => ({ ...s, isScreenShare: true }));
    const plain: SpeakerItem[] = allParticipants.map(p => ({ ...p, isScreenShare: false }));
    return [...shares, ...plain];
  }, [screenShares, allParticipants]);

  // Key identifies a speaker item so we can pin screenshares too.
  const speakerKey = (item: SpeakerItem) => item.isScreenShare ? `screen:${item.participant.identity}` : item.participant.identity;

  const spotlightKey = pinnedIdentity ?? (activeSpeakers.size > 0 ? `${Array.from(activeSpeakers)[0]}` : null);
  const spotlightItem = allSpeakerItems.find(i => speakerKey(i) === spotlightKey) ?? allSpeakerItems[0];
  const filmstripItems = allSpeakerItems.filter(i => i !== spotlightItem);

  // Pre-compute meta per participant identity once per render so the tile
  // map loops don't re-derive displayName/avatarUrl for every iteration on
  // every parent re-render (mute/speaker/etc.).
  const participantMetaById = useMemo(() => {
    const map = new Map<string, { displayName?: string; avatarUrl?: string }>();
    for (const { participant } of allParticipants) {
      const meta = voiceParticipants[participant.identity];
      map.set(participant.identity, {
        displayName: meta?.username || participant.name || undefined,
        avatarUrl: meta?.avatar_url,
      });
    }
    return map;
  }, [allParticipants, voiceParticipants]);

  const localMeta = useMemo(
    () => ({ displayName: currentUser.username, avatarUrl: currentUser.avatar_url }),
    [currentUser.username, currentUser.avatar_url]
  );

  const getMeta = useCallback(
    (identity: string, participant?: RemoteParticipant | LocalParticipant) => {
      const cached = participantMetaById.get(identity);
      if (cached) return cached;
      // Fallback for identities not in allParticipants (e.g. spotlight lookup races)
      const meta = voiceParticipants[identity];
      return { displayName: meta?.username || participant?.name || undefined, avatarUrl: meta?.avatar_url };
    },
    [participantMetaById, voiceParticipants]
  );

  // Per-participant stable callbacks — recomputed only when the participant
  // list or capability flags change. Without this, inline arrows in the map
  // loops would defeat ParticipantTile's React.memo on every parent re-render.
  type TileCallbacks = {
    onContextMenu?: (e: React.MouseEvent) => void;
    onClick?: (e: React.MouseEvent) => void;
    onShareClick: () => void;
    onShareContextMenu: (e: React.MouseEvent) => void;
  };
  const tileCallbacksById = useMemo(() => {
    const map = new Map<string, TileCallbacks>();
    for (const { participant, isLocal } of allParticipants) {
      const id = participant.identity;
      const canShowCtx = !isLocal && (canMuteMembers || canKickFromVoice);
      map.set(id, {
        onContextMenu: canShowCtx
          ? (e) => {
              e.preventDefault();
              e.stopPropagation();
              setContextMenu({ participantId: id, x: e.clientX, y: e.clientY });
            }
          : undefined,
        onClick: !isLocal && onParticipantClick
          ? (e) => onParticipantClick(id, e)
          : undefined,
        onShareClick: () => setEnlargedShare({ participant, isLocal }),
        onShareContextMenu: (e) => {
          e.preventDefault();
          e.stopPropagation();
          setShareCtxMenu({ participant, isLocal, x: e.clientX, y: e.clientY });
        },
      });
    }
    return map;
  }, [allParticipants, canMuteMembers, canKickFromVoice, onParticipantClick]);

  const statusLabel = connected ? 'Connected' : connecting ? 'Connecting…' : 'Disconnected';

  // When alone in the channel, useVoiceConnection skips LK attach, so
  // localParticipant is null. Render a static self-placeholder so the user
  // sees themselves in the grid/spotlight (matches the "you're in here" UX
  // they had before lazy LK). Drops out the moment LK attaches.
  const showSelfPlaceholder = connected && !localParticipant;
  const selfInitial = (currentUser.username.charAt(0) || '?').toUpperCase();
  const selfPlaceholder = showSelfPlaceholder ? (
    <div className="participant-tile">
      <div className="participant-tile-media">
        <div className="participant-tile-avatar">
          {currentUser.avatar_url ? (
            <img src={currentUser.avatar_url} alt={currentUser.username} />
          ) : (
            <span className="participant-tile-initial">{selfInitial}</span>
          )}
        </div>
      </div>
      <div className="participant-tile-footer">
        <span className="participant-tile-name">
          {currentUser.username}
          <span className="participant-tile-you">You</span>
        </span>
        {muted && (
          <span className="participant-tile-muted">
            <MicOff size={12} color="var(--parley-danger)" />
          </span>
        )}
      </div>
    </div>
  ) : null;

  return (
    <div className={`vc-view vc-view--${layout}`}>
      {/* Header */}
      <div className="vc-header">
        <div className="vc-header-left">
          <Volume2 size={16} color="var(--parley-accent)" />
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
          {onToggleMembers && (
            <button
              className={`vc-hdr-btn ${showMembers ? 'active' : ''}`}
              onClick={onToggleMembers}
              title={showMembers ? 'Hide members' : 'Show members'}
              aria-label={showMembers ? 'Hide members' : 'Show members'}
            >
              <Users size={16} />
            </button>
          )}
        </div>
      </div>

      {error && (
        <div className="vc-error">
          {error} — <button onClick={onRetry} className="vc-retry-btn">Retry</button>
        </div>
      )}

      {vc && <ActivitySlot vc={vc} activity={activity} />}

      {/* Main area */}
      {viewMode === 'grid' ? (
        <div className="vc-grid">
          {/* Screen share tiles — clickable to expand, right-clickable for menu */}
          {screenShares.map(({ participant, isLocal }) => {
            const meta = isLocal ? localMeta : getMeta(participant.identity, participant as RemoteParticipant);
            const cbs = tileCallbacksById.get(participant.identity);
            return (
              <div key={`screen-${participant.identity}`} className="vc-grid-share-wrap">
                <ParticipantTile
                  participant={participant}
                  isLocal={isLocal}
                  isSpeaking={activeSpeakers.has(participant.identity)}
                  isScreenShare
                  displayName={meta.displayName}
                  avatarUrl={meta.avatarUrl}
                  onClick={cbs?.onShareClick}
                  onContextMenu={cbs?.onShareContextMenu}
                />
                <button
                  className="vc-grid-share-expand"
                  title="Expand"
                  onClick={cbs?.onShareClick}
                >
                  <Expand size={14} />
                </button>
              </div>
            );
          })}
          {/* Participant tiles */}
          {allParticipants.map(({ participant, isLocal }) => {
            const meta = isLocal ? localMeta : getMeta(participant.identity, participant as RemoteParticipant);
            const cbs = tileCallbacksById.get(participant.identity);
            return (
              <div key={participant.identity} style={{ position: 'relative' }}>
                <ParticipantTile
                  participant={participant}
                  isLocal={isLocal}
                  isSpeaking={isLocal ? false : activeSpeakers.has(participant.identity)}
                  displayName={meta.displayName}
                  avatarUrl={meta.avatarUrl}
                  activeSoundEmoji={isLocal ? activeSoundEmojis?.get(currentUser.id) : activeSoundEmojis?.get(participant.identity)}
                  onContextMenu={cbs?.onContextMenu}
                  onClick={cbs?.onClick}
                />
                {canMuteMembers && !isLocal && onMuteParticipant && (
                  <button
                    style={{ position: 'absolute', top: 4, right: 4, background: 'rgba(0,0,0,0.6)', border: 'none', borderRadius: 4, padding: '2px 4px', cursor: 'pointer' }}
                    title="Force mute"
                    onClick={() => onMuteParticipant(participant.identity)}
                  >
                    <MicOff size={14} color="var(--parley-danger)" />
                  </button>
                )}
              </div>
            );
          })}
          {selfPlaceholder}
          {allParticipants.length === 0 && !showSelfPlaceholder && (
            <div className="vc-empty">No one else here yet…</div>
          )}
        </div>
      ) : (
        /* Speaker view — screenshares are first-class spotlight items */
        <div className="vc-speaker">
          {spotlightItem ? (
            <div
              className="vc-spotlight"
              onClick={() => setPinnedIdentity(
                pinnedIdentity === speakerKey(spotlightItem) ? null : speakerKey(spotlightItem)
              )}
            >
              <ParticipantTile
                participant={spotlightItem.participant}
                isLocal={spotlightItem.isLocal}
                isScreenShare={spotlightItem.isScreenShare}
                isSpeaking={activeSpeakers.has(spotlightItem.participant.identity)}
                displayName={spotlightItem.isLocal ? localMeta.displayName : getMeta(spotlightItem.participant.identity, spotlightItem.participant as RemoteParticipant).displayName}
                avatarUrl={spotlightItem.isLocal ? localMeta.avatarUrl : getMeta(spotlightItem.participant.identity, spotlightItem.participant as RemoteParticipant).avatarUrl}
              />
            </div>
          ) : showSelfPlaceholder ? (
            <div className="vc-spotlight">{selfPlaceholder}</div>
          ) : (
            <div className="vc-empty">No one here yet…</div>
          )}
          {filmstripItems.length > 0 && (
            <div className="vc-filmstrip">
              {filmstripItems.map((item) => {
                const meta = item.isLocal ? localMeta : getMeta(item.participant.identity, item.participant as RemoteParticipant);
                const cbs = tileCallbacksById.get(item.participant.identity);
                return (
                  <div
                    key={speakerKey(item)}
                    className="vc-filmstrip-tile"
                    style={{ position: 'relative' }}
                    onClick={() => setPinnedIdentity(speakerKey(item))}
                  >
                    <ParticipantTile
                      participant={item.participant}
                      isLocal={item.isLocal}
                      isScreenShare={item.isScreenShare}
                      isSpeaking={activeSpeakers.has(item.participant.identity)}
                      displayName={meta.displayName}
                      avatarUrl={meta.avatarUrl}
                      activeSoundEmoji={item.isLocal ? activeSoundEmojis?.get(currentUser.id) : activeSoundEmojis?.get(item.participant.identity)}
                      onContextMenu={item.isScreenShare ? undefined : cbs?.onContextMenu}
                    />
                    {canMuteMembers && !item.isLocal && !item.isScreenShare && onMuteParticipant && (
                      <button
                        style={{ position: 'absolute', top: 4, right: 4, background: 'rgba(0,0,0,0.6)', border: 'none', borderRadius: 4, padding: '2px 4px', cursor: 'pointer' }}
                        title="Force mute"
                        onClick={(e) => { e.stopPropagation(); onMuteParticipant(item.participant.identity); }}
                      >
                        <MicOff size={14} color="var(--parley-danger)" />
                      </button>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}

      {/* Screenshare context menu */}
      {shareCtxMenu && (() => {
        const item = shareCtxMenu;
        const style: React.CSSProperties = {
          position: 'fixed',
          top: Math.min(item.y, window.innerHeight - 120),
          left: Math.min(item.x, window.innerWidth - 200),
          zIndex: 9999,
        };
        return (
          <div className="vc-context-menu" style={style} onMouseDown={e => e.stopPropagation()}>
            <button className="vc-context-menu-item" onClick={() => {
              setEnlargedShare({ participant: item.participant, isLocal: item.isLocal });
              setShareCtxMenu(null);
            }}>Expand view</button>
            <button className="vc-context-menu-item" onClick={() => {
              setEnlargedShare({ participant: item.participant, isLocal: item.isLocal });
              setShareCtxMenu(null);
              document.documentElement.requestFullscreen().catch(() => {});
            }}>Full screen</button>
          </div>
        );
      })()}

      {/* Enlarged screenshare overlay */}
      {enlargedShare && (() => {
        const { participant, isLocal } = enlargedShare;
        const meta = isLocal ? localMeta : getMeta(participant.identity, participant as RemoteParticipant);
        return (
          <div className="vc-share-overlay" ref={overlayRef} onClick={() => setEnlargedShare(null)}>
            <div className="vc-share-modal" onClick={e => e.stopPropagation()}>
              <div className="vc-share-modal-controls">
                <button
                  className="vc-share-ctrl-btn"
                  title="Full screen"
                  onClick={() => overlayRef.current?.requestFullscreen().catch(() => {})}
                >
                  <Maximize2 size={16} />
                </button>
                <button
                  className="vc-share-ctrl-btn vc-share-ctrl-btn--close"
                  title="Close (Esc)"
                  onClick={() => setEnlargedShare(null)}
                >
                  <X size={16} />
                </button>
              </div>
              <ParticipantTile
                participant={participant}
                isLocal={isLocal}
                isScreenShare
                isSpeaking={false}
                displayName={meta.displayName}
                avatarUrl={meta.avatarUrl}
              />
            </div>
          </div>
        );
      })()}

      {contextMenu && (
        <VoiceContextMenu
          position={{ x: contextMenu.x, y: contextMenu.y }}
          targetUserID={contextMenu.participantId}
          canForceMute={!!canMuteMembers}
          canKick={!!canKickFromVoice}
          onForceMute={() => { onMuteParticipant?.(contextMenu.participantId); setContextMenu(null); }}
          onKick={async () => {
            const id = contextMenu.participantId;
            try { await kickVoiceParticipant(serverVc(channel.id), id); } catch (e) { console.error(e); }
            setContextMenu(null);
          }}
          onClose={() => setContextMenu(null)}
        />
      )}

      {/* In-channel controls (secondary; main controls are in VoiceControls widget) */}
      <div className="vc-controls">
        <button className={`vc-ctrl ${muted ? 'vc-ctrl--off' : ''}`} onClick={onToggleMute} title={muted ? 'Unmute' : 'Mute'}>
          {muted ? <MicOff size={18} color="var(--parley-danger)" /> : <Mic size={18} color="var(--parley-accent)" />}
        </button>
        <button className={`vc-ctrl ${deafened ? 'vc-ctrl--off' : ''}`} onClick={onToggleDeafen} title={deafened ? 'Undeafen' : 'Deafen'}>
          {deafened ? <HeadphoneOff size={18} color="var(--parley-danger)" /> : <Headphones size={18} color="var(--parley-accent)" />}
        </button>
        <button className={`vc-ctrl ${videoEnabled ? '' : 'vc-ctrl--off'}`} onClick={() => onToggleVideo().catch(console.error)} title={videoEnabled ? 'Turn off camera' : 'Turn on camera'}>
          {videoEnabled ? <Video size={18} color="var(--parley-accent)" /> : <VideoOff size={18} color="var(--parley-text-muted)" />}
        </button>
        <button className={`vc-ctrl ${screenSharing ? '' : 'vc-ctrl--off'}`} onClick={() => onToggleScreenShare().catch(console.error)} title={screenSharing ? 'Stop sharing' : 'Share screen'}>
          {screenSharing ? <Monitor size={18} color="var(--parley-accent)" /> : <MonitorOff size={18} color="var(--parley-text-muted)" />}
        </button>
        <div className="vc-soundboard-wrapper">
          <button
            className={`vc-ctrl${soundboardOpen ? ' vc-ctrl--active' : ''}`}
            onClick={() => setSoundboardOpen(v => !v)}
            title="Soundboard"
          >
            <Music2 size={18} color={soundboardOpen ? 'var(--parley-accent)' : 'var(--parley-text-muted)'} />
          </button>
          {soundboardOpen && connected && (
            // localParticipant is null when alone (LiveKit not attached) — that's
            // intentional. SoundboardPanel still plays sounds locally and skips
            // the LK publish in that case.
            <SoundboardPanel
              channelId={channel.id}
              localParticipant={localParticipant}
              onClose={() => setSoundboardOpen(false)}
            />
          )}
        </div>
        <button className="vc-ctrl vc-ctrl--leave" onClick={onLeave} title="Leave channel">
          <PhoneOff size={18} color="var(--parley-danger)" />
        </button>
      </div>
    </div>
  );
};
