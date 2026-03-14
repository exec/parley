import React, { useState } from 'react';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';

interface CreateServerModalProps {
  isOpen: boolean;
  onClose: () => void;
  onCreate: (name: string) => Promise<void>;
}

export const CreateServerModal: React.FC<CreateServerModalProps> = ({ isOpen, onClose, onCreate }) => {
  const [name, setName] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setError('Server name is required');
      return;
    }
    setLoading(true);
    setError('');
    try {
      await onCreate(name.trim());
      setName('');
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create server');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Create a Server">
      <form onSubmit={handleSubmit} className="modal-form">
        <p className="modal-description">
          Give your server a name to get started.
        </p>
        {error && <div className="modal-error">{error}</div>}
        <div className="form-group">
          <label className="form-label">Server Name</label>
          <input
            className="form-input"
            type="text"
            value={name}
            onChange={e => setName(e.target.value)}
            placeholder="Enter server name"
            autoFocus
          />
        </div>
        <div className="modal-actions">
          <Button type="button" variant="secondary" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={loading}>Create Server</Button>
        </div>
      </form>
    </Modal>
  );
};
