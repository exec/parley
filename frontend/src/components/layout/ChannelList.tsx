import React, { useState, useRef, useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { Volume2, Mic, MicOff, PhoneOff, Hash, Code2, X, Plus, Video, VideoOff, Monitor, MonitorOff, Headphones, HeadphoneOff, BellOff } from 'lucide-react';
import { useChannelState } from '../../context/ChannelStateContext';
import * as readStateApi from '../../api/readState';
import { CHANNEL_KIND_SERVER } from '../../api/types';
import { VoiceContextMenu } from '../voice/VoiceContextMenu';
import { muteVoiceParticipant, kickVoiceParticipant, serverVc } from '../../api/voice';
import { copyToClipboard } from '../../lib/tauri';
import {
  DndContext,
  DragEndEvent,
  DragOverlay,
  DragStartEvent,
  MouseSensor,
  TouchSensor,
  closestCenter,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import {
  SortableContext,
  arrayMove,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import './ChannelList.css';
import { ChannelOrder } from '../../api/channels';
import { ThemePopover } from '../theme/ThemePopover';
import { updateStatus, StatusType } from '../../api/status';

// ---- local types ----
interface Channel {
  id: string;
  name: string;
  type: number;       // 0=text, 1=voice, 2=bin, 3=category
  position: number;
  parent_id?: string;
  topic?: string;
}

interface User {
  id: string;
  username: string;
  display_name?: string;
  avatar?: string;
  avatar_url?: string;
  status_type?: string;
  status_text?: string;
}

// ---- context menus ----

const STATUS_OPTIONS: { type: StatusType; label: string; color: string }[] = [
  { type: 'online',    label: 'Online',          color: 'var(--parley-green, #43b581)' },
  { type: 'afk',      label: 'Away',             color: 'var(--parley-yellow, #faa61a)' },
  { type: 'dnd',      label: 'Do Not Disturb',   color: 'var(--parley-danger, #f04747)' },
  { type: 'invisible',label: 'Invisible',         color: 'var(--parley-gray, #747f8d)' },
];

const UserContextMenu: React.FC<{
  position: { top: number; left: number };
  currentUser?: { id: string; username: string; display_name?: string; avatar_url?: string };
  currentStatusType?: string;
  currentStatusText?: string;
  onClose: () => void;
  onSettings: () => void;
  onLogout: () => void;
  onStatusChange?: (statusType: StatusType, statusText: string) => void;
}> = ({ position, currentUser, currentStatusType, currentStatusText, onClose, onSettings, onLogout, onStatusChange }) => {
  const ref = useRef<HTMLDivElement>(null);
  const [statusText, setStatusText] = useState(currentStatusText || '');
  const [activeStatus, setActiveStatus] = useState<StatusType>((currentStatusType as StatusType) || 'online');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setStatusText(currentStatusText || '');
    setActiveStatus((currentStatusType as StatusType) || 'online');
  }, [currentStatusType, currentStatusText]);

  useEffect(() => {
    const handle = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener('mousedown', handle);
    return () => document.removeEventListener('mousedown', handle);
  }, [onClose]);

  const handleStatusTypeClick = async (type: StatusType) => {
    setActiveStatus(type);
    setSaving(true);
    try {
      await updateStatus(type, statusText);
      onStatusChange?.(type, statusText);
    } catch (e) {
      console.error('Failed to update status', e);
    } finally {
      setSaving(false);
    }
  };

  const handleStatusTextBlur = async () => {
    setSaving(true);
    try {
      await updateStatus(activeStatus, statusText);
      onStatusChange?.(activeStatus, statusText);
    } catch (e) {
      console.error('Failed to update status text', e);
    } finally {
      setSaving(false);
    }
  };

  const handleStatusTextKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      (e.target as HTMLInputElement).blur();
    }
    if (e.key === 'Escape') {
      onClose();
    }
  };

  return (
    <div
      ref={ref}
      className="cl-user-context-menu"
      style={{ bottom: `calc(100vh - ${position.top}px)`, left: position.left }}
      onClick={e => e.stopPropagation()}
    >
      {/* User identity header */}
      <div className="cl-ucm-header">
        <div className="cl-ucm-avatar">
          {currentUser?.avatar_url
            ? <img src={currentUser.avatar_url} alt={currentUser.username} />
            : <span>{(currentUser?.display_name || currentUser?.username || 'U').charAt(0).toUpperCase()}</span>
          }
          <span className={`cl-ucm-status-badge status-${activeStatus}`} />
        </div>
        <div className="cl-ucm-identity">
          <div className="cl-ucm-displayname">{currentUser?.display_name || currentUser?.username || 'User'}</div>
          <div className="cl-ucm-username">@{currentUser?.username}</div>
        </div>
      </div>

      <div className="cl-ucm-divider" />

      {/* Status type selector */}
      <div className="cl-ucm-section-label">Set Status</div>
      <div className="cl-ucm-status-grid">
        {STATUS_OPTIONS.map(opt => (
          <button
            key={opt.type}
            className={`cl-ucm-status-opt${activeStatus === opt.type ? ' active' : ''}`}
            onClick={() => handleStatusTypeClick(opt.type)}
            title={opt.label}
          >
            <span
              className={`cl-ucm-status-dot ${opt.type === 'invisible' ? 'invisible' : ''}`}
              style={opt.type !== 'invisible' ? { backgroundColor: opt.color } : { borderColor: opt.color }}
            />
            <span className="cl-ucm-status-label">{opt.label}</span>
            {activeStatus === opt.type && (
              <span className="cl-ucm-status-check">✓</span>
            )}
          </button>
        ))}
      </div>

      {/* Custom status text */}
      <div className="cl-ucm-status-input-wrap">
        <span className="cl-ucm-status-input-icon">💬</span>
        <input
          className="cl-ucm-status-input"
          type="text"
          placeholder="What's on your mind?"
          value={statusText}
          onChange={e => setStatusText(e.target.value.slice(0, 128))}
          onBlur={handleStatusTextBlur}
          onKeyDown={handleStatusTextKeyDown}
          maxLength={128}
        />
        {saving && <span className="cl-ucm-saving">···</span>}
      </div>

      <div className="cl-ucm-divider" />

      {/* Actions */}
      <button className="cl-user-context-item" onClick={() => { onSettings(); onClose(); }}>
        <svg viewBox="0 0 24 24" fill="currentColor" width="14" height="14">
          <path d="M19.14,12.94c0.04-0.3,0.06-0.61,0.06-0.94c0-0.32-0.02-0.64-0.07-0.94l2.03-1.58c0.18-0.14,0.23-0.41,0.12-0.61l-1.92-3.32c-0.12-0.22-0.37-0.29-0.59-0.22l-2.39,0.96c-0.5-0.38-1.03-0.7-1.62-0.94L14.4,2.81c-0.04-0.24-0.24-0.41-0.48-0.41h-3.84c-0.24,0-0.43,0.17-0.47,0.41L9.25,5.35C8.66,5.59,8.12,5.92,7.63,6.29L5.24,5.33c-0.22-0.08-0.47,0-0.59,0.22L2.74,8.87C2.62,9.08,2.66,9.34,2.86,9.48l2.03,1.58C4.84,11.36,4.8,11.69,4.8,12s0.02,0.64,0.07,0.94l-2.03,1.58c-0.18,0.14-0.23,0.41-0.12,0.61l1.92,3.32c0.12,0.22,0.37,0.29,0.59,0.22l2.39-0.96c0.5,0.38,1.03,0.7,1.62,0.94l0.36,2.54c0.05,0.24,0.24,0.41,0.48,0.41h3.84c0.24,0,0.44-0.17,0.47-0.41l0.36-2.54c0.59-0.24,1.13-0.56,1.62-0.94l2.39,0.96c0.22,0.08,0.47,0,0.59-0.22l1.92-3.32c0.12-0.22,0.07-0.47-0.12-0.61L19.14,12.94z M12,15.6c-1.98,0-3.6-1.62-3.6-3.6s1.62-3.6,3.6-3.6s3.6,1.62,3.6,3.6S13.98,15.6,12,15.6z"/>
        </svg>
        User Settings
      </button>
      <button className="cl-user-context-item danger" onClick={() => { onLogout(); onClose(); }}>
        <svg viewBox="0 0 24 24" fill="currentColor" width="14" height="14">
          <path d="M17 7l-1.41 1.41L18.17 11H8v2h10.17l-2.58 2.58L17 17l5-5zM4 5h8V3H4c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h8v-2H4V5z"/>
        </svg>
        Log Out
      </button>
    </div>
  );
};

const ServerContextMenu: React.FC<{
  position: { top: number; left: number };
  onClose: () => void;
  onLeave?: () => void;
  onSettings?: () => void;
}> = ({ position, onClose, onLeave, onSettings }) => {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const handle = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) onClose(); };
    document.addEventListener('mousedown', handle);
    return () => document.removeEventListener('mousedown', handle);
  }, [onClose]);
  return (
    <div ref={ref} className="cl-server-context-menu" style={{ top: position.top, left: position.left }}>
      {onSettings && <button className="cl-server-context-item" onClick={() => { onSettings(); onClose(); }}>Server Settings</button>}
      {onLeave && (
        <>
          <div className="cl-server-context-divider" />
          <button className="cl-server-context-item danger" onClick={() => { onLeave(); onClose(); }}>Leave Server</button>
        </>
      )}
    </div>
  );
};

const ChannelContextMenu: React.FC<{
  channel: Channel;
  position: { top: number; left: number };
  canManageChannels: boolean;
  notificationSetting: string;
  onClose: () => void;
  onRename: () => void;
  onDelete: () => void;
  onMarkAsRead: () => void;
  onChannelSettings?: () => void;
  onSetNotification: (setting: 'ALL' | 'MENTIONS_ONLY' | 'MUTED') => void;
}> = ({ channel, position, canManageChannels, notificationSetting, onClose, onRename, onDelete, onMarkAsRead, onChannelSettings, onSetNotification }) => {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const handle = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) onClose(); };
    document.addEventListener('mousedown', handle);
    return () => document.removeEventListener('mousedown', handle);
  }, [onClose]);
  const label = channel.type === 3 ? `📁 ${channel.name}` : `# ${channel.name}`;
  return (
    <div ref={ref} className="cl-channel-context-menu" style={{ top: position.top, left: position.left }}>
      <div className="cl-channel-context-header">{label}</div>
      <div className="cl-server-context-divider" />
      {channel.type !== 3 && <button className="cl-server-context-item" onClick={() => { onMarkAsRead(); onClose(); }}>Mark as Read</button>}
      <button className="cl-server-context-item" onClick={() => { void copyToClipboard(channel.id); onClose(); }}>Copy ID</button>
      {channel.type !== 3 && (
        <>
          <div className="cl-server-context-divider" />
          <div className="cl-channel-context-section-label">Notifications</div>
          <button className="cl-server-context-item" onClick={() => { onSetNotification('ALL'); onClose(); }}>
            All messages {notificationSetting === 'ALL' ? '✓' : ''}
          </button>
          <button className="cl-server-context-item" onClick={() => { onSetNotification('MENTIONS_ONLY'); onClose(); }}>
            @mentions only {notificationSetting === 'MENTIONS_ONLY' ? '✓' : ''}
          </button>
          <button className="cl-server-context-item" onClick={() => { onSetNotification('MUTED'); onClose(); }}>
            Muted {notificationSetting === 'MUTED' ? '✓' : ''}
          </button>
        </>
      )}
      {canManageChannels && (
        <>
          <div className="cl-server-context-divider" />
          {onChannelSettings && channel.type !== 3 && (
            <button className="cl-server-context-item" onClick={() => { onChannelSettings(); onClose(); }}>Channel Settings</button>
          )}
          <button className="cl-server-context-item" onClick={() => { onRename(); onClose(); }}>Rename</button>
          <button className="cl-server-context-item danger" onClick={() => { onDelete(); onClose(); }}>Delete</button>
        </>
      )}
    </div>
  );
};

// ---- sortable channel item ----

function channelIcon(type: number) {
  if (type === 1) return <Volume2 size={13} color="currentColor" />;
  if (type === 2) return <Code2 size={13} color="currentColor" />;
  return <Hash size={13} color="currentColor" />;
}

const SortableChannelItem: React.FC<{
  channel: Channel;
  isActive: boolean;
  unread: number;
  hasMention?: boolean;
  isRenaming: boolean;
  renameValue: string;
  renameInputRef: React.RefObject<HTMLInputElement>;
  hoveredChannel: string | null;
  canManageChannels: boolean;
  activeVoiceChannelId: string | null;
  voiceParticipants: Record<string, { user_id: string; username: string; avatar_url?: string }[]>;
  isMuted?: boolean;
  onSelect: () => void;
  onContextMenu: (e: React.MouseEvent) => void;
  onMouseEnter: () => void;
  onMouseLeave: () => void;
  onDelete: () => void;
  onVoiceClick?: () => void;
  onRenameChange: (v: string) => void;
  onRenameBlur: () => void;
  onRenameKeyDown: (e: React.KeyboardEvent) => void;
  isDragging?: boolean;
  canMuteMembers?: boolean;
  canKickFromVoice?: boolean;
  onParticipantContextMenu?: (channelId: string, participantId: string, x: number, y: number) => void;
  onParticipantClick?: (userId: string, clientX: number, clientY: number) => void;
}> = ({
  channel, isActive, unread, hasMention, isRenaming, renameValue, renameInputRef,
  hoveredChannel, canManageChannels, activeVoiceChannelId, voiceParticipants, isMuted,
  onSelect, onContextMenu, onMouseEnter, onMouseLeave, onDelete, onVoiceClick,
  onRenameChange, onRenameBlur, onRenameKeyDown, isDragging,
  canMuteMembers, canKickFromVoice, onParticipantContextMenu, onParticipantClick,
}) => {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging: sortableDragging } = useSortable({ id: channel.id, disabled: !canManageChannels });
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging || sortableDragging ? 0.5 : 1,
  };

  if (channel.type === 1) {
    const participants = voiceParticipants[channel.id] ?? [];
    const isVoiceActive = channel.id === activeVoiceChannelId;
    return (
      <div ref={setNodeRef} style={style} {...attributes} {...(canManageChannels ? listeners : {})}>
        <div
          className={`voice-channel-item${isVoiceActive ? ' active' : ''}`}
          onClick={onVoiceClick}
          onContextMenu={onContextMenu}
          onMouseEnter={onMouseEnter}
          onMouseLeave={onMouseLeave}
        >
          <span className="voice-icon"><Volume2 size={13} color="currentColor" /></span>
          <span className="channel-name">{channel.name}</span>
          {participants.length > 0 && <span className="voice-count">{participants.length}</span>}
          {canManageChannels && hoveredChannel === channel.id && (
            <button className="delete-channel-btn" onClick={e => { e.stopPropagation(); onDelete(); }} title="Delete"><X size={12} color="currentColor" /></button>
          )}
        </div>
        {participants.length > 0 && (
          <div className="voice-participants-list">
            {participants.map(p => (
              <div
                key={p.user_id}
                className="voice-participant-row"
                style={{ cursor: 'pointer' }}
                onClick={(e) => { e.stopPropagation(); onParticipantClick?.(p.user_id, e.clientX, e.clientY); }}
                onContextMenu={(canMuteMembers || canKickFromVoice) ? (e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  onParticipantContextMenu?.(channel.id, p.user_id, e.clientX, e.clientY);
                } : undefined}
              >
                <div className="voice-participant-avatar">
                  {p.avatar_url ? <img src={p.avatar_url} alt={p.username} /> : p.username.charAt(0).toUpperCase()}
                </div>
                <span>{p.username}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    );
  }

  return (
    <div ref={setNodeRef} style={style} {...attributes} {...(canManageChannels ? listeners : {})}>
      <div
        className={`channel-item${isActive ? ' active' : ''}${unread > 0 && !isActive ? ' unread' : ''}`}
        onClick={() => !isRenaming && onSelect()}
        onContextMenu={onContextMenu}
        onMouseEnter={onMouseEnter}
        onMouseLeave={onMouseLeave}
      >
        <span className="channel-icon">{channelIcon(channel.type)}</span>
        {isRenaming ? (
          <input
            ref={renameInputRef}
            className="channel-rename-input"
            value={renameValue}
            onChange={e => onRenameChange(e.target.value)}
            onBlur={onRenameBlur}
            onKeyDown={onRenameKeyDown}
            onClick={e => e.stopPropagation()}
          />
        ) : (
          <span className="channel-name">{channel.name}</span>
        )}
        {isMuted && !isRenaming && <BellOff size={13} className="channel-row-muted" />}
        {unread > 0 && !isActive && !isRenaming && (
          <span className={`channel-unread-badge${hasMention ? ' mention' : ' plain'}`}>{unread > 99 ? '99+' : unread}</span>
        )}
        {canManageChannels && hoveredChannel === channel.id && !isRenaming && (
          <button className="delete-channel-btn" onClick={e => { e.stopPropagation(); onDelete(); }} title="Delete"><X size={14} /></button>
        )}
      </div>
    </div>
  );
};

// ---- sortable category ----

const SortableCategoryHeader: React.FC<{
  category: Channel;
  isCollapsed: boolean;
  isRenaming: boolean;
  renameValue: string;
  renameInputRef: React.RefObject<HTMLInputElement>;
  canManageChannels: boolean;
  onToggle: () => void;
  onCreateChannel: () => void;
  onContextMenu: (e: React.MouseEvent) => void;
  onRenameChange: (v: string) => void;
  onRenameBlur: () => void;
  onRenameKeyDown: (e: React.KeyboardEvent) => void;
}> = ({
  category, isCollapsed, isRenaming, renameValue, renameInputRef,
  canManageChannels, onToggle, onCreateChannel, onContextMenu,
  onRenameChange, onRenameBlur, onRenameKeyDown,
}) => {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: category.id, disabled: !canManageChannels });
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };
  return (
    <div ref={setNodeRef} style={style}>
      <div className="category-row">
        <div
          className={`category-header${isCollapsed ? ' collapsed' : ''}`}
          onClick={onToggle}
          onContextMenu={onContextMenu}
          {...(canManageChannels ? { ...attributes, ...listeners } : {})}
        >
          <svg viewBox="0 0 24 24" fill="currentColor">
            <path d="M7 10l5 5 5-5z" />
          </svg>
          {isRenaming ? (
            <input
              ref={renameInputRef}
              className="channel-rename-input"
              value={renameValue}
              onChange={e => onRenameChange(e.target.value)}
              onBlur={onRenameBlur}
              onKeyDown={onRenameKeyDown}
              onClick={e => e.stopPropagation()}
            />
          ) : (
            category.name.toUpperCase()
          )}
        </div>
        {canManageChannels && (
          <button className="add-channel-btn" onClick={onCreateChannel} title="Create channel"><Plus size={13} color="currentColor" /></button>
        )}
      </div>
    </div>
  );
};

// ---- main props ----

interface ChannelListProps {
  serverName: string;
  channels: Channel[];
  activeChannelId: string | null;
  onChannelSelect: (channelId: string) => void;
  onCreateChannel: () => void;
  onDeleteChannel: (channelId: string) => void;
  onManageRoles?: () => void;
  onServerSettings?: () => void;
  onLeaveServer?: () => void;
  owner_id?: string;
  currentUser?: User;
  onLogout?: () => void;
  onOpenSettings?: () => void;
  onVoiceChannelClick?: (channelId: string) => void;
  voiceParticipants?: Record<string, { user_id: string; username: string; avatar_url?: string }[]>;
  activeVoiceChannelId?: string | null;
  channelUnreadCounts?: Record<string, number>;
  channelMentionCounts?: Set<string>;
  canManageChannels?: boolean;
  onRenameChannel?: (channelId: string, newName: string) => void;
  onMarkChannelRead?: (channelId: string) => void;
  onReorderChannels?: (orders: ChannelOrder[]) => void;
  onChannelSettings?: (channelId: string) => void;
  canMuteMembers?: boolean;
  canKickFromVoice?: boolean;
  onVcParticipantClick?: (userId: string, clientX: number, clientY: number) => void;
  isOpen?: boolean;
  vcConnected?: boolean;
  vcMuted?: boolean;
  vcDeafened?: boolean;
  vcVideoEnabled?: boolean;
  vcScreenSharing?: boolean;
  onVcMuteToggle?: () => void;
  onVcDeafenToggle?: () => void;
  onVcVideoToggle?: () => void;
  onVcScreenShareToggle?: () => void;
  onVcLeave?: () => void;
  onVcNavigate?: () => void;
  userStatuses?: Record<string, { status_type: string; status_text: string }>;
  onStatusChange?: (statusType: StatusType, statusText: string) => void;
}

// ---- main component ----

const ChannelList: React.FC<ChannelListProps> = ({
  serverName, channels, activeChannelId, onChannelSelect, onCreateChannel,
  onDeleteChannel, onServerSettings, onLeaveServer,
  currentUser, onLogout, onOpenSettings, onVoiceChannelClick,
  voiceParticipants = {}, activeVoiceChannelId = null, channelUnreadCounts = {}, channelMentionCounts = new Set<string>(),
  canManageChannels = false, onRenameChannel, onMarkChannelRead, onReorderChannels, onChannelSettings,
  canMuteMembers, canKickFromVoice, onVcParticipantClick,
  isOpen = true,
  vcConnected, vcMuted, vcDeafened, vcVideoEnabled, vcScreenSharing,
  onVcMuteToggle, onVcDeafenToggle, onVcVideoToggle, onVcScreenShareToggle, onVcLeave, onVcNavigate,
  userStatuses, onStatusChange,
}) => {
  const channelState = useChannelState();
  const [collapsedCategories, setCollapsedCategories] = useState<Set<string>>(new Set());
  const [hoveredChannel, setHoveredChannel] = useState<string | null>(null);
  const [vcParticipantMenu, setVcParticipantMenu] = useState<{ channelId: string; participantId: string; x: number; y: number } | null>(null);
  const [userContextMenu, setUserContextMenu] = useState<{ top: number; left: number } | null>(null);
  const [serverContextMenu, setServerContextMenu] = useState<{ top: number; left: number } | null>(null);
  const [channelContextMenu, setChannelContextMenu] = useState<{ channel: Channel; top: number; left: number } | null>(null);
  const [renamingChannelId, setRenamingChannelId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState('');
  const renameInputRef = useRef<HTMLInputElement>(null);
  const [activeId, setActiveId] = useState<string | null>(null);

  useEffect(() => { if (renamingChannelId) renameInputRef.current?.focus(); }, [renamingChannelId]);

  const startRename = (channel: Channel) => { setRenameValue(channel.name); setRenamingChannelId(channel.id); };
  const commitRename = (channelId: string) => {
    if (renameValue.trim()) onRenameChannel?.(channelId, renameValue.trim());
    setRenamingChannelId(null);
  };
  const renameKeyDown = (channelId: string) => (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') { e.preventDefault(); commitRename(channelId); }
    if (e.key === 'Escape') setRenamingChannelId(null);
  };

  const sorted = [...channels].sort((a, b) => a.position - b.position || a.name.localeCompare(b.name));
  const categories = sorted.filter(c => c.type === 3);
  const uncategorized = sorted.filter(c => c.type !== 3 && !c.parent_id);

  const childrenOf = useCallback((catId: string) =>
    sorted.filter(c => c.type !== 3 && c.parent_id === catId),
  [sorted]);

  // dnd-kit sensors — require 8px movement to start drag (prevents click hijacking)
  // Split sensors: mouse uses distance-based activation so click vs. drag is
  // fast on desktop, touch uses a 250ms press-and-hold so a normal tap on
  // the channel goes through as a click without being swallowed by the
  // drag-detection window on iOS.
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 8 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 5 } }),
  );

  const handleDragStart = (event: DragStartEvent) => {
    setActiveId(String(event.active.id));
  };

  const handleDragEnd = useCallback((event: DragEndEvent) => {
    setActiveId(null);
    if (!onReorderChannels) return;
    const { active, over } = event;
    if (!over || active.id === over.id) return;

    const activeChannel = channels.find(c => c.id === active.id);
    const overChannel = channels.find(c => c.id === over.id);
    if (!activeChannel || !overChannel) return;

    const isCategory = activeChannel.type === 3;

    if (isCategory) {
      // Reorder categories
      const catIds = categories.map(c => c.id);
      const oldIdx = catIds.indexOf(String(active.id));
      const newIdx = catIds.indexOf(String(over.id));
      if (oldIdx === -1 || newIdx === -1) return;
      const reordered = arrayMove(categories, oldIdx, newIdx);
      const orders: ChannelOrder[] = reordered.map((c, i) => ({ id: c.id, position: i * 10, parent_id: null }));
      onReorderChannels(orders);
    } else {
      // Reorder channels
      const srcParent = activeChannel.parent_id ?? null;
      const dstParent = overChannel.type === 3 ? overChannel.id : (overChannel.parent_id ?? null);

      if (srcParent === dstParent) {
        // Same container — reorder within
        const list = dstParent ? childrenOf(dstParent) : uncategorized;
        const oldIdx = list.findIndex(c => c.id === active.id);
        const newIdx = list.findIndex(c => c.id === over.id);
        if (oldIdx === -1 || newIdx === -1) return;
        const reordered = arrayMove(list, oldIdx, newIdx);
        const orders: ChannelOrder[] = reordered.map((c, i) => ({ id: c.id, position: i * 10, parent_id: dstParent }));
        onReorderChannels(orders);
      } else {
        // Cross-container move
        const srcList = srcParent ? childrenOf(srcParent) : uncategorized;
        const dstList = dstParent ? childrenOf(dstParent) : uncategorized;
        const newSrc = srcList.filter(c => c.id !== active.id);
        const dstIdx = dstParent && overChannel.type === 3 ? dstList.length : dstList.findIndex(c => c.id === over.id);
        const newDst = [...dstList];
        newDst.splice(dstIdx === -1 ? newDst.length : dstIdx, 0, activeChannel);
        const orders: ChannelOrder[] = [
          ...newSrc.map((c, i) => ({ id: c.id, position: i * 10, parent_id: srcParent })),
          ...newDst.map((c, i) => ({ id: c.id, position: i * 10, parent_id: dstParent })),
        ];
        onReorderChannels(orders);
      }
    }
  }, [channels, categories, uncategorized, childrenOf, onReorderChannels]);

  const renderChannelItem = (ch: Channel) => {
    const isMuted = channelState.getNotificationSetting(CHANNEL_KIND_SERVER, ch.id) === 'MUTED';
    return (
      <SortableChannelItem
        key={ch.id}
        channel={ch}
        isActive={ch.id === activeChannelId}
        unread={channelUnreadCounts[ch.id] ?? 0}
        hasMention={channelMentionCounts.has(ch.id)}
        isRenaming={renamingChannelId === ch.id}
        renameValue={renameValue}
        renameInputRef={renameInputRef}
        hoveredChannel={hoveredChannel}
        canManageChannels={canManageChannels}
        activeVoiceChannelId={activeVoiceChannelId}
        voiceParticipants={voiceParticipants}
        isMuted={isMuted}
        onSelect={() => onChannelSelect(ch.id)}
        onContextMenu={e => { e.preventDefault(); setChannelContextMenu({ channel: ch, top: e.clientY, left: e.clientX }); }}
        onMouseEnter={() => setHoveredChannel(ch.id)}
        onMouseLeave={() => setHoveredChannel(null)}
        onDelete={() => onDeleteChannel(ch.id)}
        onVoiceClick={() => onVoiceChannelClick?.(ch.id)}
        onRenameChange={setRenameValue}
        onRenameBlur={() => commitRename(ch.id)}
        onRenameKeyDown={renameKeyDown(ch.id)}
        isDragging={activeId === ch.id}
        canMuteMembers={canMuteMembers}
        canKickFromVoice={canKickFromVoice}
        onParticipantContextMenu={(channelId, participantId, x, y) => setVcParticipantMenu({ channelId, participantId, x, y })}
        onParticipantClick={onVcParticipantClick}
      />
    );
  };

  const activeItem = activeId ? channels.find(c => c.id === activeId) : null;

  return (
    <div className={`channel-list${isOpen ? ' channel-list--open' : ''}`}>
      <div
        className="server-header clickable"
        onClick={e => { const r = (e.currentTarget as HTMLElement).getBoundingClientRect(); setServerContextMenu({ top: r.top, left: r.left }); }}
      >
        <span className="server-name">{serverName}</span>
        <div className="server-header-actions" data-tauri-drag-region="false">
          {onServerSettings && (
            <button className="server-settings-btn" onClick={e => { e.stopPropagation(); onServerSettings(); }} title="Server Settings">
              <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20">
                <path d="M19.14 12.94c.04-.31.06-.63.06-.94 0-.31-.02-.63-.06-.94l2.03-1.58c.18-.14.23-.41.12-.61l-1.92-3.32c-.12-.22-.37-.29-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54c-.04-.24-.24-.41-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.04.31-.06.63-.06.94s.02.63.06.94l-2.03 1.58c-.18.14-.23.41-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z" />
              </svg>
            </button>
          )}
          <div className="server-menu-icon" title="Server options">
            <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16">
              <path d="M12 8c1.1 0 2-.9 2-2s-.9-2-2-2-2 .9-2 2 .9 2 2 2zm0 2c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2zm0 6c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2z" />
            </svg>
          </div>
        </div>
      </div>

      <div className="channels-container">
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragStart={handleDragStart}
          onDragEnd={handleDragEnd}
        >
          {/* Uncategorized channels */}
          {uncategorized.length > 0 && (
            <SortableContext items={uncategorized.map(c => c.id)} strategy={verticalListSortingStrategy}>
              <div className="uncategorized-channels">
                {uncategorized.map(ch => renderChannelItem(ch))}
              </div>
            </SortableContext>
          )}

          {/* Categories */}
          <SortableContext items={categories.map(c => c.id)} strategy={verticalListSortingStrategy}>
            {categories.map(cat => {
              const isCollapsed = collapsedCategories.has(cat.id);
              const children = childrenOf(cat.id);
              return (
                <div key={cat.id}>
                  <SortableCategoryHeader
                    category={cat}
                    isCollapsed={isCollapsed}
                    isRenaming={renamingChannelId === cat.id}
                    renameValue={renameValue}
                    renameInputRef={renameInputRef}
                    canManageChannels={canManageChannels}
                    onToggle={() => setCollapsedCategories(prev => {
                      const next = new Set(prev);
                      if (next.has(cat.id)) next.delete(cat.id); else next.add(cat.id);
                      return next;
                    })}
                    onCreateChannel={onCreateChannel}
                    onContextMenu={e => { e.preventDefault(); setChannelContextMenu({ channel: cat, top: e.clientY, left: e.clientX }); }}
                    onRenameChange={setRenameValue}
                    onRenameBlur={() => commitRename(cat.id)}
                    onRenameKeyDown={renameKeyDown(cat.id)}
                  />
                  {!isCollapsed && (
                    <SortableContext items={children.map(c => c.id)} strategy={verticalListSortingStrategy}>
                      <div className="category-channels">
                        {children.map(ch => renderChannelItem(ch))}
                      </div>
                    </SortableContext>
                  )}
                </div>
              );
            })}
          </SortableContext>

          <DragOverlay>
            {activeItem && (
              <div className={activeItem.type === 3 ? 'category-drag-overlay' : 'channel-drag-overlay'}>
                {activeItem.type === 3 ? activeItem.name.toUpperCase() : (
                  <>
                    <span className="channel-icon">{channelIcon(activeItem.type)}</span>
                    <span className="channel-name">{activeItem.name}</span>
                  </>
                )}
              </div>
            )}
          </DragOverlay>
        </DndContext>

        {categories.length === 0 && canManageChannels && (
          <div className="category-row">
            <div className="category-header" style={{ cursor: 'default' }}>Channels</div>
            <button className="add-channel-btn" onClick={onCreateChannel} title="Create channel"><Plus size={13} color="currentColor" /></button>
          </div>
        )}

      </div>

      {activeVoiceChannelId && (
        <div className="voice-bar">
          <div className="voice-bar-status">
            <div className={`voice-bar-dot${vcConnected ? '' : ' connecting'}`} />
            <span className="voice-bar-label">{vcConnected ? 'Voice Connected' : 'Connecting…'}</span>
          </div>
          {(() => {
            const vcCh = channels.find(c => c.id === activeVoiceChannelId);
            return vcCh ? <div className="voice-bar-channel" onClick={onVcNavigate} title="Go to voice channel"><Volume2 size={12} color="currentColor" /> {vcCh.name}</div> : null;
          })()}
          <div className="voice-bar-controls">
            <button className={`voice-bar-btn${vcMuted ? ' active' : ''}`} onClick={onVcMuteToggle} title={vcMuted ? 'Unmute' : 'Mute'}>{vcMuted ? <MicOff size={13} color="var(--parley-danger)" /> : <Mic size={13} color="var(--parley-accent)" />}</button>
            <button className={`voice-bar-btn${vcDeafened ? ' active' : ''}`} onClick={onVcDeafenToggle} title={vcDeafened ? 'Undeafen' : 'Deafen'}>{vcDeafened ? <HeadphoneOff size={13} color="var(--parley-danger)" /> : <Headphones size={13} color="var(--parley-accent)" />}</button>
            <button className={`voice-bar-btn${vcVideoEnabled ? '' : ' active'}`} onClick={onVcVideoToggle} title={vcVideoEnabled ? 'Camera off' : 'Camera on'}>{vcVideoEnabled ? <Video size={13} color="var(--parley-accent)" /> : <VideoOff size={13} color="var(--parley-text-muted)" />}</button>
            <button className={`voice-bar-btn${vcScreenSharing ? '' : ' active'}`} onClick={onVcScreenShareToggle} title={vcScreenSharing ? 'Stop sharing' : 'Share screen'}>{vcScreenSharing ? <Monitor size={13} color="var(--parley-accent)" /> : <MonitorOff size={13} color="var(--parley-text-muted)" />}</button>
            <button className="voice-bar-btn leave" onClick={onVcLeave} title="Leave voice"><PhoneOff size={13} color="var(--parley-danger)" /></button>
          </div>
        </div>
      )}

      {(() => {
        const myStatus = userStatuses?.[currentUser?.id || '']?.status_type || currentUser?.status_type || 'online';
        const myStatusText = userStatuses?.[currentUser?.id || '']?.status_text || currentUser?.status_text || '';
        const statusLabel = myStatus === 'dnd' ? 'Do Not Disturb' : myStatus === 'afk' ? 'Away' : myStatus === 'invisible' ? 'Invisible' : 'Online';
        return (
          <div className="user-area clickable" onClick={e => { const r = (e.currentTarget as HTMLElement).getBoundingClientRect(); setUserContextMenu({ top: r.top, left: r.left }); }} title="Click for user settings">
            <div className="user-info">
              <div className="user-avatar-wrap">
                <div className="user-avatar">
                  {currentUser?.avatar_url
                    ? <img src={currentUser.avatar_url} alt={currentUser.username} style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: '50%' }} />
                    : <span className="user-avatar-placeholder">{currentUser?.username?.charAt(0).toUpperCase() || 'U'}</span>
                  }
                </div>
                <span className={`user-status-dot ${myStatus}`} />
              </div>
              <div className="user-details">
                <div className="username">{currentUser?.display_name || currentUser?.username || 'User'}</div>
                <div className="user-status">{myStatusText || statusLabel}</div>
              </div>
              <span onClick={e => e.stopPropagation()}><ThemePopover onOpenSettings={() => onOpenSettings?.()} /></span>
              <div className="cl-settings-icon" title="User settings">
                <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18">
                  <path d="M19.14,12.94c0.04-0.3,0.06-0.61,0.06-0.94c0-0.32-0.02-0.64-0.07-0.94l2.03-1.58c0.18-0.14,0.23-0.41,0.12-0.61 l-1.92-3.32c-0.12-0.22-0.37-0.29-0.59-0.22l-2.39,0.96c-0.5-0.38-1.03-0.7-1.62-0.94L14.4,2.81c-0.04-0.24-0.24-0.41-0.48-0.41 h-3.84c-0.24,0-0.43,0.17-0.47,0.41L9.25,5.35C8.66,5.59,8.12,5.92,7.63,6.29L5.24,5.33c-0.22-0.08-0.47,0-0.59,0.22L2.74,8.87 C2.62,9.08,2.66,9.34,2.86,9.48l2.03,1.58C4.84,11.36,4.8,11.69,4.8,12s0.02,0.64,0.07,0.94l-2.03,1.58 c-0.18,0.14-0.23,0.41-0.12,0.61l1.92,3.32c0.12,0.22,0.37,0.29,0.59,0.22l2.39-0.96c0.5,0.38,1.03,0.7,1.62,0.94l0.36,2.54 c0.05,0.24,0.24,0.41,0.48,0.41h3.84c0.24,0,0.44-0.17,0.47-0.41l0.36-2.54c0.59-0.24,1.13-0.56,1.62-0.94l2.39,0.96 c0.22,0.08,0.47,0,0.59-0.22l1.92-3.32c0.12-0.22,0.07-0.47-0.12-0.61L19.14,12.94z M12,15.6c-1.98,0-3.6-1.62-3.6-3.6 s1.62-3.6,3.6-3.6s3.6,1.62,3.6,3.6S13.98,15.6,12,15.6z"/>
                </svg>
              </div>
            </div>
          </div>
        );
      })()}

      {userContextMenu && createPortal(
        <UserContextMenu
          position={userContextMenu}
          currentUser={currentUser}
          currentStatusType={userStatuses?.[currentUser?.id || '']?.status_type || currentUser?.status_type || 'online'}
          currentStatusText={userStatuses?.[currentUser?.id || '']?.status_text || currentUser?.status_text || ''}
          onClose={() => setUserContextMenu(null)}
          onSettings={() => onOpenSettings?.()}
          onLogout={() => onLogout?.()}
          onStatusChange={onStatusChange}
        />,
        document.body
      )}
      {serverContextMenu && createPortal(
        <ServerContextMenu position={serverContextMenu} onClose={() => setServerContextMenu(null)} onSettings={onServerSettings} onLeave={onLeaveServer} />,
        document.body
      )}
      {channelContextMenu && createPortal(
        <ChannelContextMenu
          channel={channelContextMenu.channel}
          position={{ top: channelContextMenu.top, left: channelContextMenu.left }}
          canManageChannels={canManageChannels}
          notificationSetting={channelState.getNotificationSetting(CHANNEL_KIND_SERVER, channelContextMenu.channel.id)}
          onClose={() => setChannelContextMenu(null)}
          onRename={() => startRename(channelContextMenu.channel)}
          onDelete={() => onDeleteChannel(channelContextMenu.channel.id)}
          onMarkAsRead={() => onMarkChannelRead?.(channelContextMenu.channel.id)}
          onChannelSettings={onChannelSettings ? () => onChannelSettings(channelContextMenu.channel.id) : undefined}
          onSetNotification={(setting) => {
            const id = channelContextMenu.channel.id;
            readStateApi.setNotificationSetting(CHANNEL_KIND_SERVER, id, setting).catch(e => console.error('Failed to set notification setting', e));
            channelState.setNotificationSettingLocal(CHANNEL_KIND_SERVER, id, setting);
          }}
        />,
        document.body
      )}
      {vcParticipantMenu && (
        <VoiceContextMenu
          position={{ x: vcParticipantMenu.x, y: vcParticipantMenu.y }}
          targetUserID={vcParticipantMenu.participantId}
          canForceMute={!!canMuteMembers}
          canKick={!!canKickFromVoice}
          onForceMute={async () => {
            try { await muteVoiceParticipant(serverVc(vcParticipantMenu.channelId), vcParticipantMenu.participantId); } catch (e) { console.error(e); }
            setVcParticipantMenu(null);
          }}
          onKick={async () => {
            const id = vcParticipantMenu.participantId;
            try { await kickVoiceParticipant(serverVc(vcParticipantMenu.channelId), id); } catch (e) { console.error(e); }
            setVcParticipantMenu(null);
          }}
          onClose={() => setVcParticipantMenu(null)}
        />
      )}
    </div>
  );
};

export default ChannelList;
