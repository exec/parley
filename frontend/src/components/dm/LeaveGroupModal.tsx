import React, { useState } from 'react';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { leaveDm } from '../../api/dms';
import type { DmChannelMember } from '../../api/types';
import './GroupModals.css';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  channelId: string;
  isOwner: boolean;
  members: DmChannelMember[];
  currentUserId: string;
  onLeft: () => void;
}

export const LeaveGroupModal: React.FC<Props> = ({
  isOpen, onClose, channelId, isOwner, members, currentUserId, onLeft,
}) => {
  const [transferTo, setTransferTo] = useState<string>('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const others = members.filter(m => m.user_id !== currentUserId);

  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      await leaveDm(channelId, transferTo || undefined);
      onLeft();
      handleClose();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to leave');
    } finally {
      setBusy(false);
    }
  };

  const handleClose = () => {
    setTransferTo('');
    setError(null);
    onClose();
  };

  return (
    <Modal isOpen={isOpen} onClose={handleClose} title="Leave Group">
      <div className="group-modal-body">
        {isOwner ? (
          <>
            <p className="group-modal-helper">
              You're the owner of this group. You can pick a successor — or leave without transferring,
              in which case no one will be able to kick members afterwards.
            </p>
            <select
              className="form-input"
              value={transferTo}
              onChange={e => setTransferTo(e.target.value)}
            >
              <option value="">Leave without transferring (kick power evaporates)</option>
              {others.map(m => (
                <option key={m.user_id} value={m.user_id}>
                  Transfer to {m.display_name || m.username}
                </option>
              ))}
            </select>
          </>
        ) : (
          <p className="group-modal-helper">
            You'll stop receiving messages in this group. You can be re-added by any current member.
          </p>
        )}
        {error && <div className="group-modal-error">{error}</div>}
        <div className="modal-actions">
          <Button type="button" variant="secondary" onClick={handleClose}>Cancel</Button>
          <Button type="button" variant="danger" loading={busy} onClick={submit}>Leave</Button>
        </div>
      </div>
    </Modal>
  );
};
