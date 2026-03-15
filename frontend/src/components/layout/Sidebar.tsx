import React, { useState, useRef, useEffect } from 'react';
import './Sidebar.css';

interface Server {
  id: string;
  name: string;
  icon_url?: string;
  owner_id?: string;
}

interface ContextMenu {
  server: Server;
  x: number;
  y: number;
}

interface SidebarProps {
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

const Sidebar: React.FC<SidebarProps> = ({
  servers,
  activeServerId,
  currentUserId,
  onServerSelect,
  onCreateServer,
  onHomepage,
  serverUnreadCounts = {},
  onMarkServerRead,
  onNotificationSettings,
  onServerSettings,
  onLeaveServer,
}) => {
  const [contextMenu, setContextMenu] = useState<ContextMenu | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!contextMenu) return;
    const close = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setContextMenu(null);
      }
    };
    document.addEventListener('mousedown', close);
    return () => document.removeEventListener('mousedown', close);
  }, [contextMenu]);

  const handleContextMenu = (e: React.MouseEvent, server: Server) => {
    e.preventDefault();
    setContextMenu({ server, x: e.clientX, y: e.clientY });
  };

  const close = () => setContextMenu(null);

  const isOwner = contextMenu ? contextMenu.server.owner_id === currentUserId : false;

  return (
    <div className="sidebar">
      <div
        className={`home-button ${activeServerId === null ? 'active' : ''}`}
        onClick={() => onHomepage?.()}
      >
        <svg viewBox="0 0 24 24" fill="currentColor" xmlns="http://www.w3.org/2000/svg">
          <path d="M10 20v-6h4v6h5v-8h3L12 3 2 12h3v8z" />
        </svg>
        <span className="tooltip">Home</span>
      </div>

      <div className="divider" />

      <div className="server-list">
        {servers.map(server => {
          const isActive = server.id === activeServerId;
          const unread = serverUnreadCounts[server.id] ?? 0;
          return (
            <div
              key={server.id}
              className={`server-icon-container ${isActive ? 'active' : ''}`}
              onClick={() => onServerSelect(server.id)}
              onContextMenu={e => handleContextMenu(e, server)}
            >
              {server.icon_url ? (
                <img src={server.icon_url} alt={server.name} className="server-icon" />
              ) : (
                <span className="server-icon-placeholder">
                  {server.name.charAt(0).toUpperCase()}
                </span>
              )}
              <span className="tooltip">{server.name}</span>
              {unread > 0 && !isActive && (
                <span className="server-unread-badge">{unread > 99 ? '99+' : unread}</span>
              )}
            </div>
          );
        })}
      </div>

      <div className="divider" />

      <div className="add-server-button" onClick={onCreateServer}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" xmlns="http://www.w3.org/2000/svg">
          <path d="M12 5v14M5 12h14" />
        </svg>
        <span className="tooltip">Create Server</span>
      </div>

      {contextMenu && (
        <div
          ref={menuRef}
          className="sidebar-context-menu"
          style={{ top: contextMenu.y, left: contextMenu.x }}
          onClick={e => e.stopPropagation()}
        >
          <div className="sidebar-ctx-header">{contextMenu.server.name}</div>
          <div className="sidebar-ctx-divider" />

          <button className="sidebar-ctx-item" onClick={() => {
            onMarkServerRead?.(contextMenu.server.id);
            close();
          }}>
            Mark as Read
          </button>

          <button className="sidebar-ctx-item" onClick={() => {
            onNotificationSettings?.(contextMenu.server.id);
            close();
          }}>
            Notification Settings
          </button>

          <button className="sidebar-ctx-item" onClick={() => {
            navigator.clipboard.writeText(contextMenu.server.id);
            close();
          }}>
            Copy Server ID
          </button>

          <div className="sidebar-ctx-divider" />

          {isOwner ? (
            <button className="sidebar-ctx-item" onClick={() => {
              onServerSettings?.(contextMenu.server.id);
              close();
            }}>
              Server Settings
            </button>
          ) : (
            <button className="sidebar-ctx-item danger" onClick={() => {
              onLeaveServer?.(contextMenu.server.id);
              close();
            }}>
              Leave Server
            </button>
          )}
        </div>
      )}
    </div>
  );
};

export default Sidebar;
