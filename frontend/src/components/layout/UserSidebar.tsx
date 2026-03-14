import React from 'react';
import './UserSidebar.css';

interface User {
  id: string;
  username: string;
  avatar?: string;
}

interface ServerMember {
  id: string;
  userId: string;
  role: 'owner' | 'admin' | 'member';
  nickname?: string;
  user?: User;
}

interface UserSidebarProps {
  members: ServerMember[];
}

type RoleKey = 'owner' | 'admin' | 'member';

const UserSidebar: React.FC<UserSidebarProps> = ({ members }) => {
  const groupedMembers: Record<RoleKey, ServerMember[]> = {
    owner: [],
    admin: [],
    member: [],
  };

  members.forEach((member) => {
    groupedMembers[member.role].push(member);
  });

  const renderMember = (member: ServerMember) => {
    const displayName = member.user?.username || 'Unknown User';

    return (
      <div key={member.id} className="member-item">
        <div className="member-avatar">
          {member.user?.avatar ? (
            <img src={member.user.avatar} alt={displayName} />
          ) : (
            <span className="member-avatar-placeholder">
              {displayName.charAt(0).toUpperCase()}
            </span>
          )}
          <span className="member-status" />
        </div>
        <div className="member-info">
          <div className="member-name">
            {displayName}
            {member.role !== 'member' && (
              <span className={`role-badge ${member.role}`}>{member.role}</span>
            )}
          </div>
          {member.nickname && (
            <div className="member-role">{member.nickname}</div>
          )}
        </div>
        <div className="member-actions">
          <button title="Voice Chat">
            <svg
              viewBox="0 0 24 24"
              fill="currentColor"
              xmlns="http://www.w3.org/2000/svg"
            >
              <path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3zm5.91-3c-.49 0-.9.36-.98.85C16.52 14.2 14.47 16 12 16s-4.52-1.8-4.93-4.15c-.08-.49-.49-.85-.98-.85-.61 0-1.09.54-1 1.14.49 3 2.89 5.35 5.91 5.78V20c0 .55.45 1 1 1s1-.45 1-1v-2.08c3.02-.43 5.42-2.78 5.91-5.78.1-.6-.39-1.14-1-1.14z" />
            </svg>
          </button>
          <button title="Direct Message">
            <svg
              viewBox="0 0 24 24"
              fill="currentColor"
              xmlns="http://www.w3.org/2000/svg"
            >
              <path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2z" />
            </svg>
          </button>
        </div>
      </div>
    );
  };

  return (
    <div className="user-sidebar">
      <div className="user-sidebar-header">Members</div>
      <div className="members-container">
        {members.length === 0 ? (
          <div className="no-members">No members in this server</div>
        ) : (
          <>
            {groupedMembers.owner.length > 0 && (
              <div className="member-group">
                <div className="member-group-header">Owner</div>
                {groupedMembers.owner.map(renderMember)}
              </div>
            )}

            {groupedMembers.admin.length > 0 && (
              <div className="member-group">
                <div className="member-group-header">Admin</div>
                {groupedMembers.admin.map(renderMember)}
              </div>
            )}

            {groupedMembers.member.length > 0 && (
              <div className="member-group">
                <div className="member-group-header">Member</div>
                {groupedMembers.member.map(renderMember)}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
};

export default UserSidebar;