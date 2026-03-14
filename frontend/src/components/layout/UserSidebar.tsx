import React, { useState, useRef, useEffect } from 'react';
import './UserSidebar.css';

interface ServerMember {
  id: string;
  user_id: string;
  username: string;
  nickname?: string;
}

interface UserContextMenuProps {
  member: ServerMember;
  isCurrentUser: boolean;
  position: { top: number; left: number };
  onClose: () => void;
  onViewProfile?: (userId: string) => void;
  onSendMessage?: (userId: string) => void;
}

const UserContextMenu: React.FC<UserContextMenuProps> = ({ member, isCurrentUser, position, onClose, onViewProfile, onSendMessage }) => {
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
    <div
      ref={ref}
      className="user-context-menu"
      style={{ top: position.top, left: position.left }}
    >
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
}

const UserSidebar: React.FC<UserSidebarProps> = ({ members, ownerId, currentUserId, onViewProfile, onSendMessage, onlineUserIds }) => {
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
      <div className="member-avatar">
        <span className="member-avatar-placeholder">
          {member.username.charAt(0).toUpperCase()}
        </span>
        <span className={`member-status ${isOnline ? 'status-online' : 'status-offline'}`} />
      </div>
      <div className="member-info">
        <div className="member-name">
          {member.username}
          {isOwner && <span className="role-badge owner">owner</span>}
        </div>
        {member.nickname && <div className="member-nickname-text">{member.nickname}</div>}
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
          position={contextMenu.position}
          onClose={closeContextMenu}
          onViewProfile={onViewProfile}
          onSendMessage={onSendMessage}
        />
      )}
    </div>
  );
};

export default UserSidebar;
