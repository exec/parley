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
  currentUser?: User;
  onLogout?: () => void;
}

const ChannelList: React.FC<ChannelListProps> = ({
  serverName,
  channels,
  activeChannelId,
  onChannelSelect,
  onCreateChannel,
  onDeleteChannel,
  onManageRoles,
  currentUser,
  onLogout,
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
