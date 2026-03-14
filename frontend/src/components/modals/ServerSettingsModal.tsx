import React, { useState } from 'react';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { Server } from '../../api/types';
import { updateServer, deleteServer, createInvite } from '../../api/servers';

interface ServerSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  server: Server | null;
  onUpdate: (server: Server) => void;
  onDelete: () => void;
  onCreateInvite: (inviteCode: string) => void;
}

export const ServerSettingsModal: React.FC<ServerSettingsModalProps> = ({
  isOpen,
  onClose,
  server,
  onUpdate,
  onDelete,
  onCreateInvite,
}) => {
  const [name, setName] = useState(server?.name || '');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [inviteCode, setInviteCode] = useState<string | null>(null);
  const [showInvite, setShowInvite] = useState(false);

  React.useEffect(() => {
    if (server) {
      setName(server.name);
      setShowDeleteConfirm(false);
      setInviteCode(null);
      setShowInvite(false);
      setError('');
    }
  }, [server, isOpen]);

  const handleSave = async () => {
    if (!server || !name.trim()) {
      setError('Server name is required');
      return;
    }

    setLoading(true);
    setError('');
    try {
      const updated = await updateServer(server.id, name.trim(), server.icon_url);
      onUpdate(updated);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update server');
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async () => {
    if (!server) return;

    setLoading(true);
    setError('');
    try {
      await deleteServer(server.id);
      onDelete();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete server');
    } finally {
      setLoading(false);
    }
  };

  const handleCreateInvite = async () => {
    if (!server) return;

    setLoading(true);
    setError('');
    try {
      const invite = await createInvite(server.id);
      setInviteCode(invite.code);
      setShowInvite(true);
      onCreateInvite(invite.code);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create invite');
    } finally {
      setLoading(false);
    }
  };

  const copyInviteLink = () => {
    if (inviteCode) {
      navigator.clipboard.writeText(`${window.location.origin}/invite/${inviteCode}`);
    }
  };

  if (!server) return null;

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Server Settings">
      <div className="server-settings-modal">
        <div className="server-icon-preview">
          <div className="server-icon-large">
            {server.name.charAt(0).toUpperCase()}
          </div>
        </div>

        <div className="form-group">
          <label className="form-label">Server Name</label>
          <input
            className="form-input"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Enter server name"
          />
        </div>

        {error && <div className="modal-error">{error}</div>}

        <div className="server-settings-section">
          <h4>Invite People</h4>
          <p className="section-description">
            Generate an invite link to share with others
          </p>

          {showInvite && inviteCode ? (
            <div className="invite-link-display">
              <input
                className="form-input invite-link-input"
                type="text"
                readOnly
                value={`${window.location.origin}/invite/${inviteCode}`}
              />
              <Button variant="secondary" onClick={copyInviteLink}>
                Copy
              </Button>
            </div>
          ) : (
            <Button
              variant="primary"
              onClick={handleCreateInvite}
              loading={loading}
            >
              Create Invite
            </Button>
          )}
        </div>

        <div className="server-settings-section danger-zone">
          <h4>Danger Zone</h4>
          <p className="section-description">
            Once you delete a server, there is no going back.
          </p>

          {!showDeleteConfirm ? (
            <Button variant="danger" onClick={() => setShowDeleteConfirm(true)}>
              Delete Server
            </Button>
          ) : (
            <div className="delete-confirm">
              <p>Are you sure? This action cannot be undone.</p>
              <div className="delete-confirm-actions">
                <Button variant="secondary" onClick={() => setShowDeleteConfirm(false)}>
                  Cancel
                </Button>
                <Button variant="danger" onClick={handleDelete} loading={loading}>
                  Delete Forever
                </Button>
              </div>
            </div>
          )}
        </div>

        <div className="modal-actions">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleSave} loading={loading}>
            Save Changes
          </Button>
        </div>
      </div>
    </Modal>
  );
};