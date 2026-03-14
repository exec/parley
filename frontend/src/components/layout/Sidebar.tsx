import React from 'react';
import './Sidebar.css';

interface Server {
  id: string;
  name: string;
  icon?: string;
}

interface SidebarProps {
  servers: Server[];
  activeServerId: string | null;
  onServerSelect: (serverId: string) => void;
  onCreateServer: () => void;
}

const Sidebar: React.FC<SidebarProps> = ({
  servers,
  activeServerId,
  onServerSelect,
  onCreateServer,
}) => {
  const renderServerIcon = (server: Server) => {
    const isActive = server.id === activeServerId;

    return (
      <div
        key={server.id}
        className={`server-icon-container ${isActive ? 'active' : ''}`}
        onClick={() => onServerSelect(server.id)}
      >
        {server.icon ? (
          <img src={server.icon} alt={server.name} className="server-icon" />
        ) : (
          <span className="server-icon-placeholder">
            {server.name.charAt(0).toUpperCase()}
          </span>
        )}
        <span className="tooltip">{server.name}</span>
      </div>
    );
  };

  return (
    <div className="sidebar">
      <div
        className={`home-button ${activeServerId === null ? 'active' : ''}`}
        onClick={() => onServerSelect('')}
      >
        <svg
          viewBox="0 0 24 24"
          fill="currentColor"
          xmlns="http://www.w3.org/2000/svg"
        >
          <path d="M10 20v-6h4v6h5v-8h3L12 3 2 12h3v8z" />
        </svg>
        <span className="tooltip">Home</span>
      </div>

      <div className="divider" />

      <div className="server-list">
        {servers.map((server) => renderServerIcon(server))}
      </div>

      <div className="divider" />

      <div className="add-server-button" onClick={onCreateServer}>
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          xmlns="http://www.w3.org/2000/svg"
        >
          <path d="M12 5v14M5 12h14" />
        </svg>
        <span className="tooltip">Create Server</span>
      </div>
    </div>
  );
};

export default Sidebar;