import React, { useState, useRef, useEffect, useCallback } from 'react';
import { X, ChevronUp, ChevronDown } from 'lucide-react';
import { Server, ServerMember, Role } from '../../api/types';
import { updateServer, deleteServer, createInvite, setVanityURL, getMyPermissions } from '../../api/servers';
import { uploadFile } from '../../api/upload';
import {
  getServerRoles,
  createServerRole,
  deleteServerRole,
  updateServerRole,
  getMemberRoles,
  assignRoleToMember,
  removeRoleFromMember,
} from '../../api/roles';
import {
  PERMISSION_CATEGORIES,
  PERM_ALL,
  PERM_ADMINISTRATOR,
  hasPerm,
  permFromNumber,
  permToNumber,
} from '../../lib/permissions';
import { BotsTab } from './BotsTab';
import './Settings.css';

type Tab = 'overview' | 'roles' | 'bots' | 'danger';
type RolesSubTab = 'roles' | 'members';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  server: Server | null;
  members: ServerMember[];
  onUpdate: (server: Server) => void;
  onDelete: () => void;
  onCreateInvite: (code: string) => void;
  initialTab?: Tab;
}

export const ServerSettings: React.FC<Props> = ({
  isOpen, onClose, server, members, onUpdate, onDelete, onCreateInvite, initialTab,
}) => {
  const [activeTab, setActiveTab] = useState<Tab>(initialTab ?? 'overview');

  // Overview fields
  const [name, setName] = useState('');
  const [vanityUrl, setVanityUrl] = useState('');
  const [iconUrl, setIconUrl] = useState('');
  const [iconUploading, setIconUploading] = useState(false);
  const [overviewLoading, setOverviewLoading] = useState(false);
  const [overviewError, setOverviewError] = useState('');
  const [inviteCode, setInviteCode] = useState<string | null>(null);
  const [inviteLoading, setInviteLoading] = useState(false);
  const iconInputRef = useRef<HTMLInputElement>(null);

  // Roles state
  const [roles, setRoles] = useState<Role[]>([]);
  const [rolesSubTab, setRolesSubTab] = useState<RolesSubTab>('roles');
  const [selectedRoleId, setSelectedRoleId] = useState<string | null>(null);
  const [rolesLoading, setRolesLoading] = useState(false);
  const [rolesError, setRolesError] = useState('');
  // Current user's base permissions for hierarchy enforcement
  const [myPerms, setMyPerms] = useState<bigint>(0n);
  // Creating new role
  const [creatingRole, setCreatingRole] = useState(false);
  const [newRoleName, setNewRoleName] = useState('');
  const [newRoleColor, setNewRoleColor] = useState('#99aab5');
  const [newRolePerms, setNewRolePerms] = useState<bigint>(0n);
  // Editing existing role
  const [editRoleName, setEditRoleName] = useState('');
  const [editRoleColor, setEditRoleColor] = useState('');
  const [editRolePerms, setEditRolePerms] = useState<bigint>(0n);
  const [editRoleHoist, setEditRoleHoist] = useState(false);
  const [editRolePosition, setEditRolePosition] = useState(0);
  // Member roles
  const [memberRoles, setMemberRoles] = useState<Record<string, Set<string>>>({});
  const [expandedMember, setExpandedMember] = useState<string | null>(null);

  // Danger
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [deleteError, setDeleteError] = useState('');

  const [unsavedConfirm, setUnsavedConfirm] = useState(false);

  // Reset on open
  useEffect(() => {
    if (isOpen && server) {
      setActiveTab(initialTab ?? 'overview');
      setName(server.name);
      setVanityUrl(server.vanity_url || '');
      setIconUrl(server.icon_url || '');
      setOverviewError('');
      setInviteCode(null);
      setShowDeleteConfirm(false);
      setDeleteError('');
      setCreatingRole(false);
      setSelectedRoleId(null);
      setRolesError('');
      setUnsavedConfirm(false);
    }
  }, [isOpen, server, initialTab]);

  // Load roles when roles tab is active
  useEffect(() => {
    if (isOpen && activeTab === 'roles' && server) {
      loadRoles();
      loadMyPerms();
    }
  }, [isOpen, activeTab, server]); // eslint-disable-line react-hooks/exhaustive-deps

  const loadRoles = async () => {
    if (!server) return;
    setRolesLoading(true);
    try {
      const r = await getServerRoles(server.id);
      // Sort: @everyone always last, then by position ascending
      const sorted = (r ?? []).sort((a, b) => {
        if (a.is_everyone) return 1;
        if (b.is_everyone) return -1;
        return (a.position ?? 0) - (b.position ?? 0);
      });
      setRoles(sorted);
    } catch (e) {
      setRolesError(e instanceof Error ? e.message : 'Failed to load roles');
    } finally {
      setRolesLoading(false);
    }
  };

  const loadMyPerms = async () => {
    if (!server) return;
    try {
      const n = await getMyPermissions(server.id);
      setMyPerms(permFromNumber(n));
    } catch {
      // If endpoint fails (e.g. user is owner and has all), assume all permissions
      setMyPerms(PERM_ALL);
    }
  };

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

  useEffect(() => {
    if (isOpen && activeTab === 'roles' && rolesSubTab === 'members') {
      loadAllMemberRoles();
    }
  }, [isOpen, activeTab, rolesSubTab, loadAllMemberRoles]);

  // Sync edit fields when selected role changes
  useEffect(() => {
    const role = roles.find(r => r.id === selectedRoleId);
    if (role) {
      setEditRoleName(role.name);
      setEditRoleColor(role.color);
      setEditRolePerms(permFromNumber(role.permissions ?? 0));
      setEditRoleHoist(role.hoist ?? false);
      setEditRolePosition(role.position ?? 0);
    }
  }, [selectedRoleId, roles]);

  // ESC to close
  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') attemptClose(); };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [isOpen]); // eslint-disable-line react-hooks/exhaustive-deps

  const hasOverviewChanges = () => server && (
    name !== server.name ||
    vanityUrl !== (server.vanity_url || '') ||
    iconUrl !== (server.icon_url || '')
  );

  const attemptClose = () => {
    if (hasOverviewChanges()) { setUnsavedConfirm(true); }
    else { onClose(); }
  };

  /* ---- Overview handlers ---- */

  const handleIconUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setIconUploading(true);
    setOverviewError('');
    try {
      const url = await uploadFile(file);
      setIconUrl(url);
    } catch (err) {
      setOverviewError(err instanceof Error ? err.message : 'Failed to upload icon');
    } finally {
      setIconUploading(false);
      if (iconInputRef.current) iconInputRef.current.value = '';
    }
  };

  const handleSaveOverview = async () => {
    if (!server || !name.trim()) { setOverviewError('Server name is required'); return; }
    setOverviewLoading(true);
    setOverviewError('');
    try {
      const updated = await updateServer(server.id, name.trim(), iconUrl || undefined);
      if (vanityUrl.trim() !== (server.vanity_url || '')) {
        const withVanity = await setVanityURL(server.id, vanityUrl.trim());
        onUpdate(withVanity);
      } else {
        onUpdate(updated);
      }
    } catch (err: unknown) {
      setOverviewError(err instanceof Error ? err.message : 'Failed to update server');
    } finally {
      setOverviewLoading(false);
    }
  };

  const handleCreateInvite = async () => {
    if (!server) return;
    setInviteLoading(true);
    try {
      const invite = await createInvite(server.id);
      setInviteCode(invite.code);
      onCreateInvite(invite.code);
    } catch (err) {
      setOverviewError(err instanceof Error ? err.message : 'Failed to create invite');
    } finally {
      setInviteLoading(false);
    }
  };

  /* ---- Roles handlers ---- */

  const handleAddRole = async () => {
    if (!server || !newRoleName.trim()) return;
    setRolesLoading(true);
    setRolesError('');
    try {
      const role = await createServerRole(server.id, newRoleName.trim(), newRoleColor, permToNumber(newRolePerms));
      setRoles(prev => [...prev, role]);
      setNewRoleName('');
      setNewRoleColor('#99aab5');
      setNewRolePerms(0n);
      setCreatingRole(false);
      setSelectedRoleId(role.id);
    } catch (e) {
      setRolesError(e instanceof Error ? e.message : 'Failed to create role');
    } finally {
      setRolesLoading(false);
    }
  };

  const handleSaveRole = async () => {
    if (!server || !selectedRoleId || !editRoleName.trim()) return;
    setRolesLoading(true);
    setRolesError('');
    try {
      const updated = await updateServerRole(server.id, selectedRoleId, editRoleName.trim(), editRoleColor, permToNumber(editRolePerms), editRoleHoist, editRolePosition);
      setRoles(prev => prev.map(r => r.id === selectedRoleId ? updated : r));
    } catch (e) {
      setRolesError(e instanceof Error ? e.message : 'Failed to update role');
    } finally {
      setRolesLoading(false);
    }
  };

  const handleDeleteRole = async (roleId: string) => {
    if (!server) return;
    setRolesLoading(true);
    try {
      await deleteServerRole(server.id, roleId);
      setRoles(prev => prev.filter(r => r.id !== roleId));
      if (selectedRoleId === roleId) setSelectedRoleId(null);
      setMemberRoles(prev => {
        const next = { ...prev };
        Object.keys(next).forEach(uid => { next[uid] = new Set([...next[uid]].filter(rid => rid !== roleId)); });
        return next;
      });
    } catch (e) {
      setRolesError(e instanceof Error ? e.message : 'Failed to delete role');
    } finally {
      setRolesLoading(false);
    }
  };

  const handleToggleMemberRole = async (userId: string, roleId: string, currentlyAssigned: boolean) => {
    if (!server) return;
    try {
      if (currentlyAssigned) {
        await removeRoleFromMember(server.id, userId, roleId);
        setMemberRoles(prev => { const s = new Set(prev[userId] ?? []); s.delete(roleId); return { ...prev, [userId]: s }; });
      } else {
        await assignRoleToMember(server.id, userId, roleId);
        setMemberRoles(prev => { const s = new Set(prev[userId] ?? []); s.add(roleId); return { ...prev, [userId]: s }; });
      }
    } catch (e) {
      setRolesError(e instanceof Error ? e.message : 'Failed to update role');
    }
  };

  /* ---- Delete handler ---- */

  const handleDeleteServer = async () => {
    if (!server) return;
    setDeleteLoading(true);
    setDeleteError('');
    try {
      await deleteServer(server.id);
      onDelete();
      onClose();
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Failed to delete server');
    } finally {
      setDeleteLoading(false);
    }
  };

  if (!isOpen || !server) return null;

  const selectedRole = roles.find(r => r.id === selectedRoleId) ?? null;

  return (
    <div className="settings-overlay">
      {/* Sidebar */}
      <div className="settings-sidebar">
        <div className="settings-sidebar-group">
          <div className="settings-sidebar-group-label">{server.name}</div>
          <button className={`settings-nav-item${activeTab === 'overview' ? ' active' : ''}`} onClick={() => setActiveTab('overview')}>
            Overview
          </button>
          <button className={`settings-nav-item${activeTab === 'roles' ? ' active' : ''}`} onClick={() => setActiveTab('roles')}>
            Roles
          </button>
          <button className={`settings-nav-item${activeTab === 'bots' ? ' active' : ''}`} onClick={() => setActiveTab('bots')}>
            Bots
          </button>
        </div>
        <div className="settings-nav-divider" />
        <div className="settings-sidebar-spacer" />
        <div className="settings-sidebar-group">
          <button className={`settings-nav-item danger${activeTab === 'danger' ? ' active' : ''}`} onClick={() => setActiveTab('danger')}>
            Delete Server
          </button>
          <div className="settings-nav-divider" />
          <button className="settings-nav-item" onClick={attemptClose}>
            Close Settings
          </button>
        </div>
      </div>

      {/* Content */}
      <div className="settings-main">
        <div className={`settings-content${activeTab === 'roles' ? ' settings-content--wide' : ''}`}>
          {activeTab === 'overview' && (
            <>
              <h2 className="settings-page-title">Server Overview</h2>

              {overviewError && <div className="settings-error">{overviewError}</div>}

              {/* Icon + Name row */}
              <div className="settings-section">
                <div className="settings-section-title">Server Identity</div>
                <div className="settings-upload-row">
                  <div className="settings-server-icon-preview">
                    {iconUrl
                      ? <img src={iconUrl} alt="Server icon" />
                      : <span>{server.name.charAt(0).toUpperCase()}</span>
                    }
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                    <input type="file" accept="image/*" ref={iconInputRef} style={{ display: 'none' }} onChange={handleIconUpload} />
                    <button className="settings-upload-btn" disabled={iconUploading || overviewLoading} onClick={() => iconInputRef.current?.click()}>
                      {iconUploading ? 'Uploading...' : 'Change Icon'}
                    </button>
                    {iconUrl && (
                      <button className="settings-upload-remove-btn" onClick={() => setIconUrl('')}>Remove Icon</button>
                    )}
                  </div>
                </div>

                <div className="settings-form-group" style={{ marginTop: 16 }}>
                  <label className="settings-form-label">Server Name</label>
                  <input className="settings-form-input" type="text" value={name} onChange={e => setName(e.target.value)} placeholder="Server name" />
                </div>
              </div>

              <div className="settings-section">
                <div className="settings-section-title">Vanity URL</div>
                <div className="settings-vanity-wrap">
                  <span className="settings-vanity-prefix">{window.location.origin}/invite/</span>
                  <input
                    className="settings-vanity-input"
                    type="text"
                    value={vanityUrl}
                    onChange={e => setVanityUrl(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))}
                    placeholder="my-server"
                    maxLength={32}
                  />
                </div>
                <div className="settings-form-hint">Letters, numbers, and hyphens only. Leave blank to disable.</div>
              </div>

              <div style={{ display: 'flex', gap: 10, marginBottom: 32 }}>
                <button className="settings-btn settings-btn-primary" onClick={handleSaveOverview} disabled={overviewLoading}>
                  {overviewLoading ? 'Saving...' : 'Save Changes'}
                </button>
                {hasOverviewChanges() && (
                  <button className="settings-btn settings-btn-secondary" onClick={() => { setName(server.name); setVanityUrl(server.vanity_url || ''); setIconUrl(server.icon_url || ''); }}>
                    Reset
                  </button>
                )}
              </div>

              <div className="settings-section">
                <div className="settings-section-title">Invite People</div>
                <p style={{ fontSize: 13, color: '#666', marginBottom: 12 }}>Generate a shareable invite link for this server.</p>
                {inviteCode ? (
                  <div className="settings-invite-box">
                    <span className="settings-invite-link">{window.location.origin}/invite/{inviteCode}</span>
                    <button className="settings-btn settings-btn-ghost" onClick={() => navigator.clipboard.writeText(`${window.location.origin}/invite/${inviteCode}`)}>Copy</button>
                    <button className="settings-btn settings-btn-ghost" onClick={() => setInviteCode(null)}>Dismiss</button>
                  </div>
                ) : (
                  <button className="settings-btn settings-btn-secondary" onClick={handleCreateInvite} disabled={inviteLoading}>
                    {inviteLoading ? 'Generating...' : 'Create Invite Link'}
                  </button>
                )}
              </div>
            </>
          )}

          {activeTab === 'roles' && (
            <>
              <h2 className="settings-page-title">Roles</h2>
              {rolesError && <div className="settings-error" style={{ marginBottom: 12 }}>{rolesError}</div>}

              {/* Sub-tabs */}
              <div style={{ display: 'flex', gap: 4, marginBottom: 20, borderBottom: '1px solid #1e2228', paddingBottom: 0 }}>
                {(['roles', 'members'] as RolesSubTab[]).map(t => (
                  <button
                    key={t}
                    onClick={() => setRolesSubTab(t)}
                    style={{
                      background: 'none', border: 'none', cursor: 'pointer', padding: '8px 16px',
                      fontSize: 13, fontWeight: 600, fontFamily: 'inherit',
                      color: rolesSubTab === t ? '#32CD32' : '#555',
                      borderBottom: rolesSubTab === t ? '2px solid #32CD32' : '2px solid transparent',
                      marginBottom: -1, letterSpacing: '0.3px', textTransform: 'capitalize',
                    }}
                  >
                    {t === 'roles' ? 'Roles' : 'Members'}
                  </button>
                ))}
              </div>

              {rolesSubTab === 'roles' && (
                <div className="roles-panel">
                  {/* Left: role list */}
                  <div className="roles-list-col">
                    {rolesLoading && roles.length === 0 && <div style={{ fontSize: 12, color: '#555' }}>Loading...</div>}
                    {roles.map(role => (
                      <button
                        key={role.id}
                        className={`roles-list-item${selectedRoleId === role.id && !creatingRole ? ' active' : ''}${role.is_everyone ? ' everyone-role' : ''}`}
                        onClick={() => { setSelectedRoleId(role.id); setCreatingRole(false); }}
                        title={role.is_everyone ? '@everyone — base role for all members' : undefined}
                      >
                        <span className="roles-list-color-dot" style={{ backgroundColor: role.color }} />
                        <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {role.is_everyone ? '@everyone' : role.name}
                        </span>
                      </button>
                    ))}
                    <button className="roles-list-add" onClick={() => { setCreatingRole(true); setSelectedRoleId(null); }}>
                      + New Role
                    </button>
                  </div>

                  {/* Right: editor */}
                  <div className="roles-edit-col">
                    {creatingRole && (
                      <div className="roles-new-form">
                        <div className="roles-edit-header">
                          <span className="roles-edit-title">New Role</span>
                        </div>
                        <div className="settings-form-group">
                          <label className="settings-form-label">Role Name</label>
                          <input className="settings-form-input" type="text" value={newRoleName} onChange={e => setNewRoleName(e.target.value)}
                            placeholder="Role name" onKeyDown={e => e.key === 'Enter' && handleAddRole()} />
                        </div>
                        <div className="settings-form-group" style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                          <label className="settings-form-label" style={{ margin: 0 }}>Color</label>
                          <input type="color" value={newRoleColor} onChange={e => setNewRoleColor(e.target.value)}
                            style={{ width: 40, height: 32, border: 'none', background: 'none', cursor: 'pointer', padding: 0 }} />
                          <span style={{ fontSize: 13, color: '#777' }}>{newRoleColor}</span>
                        </div>
                        <div className="settings-section-title" style={{ marginBottom: 8 }}>Permissions</div>
                        {PERMISSION_CATEGORIES.map(cat => (
                          <div key={cat.label} style={{ marginBottom: 16 }}>
                            <div style={{ fontSize: 11, color: '#32CD32', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.8px', marginBottom: 6 }}>
                              {cat.label}
                            </div>
                            <div className="roles-perms-grid">
                              {cat.permissions.map(p => {
                                const adminOn = hasPerm(newRolePerms, PERM_ADMINISTRATOR);
                                const isAdminPerm = p.bit === PERM_ADMINISTRATOR;
                                const isOn = adminOn && !isAdminPerm ? true : hasPerm(newRolePerms, p.bit);
                                const adminLocked = adminOn && !isAdminPerm;
                                const canToggle = !adminLocked && (hasPerm(myPerms, PERM_ADMINISTRATOR) || hasPerm(myPerms, p.bit));
                                return (
                                  <div key={String(p.bit)} className="roles-perm-row">
                                    <div>
                                      <div className="roles-perm-label">{p.name}</div>
                                      <div className="roles-perm-desc">{p.description}</div>
                                    </div>
                                    <button
                                      type="button"
                                      className={`custom-toggle${isOn ? ' on' : ''}${!canToggle ? ' disabled' : ''}`}
                                      onClick={() => canToggle && setNewRolePerms(prev => prev ^ p.bit)}
                                      aria-pressed={isOn}
                                      disabled={!canToggle}
                                      title={adminLocked ? 'Granted by Administrator' : !canToggle ? 'You lack this permission yourself' : undefined}
                                    />
                                  </div>
                                );
                              })}
                            </div>
                          </div>
                        ))}
                        <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
                          <button className="settings-btn settings-btn-primary" onClick={handleAddRole} disabled={rolesLoading || !newRoleName.trim()}>
                            {rolesLoading ? 'Creating...' : 'Create Role'}
                          </button>
                          <button className="settings-btn settings-btn-secondary" onClick={() => setCreatingRole(false)}>Cancel</button>
                        </div>
                      </div>
                    )}

                    {selectedRole && !creatingRole && (
                      <div className="roles-edit-form">
                        <div className="roles-edit-header">
                          <span className="roles-edit-title">
                            {selectedRole.is_everyone ? 'Edit @everyone' : 'Edit Role'}
                          </span>
                          {!selectedRole.is_everyone && (
                            <button className="settings-btn settings-btn-danger" onClick={() => handleDeleteRole(selectedRole.id)} disabled={rolesLoading}>
                              Delete Role
                            </button>
                          )}
                        </div>
                        {!selectedRole.is_everyone && (
                          <div className="settings-form-group">
                            <label className="settings-form-label">Role Name</label>
                            <input className="settings-form-input" type="text" value={editRoleName} onChange={e => setEditRoleName(e.target.value)} placeholder="Role name" />
                          </div>
                        )}
                        {selectedRole.is_everyone && (
                          <div style={{ fontSize: 12, color: '#555', marginBottom: 12, padding: '8px 10px', background: '#0d0f12', borderRadius: 4, border: '1px solid #1e2228' }}>
                            @everyone is the base role assigned to all server members. It cannot be renamed or deleted.
                          </div>
                        )}
                        {!selectedRole.is_everyone && (
                          <div className="settings-form-group" style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                            <label className="settings-form-label" style={{ margin: 0 }}>Color</label>
                            <input type="color" value={editRoleColor} onChange={e => setEditRoleColor(e.target.value)}
                              style={{ width: 40, height: 32, border: 'none', background: 'none', cursor: 'pointer', padding: 0 }} />
                            <span style={{ fontSize: 13, color: '#777' }}>{editRoleColor}</span>
                          </div>
                        )}
                        {!selectedRole.is_everyone && (
                          <div className="roles-perm-row" style={{ marginBottom: 12 }}>
                            <div>
                              <div className="roles-perm-label">Display separately in member list</div>
                              <div className="roles-perm-desc">Group members with this role under its own section in the sidebar</div>
                            </div>
                            <button
                              type="button"
                              className={`custom-toggle${editRoleHoist ? ' on' : ''}`}
                              onClick={() => setEditRoleHoist(h => !h)}
                              aria-pressed={editRoleHoist}
                            />
                          </div>
                        )}
                        {!selectedRole.is_everyone && (
                          <div className="settings-form-group" style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                            <label className="settings-form-label" style={{ margin: 0 }}>Position</label>
                            <input
                              type="number"
                              min={0}
                              value={editRolePosition}
                              onChange={e => setEditRolePosition(Number(e.target.value))}
                              className="settings-form-input"
                              style={{ width: 80 }}
                            />
                            <span style={{ fontSize: 12, color: '#555' }}>Lower = higher priority</span>
                          </div>
                        )}
                        <div className="settings-section-title" style={{ marginBottom: 8 }}>Permissions</div>
                        {PERMISSION_CATEGORIES.map(cat => (
                          <div key={cat.label} style={{ marginBottom: 16 }}>
                            <div style={{ fontSize: 11, color: '#32CD32', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.8px', marginBottom: 6 }}>
                              {cat.label}
                            </div>
                            <div className="roles-perms-grid">
                              {cat.permissions.map(p => {
                                const adminOn = hasPerm(editRolePerms, PERM_ADMINISTRATOR);
                                const isAdminPerm = p.bit === PERM_ADMINISTRATOR;
                                const isOn = adminOn && !isAdminPerm ? true : hasPerm(editRolePerms, p.bit);
                                const adminLocked = adminOn && !isAdminPerm;
                                const canToggle = !adminLocked && (hasPerm(myPerms, PERM_ADMINISTRATOR) || hasPerm(myPerms, p.bit));
                                return (
                                  <div key={String(p.bit)} className="roles-perm-row">
                                    <div>
                                      <div className="roles-perm-label">{p.name}</div>
                                      <div className="roles-perm-desc">{p.description}</div>
                                    </div>
                                    <button
                                      type="button"
                                      className={`custom-toggle${isOn ? ' on' : ''}${!canToggle ? ' disabled' : ''}`}
                                      onClick={() => canToggle && setEditRolePerms(prev => prev ^ p.bit)}
                                      aria-pressed={isOn}
                                      disabled={!canToggle}
                                      title={adminLocked ? 'Granted by Administrator' : !canToggle ? 'You lack this permission yourself' : undefined}
                                    />
                                  </div>
                                );
                              })}
                            </div>
                          </div>
                        ))}
                        <div style={{ marginTop: 16 }}>
                          <button className="settings-btn settings-btn-primary" onClick={handleSaveRole} disabled={rolesLoading || (!selectedRole.is_everyone && !editRoleName.trim())}>
                            {rolesLoading ? 'Saving...' : 'Save Role'}
                          </button>
                        </div>
                      </div>
                    )}

                    {!selectedRole && !creatingRole && (
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#444', fontSize: 13 }}>
                        {roles.length === 0 ? 'No roles yet — create one!' : 'Select a role to edit'}
                      </div>
                    )}
                  </div>
                </div>
              )}

              {rolesSubTab === 'members' && (
                <>
                  <p style={{ fontSize: 13, color: '#666', marginBottom: 14 }}>Click a member to assign or remove roles.</p>
                  {roles.length === 0 && (
                    <div style={{ fontSize: 13, color: '#555', padding: '16px', background: '#0d0f12', border: '1px solid #1e2228', borderRadius: 6 }}>
                      No roles exist yet. Switch to the Roles tab to create some.
                    </div>
                  )}
                  <div className="roles-members-list">
                    {members.map(member => {
                      const assigned = memberRoles[member.user_id] ?? new Set();
                      const isExpanded = expandedMember === member.user_id;
                      return (
                        <div key={member.id} className="roles-member-item">
                          <div className="roles-member-header" onClick={() => setExpandedMember(isExpanded ? null : member.user_id)}>
                            <div className="roles-member-avatar">
                              {member.avatar_url
                                ? <img src={member.avatar_url} alt={member.username} style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: '50%' }} />
                                : (member.username || 'U').charAt(0).toUpperCase()
                              }
                            </div>
                            <span className="roles-member-name">{member.nickname || member.username}</span>
                            <span className="roles-member-count">
                              {assigned.size > 0 ? `${assigned.size} role${assigned.size !== 1 ? 's' : ''}` : 'No roles'} {isExpanded ? <ChevronUp size={14} color="currentColor" /> : <ChevronDown size={14} color="currentColor" />}
                            </span>
                          </div>
                          {isExpanded && (
                            <div className="roles-member-roles-body">
                              {roles.length === 0 && <div style={{ fontSize: 12, color: '#555' }}>No roles to assign.</div>}
                              {roles.map(role => {
                                const isAssigned = assigned.has(role.id);
                                return (
                                  <div key={role.id} className="roles-member-role-row" onClick={() => handleToggleMemberRole(member.user_id, role.id, isAssigned)}>
                                    <button
                                      type="button"
                                      className={`custom-toggle${isAssigned ? ' on' : ''}`}
                                      onClick={e => { e.stopPropagation(); handleToggleMemberRole(member.user_id, role.id, isAssigned); }}
                                      aria-pressed={isAssigned}
                                    />
                                    <span style={{ width: 10, height: 10, borderRadius: '50%', backgroundColor: role.color, display: 'inline-block', flexShrink: 0 }} />
                                    <span style={{ color: role.color }}>{role.name}</span>
                                  </div>
                                );
                              })}
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </>
              )}
            </>
          )}

          {activeTab === 'bots' && (
            <BotsTab
              serverId={Number(server.id)}
              isOwner={hasPerm(myPerms, PERM_ADMINISTRATOR)}
            />
          )}

          {activeTab === 'danger' && (
            <>
              <h2 className="settings-page-title">Danger Zone</h2>

              {deleteError && <div className="settings-error">{deleteError}</div>}

              <div className="settings-danger-zone">
                <div className="settings-danger-title">Delete Server</div>
                <p className="settings-danger-desc">
                  Once you delete <strong style={{ color: '#cc7777' }}>{server.name}</strong>, there is no going back.
                  All channels, messages, and roles will be permanently deleted.
                </p>
                {!showDeleteConfirm ? (
                  <button className="settings-btn settings-btn-danger" onClick={() => setShowDeleteConfirm(true)}>
                    Delete This Server
                  </button>
                ) : (
                  <div className="settings-delete-confirm">
                    <p>Type the server name to confirm deletion, then click Delete Forever.</p>
                    <div style={{ display: 'flex', gap: 8 }}>
                      <button className="settings-btn settings-btn-secondary" onClick={() => setShowDeleteConfirm(false)}>Cancel</button>
                      <button className="settings-btn settings-btn-danger" onClick={handleDeleteServer} disabled={deleteLoading}>
                        {deleteLoading ? 'Deleting...' : 'Delete Forever'}
                      </button>
                    </div>
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      </div>

      {/* Close button */}
      <div className="settings-close-wrap">
        <button className="settings-close-btn" onClick={attemptClose} title="Close (ESC)"><X size={16} color="currentColor" /></button>
        <span className="settings-close-hint">ESC</span>
      </div>

      {/* Unsaved changes confirmation */}
      {unsavedConfirm && (
        <div style={{ position: 'fixed', inset: 0, zIndex: 1200, display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(0,0,0,0.6)' }}>
          <div style={{ background: '#16191d', border: '1px solid #2a2d32', borderRadius: 8, padding: 28, maxWidth: 380, width: '90%' }}>
            <div style={{ fontSize: 16, fontWeight: 700, color: '#ddd', marginBottom: 8 }}>Unsaved Changes</div>
            <div style={{ fontSize: 13, color: '#777', marginBottom: 20 }}>You have unsaved changes. Are you sure you want to leave?</div>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 10 }}>
              <button className="settings-btn settings-btn-secondary" onClick={() => setUnsavedConfirm(false)}>Keep Editing</button>
              <button className="settings-btn settings-btn-danger" onClick={onClose}>Leave Without Saving</button>
            </div>
          </div>
        </div>
      )}

      {/* Unsaved bar for overview */}
      {hasOverviewChanges() && !unsavedConfirm && activeTab === 'overview' && (
        <div className="settings-unsaved-bar">
          <span className="settings-unsaved-text">You have unsaved changes</span>
          <button className="settings-btn settings-btn-ghost" onClick={() => { setName(server.name); setVanityUrl(server.vanity_url || ''); setIconUrl(server.icon_url || ''); }}>Reset</button>
          <button className="settings-btn settings-btn-primary" onClick={handleSaveOverview} disabled={overviewLoading}>
            {overviewLoading ? 'Saving...' : 'Save Changes'}
          </button>
        </div>
      )}
    </div>
  );
};
