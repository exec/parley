import React, { useState, useRef, useEffect } from 'react';
import { X } from 'lucide-react';
import {
  DndContext,
  DragEndEvent,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import {
  SortableContext,
  arrayMove,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { Server, ServerMember, Role, ServerCategory } from '../../api/types';
import { siteOrigin } from '../../config';
import { updateServer, deleteServer, setVanityURL, getMyPermissions } from '../../api/servers';
import { getServerCategories, getServerCategoryAssignments, setServerCategories } from '../../api/discovery';
import '../discovery/DiscoveryPage.css';
import { uploadFile } from '../../api/upload';
import {
  getServerRoles,
  createServerRole,
  deleteServerRole,
  updateServerRole,
  reorderServerRoles,
} from '../../api/roles';
import {
  PERMISSION_CATEGORIES,
  PERM_ALL,
  PERM_ADMINISTRATOR,
  PERM_MANAGE_SERVER,
  PERM_VIEW_AUDIT_LOG,
  hasPerm,
  permFromNumber,
  permToNumber,
} from '../../lib/permissions';
import { AuditLogTab } from './AuditLogTab';
import { BotsTab } from './BotsTab';
import { InvitesTab } from './InvitesTab';
import { MembersTab } from './MembersTab';
import { SoundboardTab } from './SoundboardTab';
import './Settings.css';

function arrayEquals(a: number[], b: number[]): boolean {
  if (a.length !== b.length) return false;
  const sa = [...a].sort();
  const sb = [...b].sort();
  return sa.every((v, i) => v === sb[i]);
}

interface SortableRoleItemProps {
  role: Role;
  active: boolean;
  onSelect: () => void;
}

const SortableRoleItem: React.FC<SortableRoleItemProps> = ({ role, active, onSelect }) => {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: role.id });
  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };
  return (
    <button
      ref={setNodeRef}
      style={style}
      className={`roles-list-item${active ? ' active' : ''}`}
      onClick={onSelect}
      {...attributes}
      {...listeners}
    >
      <span className="roles-list-color-dot" style={{ backgroundColor: role.color }} />
      <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {role.name}
      </span>
    </button>
  );
};

type Tab = 'overview' | 'roles' | 'invites' | 'members' | 'bots' | 'soundboard' | 'auditlog' | 'danger';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  server: Server | null;
  members: ServerMember[];
  onUpdate: (server: Server) => void;
  onDelete: () => void;
  onCreateInvite: (code: string) => void;
  initialTab?: Tab;
  currentUserId: string;
}

export const ServerSettings: React.FC<Props> = ({
  isOpen, onClose, server, members, onUpdate, onDelete, initialTab, currentUserId,
}) => {
  const [activeTab, setActiveTab] = useState<Tab>(initialTab ?? 'overview');
  const isOwner = server?.owner_id === currentUserId;

  // Overview fields
  const [name, setName] = useState('');
  const [vanityUrl, setVanityUrl] = useState('');
  const [iconUrl, setIconUrl] = useState('');
  const [description, setDescription] = useState('');
  const [isPublic, setIsPublic] = useState(false);
  const [selectedCategoryIds, setSelectedCategoryIds] = useState<number[]>([]);
  const [initialCategoryIds, setInitialCategoryIds] = useState<number[]>([]);
  const [allCategories, setAllCategories] = useState<ServerCategory[]>([]);
  const [iconUploading, setIconUploading] = useState(false);
  const [overviewLoading, setOverviewLoading] = useState(false);
  const [overviewError, setOverviewError] = useState('');
  const iconInputRef = useRef<HTMLInputElement>(null);

  // Roles state
  const [roles, setRoles] = useState<Role[]>([]);
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

  // Danger
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deleteConfirmName, setDeleteConfirmName] = useState('');
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
      setDescription(server.description ?? '');
      setIsPublic(server.is_public ?? false);
      setOverviewError('');

      // Fetch categories
      Promise.all([
        getServerCategories(),
        getServerCategoryAssignments(server.id),
      ]).then(([cats, assigned]) => {
        setAllCategories(cats);
        const ids = assigned.map(c => c.id);
        setSelectedCategoryIds(ids);
        setInitialCategoryIds(ids);
      }).catch(() => {});
      setShowDeleteConfirm(false);
      setDeleteConfirmName('');
      setDeleteError('');
      setCreatingRole(false);
      setSelectedRoleId(null);
      setRolesError('');
      setUnsavedConfirm(false);
      loadMyPerms();
    }
  }, [isOpen, server, initialTab]); // eslint-disable-line react-hooks/exhaustive-deps

  // Load roles when roles tab is active
  useEffect(() => {
    if (isOpen && activeTab === 'roles' && server) {
      loadRoles();
      loadMyPerms();
    }
  }, [isOpen, activeTab, server]); // eslint-disable-line react-hooks/exhaustive-deps

  // Load permissions when bots tab is active (needed for isAdmin check)
  useEffect(() => {
    if (isOpen && activeTab === 'bots' && server) {
      loadMyPerms();
    }
  }, [isOpen, activeTab, server]); // eslint-disable-line react-hooks/exhaustive-deps

  // Sort: @everyone always last, then by position descending (higher position = higher in hierarchy).
  const sortRolesForDisplay = (list: Role[]): Role[] =>
    [...list].sort((a, b) => {
      if (a.is_everyone) return 1;
      if (b.is_everyone) return -1;
      return (b.position ?? 0) - (a.position ?? 0);
    });

  const loadRoles = async () => {
    if (!server) return;
    setRolesLoading(true);
    try {
      const r = await getServerRoles(server.id);
      setRoles(sortRolesForDisplay(r ?? []));
    } catch (e) {
      setRolesError(e instanceof Error ? e.message : 'Failed to load roles');
    } finally {
      setRolesLoading(false);
    }
  };

  const roleSensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));

  const handleRoleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event;
    if (!server || !over || active.id === over.id) return;

    const nonEv = roles.filter(r => !r.is_everyone);
    const oldIdx = nonEv.findIndex(r => r.id === active.id);
    const newIdx = nonEv.findIndex(r => r.id === over.id);
    if (oldIdx === -1 || newIdx === -1) return;

    const reordered = arrayMove(nonEv, oldIdx, newIdx);
    const N = reordered.length;
    // Top (index 0) gets highest position; bottom gets 1. @everyone is pinned
    // to position 0 by the backend.
    const repositioned: Role[] = reordered.map((r, i) => ({ ...r, position: N - i }));
    const everyone = roles.filter(r => r.is_everyone).map(r => ({ ...r, position: 0 }));
    setRoles([...repositioned, ...everyone]);

    try {
      const refreshed = await reorderServerRoles(server.id, reordered.map(r => r.id));
      setRoles(sortRolesForDisplay(refreshed));
    } catch (e) {
      setRolesError(e instanceof Error ? e.message : 'Failed to reorder roles');
      loadRoles();
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
    iconUrl !== (server.icon_url || '') ||
    description !== (server.description ?? '') ||
    isPublic !== (server.is_public ?? false) ||
    !arrayEquals(selectedCategoryIds, initialCategoryIds)
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
      // is_public requires a vanity URL — guard client-side in case state drifted
      const effectiveIsPublic = isPublic && !!vanityUrl.trim();
      const updated = await updateServer(server.id, name.trim(), iconUrl || undefined, description, effectiveIsPublic);
      let finalServer = updated;
      if (vanityUrl.trim() !== (server.vanity_url || '')) {
        finalServer = await setVanityURL(server.id, vanityUrl.trim());
      }
      await setServerCategories(server.id, selectedCategoryIds);
      setInitialCategoryIds(selectedCategoryIds);
      onUpdate(finalServer);
    } catch (err: unknown) {
      setOverviewError(err instanceof Error ? err.message : 'Failed to update server');
    } finally {
      setOverviewLoading(false);
    }
  };

  /* ---- Roles handlers ---- */

  const handleAddRole = async () => {
    if (!server || !newRoleName.trim()) return;
    setRolesLoading(true);
    setRolesError('');
    try {
      const role = await createServerRole(server.id, newRoleName.trim(), newRoleColor, permToNumber(newRolePerms));
      setRoles(prev => sortRolesForDisplay([...prev, role]));
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
      setRoles(prev => sortRolesForDisplay(prev.map(r => r.id === selectedRoleId ? updated : r)));
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
    } catch (e) {
      setRolesError(e instanceof Error ? e.message : 'Failed to delete role');
    } finally {
      setRolesLoading(false);
    }
  };

  /* ---- Delete handler ---- */

  const handleDeleteServer = async () => {
    if (!server) return;
    if (deleteConfirmName !== server.name) {
      setDeleteError('Server name does not match');
      return;
    }
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
  const nonEveryoneRoles = roles.filter(r => !r.is_everyone);
  const everyoneRole = roles.find(r => r.is_everyone) ?? null;

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
          <button className={`settings-nav-item${activeTab === 'invites' ? ' active' : ''}`} onClick={() => setActiveTab('invites')}>
            Invites
          </button>
          <button className={`settings-nav-item${activeTab === 'members' ? ' active' : ''}`} onClick={() => setActiveTab('members')}>
            Members
          </button>
          <button className={`settings-nav-item${activeTab === 'bots' ? ' active' : ''}`} onClick={() => setActiveTab('bots')}>
            Bots
          </button>
          {hasPerm(myPerms, PERM_MANAGE_SERVER) && (
            <button className={`settings-nav-item${activeTab === 'soundboard' ? ' active' : ''}`} onClick={() => setActiveTab('soundboard')}>
              Soundboard
            </button>
          )}
          {(hasPerm(myPerms, PERM_VIEW_AUDIT_LOG) || isOwner) && (
            <button className={`settings-nav-item${activeTab === 'auditlog' ? ' active' : ''}`} onClick={() => setActiveTab('auditlog')}>
              Audit Log
            </button>
          )}
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
                  <span className="settings-vanity-prefix">{siteOrigin()}/invite/</span>
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

              {/* Description */}
              <div className="settings-section">
                <div className="settings-section-title">Description</div>
                <textarea
                  className="settings-form-input settings-bio-input"
                  value={description}
                  onChange={e => setDescription(e.target.value.slice(0, 200))}
                  placeholder="What's your server about?"
                  rows={3}
                  maxLength={200}
                  disabled={overviewLoading}
                />
                <div className="settings-form-hint" style={{ textAlign: 'right', marginTop: 4 }}>
                  {description.length} / 200
                </div>
              </div>

              {/* Server Directory */}
              <div className="settings-section">
                <div className="settings-section-title">Server Directory</div>
                <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: vanityUrl.trim() ? 'pointer' : 'not-allowed' }}>
                  <input
                    type="checkbox"
                    checked={isPublic}
                    onChange={e => setIsPublic(e.target.checked)}
                    disabled={!vanityUrl.trim() || overviewLoading}
                  />
                  <span>List this server in the public directory</span>
                </label>
                <div className="settings-form-hint">
                  {vanityUrl.trim()
                    ? 'Your server will appear in Discover when enabled.'
                    : 'A vanity URL is required to list your server publicly.'}
                </div>

                {isPublic && allCategories.length > 0 && (
                  <div style={{ marginTop: 12 }}>
                    <div className="settings-section-title" style={{ marginBottom: 8 }}>
                      Categories <span style={{ color: 'var(--text-muted, #72767d)', fontWeight: 400 }}>(up to 3)</span>
                    </div>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                      {allCategories.map(cat => {
                        const selected = selectedCategoryIds.includes(cat.id);
                        return (
                          <button
                            key={cat.id}
                            className={`discovery-cat-pill${selected ? ' active' : ''}`}
                            onClick={() => {
                              if (selected) {
                                setSelectedCategoryIds(ids => ids.filter(id => id !== cat.id));
                              } else if (selectedCategoryIds.length < 3) {
                                setSelectedCategoryIds(ids => [...ids, cat.id]);
                              }
                            }}
                            disabled={overviewLoading}
                            type="button"
                          >
                            {cat.name}
                          </button>
                        );
                      })}
                    </div>
                  </div>
                )}
              </div>

              <div style={{ display: 'flex', gap: 10, marginBottom: 32 }}>
                <button className="settings-btn settings-btn-primary" onClick={handleSaveOverview} disabled={overviewLoading}>
                  {overviewLoading ? 'Saving...' : 'Save Changes'}
                </button>
                {hasOverviewChanges() && (
                  <button className="settings-btn settings-btn-secondary" onClick={() => { setName(server.name); setVanityUrl(server.vanity_url || ''); setIconUrl(server.icon_url || ''); setDescription(server.description ?? ''); setIsPublic(server.is_public ?? false); setSelectedCategoryIds(initialCategoryIds); }}>
                    Reset
                  </button>
                )}
              </div>

            </>
          )}

          {activeTab === 'roles' && (
            <>
              <h2 className="settings-page-title">Roles</h2>
              {rolesError && <div className="settings-error" style={{ marginBottom: 12 }}>{rolesError}</div>}

              {(
                <div className="roles-panel">
                  {/* Left: role list */}
                  <div className="roles-list-col">
                    {rolesLoading && roles.length === 0 && <div style={{ fontSize: 12, color: 'var(--parley-text-muted)' }}>Loading...</div>}
                    <DndContext sensors={roleSensors} collisionDetection={closestCenter} onDragEnd={handleRoleDragEnd}>
                      <SortableContext items={nonEveryoneRoles.map(r => r.id)} strategy={verticalListSortingStrategy}>
                        {nonEveryoneRoles.map(role => (
                          <SortableRoleItem
                            key={role.id}
                            role={role}
                            active={selectedRoleId === role.id && !creatingRole}
                            onSelect={() => { setSelectedRoleId(role.id); setCreatingRole(false); }}
                          />
                        ))}
                      </SortableContext>
                    </DndContext>
                    {everyoneRole && (
                      <button
                        key={everyoneRole.id}
                        className={`roles-list-item everyone-role${selectedRoleId === everyoneRole.id && !creatingRole ? ' active' : ''}`}
                        onClick={() => { setSelectedRoleId(everyoneRole.id); setCreatingRole(false); }}
                        title="@everyone — base role for all members"
                      >
                        <span className="roles-list-color-dot" style={{ backgroundColor: everyoneRole.color }} />
                        <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          @everyone
                        </span>
                      </button>
                    )}
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
                          <span style={{ fontSize: 13, color: 'var(--parley-text-muted)' }}>{newRoleColor}</span>
                        </div>
                        <div className="settings-section-title" style={{ marginBottom: 8 }}>Permissions</div>
                        {PERMISSION_CATEGORIES.map(cat => (
                          <div key={cat.label} style={{ marginBottom: 16 }}>
                            <div style={{ fontSize: 11, color: 'var(--parley-accent)', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.8px', marginBottom: 6 }}>
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
                          <div style={{ fontSize: 12, color: 'var(--parley-text-muted)', marginBottom: 12, padding: '8px 10px', background: 'var(--parley-bg-secondary)', borderRadius: 4, border: '1px solid var(--parley-border)' }}>
                            @everyone is the base role assigned to all server members. It cannot be renamed or deleted.
                          </div>
                        )}
                        {!selectedRole.is_everyone && (
                          <div className="settings-form-group" style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                            <label className="settings-form-label" style={{ margin: 0 }}>Color</label>
                            <input type="color" value={editRoleColor} onChange={e => setEditRoleColor(e.target.value)}
                              style={{ width: 40, height: 32, border: 'none', background: 'none', cursor: 'pointer', padding: 0 }} />
                            <span style={{ fontSize: 13, color: 'var(--parley-text-muted)' }}>{editRoleColor}</span>
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
                            <span style={{ fontSize: 12, color: 'var(--parley-text-muted)' }}>Lower = higher priority</span>
                          </div>
                        )}
                        <div className="settings-section-title" style={{ marginBottom: 8 }}>Permissions</div>
                        {PERMISSION_CATEGORIES.map(cat => (
                          <div key={cat.label} style={{ marginBottom: 16 }}>
                            <div style={{ fontSize: 11, color: 'var(--parley-accent)', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.8px', marginBottom: 6 }}>
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
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'var(--parley-text-muted)', fontSize: 13 }}>
                        {roles.length === 0 ? 'No roles yet — create one!' : 'Select a role to edit'}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </>
          )}

          {activeTab === 'invites' && <InvitesTab server={server} />}

          {activeTab === 'members' && <MembersTab server={server} members={members} />}

          {activeTab === 'bots' && (
            <BotsTab
              serverId={Number(server.id)}
              isAdmin={hasPerm(myPerms, PERM_ADMINISTRATOR)}
            />
          )}

          {activeTab === 'soundboard' && server && (
            <SoundboardTab serverId={Number(server.id)} />
          )}

          {activeTab === 'auditlog' && server && (
            <AuditLogTab server={server} currentUserId={currentUserId} />
          )}

          {activeTab === 'danger' && (
            <>
              <h2 className="settings-page-title">Danger Zone</h2>

              {deleteError && <div className="settings-error">{deleteError}</div>}

              <div className="settings-danger-zone">
                <div className="settings-danger-title">Delete Server</div>
                <p className="settings-danger-desc">
                  Once you delete <strong style={{ color: 'var(--parley-danger)' }}>{server.name}</strong>, there is no going back.
                  All channels, messages, and roles will be permanently deleted.
                </p>
                {!showDeleteConfirm ? (
                  <button className="settings-btn settings-btn-danger" onClick={() => setShowDeleteConfirm(true)}>
                    Delete This Server
                  </button>
                ) : (
                  <div className="settings-delete-confirm">
                    <p>Type the server name <strong>{server.name}</strong> to confirm deletion, then click Delete Forever.</p>
                    <input
                      className="settings-form-input"
                      type="text"
                      value={deleteConfirmName}
                      onChange={e => setDeleteConfirmName(e.target.value)}
                      placeholder={server.name}
                      autoFocus
                      style={{ marginBottom: 10 }}
                    />
                    <div style={{ display: 'flex', gap: 8 }}>
                      <button className="settings-btn settings-btn-secondary" onClick={() => { setShowDeleteConfirm(false); setDeleteConfirmName(''); }}>Cancel</button>
                      <button
                        className="settings-btn settings-btn-danger"
                        onClick={handleDeleteServer}
                        disabled={deleteLoading || deleteConfirmName !== server.name}
                      >
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
          <div style={{ background: 'var(--parley-bg-secondary)', border: '1px solid var(--parley-border)', borderRadius: 8, padding: 28, maxWidth: 380, width: '90%' }}>
            <div style={{ fontSize: 16, fontWeight: 700, color: 'var(--parley-text-normal)', marginBottom: 8 }}>Unsaved Changes</div>
            <div style={{ fontSize: 13, color: 'var(--parley-text-muted)', marginBottom: 20 }}>You have unsaved changes. Are you sure you want to leave?</div>
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
          <button className="settings-btn settings-btn-ghost" onClick={() => { setName(server.name); setVanityUrl(server.vanity_url || ''); setIconUrl(server.icon_url || ''); setDescription(server.description ?? ''); setIsPublic(server.is_public ?? false); setSelectedCategoryIds(initialCategoryIds); }}>Reset</button>
          <button className="settings-btn settings-btn-primary" onClick={handleSaveOverview} disabled={overviewLoading}>
            {overviewLoading ? 'Saving...' : 'Save Changes'}
          </button>
        </div>
      )}
    </div>
  );
};
