import React, { useState } from 'react';
import { X } from 'lucide-react';
import { ChannelPermissions } from '../settings/ChannelPermissions';
import './ChannelSettingsModal.css';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  channelId: string;
  channelName: string;
  serverId: string;
  parentId?: string;
}

type Tab = 'permissions';

export const ChannelSettingsModal: React.FC<Props> = ({
  isOpen,
  onClose,
  channelId,
  channelName,
  serverId,
  parentId,
}) => {
  const [activeTab] = useState<Tab>('permissions');

  if (!isOpen) return null;

  return (
    <div className="csm-overlay" onClick={e => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="csm-modal">
        {/* Sidebar */}
        <div className="csm-sidebar">
          <div className="csm-sidebar-group-label">#{channelName}</div>
          <button className={`csm-nav-item${activeTab === 'permissions' ? ' active' : ''}`}>
            Permissions
          </button>
          <div className="csm-nav-divider" />
          <button className="csm-nav-item" onClick={onClose}>Close</button>
        </div>

        {/* Content */}
        <div className="csm-content">
          <h2 className="csm-title">Channel Permissions</h2>
          <p className="csm-subtitle">
            Permission overwrites let you customize permissions for specific roles or members in this channel,
            overriding the server defaults.
          </p>
          <ChannelPermissions
            channelId={channelId}
            serverId={serverId}
            parentId={parentId}
          />
        </div>

        {/* Close button */}
        <div className="csm-close-wrap">
          <button className="csm-close-btn" onClick={onClose} title="Close (ESC)"><X size={16} color="currentColor" /></button>
          <span className="csm-close-hint">ESC</span>
        </div>
      </div>
    </div>
  );
};
