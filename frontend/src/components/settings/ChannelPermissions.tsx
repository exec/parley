import React, { useState, useEffect, useCallback } from 'react';
import { getServerRoles } from '../../api/roles';
import { getMembers } from '../../api/servers';
import { getOverwrites, upsertOverwrite, deleteOverwrite } from '../../api/overwrites';
import { PERMISSION_CATEGORIES } from '../../lib/permissions';
import type { Role, ServerMember } from '../../api/types';
import './ChannelPermissions.css';

interface Props {
  channelId: string;
  serverId: string;
  /** If the channel has a parent category, pass its ID for sync badge. */
  parentId?: string;
}

// tri-state: 'allow' | 'inherit' | 'deny'
type PermState = 'allow' | 'inherit' | 'deny';

function nextState(current: PermState): PermState {
  if (current === 'inherit') return 'allow';
  if (current === 'allow') return 'deny';
  return 'inherit';
}

interface OverwriteEdit {
  id?: string;          // undefined = new
  target_type: number;  // 0 = role, 1 = member
  target_id: string;
  target_name: string;
  allow: bigint;
  deny: bigint;
  modified: boolean;
  deleted: boolean;
}

const CHANNEL_PERMISSION_CATEGORIES = PERMISSION_CATEGORIES.filter(c => c.label !== 'General');

export const ChannelPermissions: React.FC<Props> = ({ channelId, serverId, parentId }) => {
  const [overwrites, setOverwrites] = useState<OverwriteEdit[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [members, setMembers] = useState<ServerMember[]>([]);
  const [selectedTargetId, setSelectedTargetId] = useState<string | null>(null);
  const [addType, setAddType] = useState<'role' | 'member'>('role');
  const [addSearch, setAddSearch] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [synced, setSynced] = useState<boolean | null>(null); // null = no parent

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const [fetchedOverwrites, fetchedRoles, fetchedMembers] = await Promise.all([
        getOverwrites(channelId),
        getServerRoles(serverId),
        getMembers(serverId),
      ]);

      // Build role/member name maps
      const roleMap = new Map(fetchedRoles.map(r => [r.id, r.name]));
      const memberMap = new Map(fetchedMembers.map(m => [m.user_id, m.nickname || m.username]));

      const edits: OverwriteEdit[] = fetchedOverwrites.map(ow => ({
        id: ow.id,
        target_type: ow.target_type,
        target_id: ow.target_id,
        target_name: ow.target_type === 0
          ? (roleMap.get(ow.target_id) ?? ow.target_id)
          : (memberMap.get(ow.target_id) ?? ow.target_id),
        allow: ow.allow,
        deny: ow.deny,
        modified: false,
        deleted: false,
      }));

      setOverwrites(edits);
      setRoles(fetchedRoles);
      setMembers(fetchedMembers);

      // Sync status — simplified: if parent exists, compare overwrite counts
      if (parentId) {
        setSynced(fetchedOverwrites.length === 0);
      } else {
        setSynced(null);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load permissions');
    } finally {
      setLoading(false);
    }
  }, [channelId, serverId, parentId]);

  useEffect(() => { load(); }, [load]);

  const getPermState = (ow: OverwriteEdit, bit: bigint): PermState => {
    if ((ow.allow & bit) === bit) return 'allow';
    if ((ow.deny & bit) === bit) return 'deny';
    return 'inherit';
  };

  const togglePerm = (targetId: string, bit: bigint) => {
    setOverwrites(prev => prev.map(ow => {
      if (ow.target_id !== targetId) return ow;
      const current = getPermState(ow, bit);
      const next = nextState(current);
      let allow = ow.allow;
      let deny = ow.deny;
      // Clear both first
      allow &= ~bit;
      deny &= ~bit;
      if (next === 'allow') allow |= bit;
      if (next === 'deny') deny |= bit;
      return { ...ow, allow, deny, modified: true };
    }));
  };

  const addTarget = (type: 'role' | 'member', id: string, name: string) => {
    // Don't add duplicates
    if (overwrites.some(ow => ow.target_id === id)) {
      setSelectedTargetId(id);
      setAddSearch('');
      return;
    }
    const newOw: OverwriteEdit = {
      target_type: type === 'role' ? 0 : 1,
      target_id: id,
      target_name: name,
      allow: 0n,
      deny: 0n,
      modified: true,
      deleted: false,
    };
    setOverwrites(prev => [...prev, newOw]);
    setSelectedTargetId(id);
    setAddSearch('');
  };

  const removeTarget = (targetId: string) => {
    setOverwrites(prev => prev.map(ow =>
      ow.target_id === targetId ? { ...ow, deleted: true, modified: true } : ow
    ));
    if (selectedTargetId === targetId) setSelectedTargetId(null);
  };

  const handleSave = async () => {
    setSaving(true);
    setError('');
    try {
      for (const ow of overwrites) {
        if (!ow.modified) continue;
        if (ow.deleted && ow.id) {
          await deleteOverwrite(channelId, ow.id);
        } else if (!ow.deleted) {
          await upsertOverwrite(channelId, {
            target_type: ow.target_type,
            target_id: ow.target_id,
            allow: ow.allow,
            deny: ow.deny,
          });
        }
      }
      // Reload to get fresh IDs and state
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save permissions');
    } finally {
      setSaving(false);
    }
  };

  const visibleOverwrites = overwrites.filter(ow => !ow.deleted);
  const selectedOw = visibleOverwrites.find(ow => ow.target_id === selectedTargetId) ?? null;
  const hasChanges = overwrites.some(ow => ow.modified);

  // Filter candidates for add dropdown
  const existingIds = new Set(visibleOverwrites.map(ow => ow.target_id));
  const filteredRoles = roles.filter(r =>
    !existingIds.has(r.id) &&
    (r.name.toLowerCase().includes(addSearch.toLowerCase()) || (r.is_everyone && '@everyone'.includes(addSearch.toLowerCase())))
  );
  const filteredMembers = members.filter(m =>
    !existingIds.has(m.user_id) &&
    (m.username.toLowerCase().includes(addSearch.toLowerCase()) ||
     (m.nickname ?? '').toLowerCase().includes(addSearch.toLowerCase()))
  );

  if (loading) {
    return <div className="cp-loading">Loading permissions...</div>;
  }

  return (
    <div className="cp-container">
      {error && <div className="cp-error">{error}</div>}

      {/* Sync badge */}
      {synced !== null && (
        <div className={`cp-sync-badge ${synced ? 'cp-sync-badge--synced' : 'cp-sync-badge--unsynced'}`}>
          {synced ? 'Synced with category' : 'Not synced with category'}
          {!synced && (
            <button
              className="cp-sync-btn"
              onClick={async () => {
                // Sync = delete all overwrites on this channel
                setSaving(true);
                try {
                  for (const ow of visibleOverwrites) {
                    if (ow.id) await deleteOverwrite(channelId, ow.id);
                  }
                  await load();
                } catch (e) {
                  setError(e instanceof Error ? e.message : 'Failed to sync');
                } finally {
                  setSaving(false);
                }
              }}
              disabled={saving}
            >
              Sync Now
            </button>
          )}
        </div>
      )}

      <div className="cp-layout">
        {/* Left column: target list + add */}
        <div className="cp-target-col">
          <div className="cp-section-label">Roles &amp; Members</div>

          {visibleOverwrites.map(ow => (
            <button
              key={ow.target_id}
              className={`cp-target-item ${selectedTargetId === ow.target_id ? 'cp-target-item--active' : ''}`}
              onClick={() => setSelectedTargetId(ow.target_id)}
            >
              <span className="cp-target-icon">{ow.target_type === 0 ? '⊕' : '◉'}</span>
              <span className="cp-target-name">{ow.target_name}</span>
              {ow.modified && <span className="cp-target-dot" title="Unsaved changes" />}
            </button>
          ))}

          {/* Add target section */}
          <div className="cp-add-section">
            <div className="cp-add-type-toggle">
              <button
                className={`cp-add-type-btn ${addType === 'role' ? 'active' : ''}`}
                onClick={() => setAddType('role')}
              >Role</button>
              <button
                className={`cp-add-type-btn ${addType === 'member' ? 'active' : ''}`}
                onClick={() => setAddType('member')}
              >Member</button>
            </div>
            <input
              className="cp-add-search"
              type="text"
              placeholder={`Search ${addType}s...`}
              value={addSearch}
              onChange={e => setAddSearch(e.target.value)}
            />
            {addSearch && (
              <div className="cp-add-dropdown">
                {addType === 'role' && filteredRoles.map(r => (
                  <button
                    key={r.id}
                    className="cp-add-item"
                    onClick={() => addTarget('role', r.id, r.is_everyone ? '@everyone' : r.name)}
                  >
                    <span className="cp-add-item-dot" style={{ backgroundColor: r.color }} />
                    {r.is_everyone ? '@everyone' : r.name}
                  </button>
                ))}
                {addType === 'member' && filteredMembers.map(m => (
                  <button
                    key={m.user_id}
                    className="cp-add-item"
                    onClick={() => addTarget('member', m.user_id, m.nickname || m.username)}
                  >
                    {m.nickname || m.username}
                  </button>
                ))}
                {addType === 'role' && filteredRoles.length === 0 && (
                  <div className="cp-add-empty">No roles found</div>
                )}
                {addType === 'member' && filteredMembers.length === 0 && (
                  <div className="cp-add-empty">No members found</div>
                )}
              </div>
            )}
          </div>
        </div>

        {/* Right column: permission grid */}
        <div className="cp-perm-col">
          {selectedOw ? (
            <>
              <div className="cp-perm-header">
                <div className="cp-perm-target-label">
                  <span className="cp-target-icon">{selectedOw.target_type === 0 ? '⊕' : '◉'}</span>
                  {selectedOw.target_name}
                </div>
                <button
                  className="cp-remove-btn"
                  onClick={() => removeTarget(selectedOw.target_id)}
                  title="Remove overwrite"
                >
                  Remove
                </button>
              </div>

              <div className="cp-perm-legend">
                <span className="cp-legend-allow">✓ Allow</span>
                <span className="cp-legend-inherit">— Inherit</span>
                <span className="cp-legend-deny">✕ Deny</span>
              </div>

              {CHANNEL_PERMISSION_CATEGORIES.map(cat => (
                <div key={cat.label} className="cp-perm-category">
                  <div className="cp-perm-category-label">{cat.label}</div>
                  {cat.permissions.map(p => {
                    const state = getPermState(selectedOw, p.bit);
                    return (
                      <div key={String(p.bit)} className="cp-perm-row">
                        <div className="cp-perm-info">
                          <div className="cp-perm-name">{p.name}</div>
                          <div className="cp-perm-desc">{p.description}</div>
                        </div>
                        <button
                          type="button"
                          className={`cp-tristate cp-tristate--${state}`}
                          onClick={() => togglePerm(selectedOw.target_id, p.bit)}
                          title={`Click to cycle: ${state} → ${nextState(state)}`}
                        >
                          {state === 'allow' ? '✓' : state === 'deny' ? '✕' : '—'}
                        </button>
                      </div>
                    );
                  })}
                </div>
              ))}
            </>
          ) : (
            <div className="cp-perm-empty">
              {visibleOverwrites.length === 0
                ? 'No permission overwrites yet. Add a role or member to customize permissions.'
                : 'Select a role or member on the left to edit their permissions.'}
            </div>
          )}
        </div>
      </div>

      {hasChanges && (
        <div className="cp-save-bar">
          <span className="cp-save-hint">You have unsaved changes</span>
          <button
            className="cp-save-btn"
            onClick={handleSave}
            disabled={saving}
          >
            {saving ? 'Saving...' : 'Save Changes'}
          </button>
          <button
            className="cp-reset-btn"
            onClick={load}
            disabled={saving}
          >
            Reset
          </button>
        </div>
      )}
    </div>
  );
};
