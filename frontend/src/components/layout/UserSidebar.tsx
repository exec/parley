import React from 'react';
import './UserSidebar.css';

interface User {
  id: string;
  username: string;
}

interface ServerMember {
  id: string;
  user_id: string;
  nickname?: string;
  user?: User;
}

interface UserSidebarProps {
  members: ServerMember[];
  ownerId?: string;
}

const UserSidebar: React.FC<UserSidebarProps> = ({ members, ownerId }) => {
  const owners = members.filter(m => m.user_id === ownerId);
  const nonOwners = members.filter(m => m.user_id !== ownerId);

  const renderMember = (member: ServerMember, isOwner: boolean) => {
    const displayName = member.user?.username || member.user_id || 'Unknown';

    return (
      <div key={member.id} className="member-item">
        <div className="member-avatar">
          <span className="member-avatar-placeholder">
            {displayName.charAt(0).toUpperCase()}
          </span>
          <span className="member-status" />
        </div>
        <div className="member-info">
          <div className="member-name">
            {displayName}
            {isOwner && <span className="role-badge owner">owner</span>}
            {member.nickname && !isOwner && (
              <span className="member-nickname"> ({member.nickname})</span>
            )}
          </div>
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
    </div>
  );
};

export default UserSidebar;
