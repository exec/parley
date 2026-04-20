import React, { useState, useRef, useEffect } from 'react';
import { createPortal } from 'react-dom';
import { copyToClipboard } from '../../lib/tauri';
import './Sidebar.css';
import '../notifications/Notifications.css';

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
}

const Sidebar: React.FC<SidebarProps> = ({
  servers,
  activeServerId,
  currentUserId,
  onServerSelect,
  onCreateServer,
  onHomepage,
  onDiscovery,
  discoveryActive = false,
  serverUnreadCounts = {},
  onMarkServerRead,
  onNotificationSettings,
  onServerSettings,
  onLeaveServer,
  unreadNotificationCount = 0,
  notifPanelOpen = false,
  onToggleNotifPanel,
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
        className={`home-button ${activeServerId === null && !discoveryActive ? 'active' : ''}`}
        onClick={() => onHomepage?.()}
      >
        <svg viewBox="0 0 24 24" fill="currentColor" xmlns="http://www.w3.org/2000/svg">
          <path d="M10 20v-6h4v6h5v-8h3L12 3 2 12h3v8z" />
        </svg>
        <span className="tooltip">Home</span>
      </div>

      <div
        className={`home-button ${discoveryActive ? 'active' : ''}`}
        onClick={() => onDiscovery?.()}
      >
        <svg viewBox="0 0 24 24" fill="currentColor" xmlns="http://www.w3.org/2000/svg">
          <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/>
        </svg>
        <span className="tooltip">Discover</span>
      </div>

      <div className="divider" />

      <div className="sidebar-scroll">
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
      </div>{/* end sidebar-scroll */}

      <div className="sidebar-bottom">
      <div className="divider" />

      <div className="add-server-button" onClick={onCreateServer}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" xmlns="http://www.w3.org/2000/svg">
          <path d="M12 5v14M5 12h14" />
        </svg>
        <span className="tooltip">Create Server</span>
      </div>

      <button
        className={`notif-bell-button${notifPanelOpen ? ' active' : ''}`}
        onClick={onToggleNotifPanel}
        aria-label="Notifications"
      >
        <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20">
          <path d="M12 22c1.1 0 2-.9 2-2h-4c0 1.1.9 2 2 2zm6-6v-5c0-3.07-1.64-5.64-4.5-6.32V4c0-.83-.67-1.5-1.5-1.5s-1.5.67-1.5 1.5v.68C7.63 5.36 6 7.92 6 11v5l-2 2v1h16v-1l-2-2z"/>
        </svg>
        {unreadNotificationCount > 0 && (
          <span className="notif-bell-badge">
            {unreadNotificationCount > 99 ? '99+' : unreadNotificationCount}
          </span>
        )}
        <span className="tooltip">Notifications</span>
      </button>
      </div>{/* end sidebar-bottom */}

      {contextMenu && createPortal(
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
            void copyToClipboard(contextMenu.server.id);
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
        </div>,
        document.body
      )}
    </div>
  );
};

export default Sidebar;
