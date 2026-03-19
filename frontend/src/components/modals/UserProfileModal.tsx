import { useState, useEffect } from 'react';
import { PublicUser } from '../../api/types';
import { getUser } from '../../api/users';
import { Spinner } from '../ui/Spinner';
import MarkdownRenderer from '../ui/MarkdownRenderer';
import BadgeList from '../ui/BadgeList';

interface UserProfileModalProps {
  isOpen: boolean;
  onClose: () => void;
  userId: string | null;
  currentUserId?: string;
  onStartDm: (userId: string) => void;
  isOnline?: boolean;
}

function formatMemberSince(dateStr: string) {
  return new Date(dateStr).toLocaleDateString('en-US', { month: 'long', year: 'numeric' });
}

export function UserProfileModal({ isOpen, onClose, userId, currentUserId, onStartDm, isOnline }: UserProfileModalProps) {
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

  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  const isOwnProfile = userId === currentUserId;

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content user-profile-modal" onClick={e => e.stopPropagation()}>

        {/* Banner */}
        <div
          className="user-profile-header"
          style={user?.banner_url
            ? { backgroundImage: `url(${user.banner_url})`, backgroundSize: 'cover', backgroundPosition: 'center' }
            : undefined}
        >
          <button className="profile-close" onClick={onClose}>&times;</button>
        </div>

        {isLoading ? (
          <div className="profile-loading"><Spinner /></div>
        ) : user ? (
          <>
            {/* Avatar — absolutely positioned straddling banner/body seam */}
            <div className="user-profile-avatar">
              {user.avatar_url
                ? <img src={user.avatar_url} alt={user.username} style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: '50%' }} />
                : user.username.charAt(0).toUpperCase()
              }
            </div>

            <div className="user-profile-body">
              <div className="user-profile-name-row">
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, flex: 1, flexWrap: 'wrap' }}>
                  <h2 className="user-profile-username">{user.display_name || user.username}</h2>
                  {isOnline !== undefined && (
                    <span className={`profile-status-dot ${isOnline ? 'online' : 'offline'}`} title={isOnline ? 'Online' : 'Offline'} />
                  )}
                </div>
                {(user.badges ?? 0) > 0 && (
                  <div className="user-profile-badge-row">
                    <BadgeList badges={user.badges!} />
                  </div>
                )}
              </div>
              <p className="user-profile-tag">@{user.username.toLowerCase()}</p>

              {user.bio && (
                <>
                  <div className="user-profile-divider" />
                  <div className="user-profile-bio">
                    <MarkdownRenderer content={user.bio} mode="bio" />
                  </div>
                </>
              )}

              <div className="user-profile-divider" />

              <div className="user-profile-since">
                <svg width="13" height="13" viewBox="0 0 16 16" fill="none" style={{ flexShrink: 0, opacity: 0.5 }}>
                  <rect x="1" y="3" width="14" height="12" rx="2" stroke="#888" strokeWidth="1.5"/>
                  <path d="M1 7h14" stroke="#888" strokeWidth="1.5"/>
                  <path d="M5 1v4M11 1v4" stroke="#888" strokeWidth="1.5" strokeLinecap="round"/>
                </svg>
                <span className="user-profile-since-label">Member since</span>
                <span className="user-profile-since-value">{formatMemberSince(user.created_at)}</span>
              </div>

              {!isOwnProfile && (
                <div className="user-profile-actions">
                  <button
                    className="profile-action-btn primary"
                    onClick={() => { onStartDm(user.id); onClose(); }}
                  >
                    Message
                  </button>
                </div>
              )}
            </div>
          </>
        ) : (
          <div className="user-profile-body">
            <div className="profile-error">Could not load user</div>
          </div>
        )}
      </div>
    </div>
  );
}
