import React, { useEffect, useRef } from 'react';
import { AppNotification } from '../../api/types';
import './Notifications.css';

interface NotificationPanelProps {
  notifications: AppNotification[];
  onMarkAllRead: () => void;
  onMarkRead: (id: string) => void;
  onNavigate?: (notif: AppNotification) => void;
  onClose: () => void;
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  return `${days}d ago`;
}

function notifIcon(type: AppNotification['type']): string {
  switch (type) {
    case 'mention': return '@';
    case 'dm': return '✉';
    case 'friend_request': return '👤';
    case 'friend_accept': return '✓';
    default: return '•';
  }
}

const NotificationPanel: React.FC<NotificationPanelProps> = ({
  notifications,
  onMarkAllRead,
  onMarkRead,
  onNavigate,
  onClose,
}) => {
  const panelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        onClose();
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [onClose]);

  const unreadCount = notifications.filter(n => !n.read).length;

  return (
    <div className="notif-panel" ref={panelRef}>
      <div className="notif-panel-header">
        <span className="notif-panel-title">Notifications</span>
        {unreadCount > 0 && (
          <button className="notif-mark-all" onClick={onMarkAllRead}>
            Mark all read
          </button>
        )}
      </div>

      <div className="notif-panel-body">
        {notifications.length === 0 ? (
          <div className="notif-empty">No notifications yet</div>
        ) : (
          notifications.map(n => (
            <div
              key={n.id}
              className={`notif-item${n.read ? '' : ' unread'}`}
              onClick={() => {
                if (!n.read) onMarkRead(n.id);
                onNavigate?.(n);
                onClose();
              }}
            >
              <div className="notif-icon-wrap">
                {n.actor_avatar_url ? (
                  <img src={n.actor_avatar_url} alt="" className="notif-avatar" />
                ) : (
                  <div className="notif-avatar-placeholder">
                    {n.actor_username.charAt(0).toUpperCase()}
                  </div>
                )}
                <span className="notif-type-badge">{notifIcon(n.type)}</span>
              </div>
              <div className="notif-content">
                <div className="notif-title">{n.title}</div>
                {n.body && <div className="notif-body">{n.body}</div>}
                <div className="notif-time">{timeAgo(n.created_at)}</div>
              </div>
              {!n.read && <div className="notif-dot" />}
            </div>
          ))
        )}
      </div>
    </div>
  );
};

export default NotificationPanel;
