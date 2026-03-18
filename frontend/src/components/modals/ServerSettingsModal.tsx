import React, { useState, useRef } from 'react';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { Server } from '../../api/types';
import { updateServer, deleteServer, createInvite, setVanityURL } from '../../api/servers';
import { uploadFile } from '../../api/upload';

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
  const [vanityUrl, setVanityUrl] = useState(server?.vanity_url || '');
  const [iconUrl, setIconUrl] = useState(server?.icon_url || '');
  const [iconUploading, setIconUploading] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [inviteCode, setInviteCode] = useState<string | null>(null);
  const [showInvite, setShowInvite] = useState(false);
  const iconFileInputRef = useRef<HTMLInputElement>(null);

  React.useEffect(() => {
    if (server) {
      setName(server.name);
      setVanityUrl(server.vanity_url || '');
      setIconUrl(server.icon_url || '');
      setShowDeleteConfirm(false);
      setInviteCode(null);
      setShowInvite(false);
      setError('');
    }
  }, [server, isOpen]);

  const handleIconFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setIconUploading(true);
    setError('');
    try {
      const url = await uploadFile(file);
      setIconUrl(url);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to upload icon');
    } finally {
      setIconUploading(false);
      // Reset input so the same file can be selected again if needed
      if (iconFileInputRef.current) iconFileInputRef.current.value = '';
    }
  };

  const handleSave = async () => {
    if (!server || !name.trim()) {
      setError('Server name is required');
      return;
    }

    setLoading(true);
    setError('');
    try {
      const updated = await updateServer(server.id, name.trim(), iconUrl || undefined);
      // Also save vanity URL if it changed
      if (vanityUrl.trim() !== (server.vanity_url || '')) {
        const withVanity = await setVanityURL(server.id, vanityUrl.trim());
        onUpdate(withVanity);
      } else {
        onUpdate(updated);
      }
      onClose();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : (err as { message?: string })?.message ?? 'Failed to update server';
      setError(msg);
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
        <div className="avatar-upload-section">
          <div className="avatar-preview">
            {iconUrl ? (
              <img src={iconUrl} alt="Server icon" />
            ) : (
              <span style={{ fontSize: 32, color: 'var(--parley-accent)', fontWeight: 'bold' }}>
                {server.name.charAt(0).toUpperCase()}
              </span>
            )}
          </div>
          <div>
            <input
              type="file"
              accept="image/*"
              ref={iconFileInputRef}
              style={{ display: 'none' }}
              onChange={handleIconFileChange}
            />
            <button
              className="upload-btn"
              type="button"
              disabled={iconUploading || loading}
              onClick={() => iconFileInputRef.current?.click()}
            >
              {iconUploading ? 'Uploading...' : 'Change Icon'}
            </button>
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

        <div className="form-group">
          <label className="form-label">Vanity URL</label>
          <div className="vanity-url-input-wrapper">
            <span className="vanity-url-prefix">{window.location.origin}/invite/</span>
            <input
              className="form-input vanity-url-input"
              type="text"
              value={vanityUrl}
              onChange={(e) => setVanityUrl(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))}
              placeholder="my-server"
              maxLength={32}
            />
          </div>
          <p className="vanity-url-hint">Letters, numbers, and hyphens only. Leave blank to disable.</p>
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