import React from 'react';
import Sidebar from './Sidebar';
import './MainLayout.css';

interface Server {
  id: string;
  name: string;
  icon_url?: string;
}

interface MainLayoutProps {
  children: React.ReactNode;
  leftPanel: React.ReactNode;
  rightPanel?: React.ReactNode;
  servers: Server[];
  activeServerId: string | null;
  onServerSelect: (serverId: string) => void;
  onCreateServer: () => void;
  onHomepage?: () => void;
  serverUnreadCounts?: Record<string, number>;
}

const MainLayout: React.FC<MainLayoutProps> = ({
  children,
  leftPanel,
  rightPanel,
  servers,
  activeServerId,
  onServerSelect,
  onCreateServer,
  onHomepage,
  serverUnreadCounts,
}) => {
  return (
    <div className="main-layout">
      <Sidebar
        servers={servers}
        activeServerId={activeServerId}
        onServerSelect={onServerSelect}
        onCreateServer={onCreateServer}
        onHomepage={onHomepage}
        serverUnreadCounts={serverUnreadCounts}
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
