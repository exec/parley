import React, { useState } from 'react';
import { Modal } from '../ui/Modal';
import { transferDmOwnership } from '../../api/dms';
import type { DmChannelMember } from '../../api/types';
import './GroupModals.css';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  channelId: string;
  members: DmChannelMember[];
  currentUserId: string;
  onTransferred: () => void;
}

export const TransferOwnershipModal: React.FC<Props> = ({
  isOpen, onClose, channelId, members, currentUserId, onTransferred,
}) => {
  const [target, setTarget] = useState<string>('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const others = members.filter(m => m.user_id !== currentUserId);

  const submit = async () => {
    if (!target) return;
    setBusy(true);
    setError(null);
    try {
      await transferDmOwnership(channelId, target);
      onTransferred();
      handleClose();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed');
    } finally {
      setBusy(false);
    }
  };

  const handleClose = () => {
    setTarget('');
    setError(null);
    onClose();
  };

  return (
    <Modal isOpen={isOpen} onClose={handleClose} title="Transfer Ownership">
      <div className="group-modal-body">
        <p className="group-modal-helper">
          The new owner will be the only one who can kick members. You'll still be a member.
        </p>
        <select
          className="form-input"
          value={target}
          onChange={e => setTarget(e.target.value)}
        >
          <option value="" disabled>Select new owner…</option>
          {others.map(m => (
            <option key={m.user_id} value={m.user_id}>
              {m.display_name || m.username}
            </option>
          ))}
        </select>
        {error && <div className="group-modal-error">{error}</div>}
        <div className="modal-actions">
          <button type="button" className="btn-secondary" onClick={handleClose}>Cancel</button>
          <button type="button" className="btn-primary" disabled={!target || busy} onClick={submit}>
            {busy ? 'Transferring…' : 'Transfer'}
          </button>
        </div>
      </div>
    </Modal>
  );
};
