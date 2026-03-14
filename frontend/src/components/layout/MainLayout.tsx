import React from 'react';
import Sidebar from './Sidebar';
import ChannelList from './ChannelList';
import UserSidebar from './UserSidebar';
import './MainLayout.css';

interface Server {
  id: string;
  name: string;
  icon_url?: string;
}

interface Channel {
  id: string;
  name: string;
  type: number;
}

interface User {
  id: string;
  username: string;
}

interface ServerMember {
  id: string;
  server_id: string;
  user_id: string;
  nickname?: string;
  user?: User;
}

interface MainLayoutProps {
  children: React.ReactNode;
  servers: Server[];
  activeServerId: string | null;
  onServerSelect: (serverId: string) => void;
  onCreateServer: () => void;
  channels: Channel[];
  activeChannelId: string | null;
  onChannelSelect: (channelId: string) => void;
  onCreateChannel: () => void;
  onDeleteChannel: (channelId: string) => void;
  onManageRoles: () => void;
  serverName: string;
  members: ServerMember[];
  currentUser?: User;
  ownerId?: string;
  onLogout?: () => void;
}

const MainLayout: React.FC<MainLayoutProps> = ({
  children,
  servers,
  activeServerId,
  onServerSelect,
  onCreateServer,
  channels,
  activeChannelId,
  onChannelSelect,
  onCreateChannel,
  onDeleteChannel,
  onManageRoles,
  serverName,
  members,
  currentUser,
  ownerId,
  onLogout,
}) => {
  const showChannelList = !!activeServerId;

  return (
    <div className="main-layout">
      <Sidebar
        servers={servers}
        activeServerId={activeServerId}
        onServerSelect={onServerSelect}
        onCreateServer={onCreateServer}
      />

      {showChannelList ? (
        <>
          <ChannelList
            serverName={serverName}
            channels={channels}
            activeChannelId={activeChannelId}
            onChannelSelect={onChannelSelect}
            onCreateChannel={onCreateChannel}
            onDeleteChannel={onDeleteChannel}
            onManageRoles={onManageRoles}
            currentUser={currentUser}
            onLogout={onLogout}
          />

          <div className="main-content">
            <div className="content-area">
              {children}
            </div>
          </div>

          <UserSidebar members={members} ownerId={ownerId} />
        </>
      ) : (
        <div className="main-content no-server">
          <ChannelList
            serverName=""
            channels={[]}
            activeChannelId={null}
            onChannelSelect={() => {}}
            onCreateChannel={() => {}}
            onDeleteChannel={() => {}}
            onManageRoles={() => {}}
            currentUser={currentUser}
            onLogout={onLogout}
          />
          <div className="welcome-content">
            <div className="welcome-screen">
              <svg
                className="welcome-icon"
                viewBox="0 0 24 24"
                fill="currentColor"
                xmlns="http://www.w3.org/2000/svg"
              >
                <path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H6l-2 2V4h16v12z" />
                <path d="M7 9h10v2H7zm0-3h10v2H7z" />
              </svg>
              <h1 className="welcome-title">Welcome to Parley</h1>
              <p className="welcome-subtitle">
                Select a server from the left or create a new one to get started
              </p>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default MainLayout;
