import React, { useRef, useEffect } from 'react';
import MarkdownRenderer from '../ui/MarkdownRenderer';
import BadgeList from '../ui/BadgeList';
import './MiniProfile.css';

interface Role {
  id: string;
  name: string;
  color: string;
}

interface MiniProfileMember {
  id: string;
  user_id: string;
  username: string;
  nickname?: string;
  avatar_url?: string;
  banner_url?: string;
  bio?: string;
  badges?: number;
  roles?: Role[];
}

interface MiniProfileProps {
  member: MiniProfileMember;
  isCurrentUser: boolean;
  isOnline: boolean;
  position: { top: number; left: number };
  onClose: () => void;
  onSendMessage?: (userId: string) => void;
  onViewProfile?: (userId: string) => void;
  canManageRoles?: boolean;
  onManageRoles?: () => void;
}

const MiniProfile: React.FC<MiniProfileProps> = ({
  member, isCurrentUser, isOnline, position, onClose,
  onSendMessage, onViewProfile, canManageRoles, onManageRoles,
}) => {
  const ref = useRef<HTMLDivElement>(null);

  // Close on outside click
  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onClose();
      }
    };
    // Slight delay so the opening click doesn't also close it
    const timeout = setTimeout(() => document.addEventListener('mousedown', handleClick), 50);
    return () => {
      clearTimeout(timeout);
      document.removeEventListener('mousedown', handleClick);
    };
  }, [onClose]);

  // Close on ESC
  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [onClose]);

  // Clamp position so the popup stays within the viewport
  const vpW = window.innerWidth;
  const vpH = window.innerHeight;
  const popupW = 280;
  const popupH = 320;
  const left = Math.max(8, Math.min(position.left, vpW - popupW - 8));
  const top  = Math.max(8, Math.min(position.top,  vpH - popupH - 8));

  const displayName = member.nickname || member.username;
  const subName = member.nickname ? member.username : null;
  const showAddRole = canManageRoles && !isCurrentUser;

  return (
    <div ref={ref} className="mini-profile" style={{ top, left }}>
      {/* Banner */}
      <div
        className="mini-profile-banner"
        style={member.banner_url ? { backgroundImage: `url(${member.banner_url})` } : undefined}
      />

      {/* Avatar row */}
      <div className="mini-profile-avatar-row">
        <div style={{ position: 'relative' }}>
          <div className="mini-profile-avatar">
            {member.avatar_url
              ? <img src={member.avatar_url} alt={member.username} />
              : <span>{member.username.charAt(0).toUpperCase()}</span>
            }
          </div>
          <span className={`mini-profile-status-dot ${isOnline ? 'online' : 'offline'}`} />
        </div>
        {(member.badges ?? 0) > 0 && (
          <div className="mini-profile-badge-row">
            <BadgeList badges={member.badges!} />
          </div>
        )}
      </div>

      {/* Body */}
      <div className="mini-profile-body">
        <div className="mini-profile-username">{displayName}</div>
        {subName && <div className="mini-profile-nickname">{subName}</div>}
        {member.bio && <div className="mini-profile-bio"><MarkdownRenderer content={member.bio} mode="bio" /></div>}

        <div className="mini-profile-divider" />

        <div className="mini-profile-roles-header">
          <span className="mini-profile-section-label">Roles</span>
          {showAddRole && (
            <button
              className="mini-profile-add-role-btn"
              title="Manage roles"
              onClick={() => { onManageRoles?.(); onClose(); }}
            >
              +
            </button>
          )}
        </div>

        {member.roles && member.roles.length > 0 ? (
          <div className="mini-profile-roles">
            {member.roles.map(role => (
              <span
                key={role.id}
                className="mini-profile-role-tag"
                style={{ backgroundColor: role.color + '22', color: role.color, borderColor: role.color + '55' }}
              >
                {role.name}
              </span>
            ))}
          </div>
        ) : (
          <div className="mini-profile-no-roles">
            {showAddRole ? 'No roles — click + to assign' : 'No roles assigned'}
          </div>
        )}

        <div className="mini-profile-actions">
          {!isCurrentUser && onSendMessage && (
            <button
              className="mini-profile-action-btn primary"
              onClick={() => { onSendMessage(member.user_id); onClose(); }}
            >
              Message
            </button>
          )}
          <button
            className="mini-profile-action-btn secondary"
            onClick={() => { onViewProfile?.(member.user_id); onClose(); }}
          >
            View Profile
          </button>
        </div>
      </div>
    </div>
  );
};

export default MiniProfile;
