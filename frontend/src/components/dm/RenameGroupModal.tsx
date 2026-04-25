import React, { useState, useEffect } from 'react';
import { Modal } from '../ui/Modal';
import { renameDmGroup } from '../../api/dms';
import './GroupModals.css';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  channelId: string;
  currentName: string | null | undefined;
  onRenamed: (newName: string) => void;
}

export const RenameGroupModal: React.FC<Props> = ({
  isOpen, onClose, channelId, currentName, onRenamed,
}) => {
  const [name, setName] = useState(currentName ?? '');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (isOpen) {
      setName(currentName ?? '');
      setError(null);
    }
  }, [isOpen, currentName]);

  const trimmed = name.trim();
  const canSubmit = trimmed.length > 0 && trimmed.length <= 100 && trimmed !== (currentName ?? '') && !busy;

  const submit = async () => {
    if (!canSubmit) return;
    setBusy(true);
    setError(null);
    try {
      await renameDmGroup(channelId, trimmed);
      onRenamed(trimmed);
      onClose();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to rename');
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Rename Group">
      <div className="group-modal-body">
        <p className="group-modal-helper">
          A name change is logged in the chat as a system message. Anyone in the group can rename.
        </p>
        <input
          type="text"
          className="form-input"
          placeholder="Group name"
          value={name}
          onChange={e => setName(e.target.value)}
          maxLength={100}
          autoFocus
          onKeyDown={e => { if (e.key === 'Enter' && canSubmit) submit(); }}
        />
        {error && <div className="group-modal-error">{error}</div>}
        <div className="modal-actions">
          <button type="button" className="btn-secondary" onClick={onClose}>Cancel</button>
          <button type="button" className="btn-primary" disabled={!canSubmit} onClick={submit}>
            {busy ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </Modal>
  );
};
