import React, { useState, useEffect, useRef, useCallback } from 'react';
import { ChevronDown, ChevronUp, UserX } from 'lucide-react';
import { Server, ServerMember, Role } from '../../api/types';
import { Invite, listServerInvites, revokeInvite, ServerBan, listServerBans, unbanMember } from '../../api/servers';
import { getServerRoles, getMemberRoles, assignRoleToMember, removeRoleFromMember } from '../../api/roles';
import './MembersTab.css';

interface Props {
  server: Server | null;
  members: ServerMember[];
}

function relativeTime(dateStr: string): string {
  const diff = (Date.now() - new Date(dateStr).getTime()) / 1000;
  if (diff < 60) return 'just now';
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

function timeRemaining(dateStr: string): string {
  const diff = (new Date(dateStr).getTime() - Date.now()) / 1000;
  if (diff <= 0) return 'Expired';
  if (diff < 3600) return `${Math.ceil(diff / 60)}m remaining`;
  if (diff < 86400) return `${Math.ceil(diff / 3600)}h remaining`;
  return `${Math.ceil(diff / 86400)}d remaining`;
}

interface InviteTooltipProps {
  invite: Invite;
  serverId: string;
  onRevoked: () => void;
  anchorRef: React.RefObject<HTMLElement | null>;
  onClose: () => void;
}

const InviteTooltip: React.FC<InviteTooltipProps> = ({ invite, serverId, onRevoked, anchorRef, onClose }) => {
  const tooltipRef = useRef<HTMLDivElement>(null);
  const [style, setStyle] = useState<React.CSSProperties>({ position: 'fixed', opacity: 0 });
  const [revoking, setRevoking] = useState(false);

  useEffect(() => {
    const anchor = anchorRef.current;
    const tooltip = tooltipRef.current;
    if (!anchor || !tooltip) return;

    const rect = anchor.getBoundingClientRect();
    const tipH = tooltip.offsetHeight || 160;
    const tipW = tooltip.offsetWidth || 260;

    let top = rect.bottom + 6;
    let left = rect.left;

    // Avoid bottom overflow
    if (top + tipH > window.innerHeight - 8) {
      top = rect.top - tipH - 6;
    }
    // Avoid right overflow
    if (left + tipW > window.innerWidth - 8) {
      left = window.innerWidth - tipW - 8;
    }
    // Avoid left overflow
    if (left < 8) left = 8;

    setStyle({ position: 'fixed', top, left, opacity: 1 });
  }, [anchorRef]);

  const isExpiredTime = invite.expires_at ? new Date(invite.expires_at).getTime() < Date.now() : false;
  const usesLabel = invite.max_uses != null
    ? `${invite.use_count} / ${invite.max_uses} uses`
    : `${invite.use_count} use${invite.use_count !== 1 ? 's' : ''}`;
  const expiryLabel = invite.expires_at
    ? timeRemaining(invite.expires_at)
    : 'Never expires';

  const handleRevoke = async () => {
    setRevoking(true);
    try {
      await revokeInvite(serverId, invite.code);
      onRevoked();
      onClose();
    } catch {
      setRevoking(false);
    }
  };

  return (
    <div
      ref={tooltipRef}
      className="members-invite-tooltip"
      style={style}
      onMouseLeave={onClose}
    >
      <div className="members-invite-tooltip-code">{invite.code}</div>
      <div className="members-invite-tooltip-row">
        <span className="members-invite-tooltip-label">Creator</span>
        <span className="members-invite-tooltip-value">{invite.creator_username}</span>
      </div>
      <div className="members-invite-tooltip-row">
        <span className="members-invite-tooltip-label">Uses</span>
        <span className="members-invite-tooltip-value">{usesLabel}</span>
      </div>
      <div className="members-invite-tooltip-row">
        <span className="members-invite-tooltip-label">Expiry</span>
        <span className={`members-invite-tooltip-value${isExpiredTime ? ' expired' : ''}`}>{expiryLabel}</span>
      </div>
      <button
        className="members-invite-tooltip-revoke"
        onClick={handleRevoke}
        disabled={revoking}
      >
        {revoking ? 'Revoking...' : 'Revoke Invite'}
      </button>
    </div>
  );
};

export const MembersTab: React.FC<Props> = ({ server, members }) => {
  const [search, setSearch] = useState('');
  const [invites, setInvites] = useState<Invite[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [memberRoles, setMemberRoles] = useState<Record<string, Set<string>>>({});
  const [expandedMember, setExpandedMember] = useState<string | null>(null);
  const [error, setError] = useState('');
  const [rolesError, setRolesError] = useState('');
  const [tooltipMember, setTooltipMember] = useState<string | null>(null);
  const anchorRefs = useRef<Record<string, HTMLElement | null>>({});

  const [bans, setBans] = useState<ServerBan[]>([]);
  const [bansLoading, setBansLoading] = useState(false);
  const [bansError, setBansError] = useState('');
  const [unbanning, setUnbanning] = useState<string | null>(null);

  const loadInvites = useCallback(async () => {
    if (!server) return;
    try {
      const list = await listServerInvites(server.id);
      setInvites(list ?? []);
    } catch {
      // silently ignore — invites just won't show extra info
    }
  }, [server]);

  const loadRoles = useCallback(async () => {
    if (!server) return;
    try {
      const r = await getServerRoles(server.id);
      const sorted = (r ?? []).sort((a: Role, b: Role) => {
        if (a.is_everyone) return 1;
        if (b.is_everyone) return -1;
        return (a.position ?? 0) - (b.position ?? 0);
      });
      setRoles(sorted);
    } catch (e) {
      setRolesError(e instanceof Error ? e.message : 'Failed to load roles');
    }
  }, [server]);

  const loadAllMemberRoles = useCallback(async () => {
    if (!server || members.length === 0) return;
    const entries = await Promise.all(
      members.map(async m => {
        try {
          const r = await getMemberRoles(server.id, m.user_id);
          return [m.user_id, new Set((r ?? []).map((role: Role) => role.id))] as const;
        } catch {
          return [m.user_id, new Set<string>()] as const;
        }
      })
    );
    setMemberRoles(Object.fromEntries(entries));
  }, [server, members]);

  const loadBans = useCallback(async () => {
    if (!server) return;
    setBansLoading(true);
    setBansError('');
    try {
      const list = await listServerBans(server.id);
      setBans(list ?? []);
    } catch (e) {
      setBansError(e instanceof Error ? e.message : 'Failed to load bans');
    } finally {
      setBansLoading(false);
    }
  }, [server]);

  const handleUnban = async (userId: string) => {
    if (!server) return;
    setUnbanning(userId);
    try {
      await unbanMember(server.id, userId);
      setBans(prev => prev.filter(b => b.user_id !== userId));
    } catch (e) {
      setBansError(e instanceof Error ? e.message : 'Failed to unban member');
    } finally {
      setUnbanning(null);
    }
  };

  useEffect(() => {
    loadInvites();
    loadRoles();
    loadBans();
  }, [loadInvites, loadRoles, loadBans]);

  useEffect(() => {
    if (expandedMember) {
      loadAllMemberRoles();
    }
  }, [expandedMember, loadAllMemberRoles]);

  const handleToggleMemberRole = async (userId: string, roleId: string, currentlyAssigned: boolean) => {
    if (!server) return;
    try {
      if (currentlyAssigned) {
        await removeRoleFromMember(server.id, userId, roleId);
        setMemberRoles(prev => {
          const s = new Set(prev[userId] ?? []);
          s.delete(roleId);
          return { ...prev, [userId]: s };
        });
      } else {
        await assignRoleToMember(server.id, userId, roleId);
        setMemberRoles(prev => {
          const s = new Set(prev[userId] ?? []);
          s.add(roleId);
          return { ...prev, [userId]: s };
        });
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to update role');
    }
  };

  const inviteMap = Object.fromEntries(invites.map(inv => [inv.code, inv]));

  const filtered = members.filter(m => {
    const q = search.toLowerCase();
    return (
      (m.username || '').toLowerCase().includes(q) ||
      (m.display_name || '').toLowerCase().includes(q) ||
      (m.nickname || '').toLowerCase().includes(q)
    );
  });

  if (!server) return null;

  return (
    <div className="members-tab">
      <h2 className="settings-page-title">Members</h2>
      {error && <div className="settings-error">{error}</div>}
      {rolesError && <div className="settings-error">{rolesError}</div>}

      <input
        type="text"
        className="settings-form-input members-search"
        placeholder="Search members..."
        value={search}
        onChange={e => setSearch(e.target.value)}
      />

      <div className="members-list">
        {filtered.length === 0 && (
          <div className="members-empty">
            {search ? 'No members match your search.' : 'No members found.'}
          </div>
        )}
        {filtered.map(member => {
          const assigned = memberRoles[member.user_id] ?? new Set();
          const isExpanded = expandedMember === member.user_id;
          const inviteCode = member.invite_code;
          const invite = inviteCode ? inviteMap[inviteCode] : undefined;
          const isTooltipOpen = tooltipMember === member.user_id;

          return (
            <div key={member.id} className="members-item">
              {/* Header row */}
              <div className="members-item-header" onClick={() => setExpandedMember(isExpanded ? null : member.user_id)}>
                <div className="members-item-avatar">
                  {member.avatar_url
                    ? <img src={member.avatar_url} alt={member.username} />
                    : (member.username || 'U').charAt(0).toUpperCase()
                  }
                </div>
                <div className="members-item-names">
                  <span className="members-item-display">
                    {member.nickname || member.display_name || member.username}
                  </span>
                  {(member.nickname || member.display_name) && (
                    <span className="members-item-username">@{member.username}</span>
                  )}
                </div>
                <div className="members-item-meta">
                  <span className="members-item-joined">{relativeTime(member.joined_at)}</span>
                </div>
                <div className="members-item-invite-col" onClick={e => e.stopPropagation()}>
                  {inviteCode ? (
                    <span
                      className="members-item-invite-code"
                      ref={el => { anchorRefs.current[member.user_id] = el; }}
                      onMouseEnter={() => setTooltipMember(member.user_id)}
                      onMouseLeave={() => {
                        // small delay so user can move mouse to tooltip
                        setTimeout(() => {
                          setTooltipMember(prev => prev === member.user_id ? null : prev);
                        }, 120);
                      }}
                    >
                      {inviteCode}
                    </span>
                  ) : (
                    <span className="members-item-direct-join">Direct join</span>
                  )}
                </div>
                <span className="members-item-roles-count">
                  {assigned.size > 0 ? `${assigned.size} role${assigned.size !== 1 ? 's' : ''}` : 'No roles'}
                  {isExpanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
                </span>
              </div>

              {/* Roles body */}
              {isExpanded && (
                <div className="members-item-roles-body">
                  {roles.length === 0 && (
                    <div className="members-roles-empty">No roles exist yet.</div>
                  )}
                  {roles.map(role => {
                    const isAssigned = assigned.has(role.id);
                    return (
                      <div
                        key={role.id}
                        className="members-role-row"
                        onClick={() => handleToggleMemberRole(member.user_id, role.id, isAssigned)}
                      >
                        <button
                          type="button"
                          className={`custom-toggle${isAssigned ? ' on' : ''}`}
                          onClick={e => {
                            e.stopPropagation();
                            handleToggleMemberRole(member.user_id, role.id, isAssigned);
                          }}
                          aria-pressed={isAssigned}
                        />
                        <span style={{ width: 10, height: 10, borderRadius: '50%', backgroundColor: role.color, display: 'inline-block', flexShrink: 0 }} />
                        <span style={{ color: role.color }}>{role.is_everyone ? '@everyone' : role.name}</span>
                      </div>
                    );
                  })}
                </div>
              )}

              {/* Invite tooltip */}
              {isTooltipOpen && invite && (
                <InviteTooltip
                  invite={invite}
                  serverId={server.id}
                  anchorRef={{ current: anchorRefs.current[member.user_id] }}
                  onClose={() => setTooltipMember(null)}
                  onRevoked={async () => {
                    setTooltipMember(null);
                    await loadInvites();
                  }}
                />
              )}
            </div>
          );
        })}
      </div>

      {/* Banned Members */}
      <h3 className="settings-section-title" style={{ marginTop: '2rem' }}>Banned Members</h3>
      {bansError && <div className="settings-error">{bansError}</div>}
      {bansLoading ? (
        <div className="members-empty">Loading…</div>
      ) : bans.length === 0 ? (
        <div className="members-empty">No banned members.</div>
      ) : (
        <div className="members-list">
          {bans.map(ban => (
            <div key={ban.user_id} className="members-item">
              <div className="members-item-header" style={{ cursor: 'default' }}>
                <div className="members-item-avatar">
                  {ban.avatar_url
                    ? <img src={ban.avatar_url} alt={ban.username} />
                    : ban.username.charAt(0).toUpperCase()
                  }
                </div>
                <div className="members-item-names">
                  <span className="members-item-display">{ban.username}</span>
                </div>
                {ban.reason && (
                  <div className="members-item-meta">
                    <span className="members-item-joined" title="Ban reason">{ban.reason}</span>
                  </div>
                )}
                <button
                  className="settings-btn settings-btn-secondary"
                  style={{ marginLeft: 'auto', fontSize: '0.8rem', display: 'flex', alignItems: 'center', gap: 4 }}
                  onClick={() => handleUnban(ban.user_id)}
                  disabled={unbanning === ban.user_id}
                >
                  <UserX size={13} />
                  {unbanning === ban.user_id ? 'Unbanning…' : 'Unban'}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
