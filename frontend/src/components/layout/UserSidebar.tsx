import React, { useState, useRef, useEffect } from 'react';
import './UserSidebar.css';
import MiniProfile from './MiniProfile';

interface Role {
  id: string;
  name: string;
  color: string;
  hoist: boolean;
  position: number;
}

interface ServerMember {
  id: string;
  user_id: string;
  username: string;
  nickname?: string;
  avatar_url?: string;
  banner_url?: string;
  roles?: Role[];
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

/* ---- Grouping logic ---- */

interface MemberGroup {
  label: string;
  color: string | null; // null = no role color (online/offline groups)
  members: ServerMember[];
}

/** Returns the highest-position hoisted role for a member, or null. */
function topHoistedRole(member: ServerMember): Role | null {
  if (!member.roles || member.roles.length === 0) return null;
  const hoisted = member.roles.filter(r => r.hoist);
  if (hoisted.length === 0) return null;
  return hoisted.reduce((a, b) => (a.position <= b.position ? a : b));
}

/** Returns the highest-position role for a member (for the inline tag). */
function topRole(member: ServerMember): Role | null {
  if (!member.roles || member.roles.length === 0) return null;
  return member.roles.reduce((a, b) => (a.position <= b.position ? a : b));
}

function buildGroups(members: ServerMember[], ownerId?: string, onlineIds?: Set<string>): MemberGroup[] {
  const online = members.filter(m => onlineIds ? onlineIds.has(m.user_id) : true);
  const offline = members.filter(m => onlineIds ? !onlineIds.has(m.user_id) : false);

  const groups: MemberGroup[] = [];

  // Collect all distinct hoisted roles present across online members, sorted by position
  const hoistedRolesMap = new Map<string, Role>();
  for (const m of online) {
    const r = topHoistedRole(m);
    if (r) hoistedRolesMap.set(r.id, r);
  }
  const hoistedRoles = Array.from(hoistedRolesMap.values()).sort((a, b) => a.position - b.position);

  // Track which online members have been placed
  const placed = new Set<string>();

  for (const role of hoistedRoles) {
    const group = online.filter(m => topHoistedRole(m)?.id === role.id);
    if (group.length === 0) continue;
    // Sort: owner first, then alphabetical
    group.sort((a, b) => {
      if (a.user_id === ownerId) return -1;
      if (b.user_id === ownerId) return 1;
      return (a.nickname || a.username).localeCompare(b.nickname || b.username);
    });
    groups.push({ label: role.name, color: role.color, members: group });
    group.forEach(m => placed.add(m.user_id));
  }

  // Remaining online members (no hoisted role)
  const ungroupedOnline = online.filter(m => !placed.has(m.user_id));
  if (ungroupedOnline.length > 0) {
    ungroupedOnline.sort((a, b) => {
      if (a.user_id === ownerId) return -1;
      if (b.user_id === ownerId) return 1;
      return (a.nickname || a.username).localeCompare(b.nickname || b.username);
    });
    groups.push({ label: `Online — ${ungroupedOnline.length}`, color: null, members: ungroupedOnline });
  }

  // Offline
  if (offline.length > 0) {
    offline.sort((a, b) =>
      (a.nickname || a.username).localeCompare(b.nickname || b.username)
    );
    groups.push({ label: `Offline — ${offline.length}`, color: null, members: offline });
  }

  return groups;
}

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

  const handleMemberClick = (member: ServerMember, e: React.MouseEvent) => {
    e.stopPropagation();
    setContextMenu(null);
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    setMiniProfile({ member, position: { top: rect.top, left: rect.left - 300 } });
  };

  const handleMemberContextMenu = (member: ServerMember, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setMiniProfile(null);
    setContextMenu({ member, position: { top: e.clientY, left: e.clientX - 200 } });
  };

  const renderMember = (member: ServerMember) => {
    const isOnline = onlineUserIds ? onlineUserIds.has(member.user_id) : false;
    const isOwner = member.user_id === ownerId;
    const top = topRole(member);

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
            {top && (
              <span className="role-tag" style={{ backgroundColor: top.color + '33', color: top.color }}>
                {top.name}
              </span>
            )}
          </div>
          {member.nickname && <div className="member-nickname-text">{member.username}</div>}
        </div>
      </div>
    );
  };

  const groups = buildGroups(members, ownerId, onlineUserIds);

  return (
    <div className="user-sidebar">
      <div className="user-sidebar-header">Members — {members.length}</div>
      <div className="members-container">
        {members.length === 0 ? (
          <div className="no-members">No members yet</div>
        ) : (
          groups.map((group, i) => (
            <div key={i} className="member-group">
              <div className="member-group-header" style={group.color ? { color: group.color } : undefined}>
                {group.label}
                <span className="member-group-count">{group.members.length}</span>
              </div>
              {group.members.map(m => renderMember(m))}
            </div>
          ))
        )}
      </div>

      {miniProfile && (
        <MiniProfile
          member={miniProfile.member}
          isCurrentUser={miniProfile.member.user_id === currentUserId}
          isOnline={onlineUserIds ? onlineUserIds.has(miniProfile.member.user_id) : false}
          position={miniProfile.position}
          onClose={() => setMiniProfile(null)}
          onSendMessage={onSendMessage}
          onViewProfile={onViewProfile}
          canManageRoles={(currentUserIsOwner === true || canKickMembers === true) && miniProfile.member.user_id !== currentUserId}
          onManageRoles={() => { onManageRoles?.(miniProfile.member.user_id); setMiniProfile(null); }}
        />
      )}

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
