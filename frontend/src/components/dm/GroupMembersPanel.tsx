import React, { useState } from 'react';
import { useGroupMembers } from '../../hooks/useGroupMembers';
import { AddPeopleModal } from './AddPeopleModal';
import { LeaveGroupModal } from './LeaveGroupModal';
import { TransferOwnershipModal } from './TransferOwnershipModal';
import { RenameGroupModal } from './RenameGroupModal';
import { kickDmMember } from '../../api/dms';
import { X, UserPlus, UserCog, LogOut, Pencil } from 'lucide-react';
import './GroupMembersPanel.css';

interface Props {
  channelId: string;
  groupName?: string | null;
  ownerId: string | null | undefined;
  currentUserId: string;
  isOpen: boolean;
  onClose: () => void;
  onLeft?: () => void;
}

export const GroupMembersPanel: React.FC<Props> = ({
  channelId,
  groupName,
  ownerId,
  currentUserId,
  isOpen,
  onClose,
  onLeft,
}) => {
  const { members, refetch } = useGroupMembers(isOpen ? channelId : null);
  const [showAdd, setShowAdd] = useState(false);
  const [showLeave, setShowLeave] = useState(false);
  const [showTransfer, setShowTransfer] = useState(false);
  const [showRename, setShowRename] = useState(false);

  const isOwner = ownerId != null && ownerId === currentUserId;

  if (!isOpen) return null;

  const kick = async (userId: string, displayName: string) => {
    if (!window.confirm(`Remove ${displayName} from the group?`)) return;
    try {
      await kickDmMember(channelId, userId);
      void refetch();
    } catch (e) {
      window.alert(e instanceof Error ? e.message : 'Failed to remove member');
    }
  };

  return (
    <>
      <aside className="group-members-panel" role="dialog" aria-label="Group members">
        <header className="gmp-header">
          <h3 className="gmp-title">Members ({members.length})</h3>
          <button className="gmp-close" onClick={onClose} aria-label="Close members panel">
            <X size={18} />
          </button>
        </header>

        <div className="gmp-actions">
          <button className="gmp-action" onClick={() => setShowAdd(true)}>
            <UserPlus size={16} />
            <span>Add People</span>
          </button>
          <button className="gmp-action" onClick={() => setShowRename(true)}>
            <Pencil size={16} />
            <span>Rename Group</span>
          </button>
          {isOwner && (
            <button className="gmp-action" onClick={() => setShowTransfer(true)}>
              <UserCog size={16} />
              <span>Transfer Ownership</span>
            </button>
          )}
          <button className="gmp-action gmp-action-danger" onClick={() => setShowLeave(true)}>
            <LogOut size={16} />
            <span>Leave Group</span>
          </button>
        </div>

        <ul className="gmp-member-list">
          {members.map(m => {
            const displayName = m.display_name || m.username || 'Unknown';
            const isMemberOwner = ownerId === m.user_id;
            const canKick = isOwner && m.user_id !== currentUserId;
            return (
              <li key={m.user_id} className="gmp-member">
                <div className="gmp-avatar">
                  {m.avatar_url
                    ? <img src={m.avatar_url} alt="" />
                    : displayName.charAt(0).toUpperCase()}
                </div>
                <div className="gmp-member-info">
                  <span className="gmp-member-name">
                    {displayName}
                    {isMemberOwner && <span className="gmp-owner-badge" title="Owner">👑</span>}
                  </span>
                  {m.username && m.display_name && (
                    <span className="gmp-member-username">@{m.username}</span>
                  )}
                </div>
                {canKick && (
                  <button
                    className="gmp-kick-btn"
                    onClick={() => kick(m.user_id, displayName)}
                    aria-label={`Remove ${displayName}`}
                  >
                    Kick
                  </button>
                )}
              </li>
            );
          })}
        </ul>
      </aside>

      <AddPeopleModal
        isOpen={showAdd}
        onClose={() => setShowAdd(false)}
        context={{
          kind: 'add-to-group',
          channelId,
          existingMemberIds: members.map(m => m.user_id),
        }}
        onCompleted={() => {
          setShowAdd(false);
          void refetch();
        }}
      />

      <LeaveGroupModal
        isOpen={showLeave}
        onClose={() => setShowLeave(false)}
        channelId={channelId}
        isOwner={isOwner}
        members={members}
        currentUserId={currentUserId}
        onLeft={() => {
          setShowLeave(false);
          onClose();
          onLeft?.();
        }}
      />

      <TransferOwnershipModal
        isOpen={showTransfer}
        onClose={() => setShowTransfer(false)}
        channelId={channelId}
        members={members}
        currentUserId={currentUserId}
        onTransferred={() => {
          setShowTransfer(false);
          void refetch();
        }}
      />

      <RenameGroupModal
        isOpen={showRename}
        onClose={() => setShowRename(false)}
        channelId={channelId}
        currentName={groupName}
        onRenamed={() => {
          setShowRename(false);
          // The DM_CHANNEL_UPDATE WS event will refresh the chat header /
          // DM panel name. The system message lands via the standard
          // dm_message broadcast.
        }}
      />
    </>
  );
};
