import React, { useState, useRef, useEffect } from 'react';
import './ChannelList.css';

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
      className="cl-user-context-menu"
      style={{ bottom: `calc(100vh - ${position.top}px)`, left: position.left }}
    >
      <button className="cl-user-context-item" onClick={() => { onSettings(); onClose(); }}>
        User Settings
      </button>
      <div className="cl-user-context-divider" />
      <button className="cl-user-context-item danger" onClick={() => { onLogout(); onClose(); }}>
        Log Out
      </button>
    </div>
  );
};

interface ServerContextMenuProps {
  position: { top: number; left: number };
  serverName: string;
  isOwner: boolean;
  onClose: () => void;
  onLeave?: () => void;
  onSettings?: () => void;
}

const ServerContextMenu: React.FC<ServerContextMenuProps> = ({ position, onClose, onLeave, onSettings }) => {
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
      className="cl-server-context-menu"
      style={{ top: position.top, left: position.left }}
    >
      {onSettings && (
        <button className="cl-server-context-item" onClick={() => { onSettings(); onClose(); }}>
          Server Settings
        </button>
      )}
      {onLeave && (
        <>
          <div className="cl-server-context-divider" />
          <button className="cl-server-context-item danger" onClick={() => { onLeave(); onClose(); }}>
            Leave Server
          </button>
        </>
      )}
    </div>
  );
};

interface Channel {
  id: string;
  name: string;
  type: number;
  topic?: string;
}

interface ChannelContextMenuProps {
  channel: Channel;
  position: { top: number; left: number };
  canManageChannels: boolean;
  onClose: () => void;
  onRename: () => void;
  onDelete: () => void;
  onMarkAsRead: () => void;
}

const ChannelContextMenu: React.FC<ChannelContextMenuProps> = ({ channel, position, canManageChannels, onClose, onRename, onDelete, onMarkAsRead }) => {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [onClose]);

  return (
    <div ref={ref} className="cl-channel-context-menu" style={{ top: position.top, left: position.left }}>
      <div className="cl-channel-context-header"># {channel.name}</div>
      <div className="cl-server-context-divider" />
      <button className="cl-server-context-item" onClick={() => { onMarkAsRead(); onClose(); }}>Mark as Read</button>
      <button className="cl-server-context-item" onClick={() => { navigator.clipboard.writeText(channel.id); onClose(); }}>Copy Channel ID</button>
      {canManageChannels && (
        <>
          <div className="cl-server-context-divider" />
          <button className="cl-server-context-item" onClick={() => { onRename(); onClose(); }}>Rename Channel</button>
          <button className="cl-server-context-item danger" onClick={() => { onDelete(); onClose(); }}>Delete Channel</button>
        </>
      )}
    </div>
  );
};

interface User {
  id: string;
  username: string;
  avatar?: string;
}

interface ChannelListProps {
  serverName: string;
  channels: Channel[];
  activeChannelId: string | null;
  onChannelSelect: (channelId: string) => void;
  onCreateChannel: () => void;
  onDeleteChannel: (channelId: string) => void;
  onManageRoles: () => void;
  onServerSettings?: () => void;
  onLeaveServer?: () => void;
  owner_id?: string;
  currentUser?: User;
  onLogout?: () => void;
  onOpenSettings?: () => void;
  onVoiceChannelClick?: () => void;
  channelUnreadCounts?: Record<string, number>;
  canManageChannels?: boolean;
  onRenameChannel?: (channelId: string, newName: string) => void;
  onMarkChannelRead?: (channelId: string) => void;
}

const ChannelList: React.FC<ChannelListProps> = ({
  serverName,
  channels,
  activeChannelId,
  onChannelSelect,
  onCreateChannel,
  onDeleteChannel,
  onManageRoles,
  onServerSettings,
  onLeaveServer,
  owner_id,
  currentUser,
  onLogout,
  onOpenSettings,
  onVoiceChannelClick,
  channelUnreadCounts = {},
  canManageChannels = false,
  onRenameChannel,
  onMarkChannelRead,
}) => {
  const [textChannelsCollapsed, setTextChannelsCollapsed] = useState(false);
  const [voiceChannelsCollapsed, setVoiceChannelsCollapsed] = useState(false);
  const [hoveredChannel, setHoveredChannel] = useState<string | null>(null);
  const [userContextMenu, setUserContextMenu] = useState<{ top: number; left: number } | null>(null);
  const [serverContextMenu, setServerContextMenu] = useState<{ top: number; left: number } | null>(null);
  const [channelContextMenu, setChannelContextMenu] = useState<{ channel: Channel; top: number; left: number } | null>(null);
  const [renamingChannelId, setRenamingChannelId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState('');
  const renameInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (renamingChannelId) renameInputRef.current?.focus();
  }, [renamingChannelId]);

  const startRename = (channel: Channel) => {
    setRenameValue(channel.name);
    setRenamingChannelId(channel.id);
  };

  const commitRename = (channelId: string) => {
    if (renameValue.trim()) onRenameChannel?.(channelId, renameValue.trim());
    setRenamingChannelId(null);
  };

  const handleUserAreaClick = (e: React.MouseEvent) => {
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    setUserContextMenu({ top: rect.top, left: rect.left });
  };

  const handleServerHeaderClick = (e: React.MouseEvent) => {
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    setServerContextMenu({ top: rect.top, left: rect.left });
  };

  const textChannels = channels.filter(ch => ch.type === 0);
  const voiceChannels = channels.filter(ch => ch.type === 1 || ch.type === 2);

  return (
    <div className="channel-list">
      <div
        className="server-header clickable"
        onClick={handleServerHeaderClick}
      >
        <span className="server-name">{serverName}</span>
        <div className="server-header-actions">
          {onServerSettings && (
            <button
              className="server-settings-btn"
              onClick={(e) => { e.stopPropagation(); onServerSettings(); }}
              title="Server Settings"
            >
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
        <div className="category-row">
          <div
            className={`category-header ${textChannelsCollapsed ? 'collapsed' : ''}`}
            onClick={() => setTextChannelsCollapsed(!textChannelsCollapsed)}
          >
            <svg viewBox="0 0 24 24" fill="currentColor">
              <path d="M7 10l5 5 5-5z" />
            </svg>
            Text Channels
          </div>
          {canManageChannels && (
            <button className="add-channel-btn" onClick={onCreateChannel} title="Create channel">
              +
            </button>
          )}
        </div>

        {!textChannelsCollapsed && textChannels.map(channel => {
          const unread = channelUnreadCounts[channel.id] ?? 0;
          const isActive = channel.id === activeChannelId;
          const isRenaming = renamingChannelId === channel.id;
          return (
            <div
              key={channel.id}
              className={`channel-item ${isActive ? 'active' : ''} ${unread > 0 && !isActive ? 'unread' : ''}`}
              onClick={() => !isRenaming && onChannelSelect(channel.id)}
              onContextMenu={e => { e.preventDefault(); setChannelContextMenu({ channel, top: e.clientY, left: e.clientX }); }}
              onMouseEnter={() => setHoveredChannel(channel.id)}
              onMouseLeave={() => setHoveredChannel(null)}
            >
              <span className="channel-icon">#</span>
              {isRenaming ? (
                <input
                  ref={renameInputRef}
                  className="channel-rename-input"
                  value={renameValue}
                  onChange={e => setRenameValue(e.target.value)}
                  onBlur={() => commitRename(channel.id)}
                  onKeyDown={e => {
                    if (e.key === 'Enter') { e.preventDefault(); commitRename(channel.id); }
                    if (e.key === 'Escape') setRenamingChannelId(null);
                  }}
                  onClick={e => e.stopPropagation()}
                />
              ) : (
                <span className="channel-name">{channel.name}</span>
              )}
              {unread > 0 && !isActive && !isRenaming && (
                <span className="channel-unread-badge">{unread > 99 ? '99+' : unread}</span>
              )}
              {canManageChannels && hoveredChannel === channel.id && !isRenaming && (
                <button
                  className="delete-channel-btn"
                  onClick={e => { e.stopPropagation(); onDeleteChannel(channel.id); }}
                  title="Delete channel"
                >
                  ×
                </button>
              )}
            </div>
          );
        })}

        <div className="category-row">
          <div
            className={`category-header ${voiceChannelsCollapsed ? 'collapsed' : ''}`}
            onClick={() => setVoiceChannelsCollapsed(!voiceChannelsCollapsed)}
          >
            <svg viewBox="0 0 24 24" fill="currentColor">
              <path d="M7 10l5 5 5-5z" />
            </svg>
            Voice Channels
          </div>
        </div>

        {!voiceChannelsCollapsed && voiceChannels.map(channel => (
          <div
            key={channel.id}
            className={`voice-channel-item ${channel.id === activeChannelId ? 'active' : ''}`}
            onClick={() => onVoiceChannelClick?.()}
            onContextMenu={e => { e.preventDefault(); setChannelContextMenu({ channel, top: e.clientY, left: e.clientX }); }}
            onMouseEnter={() => setHoveredChannel(channel.id)}
            onMouseLeave={() => setHoveredChannel(null)}
          >
            <span className="voice-icon">🔊</span>
            <span className="channel-name">{channel.name}</span>
            {canManageChannels && hoveredChannel === channel.id && (
              <button
                className="delete-channel-btn"
                onClick={e => { e.stopPropagation(); onDeleteChannel(channel.id); }}
                title="Delete channel"
              >
                ×
              </button>
            )}
          </div>
        ))}

        {(owner_id === currentUser?.id) && (
          <div className="channel-item manage-roles-item" onClick={onManageRoles}>
            <span className="channel-icon">⚙</span>
            <span className="channel-name">Manage Roles</span>
          </div>
        )}
      </div>

      <div
        className="user-area clickable"
        onClick={handleUserAreaClick}
        title="Click for user settings"
      >
        <div className="user-info">
          <div className="user-avatar">
            <span className="user-avatar-placeholder">
              {currentUser?.username?.charAt(0).toUpperCase() || 'U'}
            </span>
          </div>
          <div className="user-details">
            <div className="username">{currentUser?.username || 'User'}</div>
            <div className="user-status">Online</div>
          </div>
          <div className="cl-settings-icon" title="User settings">
            <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18">
              <path d="M19.14,12.94c0.04-0.3,0.06-0.61,0.06-0.94c0-0.32-0.02-0.64-0.07-0.94l2.03-1.58c0.18-0.14,0.23-0.41,0.12-0.61 l-1.92-3.32c-0.12-0.22-0.37-0.29-0.59-0.22l-2.39,0.96c-0.5-0.38-1.03-0.7-1.62-0.94L14.4,2.81c-0.04-0.24-0.24-0.41-0.48-0.41 h-3.84c-0.24,0-0.43,0.17-0.47,0.41L9.25,5.35C8.66,5.59,8.12,5.92,7.63,6.29L5.24,5.33c-0.22-0.08-0.47,0-0.59,0.22L2.74,8.87 C2.62,9.08,2.66,9.34,2.86,9.48l2.03,1.58C4.84,11.36,4.8,11.69,4.8,12s0.02,0.64,0.07,0.94l-2.03,1.58 c-0.18,0.14-0.23,0.41-0.12,0.61l1.92,3.32c0.12,0.22,0.37,0.29,0.59,0.22l2.39-0.96c0.5,0.38,1.03,0.7,1.62,0.94l0.36,2.54 c0.05,0.24,0.24,0.41,0.48,0.41h3.84c0.24,0,0.44-0.17,0.47-0.41l0.36-2.54c0.59-0.24,1.13-0.56,1.62-0.94l2.39,0.96 c0.22,0.08,0.47,0,0.59-0.22l1.92-3.32c0.12-0.22,0.07-0.47-0.12-0.61L19.14,12.94z M12,15.6c-1.98,0-3.6-1.62-3.6-3.6 s1.62-3.6,3.6-3.6s3.6,1.62,3.6,3.6S13.98,15.6,12,15.6z"/>
            </svg>
          </div>
        </div>
      </div>

      {userContextMenu && (
        <UserContextMenu
          position={userContextMenu}
          onClose={() => setUserContextMenu(null)}
          onSettings={() => onOpenSettings?.()}
          onLogout={() => onLogout?.()}
        />
      )}

      {serverContextMenu && (
        <ServerContextMenu
          position={serverContextMenu}
          serverName={serverName}
          isOwner={owner_id === currentUser?.id}
          onClose={() => setServerContextMenu(null)}
          onLeave={onLeaveServer}
          onSettings={onServerSettings}
        />
      )}

      {channelContextMenu && (
        <ChannelContextMenu
          channel={channelContextMenu.channel}
          position={{ top: channelContextMenu.top, left: channelContextMenu.left }}
          canManageChannels={canManageChannels}
          onClose={() => setChannelContextMenu(null)}
          onRename={() => startRename(channelContextMenu.channel)}
          onDelete={() => onDeleteChannel(channelContextMenu.channel.id)}
          onMarkAsRead={() => onMarkChannelRead?.(channelContextMenu.channel.id)}
        />
      )}
    </div>
  );
};

export default ChannelList;
