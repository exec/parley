import React, { useState } from 'react';
import './ChannelList.css';

interface Channel {
  id: string;
  name: string;
  type: number;
  position: number;
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
  currentUser?: User;
}

const ChannelList: React.FC<ChannelListProps> = ({
  serverName,
  channels,
  activeChannelId,
  onChannelSelect,
  currentUser,
}) => {
  const [textChannelsCollapsed, setTextChannelsCollapsed] = useState(false);
  const [voiceChannelsCollapsed, setVoiceChannelsCollapsed] = useState(false);

  const textChannels = channels.filter((ch) => ch.type === 0);
  const voiceChannels = channels.filter((ch) => ch.type === 2);

  return (
    <div className="channel-list">
      <div className="server-header">
        <span className="server-name">{serverName}</span>
        <div className="server-header-actions">
          <svg
            viewBox="0 0 24 24"
            fill="currentColor"
            xmlns="http://www.w3.org/2000/svg"
          >
            <path d="M7 10l5 5 5-5z" />
          </svg>
        </div>
      </div>

      <div className="channels-container">
        <div
          className={`category-header ${textChannelsCollapsed ? 'collapsed' : ''}`}
          onClick={() => setTextChannelsCollapsed(!textChannelsCollapsed)}
        >
          <svg
            viewBox="0 0 24 24"
            fill="currentColor"
            xmlns="http://www.w3.org/2000/svg"
          >
            <path d="M7 10l5 5 5-5z" />
          </svg>
          Text Channels
        </div>

        {!textChannelsCollapsed &&
          textChannels.map((channel) => (
            <div
              key={channel.id}
              className={`channel-item ${
                channel.id === activeChannelId ? 'active' : ''
              }`}
              onClick={() => onChannelSelect(channel.id)}
            >
              <span className="channel-icon">
                <svg
                  viewBox="0 0 24 24"
                  fill="currentColor"
                  xmlns="http://www.w3.org/2000/svg"
                >
                  <path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2z" />
                </svg>
              </span>
              <span className="channel-name">{channel.name}</span>
            </div>
          ))}

        <div
          className={`category-header ${voiceChannelsCollapsed ? 'collapsed' : ''}`}
          onClick={() => setVoiceChannelsCollapsed(!voiceChannelsCollapsed)}
        >
          <svg
            viewBox="0 0 24 24"
            fill="currentColor"
            xmlns="http://www.w3.org/2000/svg"
          >
            <path d="M7 10l5 5 5-5z" />
          </svg>
          Voice Channels
        </div>

        {!voiceChannelsCollapsed &&
          voiceChannels.map((channel) => (
            <div
              key={channel.id}
              className={`voice-channel-item ${
                channel.id === activeChannelId ? 'active' : ''
              }`}
              onClick={() => onChannelSelect(channel.id)}
            >
              <span className="voice-icon">
                <svg
                  viewBox="0 0 24 24"
                  fill="currentColor"
                  xmlns="http://www.w3.org/2000/svg"
                >
                  <path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3zm5.91-3c-.49 0-.9.36-.98.85C16.52 14.2 14.47 16 12 16s-4.52-1.8-4.93-4.15c-.08-.49-.49-.85-.98-.85-.61 0-1.09.54-1 1.14.49 3 2.89 5.35 5.91 5.78V20c0 .55.45 1 1 1s1-.45 1-1v-2.08c3.02-.43 5.42-2.78 5.91-5.78.1-.6-.39-1.14-1-1.14z" />
                </svg>
              </span>
              <span className="channel-name">{channel.name}</span>
            </div>
          ))}
      </div>

      <div className="user-area">
        <div className="user-info">
          <div className="user-avatar">
            {currentUser?.avatar ? (
              <img src={currentUser.avatar} alt={currentUser.username} />
            ) : (
              <span className="user-avatar-placeholder">
                {currentUser?.username?.charAt(0).toUpperCase() || 'U'}
              </span>
            )}
          </div>
          <div className="user-details">
            <div className="username">{currentUser?.username || 'User'}</div>
            <div className="user-status">Online</div>
          </div>
          <div className="user-actions">
            <button>
              <svg
                viewBox="0 0 24 24"
                fill="currentColor"
                xmlns="http://www.w3.org/2000/svg"
              >
                <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8z" />
                <circle cx="12" cy="12" r="5" />
              </svg>
            </button>
            <button>
              <svg
                viewBox="0 0 24 24"
                fill="currentColor"
                xmlns="http://www.w3.org/2000/svg"
              >
                <path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2z" />
              </svg>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default ChannelList;