import React from 'react';
import { DmChannel, User } from '../../api/types';
import './DmPanel.css';

interface DmPanelProps {
  dmChannels: DmChannel[];
  activeDmChannelId: string | null;
  currentUser?: User | null;
  onSelectDm: (channelId: string) => void;
  onLogout?: () => void;
}

const DmPanel: React.FC<DmPanelProps> = ({
  dmChannels,
  activeDmChannelId,
  currentUser,
  onSelectDm,
  onLogout,
}) => {
  return (
    <div className="dm-panel">
      <div className="dm-panel-header">
        <span className="dm-panel-title">Direct Messages</span>
      </div>

      <div className="dm-panel-list">
        {dmChannels.length === 0 ? (
          <div className="dm-panel-empty">No direct messages yet</div>
        ) : (
          dmChannels.map(channel => (
            <div
              key={channel.id}
              className={`dm-panel-item ${channel.id === activeDmChannelId ? 'active' : ''}`}
              onClick={() => onSelectDm(channel.id)}
            >
              <div className="dm-panel-avatar">
                {channel.other_username.charAt(0).toUpperCase()}
              </div>
              <span className="dm-panel-name">{channel.other_username}</span>
            </div>
          ))
        )}
      </div>

      <div className="dm-panel-user-area">
        <div className="dm-panel-user-info">
          <div className="dm-panel-user-avatar">
            {currentUser?.username?.charAt(0).toUpperCase() || '?'}
          </div>
          <div className="dm-panel-user-details">
            <div className="dm-panel-username">{currentUser?.username || 'User'}</div>
            <div className="dm-panel-status">Online</div>
          </div>
          {onLogout && (
            <button className="dm-panel-logout" onClick={onLogout} title="Logout">
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

export default DmPanel;
