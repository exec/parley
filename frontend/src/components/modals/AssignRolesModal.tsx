import React, { useState, useEffect, useCallback } from 'react';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { Role } from '../../api/types';
import { getServerRoles, getMemberRoles, assignRoleToMember, removeRoleFromMember } from '../../api/roles';

interface AssignRolesModalProps {
  isOpen: boolean;
  onClose: () => void;
  serverId: string;
  userId: string;
  username: string;
}

export const AssignRolesModal: React.FC<AssignRolesModalProps> = ({
  isOpen, onClose, serverId, userId, username,
}) => {
  const [roles, setRoles] = useState<Role[]>([]);
  const [assigned, setAssigned] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    if (!serverId || !userId) return;
    setLoading(true);
    setError('');
    try {
      const [allRoles, memberRoles] = await Promise.all([
        getServerRoles(serverId),
        getMemberRoles(serverId, userId),
      ]);
      setRoles((allRoles ?? []).filter((r: Role) => !r.is_everyone));
      setAssigned(new Set((memberRoles ?? []).map((r: Role) => r.id)));
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load roles');
    } finally {
      setLoading(false);
    }
  }, [serverId, userId]);

  useEffect(() => {
    if (isOpen) load();
  }, [isOpen, load]);

  const handleToggle = async (roleId: string, currentlyAssigned: boolean) => {
    try {
      if (currentlyAssigned) {
        await removeRoleFromMember(serverId, userId, roleId);
        setAssigned(prev => { const s = new Set(prev); s.delete(roleId); return s; });
      } else {
        await assignRoleToMember(serverId, userId, roleId);
        setAssigned(prev => new Set([...prev, roleId]));
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to update role');
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={`Roles — ${username}`}>
      <div style={{ minWidth: 280 }}>
        {error && <div className="modal-error" style={{ marginBottom: 12 }}>{error}</div>}

        {loading ? (
          <div style={{ padding: '24px', textAlign: 'center', color: '#555', fontSize: 13 }}>Loading...</div>
        ) : roles.length === 0 ? (
          <div style={{ padding: '16px', color: '#555', fontSize: 13 }}>
            No roles exist for this server. Create some in Server Settings → Roles.
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginBottom: 16 }}>
            {roles.map(role => {
              const isAssigned = assigned.has(role.id);
              return (
                <label
                  key={role.id}
                  style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '8px 12px', background: 'var(--parley-bg-primary)', border: '1px solid var(--parley-border)', borderRadius: 4, cursor: 'pointer' }}
                >
                  <button
                    type="button"
                    className={`custom-toggle${isAssigned ? ' on' : ''}`}
                    onClick={() => handleToggle(role.id, isAssigned)}
                    aria-pressed={isAssigned}
                  />
                  <span style={{ width: 10, height: 10, borderRadius: '50%', backgroundColor: role.color, display: 'inline-block', flexShrink: 0 }} />
                  <span style={{ color: role.color, fontSize: 13, fontWeight: 600 }}>{role.name}</span>
                  {isAssigned && <span style={{ fontSize: 11, color: '#555', marginLeft: 'auto' }}>assigned</span>}
                </label>
              );
            })}
          </div>
        )}

        <div className="modal-actions">
          <Button variant="secondary" onClick={onClose}>Done</Button>
        </div>
      </div>
    </Modal>
  );
};
