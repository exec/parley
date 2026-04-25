import React, { useState } from 'react';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { UserMultiPicker } from './UserMultiPicker';
import { createGroupDm, addDmMembers } from '../../api/dms';
import type { PublicUser } from '../../api/types';
import './GroupModals.css';

type Context =
  | { kind: 'spawn-from-1to1'; otherUserId: string; otherUserName: string }
  | { kind: 'add-to-group'; channelId: string; existingMemberIds: string[] };

interface Props {
  isOpen: boolean;
  onClose: () => void;
  context: Context;
  onCompleted: (channelIdToOpen: string) => void;
}

export const AddPeopleModal: React.FC<Props> = ({ isOpen, onClose, context, onCompleted }) => {
  const [picked, setPicked] = useState<PublicUser[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    if (picked.length === 0) return;
    setBusy(true);
    setError(null);
    try {
      if (context.kind === 'spawn-from-1to1') {
        const ch = await createGroupDm([context.otherUserId, ...picked.map(u => u.id)]);
        onCompleted(ch.id);
      } else {
        await addDmMembers(context.channelId, picked.map(u => u.id));
        onCompleted(context.channelId);
      }
      handleClose();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed');
    } finally {
      setBusy(false);
    }
  };

  const handleClose = () => {
    setPicked([]);
    setError(null);
    onClose();
  };

  const helper = context.kind === 'spawn-from-1to1'
    ? `This will create a new group with you, ${context.otherUserName}, and ${picked.length} other(s). Your existing 1:1 DM stays intact.`
    : `Anyone you add can see all messages going forward.`;

  const exclude = context.kind === 'add-to-group' ? context.existingMemberIds : [context.otherUserId];

  return (
    <Modal isOpen={isOpen} onClose={handleClose} title={context.kind === 'spawn-from-1to1' ? 'New Group' : 'Add People'}>
      <div className="group-modal-body">
        <p className="group-modal-helper">{helper}</p>
        <UserMultiPicker selected={picked} onChange={setPicked} excludeUserIds={exclude} />
        {error && <div className="group-modal-error">{error}</div>}
        <div className="modal-actions">
          <Button type="button" variant="secondary" onClick={handleClose}>Cancel</Button>
          <Button type="button" variant="primary" disabled={picked.length === 0} loading={busy} onClick={submit}>
            {context.kind === 'spawn-from-1to1' ? 'Create' : 'Add'}
          </Button>
        </div>
      </div>
    </Modal>
  );
};
