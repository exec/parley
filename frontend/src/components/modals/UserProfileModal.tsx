import { useState, useEffect } from 'react';
import { PublicUser } from '../../api/types';
import { getUser } from '../../api/users';
import { Spinner } from '../ui/Spinner';

interface UserProfileModalProps {
  isOpen: boolean;
  onClose: () => void;
  userId: string | null;
  onStartDm: (userId: string) => void;
}

export function UserProfileModal({ isOpen, onClose, userId, onStartDm }: UserProfileModalProps) {
  const [user, setUser] = useState<PublicUser | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    if (!isOpen || !userId) return;

    setIsLoading(true);
    getUser(userId)
      .then(setUser)
      .catch(console.error)
      .finally(() => setIsLoading(false));
  }, [isOpen, userId]);

  if (!isOpen) return null;

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content user-profile-modal" onClick={e => e.stopPropagation()}>
        <div className="user-profile-header">
          <div className="user-profile-cover" />
          <button className="modal-close profile-close" onClick={onClose}>&times;</button>
        </div>

        <div className="user-profile-body">
          {isLoading ? (
            <div className="profile-loading">
              <Spinner />
            </div>
          ) : user ? (
            <>
              <div className="user-profile-avatar">
                {user.username.charAt(0).toUpperCase()}
              </div>
              <div className="user-profile-info">
                <h2 className="user-profile-username">{user.username}</h2>
                <p className="user-profile-tag">@{user.username}</p>
              </div>

              <div className="user-profile-actions">
                <button
                  className="profile-action-btn primary"
                  onClick={() => {
                    onStartDm(user.id);
                    onClose();
                  }}
                >
                  Message
                </button>
              </div>
            </>
          ) : (
            <div className="profile-error">Could not load user</div>
          )}
        </div>
      </div>
    </div>
  );
}