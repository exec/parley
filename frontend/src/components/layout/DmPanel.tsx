import React, { useState, useRef, useEffect } from 'react';
import { MessageSquare, BellOff } from 'lucide-react';
import { DmChannel, User, CHANNEL_KIND_DM } from '../../api/types';
import { useChannelState } from '../../context/ChannelStateContext';
import './DmPanel.css';
import { ThemePopover } from '../theme/ThemePopover';

interface UserContextMenuProps {
  position: { top: number; left: number };
  onClose: () => void;
  onSettings: () => void;
  onLogout: () => void;
}

const UserContextMenu: React.FC<UserContextMenuProps> = ({ position, onClose, onSettings, onLogout }) => {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onClose();
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [onClose]);

  return (
    <div
      ref={ref}
      className="user-context-menu"
      style={{ bottom: `calc(100vh - ${position.top}px)`, left: position.left }}
    >
      <button className="user-context-item" onClick={() => { onSettings(); onClose(); }}>
        User Settings
      </button>
      <div className="user-context-divider" />
      <button className="user-context-item danger" onClick={() => { onLogout(); onClose(); }}>
        Log Out
      </button>
    </div>
  );
};

interface DmPanelProps {
  dmChannels: DmChannel[];
  activeDmChannelId: string | null;
  currentUser?: User | null;
  userStatuses?: Record<string, { status_type: string; status_text: string }>;
  onSelectDm: (channelId: string) => void;
  onLogout?: () => void;
  onOpenSettings?: () => void;
  dmUnreadCounts?: Record<string, number>;
  onlineUserIds?: Set<string>;
  onMarkDmRead?: (dmChannelId: string) => void;
}

const DmPanel: React.FC<DmPanelProps> = ({
  dmChannels,
  activeDmChannelId,
  currentUser,
  userStatuses,
  onSelectDm,
  onLogout,
  onOpenSettings,
  dmUnreadCounts = {},
  onlineUserIds,
  onMarkDmRead,
}) => {
  const channelState = useChannelState();
  const [contextMenu, setContextMenu] = useState<{ top: number; left: number } | null>(null);
  const [dmContextMenu, setDmContextMenu] = useState<{ top: number; left: number; channelId: string } | null>(null);
  const userAreaRef = useRef<HTMLDivElement>(null);

  const handleUserAreaClick = (e: React.MouseEvent) => {
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    setContextMenu({ top: rect.top, left: rect.left });
  };

  const handleDmContextMenu = (e: React.MouseEvent, channelId: string) => {
    e.preventDefault();
    setDmContextMenu({ top: e.clientY, left: e.clientX, channelId });
  };

  // Close DM context menu on outside click
  useEffect(() => {
    if (!dmContextMenu) return;
    const handler = () => setDmContextMenu(null);
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [dmContextMenu]);

  return (
    <div className="dm-panel">
      <div className="dm-panel-header">
        <span className="dm-panel-title">Direct Messages</span>
      </div>

      <div className="dm-panel-list">
        {dmChannels.length === 0 ? (
          <div className="dm-panel-empty">
            <MessageSquare className="dm-panel-empty-icon" size={48} strokeWidth={1.5} aria-hidden="true" />
            <div className="dm-panel-empty-title">No direct messages</div>
            <div className="dm-panel-empty-hint">Start a conversation from a friend&apos;s profile.</div>
          </div>
        ) : (
          dmChannels.map(channel => {
            const unread = dmUnreadCounts[channel.id] ?? 0;
            const isActive = channel.id === activeDmChannelId;
            const isOtherOnline = onlineUserIds?.has(channel.other_user_id) ?? false;
            const isMuted = channelState.getNotificationSetting(CHANNEL_KIND_DM, channel.id) === 'MUTED';
            return (
              <div
                key={channel.id}
                className={`dm-panel-item ${isActive ? 'active' : ''} ${unread > 0 && !isActive ? 'unread' : ''}`}
                onClick={() => onSelectDm(channel.id)}
                onContextMenu={(e) => handleDmContextMenu(e, channel.id)}
              >
                <div className="dm-panel-avatar-wrap">
                  <div className="dm-panel-avatar">
                    {channel.other_avatar_url
                      ? <img src={channel.other_avatar_url} alt={channel.other_username} style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: '50%' }} />
                      : channel.other_username.charAt(0).toUpperCase()}
                  </div>
                  <span className={`dm-panel-status-dot ${isOtherOnline ? 'online' : 'offline'}`} />
                </div>
                <span className="dm-panel-name">{channel.other_display_name || channel.other_username}</span>
                {isMuted && <BellOff size={14} className="dm-row-muted" />}
                {unread > 0 && !isActive && (
                  <span
                    className="dm-unread-badge"
                    aria-label={`${unread} unread message${unread === 1 ? '' : 's'}`}
                  >
                    {unread > 99 ? '99+' : unread}
                  </span>
                )}
              </div>
            );
          })
        )}
      </div>

      <div
        ref={userAreaRef}
        className="dm-panel-user-area clickable"
        onClick={handleUserAreaClick}
        title="Click for user settings"
      >
        <div className="dm-panel-user-info">
          <div className="dm-panel-avatar-wrap">
            <div className="dm-panel-user-avatar">
              {currentUser?.avatar_url
                ? <img src={currentUser.avatar_url} alt={currentUser.username} style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: '50%' }} />
                : currentUser?.username?.charAt(0).toUpperCase() || '?'}
            </div>
            <span className={`dm-panel-status-dot ${userStatuses?.[currentUser?.id || '']?.status_type || currentUser?.status_type || 'online'}`} />
          </div>
          <div className="dm-panel-user-details">
            <div className="dm-panel-username">{currentUser?.display_name || currentUser?.username || 'User'}</div>
            <div className="dm-panel-status">{userStatuses?.[currentUser?.id || '']?.status_text || currentUser?.status_text || (() => { const st = userStatuses?.[currentUser?.id || '']?.status_type || currentUser?.status_type || 'online'; return st === 'dnd' ? 'Do Not Disturb' : st === 'afk' ? 'Away' : st === 'invisible' ? 'Invisible' : 'Online'; })()}</div>
          </div>
          <span onClick={e => e.stopPropagation()}><ThemePopover onOpenSettings={() => onOpenSettings?.()} /></span>
          <div className="dm-panel-settings-icon" title="User settings">
            <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18">
              <path d="M19.14,12.94c0.04-0.3,0.06-0.61,0.06-0.94c0-0.32-0.02-0.64-0.07-0.94l2.03-1.58c0.18-0.14,0.23-0.41,0.12-0.61 l-1.92-3.32c-0.12-0.22-0.37-0.29-0.59-0.22l-2.39,0.96c-0.5-0.38-1.03-0.7-1.62-0.94L14.4,2.81c-0.04-0.24-0.24-0.41-0.48-0.41 h-3.84c-0.24,0-0.43,0.17-0.47,0.41L9.25,5.35C8.66,5.59,8.12,5.92,7.63,6.29L5.24,5.33c-0.22-0.08-0.47,0-0.59,0.22L2.74,8.87 C2.62,9.08,2.66,9.34,2.86,9.48l2.03,1.58C4.84,11.36,4.8,11.69,4.8,12s0.02,0.64,0.07,0.94l-2.03,1.58 c-0.18,0.14-0.23,0.41-0.12,0.61l1.92,3.32c0.12,0.22,0.37,0.29,0.59,0.22l2.39-0.96c0.5,0.38,1.03,0.7,1.62,0.94l0.36,2.54 c0.05,0.24,0.24,0.41,0.48,0.41h3.84c0.24,0,0.44-0.17,0.47-0.41l0.36-2.54c0.59-0.24,1.13-0.56,1.62-0.94l2.39,0.96 c0.22,0.08,0.47,0,0.59-0.22l1.92-3.32c0.12-0.22,0.07-0.47-0.12-0.61L19.14,12.94z M12,15.6c-1.98,0-3.6-1.62-3.6-3.6 s1.62-3.6,3.6-3.6s3.6,1.62,3.6,3.6S13.98,15.6,12,15.6z"/>
            </svg>
          </div>
        </div>
      </div>

      {dmContextMenu && (
        <div
          className="dm-item-context-menu"
          style={{ position: 'fixed', top: dmContextMenu.top, left: dmContextMenu.left, zIndex: 9999 }}
          onMouseDown={(e) => e.stopPropagation()}
        >
          <button
            className="dm-item-context-option"
            onClick={() => { onMarkDmRead?.(dmContextMenu.channelId); setDmContextMenu(null); }}
          >
            Mark as Read
          </button>
        </div>
      )}
      {contextMenu && (
        <UserContextMenu
          position={contextMenu}
          onClose={() => setContextMenu(null)}
          onSettings={() => onOpenSettings?.()}
          onLogout={() => onLogout?.()}
        />
      )}
    </div>
  );
};

export default DmPanel;
