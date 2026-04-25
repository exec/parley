import React, { useState } from 'react';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { UserMultiPicker } from './UserMultiPicker';
import { createGroupDm } from '../../api/dms';
import type { PublicUser } from '../../api/types';
import './GroupModals.css';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  onCreated: (channelId: string) => void;
}

export const CreateGroupModal: React.FC<Props> = ({ isOpen, onClose, onCreated }) => {
  const [picked, setPicked] = useState<PublicUser[]>([]);
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const canSubmit = picked.length >= 2 && !busy;

  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      const ch = await createGroupDm(picked.map(u => u.id), name.trim() || undefined);
      onCreated(ch.id);
      handleClose();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to create group');
    } finally {
      setBusy(false);
    }
  };

  const handleClose = () => {
    setPicked([]);
    setName('');
    setError(null);
    onClose();
  };

  return (
    <Modal isOpen={isOpen} onClose={handleClose} title="New Group">
      <div className="group-modal-body">
        <p className="group-modal-helper">Pick at least 2 people to create a group DM.</p>
        <UserMultiPicker selected={picked} onChange={setPicked} maxCount={99} />
        <input
          type="text"
          className="form-input"
          placeholder="Group name (optional)"
          value={name}
          onChange={e => setName(e.target.value)}
          maxLength={100}
        />
        {error && <div className="group-modal-error">{error}</div>}
        <div className="modal-actions">
          <Button type="button" variant="secondary" onClick={handleClose}>Cancel</Button>
          <Button type="button" variant="primary" disabled={!canSubmit} loading={busy} onClick={submit}>
            Create
          </Button>
        </div>
      </div>
    </Modal>
  );
};
