import React, { useState, useRef, useEffect } from 'react';
import './UserSidebar.css';

interface ServerMember {
  id: string;
  user_id: string;
  username: string;
  nickname?: string;
  roles?: Array<{ id: string; name: string; color: string }>;
}

interface UserContextMenuProps {
  member: ServerMember;
  isCurrentUser: boolean;
  isOwner: boolean;
  canManageRoles: boolean;
  position: { top: number; left: number };
  onClose: () => void;
  onViewProfile?: (userId: string) => void;
  onSendMessage?: (userId: string) => void;
  onManageRoles?: () => void;
}

const UserContextMenu: React.FC<UserContextMenuProps> = ({ member, isCurrentUser, isOwner: _isOwner, canManageRoles, position, onClose, onViewProfile, onSendMessage, onManageRoles }) => {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onClose();
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [onClose]);

  return (
    <div ref={ref} className="user-context-menu" style={{ top: position.top, left: position.left }}>
      <div className="user-context-menu-header">{member.username}</div>
      <div className="user-context-menu-divider" />
      <button className="user-context-menu-item" onClick={() => { onViewProfile?.(member.user_id); onClose(); }}>
        View Profile
      </button>
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
    </div>
  );
};

interface UserSidebarProps {
  members: ServerMember[];
  ownerId?: string;
  currentUserId?: string;
  onViewProfile?: (userId: string) => void;
  onSendMessage?: (userId: string) => void;
  onlineUserIds?: Set<string>;
  serverId?: string;
  currentUserIsOwner?: boolean;
  onManageRoles?: (memberId: string) => void;
}

const UserSidebar: React.FC<UserSidebarProps> = ({ members, ownerId, currentUserId, onViewProfile, onSendMessage, onlineUserIds, currentUserIsOwner, onManageRoles }) => {
  const [contextMenu, setContextMenu] = useState<{ member: ServerMember; position: { top: number; left: number } } | null>(null);

  const handleMemberClick = (member: ServerMember, e: React.MouseEvent) => {
    e.stopPropagation();
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    // Position menu to the left of the sidebar panel
    setContextMenu({
      member,
      position: { top: rect.top, left: rect.left - 190 },
    });
  };

  const handleMemberContextMenu = (member: ServerMember, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    // Use cursor position for right-click for precision
    setContextMenu({
      member,
      position: { top: e.clientY, left: e.clientX - 190 },
    });
  };

  const closeContextMenu = () => setContextMenu(null);

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
      title="Click for options"
    >
      <div className="member-avatar-wrapper">
        <div className="member-avatar">
          <span className="member-avatar-placeholder">
            {member.username.charAt(0).toUpperCase()}
          </span>
        </div>
        <span className={`member-status ${isOnline ? 'status-online' : 'status-offline'}`} />
      </div>
      <div className="member-info">
        <div className="member-name">
          {member.username}
          {isOwner && (
            <span className="owner-hat" title="Server Owner" aria-label="Server Owner">
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" style={{ display: 'inline-block', verticalAlign: 'middle', marginLeft: '4px' }}>
                {/* brim */}
                <ellipse cx="8" cy="12" rx="7" ry="2" fill="#32CD32"/>
                {/* crown */}
                <path d="M3 12 L3 7 Q3 5 5 5 L5 4 Q5 2 8 2 Q11 2 11 4 L11 5 Q13 5 13 7 L13 12 Z" fill="#32CD32"/>
                {/* band */}
                <rect x="3" y="9" width="10" height="1.5" fill="#1a7a1a"/>
                {/* feather */}
                <path d="M11 6 Q13 4 14 3 Q13 5 12 7" stroke="#22a322" strokeWidth="0.8" fill="none"/>
              </svg>
            </span>
          )}
        </div>
        {member.nickname && <div className="member-nickname-text">{member.nickname}</div>}
        {member.roles && member.roles.length > 0 && (
          <div className="member-roles">
            {member.roles.map(role => (
              <span key={role.id} className="role-tag" style={{ backgroundColor: role.color + '33', color: role.color }}>
                {role.name}
              </span>
            ))}
          </div>
        )}
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
      {contextMenu && (
        <UserContextMenu
          member={contextMenu.member}
          isCurrentUser={contextMenu.member.user_id === currentUserId}
          isOwner={contextMenu.member.user_id === ownerId}
          canManageRoles={currentUserIsOwner === true}
          position={contextMenu.position}
          onClose={closeContextMenu}
          onViewProfile={onViewProfile}
          onSendMessage={onSendMessage}
          onManageRoles={() => onManageRoles?.(contextMenu.member.user_id)}
        />
      )}
    </div>
  );
};

export default UserSidebar;
