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
  servers: Server[];
  activeServerId: string | null;
  currentUserId?: string;
  onServerSelect: (serverId: string) => void;
  onCreateServer: () => void;
  onHomepage?: () => void;
  serverUnreadCounts?: Record<string, number>;
  onMarkServerRead?: (serverId: string) => void;
  onNotificationSettings?: (serverId: string) => void;
  onServerSettings?: (serverId: string) => void;
  onLeaveServer?: (serverId: string) => void;
}

const MainLayout: React.FC<MainLayoutProps> = ({
  children,
  leftPanel,
  rightPanel,
  servers,
  activeServerId,
  currentUserId,
  onServerSelect,
  onCreateServer,
  onHomepage,
  serverUnreadCounts,
  onMarkServerRead,
  onNotificationSettings,
  onServerSettings,
  onLeaveServer,
}) => {
  return (
    <div className="main-layout">
      <Sidebar
        servers={servers}
        activeServerId={activeServerId}
        currentUserId={currentUserId}
        onServerSelect={onServerSelect}
        onCreateServer={onCreateServer}
        onHomepage={onHomepage}
        serverUnreadCounts={serverUnreadCounts}
        onMarkServerRead={onMarkServerRead}
        onNotificationSettings={onNotificationSettings}
        onServerSettings={onServerSettings}
        onLeaveServer={onLeaveServer}
      />
      {leftPanel}
      <div className="main-content">
        {children}
      </div>
      {rightPanel}
    </div>
  );
};

export default MainLayout;
