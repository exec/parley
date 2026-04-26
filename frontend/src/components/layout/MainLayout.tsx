import React from 'react';
import Sidebar from './Sidebar';
import './MainLayout.css';

interface Server {
  id: string;
  name: string;
  icon_url?: string;
  owner_id?: string;
}

interface MainLayoutProps {
  children: React.ReactNode;
  leftPanel: React.ReactNode;
  rightPanel?: React.ReactNode;
  leftPanelOpen?: boolean;
  servers: Server[];
  activeServerId: string | null;
  currentUserId?: string;
  onServerSelect: (serverId: string) => void;
  onCreateServer: () => void;
  onHomepage?: () => void;
  onDiscovery?: () => void;
  discoveryActive?: boolean;
  serverUnreadCounts?: Record<string, number>;
  onMarkServerRead?: (serverId: string) => void;
  onNotificationSettings?: (serverId: string) => void;
  onServerSettings?: (serverId: string) => void;
  onLeaveServer?: (serverId: string) => void;
  unreadNotificationCount?: number;
  notifPanelOpen?: boolean;
  onToggleNotifPanel?: () => void;
  onReorderServers?: (serverIds: string[]) => void;
}

const MainLayout: React.FC<MainLayoutProps> = ({
  children,
  leftPanel,
  rightPanel,
  leftPanelOpen = false,
  servers,
  activeServerId,
  currentUserId,
  onServerSelect,
  onCreateServer,
  onHomepage,
  onDiscovery,
  discoveryActive,
  serverUnreadCounts,
  onMarkServerRead,
  onNotificationSettings,
  onServerSettings,
  onLeaveServer,
  unreadNotificationCount,
  notifPanelOpen,
  onToggleNotifPanel,
  onReorderServers,
}) => {
  return (
    <div className="main-layout">
      <div className={`left-drawer${leftPanelOpen ? ' left-drawer--open' : ''}`}>
        <Sidebar
          servers={servers}
          activeServerId={activeServerId}
          currentUserId={currentUserId}
          onServerSelect={onServerSelect}
          onCreateServer={onCreateServer}
          onHomepage={onHomepage}
          onDiscovery={onDiscovery}
          discoveryActive={discoveryActive}
          serverUnreadCounts={serverUnreadCounts}
          onMarkServerRead={onMarkServerRead}
          onNotificationSettings={onNotificationSettings}
          onServerSettings={onServerSettings}
          onLeaveServer={onLeaveServer}
          unreadNotificationCount={unreadNotificationCount}
          notifPanelOpen={notifPanelOpen}
          onToggleNotifPanel={onToggleNotifPanel}
          onReorderServers={onReorderServers}
        />
        {leftPanel}
      </div>
      <div className="main-content">
        {children}
      </div>
      {rightPanel}
    </div>
  );
};

export default MainLayout;
