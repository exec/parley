import React, { useState, useRef, useEffect } from 'react';
import './UserSidebar.css';
import MiniProfile from './MiniProfile';

interface ServerMember {
  id: string;
  user_id: string;
  username: string;
  nickname?: string;
  avatar_url?: string;
  banner_url?: string;
  roles?: Array<{ id: string; name: string; color: string }>;
}

/* ---- Right-click context menu ---- */

interface UserContextMenuProps {
  member: ServerMember;
  isCurrentUser: boolean;
  isOwner: boolean;
  canManageRoles: boolean;
  position: { top: number; left: number };
  onClose: () => void;
  onSendMessage?: (userId: string) => void;
  onManageRoles?: () => void;
  onKick?: (userId: string) => void;
  onBan?: (userId: string) => void;
}

const UserContextMenu: React.FC<UserContextMenuProps> = ({
  member, isCurrentUser, isOwner, canManageRoles, position, onClose,
  onSendMessage, onManageRoles, onKick, onBan,
}) => {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [onClose]);

  // Clamp position
  const left = Math.min(position.left, window.innerWidth - 200);

  return (
    <div ref={ref} className="user-context-menu" style={{ top: position.top, left }}>
      <div className="user-context-menu-header">{member.username}</div>
      <div className="user-context-menu-divider" />
      {!isCurrentUser && (
        <button className="user-context-menu-item" onClick={() => { onSendMessage?.(member.user_id); onClose(); }}>
          Send Message
        </button>
      )}
      {canManageRoles && (
        <button className="user-context-menu-item" onClick={() => { onManageRoles?.(); onClose(); }}>
          Manage Roles
        </button>
      )}
      {canManageRoles && !isCurrentUser && !isOwner && (
        <>
          <div className="user-context-menu-divider" />
          <button className="user-context-menu-item" style={{ color: '#FFB347' }} onClick={() => { onKick?.(member.user_id); onClose(); }}>
            Kick Member
          </button>
          <button className="user-context-menu-item" style={{ color: '#FF4444' }} onClick={() => { onBan?.(member.user_id); onClose(); }}>
            Ban Member
          </button>
        </>
      )}
    </div>
  );
};

/* ---- Sidebar ---- */

interface UserSidebarProps {
  members: ServerMember[];
  ownerId?: string;
  currentUserId?: string;
  onViewProfile?: (userId: string) => void;
  onSendMessage?: (userId: string) => void;
  onlineUserIds?: Set<string>;
  currentUserIsOwner?: boolean;
  canKickMembers?: boolean;
  onManageRoles?: (memberId: string) => void;
  onKick?: (userId: string) => void;
  onBan?: (userId: string) => void;
}

const UserSidebar: React.FC<UserSidebarProps> = ({
  members, ownerId, currentUserId, onViewProfile, onSendMessage,
  onlineUserIds, currentUserIsOwner, canKickMembers, onManageRoles, onKick, onBan,
}) => {
  const [miniProfile, setMiniProfile] = useState<{ member: ServerMember; position: { top: number; left: number } } | null>(null);
  const [contextMenu, setContextMenu] = useState<{ member: ServerMember; position: { top: number; left: number } } | null>(null);

  // Left click → mini profile
  const handleMemberClick = (member: ServerMember, e: React.MouseEvent) => {
    e.stopPropagation();
    // Close context menu if open
    setContextMenu(null);
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    setMiniProfile({
      member,
      position: { top: rect.top, left: rect.left - 300 },
    });
  };

  // Right click → context menu
  const handleMemberContextMenu = (member: ServerMember, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setMiniProfile(null);
    setContextMenu({
      member,
      position: { top: e.clientY, left: e.clientX - 200 },
    });
  };

  const owners = members.filter(m => m.user_id === ownerId);
  const nonOwners = members.filter(m => m.user_id !== ownerId);

  const renderMember = (member: ServerMember, isOwner: boolean) => {
    const isOnline = onlineUserIds ? onlineUserIds.has(member.user_id) : false;
    return (
      <div
        key={member.id}
        className={`member-item ${isOnline ? '' : 'member-offline'}`}
        onClick={(e) => handleMemberClick(member, e)}
        onContextMenu={(e) => handleMemberContextMenu(member, e)}
        title="Click for profile"
      >
        <div className="member-avatar-wrapper">
          <div className="member-avatar">
            {member.avatar_url ? (
              <img src={member.avatar_url} alt={member.username} className="member-avatar-img" />
            ) : (
              <span className="member-avatar-placeholder">
                {member.username.charAt(0).toUpperCase()}
              </span>
            )}
          </div>
          <span className={`member-status ${isOnline ? 'status-online' : 'status-offline'}`} />
        </div>
        <div className="member-info">
          <div className="member-name">
            {member.nickname || member.username}
            {isOwner && (
              <span className="owner-hat" title="Server Owner" aria-label="Server Owner">
                <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" style={{ display: 'inline-block', verticalAlign: 'middle', marginLeft: '4px' }}>
                  <ellipse cx="8" cy="12" rx="7" ry="2" fill="#32CD32"/>
                  <path d="M3 12 L3 7 Q3 5 5 5 L5 4 Q5 2 8 2 Q11 2 11 4 L11 5 Q13 5 13 7 L13 12 Z" fill="#32CD32"/>
                  <rect x="3" y="9" width="10" height="1.5" fill="#1a7a1a"/>
                  <path d="M11 6 Q13 4 14 3 Q13 5 12 7" stroke="#22a322" strokeWidth="0.8" fill="none"/>
                </svg>
              </span>
            )}
            {member.roles && member.roles.slice(0, 2).map(role => (
              <span key={role.id} className="role-tag" style={{ backgroundColor: role.color + '33', color: role.color }}>
                {role.name}
              </span>
            ))}
          </div>
          {member.nickname && <div className="member-nickname-text">{member.username}</div>}
        </div>
      </div>
    );
  };

  return (
    <div className="user-sidebar">
      <div className="user-sidebar-header">Members — {members.length}</div>
      <div className="members-container">
        {members.length === 0 ? (
          <div className="no-members">No members yet</div>
        ) : (
          <>
            {owners.length > 0 && (
              <div className="member-group">
                <div className="member-group-header">Owner</div>
                {owners.map(m => renderMember(m, true))}
              </div>
            )}
            {nonOwners.length > 0 && (
              <div className="member-group">
                <div className="member-group-header">Members</div>
                {nonOwners.map(m => renderMember(m, false))}
              </div>
            )}
          </>
        )}
      </div>

      {/* Left-click: mini profile popup */}
      {miniProfile && (
        <MiniProfile
          member={miniProfile.member}
          isCurrentUser={miniProfile.member.user_id === currentUserId}
          isOnline={onlineUserIds ? onlineUserIds.has(miniProfile.member.user_id) : false}
          position={miniProfile.position}
          onClose={() => setMiniProfile(null)}
          onSendMessage={onSendMessage}
          onViewProfile={onViewProfile}
        />
      )}

      {/* Right-click: context menu */}
      {contextMenu && (
        <UserContextMenu
          member={contextMenu.member}
          isCurrentUser={contextMenu.member.user_id === currentUserId}
          isOwner={contextMenu.member.user_id === ownerId}
          canManageRoles={currentUserIsOwner === true || canKickMembers === true}
          position={contextMenu.position}
          onClose={() => setContextMenu(null)}
          onSendMessage={onSendMessage}
          onManageRoles={() => onManageRoles?.(contextMenu.member.user_id)}
          onKick={onKick}
          onBan={onBan}
        />
      )}
    </div>
  );
};

export default UserSidebar;
