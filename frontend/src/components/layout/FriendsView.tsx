import React, { useState } from 'react';
import { FriendUser, FriendRequestsResponse } from '../../api/types';
import './FriendsView.css';

type Tab = 'all' | 'pending' | 'add';

interface FriendsViewProps {
  friends: FriendUser[];
  friendRequests: FriendRequestsResponse;
  onlineUserIds: Set<string>;
  currentUserId: string;
  onMessage: (userId: string) => void;
  onAccept: (requestId: string) => Promise<void>;
  onDeclineOrCancel: (requestId: string) => Promise<void>;
  onRemove: (userId: string) => Promise<void>;
  onSendRequest: (username: string) => Promise<void>;
}

const FriendAvatar: React.FC<{ user: FriendUser; online: boolean }> = ({ user, online }) => (
  <div className="friends-avatar-wrap">
    <div className="friends-avatar">
      {user.avatar_url
        ? <img src={user.avatar_url} alt={user.username} style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: '50%' }} />
        : (user.display_name || user.username).charAt(0).toUpperCase()}
    </div>
    <span className={`friends-status-dot ${online ? 'online' : ''}`} />
  </div>
);

const FriendsView: React.FC<FriendsViewProps> = ({
  friends,
  friendRequests,
  onlineUserIds,
  onMessage,
  onAccept,
  onDeclineOrCancel,
  onRemove,
  onSendRequest,
}) => {
  const [tab, setTab] = useState<Tab>('all');
  const [addUsername, setAddUsername] = useState('');
  const [addFeedback, setAddFeedback] = useState<{ msg: string; ok: boolean } | null>(null);
  const [addLoading, setAddLoading] = useState(false);

  const pendingCount = friendRequests.incoming.length;

  const handleSendRequest = async () => {
    if (!addUsername.trim()) return;
    setAddLoading(true);
    setAddFeedback(null);
    try {
      await onSendRequest(addUsername.trim());
      setAddFeedback({ msg: `Friend request sent to ${addUsername}!`, ok: true });
      setAddUsername('');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to send request';
      setAddFeedback({ msg, ok: false });
    } finally {
      setAddLoading(false);
    }
  };

  const sortedFriends = [...friends].sort((a, b) => {
    const aOnline = onlineUserIds.has(a.id);
    const bOnline = onlineUserIds.has(b.id);
    if (aOnline !== bOnline) return aOnline ? -1 : 1;
    return (a.display_name || a.username).localeCompare(b.display_name || b.username);
  });

  return (
    <div className="friends-view">
      <div className="friends-view-header">
        <button className={`friends-tab ${tab === 'all' ? 'active' : ''}`} onClick={() => setTab('all')}>
          All Friends
        </button>
        <button className={`friends-tab ${tab === 'pending' ? 'active' : ''}`} onClick={() => setTab('pending')}>
          Pending
          {pendingCount > 0 && <span className="friends-tab-badge">{pendingCount}</span>}
        </button>
        <button className={`friends-tab ${tab === 'add' ? 'active' : ''}`} onClick={() => setTab('add')}>
          Add Friend
        </button>
      </div>

      <div className="friends-view-body">
        {tab === 'all' && (
          <>
            {sortedFriends.length === 0 ? (
              <div className="friends-empty">No friends yet. Add some using the Add Friend tab!</div>
            ) : (
              <>
                <div className="friends-section-label">All Friends — {sortedFriends.length}</div>
                {sortedFriends.map(f => (
                  <div key={f.id} className="friends-list-item">
                    <FriendAvatar user={f} online={onlineUserIds.has(f.id)} />
                    <span className="friends-name">{f.display_name || f.username}</span>
                    <div className="friends-actions">
                      <button className="friends-btn primary" onClick={() => onMessage(f.id)}>Message</button>
                      <button className="friends-btn danger" onClick={() => onRemove(f.id)}>Remove</button>
                    </div>
                  </div>
                ))}
              </>
            )}
          </>
        )}

        {tab === 'pending' && (
          <>
            {friendRequests.incoming.length === 0 && friendRequests.outgoing.length === 0 && (
              <div className="friends-empty">No pending friend requests.</div>
            )}

            {friendRequests.incoming.length > 0 && (
              <>
                <div className="friends-section-label">Incoming — {friendRequests.incoming.length}</div>
                {friendRequests.incoming.map(req => (
                  <div key={req.id} className="friends-list-item">
                    <FriendAvatar user={req.user} online={onlineUserIds.has(req.user.id)} />
                    <span className="friends-name">{req.user.display_name || req.user.username}</span>
                    <div className="friends-actions" style={{ opacity: 1 }}>
                      <button className="friends-btn primary" onClick={() => onAccept(req.id)}>Accept</button>
                      <button className="friends-btn danger" onClick={() => onDeclineOrCancel(req.id)}>Decline</button>
                    </div>
                  </div>
                ))}
              </>
            )}

            {friendRequests.outgoing.length > 0 && (
              <>
                <div className="friends-section-label">Outgoing — {friendRequests.outgoing.length}</div>
                {friendRequests.outgoing.map(req => (
                  <div key={req.id} className="friends-list-item">
                    <FriendAvatar user={req.user} online={onlineUserIds.has(req.user.id)} />
                    <span className="friends-name">{req.user.display_name || req.user.username}</span>
                    <div className="friends-actions" style={{ opacity: 1 }}>
                      <button className="friends-btn ghost" onClick={() => onDeclineOrCancel(req.id)}>Cancel</button>
                    </div>
                  </div>
                ))}
              </>
            )}
          </>
        )}

        {tab === 'add' && (
          <>
            <div className="friends-section-label">Add a Friend</div>
            <p style={{ fontSize: 14, color: 'var(--text-muted, #96989d)', marginBottom: 12 }}>
              Enter their exact username to send a friend request.
            </p>
            <div className="add-friend-form">
              <input
                className="add-friend-input"
                type="text"
                placeholder="Enter a username"
                value={addUsername}
                onChange={e => setAddUsername(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleSendRequest()}
                autoFocus
              />
              <button
                className="friends-btn primary"
                onClick={handleSendRequest}
                disabled={addLoading || !addUsername.trim()}
              >
                {addLoading ? '...' : 'Send Request'}
              </button>
            </div>
            {addFeedback && (
              <div className={`add-friend-feedback ${addFeedback.ok ? 'success' : 'error'}`}>
                {addFeedback.msg}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
};

export default FriendsView;
