import React, { useState } from 'react';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { ServerMember } from '../../api/types';

interface Role {
  id: string;
  name: string;
  color: string;
}

interface ManageRolesModalProps {
  isOpen: boolean;
  onClose: () => void;
  members: ServerMember[];
}

const DEFAULT_ROLES: Role[] = [
  { id: 'admin', name: 'Admin', color: '#f04747' },
  { id: 'moderator', name: 'Moderator', color: '#faa61a' },
  { id: 'member', name: 'Member', color: '#43b581' },
];

export const ManageRolesModal: React.FC<ManageRolesModalProps> = ({ isOpen, onClose, members }) => {
  const [roles, setRoles] = useState<Role[]>(DEFAULT_ROLES);
  const [newRoleName, setNewRoleName] = useState('');
  const [newRoleColor, setNewRoleColor] = useState('#99aab5');
  const [activeTab, setActiveTab] = useState<'roles' | 'members'>('roles');

  const addRole = () => {
    if (!newRoleName.trim()) return;
    setRoles(prev => [...prev, {
      id: Date.now().toString(),
      name: newRoleName.trim(),
      color: newRoleColor,
    }]);
    setNewRoleName('');
    setNewRoleColor('#99aab5');
  };

  const deleteRole = (id: string) => {
    setRoles(prev => prev.filter(r => r.id !== id));
  };

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

        {activeTab === 'roles' && (
          <div className="roles-content">
            <p className="modal-description">
              Roles let you organize server members and grant them permissions.
            </p>
            <div className="roles-list">
              {roles.map(role => (
                <div key={role.id} className="role-item">
                  <div className="role-color" style={{ backgroundColor: role.color }} />
                  <span className="role-name">{role.name}</span>
                  <button
                    className="role-delete-btn"
                    onClick={() => deleteRole(role.id)}
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
                onKeyDown={e => e.key === 'Enter' && addRole()}
              />
              <input
                type="color"
                value={newRoleColor}
                onChange={e => setNewRoleColor(e.target.value)}
                className="color-picker"
                title="Role color"
              />
              <Button type="button" variant="primary" onClick={addRole}>Add Role</Button>
            </div>
          </div>
        )}

        {activeTab === 'members' && (
          <div className="roles-content">
            <p className="modal-description">
              Assign roles to server members.
            </p>
            <div className="members-roles-list">
              {members.map(member => (
                <div key={member.id} className="member-role-item">
                  <div className="member-avatar-small">
                    {(member.user?.username || 'U').charAt(0).toUpperCase()}
                  </div>
                  <span className="member-name-small">
                    {member.user?.username || member.user_id}
                  </span>
                  <select className="role-select">
                    <option value="">No role</option>
                    {roles.map(role => (
                      <option key={role.id} value={role.id}>{role.name}</option>
                    ))}
                  </select>
                </div>
              ))}
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
