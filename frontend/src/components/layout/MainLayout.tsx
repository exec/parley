import React from 'react';
import Sidebar from './Sidebar';
import ChannelList from './ChannelList';
import UserSidebar from './UserSidebar';
import './MainLayout.css';

interface Server {
  id: string;
  name: string;
  icon?: string;
}

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

interface ServerMember {
  id: string;
  serverId: string;
  userId: string;
  role: 'owner' | 'admin' | 'member';
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
  serverName: string;
  members: ServerMember[];
  currentUser?: User;
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
  serverName,
  members,
  currentUser,
}) => {
  const showChannelList = activeServerId && serverName;
  const showUserSidebar = activeServerId && members.length > 0;

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
            currentUser={currentUser}
          />

          <div className="main-content">
            <div className="content-area">
              {children}
            </div>
          </div>

          {showUserSidebar && <UserSidebar members={members} />}
        </>
      ) : (
        <div className="main-content">
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
              Select a server from the left to start chatting with your team
            </p>
          </div>
        </div>
      )}
    </div>
  );
};

export default MainLayout;