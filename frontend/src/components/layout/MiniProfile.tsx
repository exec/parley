import React, { useRef, useEffect, useLayoutEffect, useState } from 'react';
import MarkdownRenderer from '../ui/MarkdownRenderer';
import BadgeList from '../ui/BadgeList';
import './MiniProfile.css';

interface Role {
  id: string;
  name: string;
  color: string;
  is_everyone?: boolean;
}

interface MiniProfileMember {
  id: string;
  user_id: string;
  username: string;
  display_name?: string;
  nickname?: string;
  avatar_url?: string;
  banner_url?: string;
  bio?: string;
  badges?: number;
  roles?: Role[];
  status_type?: string;
  status_text?: string;
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
  hideRoles?: boolean;
}

const MiniProfile: React.FC<MiniProfileProps> = ({
  member, isCurrentUser, isOnline, position, onClose,
  onSendMessage, onViewProfile, canManageRoles, onManageRoles, hideRoles,
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

  // Measure actual rendered size before first paint so clamping is exact
  const [style, setStyle] = useState<React.CSSProperties>({
    top: position.top,
    left: position.left,
    visibility: 'hidden',
  });

  useLayoutEffect(() => {
    if (!ref.current) return;
    const { width, height } = ref.current.getBoundingClientRect();
    const vpW = window.innerWidth;
    const vpH = window.innerHeight;
    const clampedLeft = Math.max(8, Math.min(position.left, vpW - width - 8));
    const clampedTop  = Math.max(8, Math.min(position.top,  vpH - height - 8));
    setStyle({ top: clampedTop, left: clampedLeft, visibility: 'visible' });
  }, [position.left, position.top]);

  const displayName = member.display_name || member.nickname || member.username;
  const subName = (member.display_name || member.nickname) ? member.username : null;
  const showAddRole = canManageRoles && !isCurrentUser;

  return (
    <div ref={ref} className="mini-profile" style={style}>
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
          <span className={`mini-profile-status-dot ${member.status_type || (isOnline ? 'online' : 'offline')}`} />
        </div>
      </div>

      {/* Body */}
      <div className="mini-profile-body">
        <div className="mini-profile-name-row">
          <div>
            <div className="mini-profile-username">{displayName}</div>
            {subName && <div className="mini-profile-nickname">{subName}</div>}
          </div>
          {(member.badges ?? 0) > 0 && (
            <div className="mini-profile-badge-row">
              <BadgeList badges={member.badges!} />
            </div>
          )}
        </div>
        {member.status_text && (
          <div className="mini-profile-status-text">{member.status_text}</div>
        )}
        {member.bio && <div className="mini-profile-bio"><MarkdownRenderer content={member.bio} mode="bio" /></div>}

        {!hideRoles && (
          <>
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

            {member.roles && member.roles.filter(r => !r.is_everyone).length > 0 ? (
              <div className="mini-profile-roles">
                {member.roles.filter(r => !r.is_everyone).map(role => (
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
          </>
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
