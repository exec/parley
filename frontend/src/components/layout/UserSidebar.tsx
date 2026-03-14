import React, { useState, useRef, useEffect } from 'react';
import './UserSidebar.css';

interface ServerMember {
  id: string;
  user_id: string;
  username: string;
  nickname?: string;
}

interface UserPopoverProps {
  member: ServerMember;
  isOwner: boolean;
  position: { top: number; left: number };
  onClose: () => void;
}

const UserPopover: React.FC<UserPopoverProps> = ({ member, isOwner, position, onClose }) => {
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
      className="user-popover"
      style={{ top: position.top, left: position.left }}
    >
      <div className="user-popover-header">
        <div className="user-popover-avatar">
          {member.username.charAt(0).toUpperCase()}
        </div>
        <div>
          <div className="user-popover-name">{member.username}</div>
          {member.nickname && <div className="user-popover-nick">{member.nickname}</div>}
          {isOwner && <div className="user-popover-role">Owner</div>}
        </div>
      </div>
      <div className="user-popover-divider" />
      <button className="user-popover-item" disabled>
        Send Message <span className="coming-soon">soon</span>
      </button>
      <button className="user-popover-item" disabled>
        View Profile <span className="coming-soon">soon</span>
      </button>
    </div>
  );
};

interface UserSidebarProps {
  members: ServerMember[];
  ownerId?: string;
}

const UserSidebar: React.FC<UserSidebarProps> = ({ members, ownerId }) => {
  const [popover, setPopover] = useState<{ member: ServerMember; position: { top: number; left: number } } | null>(null);

  const handleMemberClick = (member: ServerMember, e: React.MouseEvent) => {
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    setPopover({
      member,
      position: { top: rect.top, left: rect.left - 200 },
    });
  };

  const owners = members.filter(m => m.user_id === ownerId);
  const nonOwners = members.filter(m => m.user_id !== ownerId);

  const renderMember = (member: ServerMember, isOwner: boolean) => (
    <div
      key={member.id}
      className="member-item"
      onClick={(e) => handleMemberClick(member, e)}
      title="Click for options"
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
      {popover && (
        <UserPopover
          member={popover.member}
          isOwner={popover.member.user_id === ownerId}
          position={popover.position}
          onClose={() => setPopover(null)}
        />
      )}
    </div>
  );
};

export default UserSidebar;
