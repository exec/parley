import React, { useState } from 'react';
import './ChannelList.css';

interface Channel {
  id: string;
  name: string;
  type: number;
}

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
  currentUser?: User;
  onLogout?: () => void;
  onVoiceChannelClick?: () => void;
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
  currentUser,
  onLogout,
  onVoiceChannelClick,
}) => {
  const [textChannelsCollapsed, setTextChannelsCollapsed] = useState(false);
  const [voiceChannelsCollapsed, setVoiceChannelsCollapsed] = useState(false);
  const [hoveredChannel, setHoveredChannel] = useState<string | null>(null);

  const textChannels = channels.filter(ch => ch.type === 0);
  const voiceChannels = channels.filter(ch => ch.type === 1 || ch.type === 2);

  return (
    <div className="channel-list">
      <div className="server-header">
        <span className="server-name">{serverName}</span>
        {onServerSettings && (
          <button
            className="server-settings-btn"
            onClick={onServerSettings}
            title="Server Settings"
          >
            <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20">
              <path d="M19.14 12.94c.04-.31.06-.63.06-.94 0-.31-.02-.63-.06-.94l2.03-1.58c.18-.14.23-.41.12-.61l-1.92-3.32c-.12-.22-.37-.29-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54c-.04-.24-.24-.41-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.04.31-.06.63-.06.94s.02.63.06.94l-2.03 1.58c-.18.14-.23.41-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z" />
            </svg>
          </button>
        )}
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
          <button className="add-channel-btn" onClick={onCreateChannel} title="Create channel">
            +
          </button>
        </div>

        {!textChannelsCollapsed && textChannels.map(channel => (
          <div
            key={channel.id}
            className={`channel-item ${channel.id === activeChannelId ? 'active' : ''}`}
            onClick={() => onChannelSelect(channel.id)}
            onMouseEnter={() => setHoveredChannel(channel.id)}
            onMouseLeave={() => setHoveredChannel(null)}
          >
            <span className="channel-icon">#</span>
            <span className="channel-name">{channel.name}</span>
            {hoveredChannel === channel.id && (
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
            onMouseEnter={() => setHoveredChannel(channel.id)}
            onMouseLeave={() => setHoveredChannel(null)}
          >
            <span className="voice-icon">🔊</span>
            <span className="channel-name">{channel.name}</span>
            {hoveredChannel === channel.id && (
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

        <div className="channel-item manage-roles-item" onClick={onManageRoles}>
          <span className="channel-icon">⚙</span>
          <span className="channel-name">Manage Roles</span>
        </div>
      </div>

      <div className="user-area">
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
          {onLogout && (
            <button className="logout-btn" onClick={onLogout} title="Logout">
              <svg viewBox="0 0 24 24" fill="currentColor">
                <path d="M17 7l-1.41 1.41L18.17 11H8v2h10.17l-2.58 2.58L17 17l5-5zM4 5h8V3H4c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h8v-2H4V5z" />
              </svg>
            </button>
          )}
        </div>
      </div>
    </div>
  );
};

export default ChannelList;
