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
      {!isCurrentUser && (
        <button className="user-context-menu-item" onClick={() => { onSendMessage?.(member.user_id); onClose(); }}>
          Send Message
        </button>
      )}
      <button className="user-context-menu-item" onClick={() => { onViewProfile?.(member.user_id); onClose(); }}>
        View Profile
      </button>
    </div>
  );
};

interface UserSidebarProps {
  members: ServerMember[];
  ownerId?: string;
  currentUserId?: string;
  onViewProfile?: (userId: string) => void;
  onSendMessage?: (userId: string) => void;
}

const UserSidebar: React.FC<UserSidebarProps> = ({ members, ownerId, currentUserId, onViewProfile, onSendMessage }) => {
  const [contextMenu, setContextMenu] = useState<{ member: ServerMember; position: { top: number; left: number } } | null>(null);

  const handleMemberClick = (member: ServerMember, e: React.MouseEvent) => {
    e.stopPropagation();
    // Left-click shows mini profile popup, right-click shows context menu
    if (e.button === 2) {
      // Right-click
      const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
      setContextMenu({
        member,
        position: { top: rect.top, left: rect.left - 200 },
      });
    } else {
      // Left-click - do nothing (mini profile popup is handled by click handler in channel)
    }
  };

  const closeContextMenu = () => setContextMenu(null);

  const owners = members.filter(m => m.user_id === ownerId);
  const nonOwners = members.filter(m => m.user_id !== ownerId);

  const renderMember = (member: ServerMember, isOwner: boolean) => (
    <div
      key={member.id}
      className="member-item"
      onClick={(e) => handleMemberClick(member, e)}
      onContextMenu={(e) => handleMemberClick(member, e)}
      title="Click for profile, right-click for options"
    >
      <div className="member-avatar">
        <span className="member-avatar-placeholder">
          {member.username.charAt(0).toUpperCase()}
        </span>
        <span className="member-status" />
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
