import React, { useState, useEffect, useCallback } from 'react';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { ServerMember, Role } from '../../api/types';
import {
  getServerRoles,
  createServerRole,
  deleteServerRole,
  getMemberRoles,
  assignRoleToMember,
  removeRoleFromMember,
} from '../../api/roles';

interface ManageRolesModalProps {
  isOpen: boolean;
  onClose: () => void;
  serverId: string;
  members: ServerMember[];
  focusUserId?: string; // open directly to a specific user's role management
}

export const ManageRolesModal: React.FC<ManageRolesModalProps> = ({
  isOpen,
  onClose,
  serverId,
  members,
  focusUserId,
}) => {
  const [roles, setRoles] = useState<Role[]>([]);
  const [memberRoles, setMemberRoles] = useState<Record<string, Set<string>>>({}); // userId -> Set<roleId>
  const [activeTab, setActiveTab] = useState<'roles' | 'members'>('roles');
  const [newRoleName, setNewRoleName] = useState('');
  const [newRoleColor, setNewRoleColor] = useState('#99aab5');
  const [newRolePerms, setNewRolePerms] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [expandedMember, setExpandedMember] = useState<string | null>(focusUserId ?? null);

  const loadRoles = useCallback(async () => {
    if (!serverId) return;
    try {
      const r = await getServerRoles(serverId);
      setRoles(r ?? []);
    } catch (e) {
      console.error('Failed to load roles', e);
    }
  }, [serverId]);

  const loadAllMemberRoles = useCallback(async (memberList: ServerMember[]) => {
    if (!serverId || memberList.length === 0) return;
    const entries = await Promise.all(
      memberList.map(async m => {
        try {
          const r = await getMemberRoles(serverId, m.user_id);
          return [m.user_id, new Set((r ?? []).map(role => role.id))] as const;
        } catch {
          return [m.user_id, new Set<string>()] as const;
        }
      })
    );
    setMemberRoles(Object.fromEntries(entries));
  }, [serverId]);

  useEffect(() => {
    if (!isOpen || !serverId) return;
    loadRoles();
    loadAllMemberRoles(members);
    if (focusUserId) {
      setActiveTab('members');
      setExpandedMember(focusUserId);
    }
  }, [isOpen, serverId]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleAddRole = async () => {
    if (!newRoleName.trim()) return;
    setLoading(true);
    setError('');
    try {
      const role = await createServerRole(serverId, newRoleName.trim(), newRoleColor, newRolePerms);
      setRoles(prev => [...prev, role]);
      setNewRoleName('');
      setNewRoleColor('#99aab5');
      setNewRolePerms(0);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to create role');
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteRole = async (roleId: string) => {
    setLoading(true);
    try {
      await deleteServerRole(serverId, roleId);
      setRoles(prev => prev.filter(r => r.id !== roleId));
      // Remove this role from all members locally
      setMemberRoles(prev => {
        const next = { ...prev };
        Object.keys(next).forEach(uid => {
          next[uid] = new Set([...next[uid]].filter(rid => rid !== roleId));
        });
        return next;
      });
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to delete role');
    } finally {
      setLoading(false);
    }
  };

  const handleToggleRole = async (userId: string, roleId: string, currentlyAssigned: boolean) => {
    try {
      if (currentlyAssigned) {
        await removeRoleFromMember(serverId, userId, roleId);
        setMemberRoles(prev => {
          const s = new Set(prev[userId] ?? []);
          s.delete(roleId);
          return { ...prev, [userId]: s };
        });
      } else {
        await assignRoleToMember(serverId, userId, roleId);
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

  // Permission flags scaffolding
  const PERMISSIONS = [
    { label: 'Send Messages', value: 1 },
    { label: 'Manage Messages', value: 2 },
    { label: 'Manage Channels', value: 4 },
    { label: 'Kick Members', value: 8 },
    { label: 'Manage Server', value: 16 },
  ];

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Manage Roles">
      <div className="roles-modal">
        <div className="roles-tabs">
          <button
            className={`roles-tab ${activeTab === 'roles' ? 'active' : ''}`}
            onClick={() => setActiveTab('roles')}
          >
            Roles
          </button>
          <button
            className={`roles-tab ${activeTab === 'members' ? 'active' : ''}`}
            onClick={() => setActiveTab('members')}
          >
            Members
          </button>
        </div>

        {error && <div className="modal-error" style={{ marginBottom: 8 }}>{error}</div>}

        {activeTab === 'roles' && (
          <div className="roles-content">
            <p className="modal-description">
              Roles organize members and grant permissions.
            </p>
            <div className="roles-list">
              {roles.length === 0 && <div style={{ color: '#555', fontSize: 13 }}>No roles yet.</div>}
              {roles.map(role => (
                <div key={role.id} className="role-item">
                  <div className="role-color" style={{ backgroundColor: role.color }} />
                  <span className="role-name">{role.name}</span>
                  <button
                    className="role-delete-btn"
                    onClick={() => handleDeleteRole(role.id)}
                    disabled={loading}
                    title="Delete role"
                  >
                    ×
                  </button>
                </div>
              ))}
            </div>
            <div className="add-role-form">
              <input
                className="form-input"
                type="text"
                value={newRoleName}
                onChange={e => setNewRoleName(e.target.value)}
                placeholder="New role name"
                onKeyDown={e => e.key === 'Enter' && handleAddRole()}
                disabled={loading}
              />
              <input
                type="color"
                value={newRoleColor}
                onChange={e => setNewRoleColor(e.target.value)}
                className="color-picker"
                title="Role color"
              />
              <Button type="button" variant="primary" onClick={handleAddRole} loading={loading}>
                Add Role
              </Button>
            </div>
            <div style={{ marginTop: 12 }}>
              <div style={{ fontSize: 12, color: '#555', marginBottom: 6, textTransform: 'uppercase', letterSpacing: '0.5px' }}>Permissions</div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                {PERMISSIONS.map(p => (
                  <label key={p.value} style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, color: '#aaa', cursor: 'pointer' }}>
                    <button
                      type="button"
                      className={`custom-toggle${(newRolePerms & p.value) !== 0 ? ' on' : ''}`}
                      onClick={() => setNewRolePerms(prev => prev ^ p.value)}
                      aria-pressed={(newRolePerms & p.value) !== 0}
                    />
                    {p.label}
                  </label>
                ))}
              </div>
            </div>
          </div>
        )}

        {activeTab === 'members' && (
          <div className="roles-content">
            <p className="modal-description">
              Click a member to assign or remove roles.
            </p>
            <div className="members-roles-list">
              {members.map(member => {
                const assigned = memberRoles[member.user_id] ?? new Set();
                const isExpanded = expandedMember === member.user_id;
                return (
                  <div key={member.id} className="member-role-item" style={{ flexDirection: 'column', alignItems: 'stretch' }}>
                    <div
                      style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', padding: '6px 0', flexDirection: 'row' }}
                      onClick={() => setExpandedMember(isExpanded ? null : member.user_id)}
                    >
                      <div className="member-avatar-small">
                        {(member.username || 'U').charAt(0).toUpperCase()}
                      </div>
                      <span className="member-name-small">{member.username || member.user_id}</span>
                      <span style={{ fontSize: 11, color: '#555', marginLeft: 'auto' }}>
                        {assigned.size > 0 ? `${assigned.size} role${assigned.size > 1 ? 's' : ''}` : 'No roles'} {isExpanded ? '▲' : '▼'}
                      </span>
                    </div>
                    {isExpanded && (
                      <div style={{ paddingLeft: 40, paddingBottom: 8 }}>
                        {roles.length === 0 && <div style={{ fontSize: 12, color: '#555' }}>No roles to assign.</div>}
                        {roles.map(role => {
                          const isAssigned = assigned.has(role.id);
                          return (
                            <label
                              key={role.id}
                              style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '3px 0', cursor: 'pointer', fontSize: 13 }}
                            >
                              <button
                                type="button"
                                className={`custom-toggle${isAssigned ? ' on' : ''}`}
                                onClick={() => handleToggleRole(member.user_id, role.id, isAssigned)}
                                aria-pressed={isAssigned}
                              />
                              <span style={{ width: 10, height: 10, borderRadius: '50%', backgroundColor: role.color, display: 'inline-block', flexShrink: 0 }} />
                              <span style={{ color: role.color }}>{role.name}</span>
                            </label>
                          );
                        })}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        )}

        <div className="modal-actions">
          <Button type="button" variant="secondary" onClick={onClose}>Close</Button>
        </div>
      </div>
    </Modal>
  );
};
