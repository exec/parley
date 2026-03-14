import React, { useState, useRef } from 'react';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { User } from '../../api/types';
import { updateProfile, resendVerification, changeEmail } from '../../api/auth';
import { uploadFile } from '../../api/upload';

interface UserSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  currentUser: User | null;
  onUpdate: (user: User) => void;
}

export const UserSettingsModal: React.FC<UserSettingsModalProps> = ({
  isOpen,
  onClose,
  currentUser,
  onUpdate,
}) => {
  const [username, setUsername] = useState(currentUser?.username || '');
  const [avatarUrl, setAvatarUrl] = useState(currentUser?.avatar_url || '');
  const [bannerUrl, setBannerUrl] = useState(currentUser?.banner_url || '');
  const [avatarUploading, setAvatarUploading] = useState(false);
  const [bannerUploading, setBannerUploading] = useState(false);
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [resendLoading, setResendLoading] = useState(false);
  const [resendMessage, setResendMessage] = useState('');
  const [showChangeEmail, setShowChangeEmail] = useState(false);
  const [newEmail, setNewEmail] = useState('');
  const [emailPassword, setEmailPassword] = useState('');
  const [emailChangeLoading, setEmailChangeLoading] = useState(false);
  const [emailChangeMessage, setEmailChangeMessage] = useState<{ text: string; ok: boolean } | null>(null);
  const avatarFileInputRef = useRef<HTMLInputElement>(null);
  const bannerFileInputRef = useRef<HTMLInputElement>(null);

  React.useEffect(() => {
    if (isOpen && currentUser) {
      setUsername(currentUser.username);
      setAvatarUrl(currentUser.avatar_url || '');
      setBannerUrl(currentUser.banner_url || '');
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
      setError('');
      setSuccess('');
      setResendMessage('');
      setShowChangeEmail(false);
      setNewEmail('');
      setEmailPassword('');
      setEmailChangeMessage(null);
    }
  }, [isOpen, currentUser]);

  const handleAvatarFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setAvatarUploading(true);
    setError('');
    try {
      const url = await uploadFile(file);
      setAvatarUrl(url);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to upload avatar');
    } finally {
      setAvatarUploading(false);
      if (avatarFileInputRef.current) avatarFileInputRef.current.value = '';
    }
  };

  const handleBannerFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setBannerUploading(true);
    setError('');
    try {
      const url = await uploadFile(file);
      setBannerUrl(url);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to upload banner');
    } finally {
      setBannerUploading(false);
      if (bannerFileInputRef.current) bannerFileInputRef.current.value = '';
    }
  };

  const handleSave = async () => {
    setError('');
    setSuccess('');

    if (!username.trim()) {
      setError('Username is required');
      return;
    }

    if (newPassword && newPassword !== confirmPassword) {
      setError('New passwords do not match');
      return;
    }

    if (newPassword && newPassword.length < 6) {
      setError('New password must be at least 6 characters');
      return;
    }

    const req: { username?: string; current_password?: string; new_password?: string; avatar_url?: string; banner_url?: string } = {};

    if (username.trim() !== currentUser?.username) {
      req.username = username.trim();
    }

    if (newPassword) {
      req.current_password = currentPassword;
      req.new_password = newPassword;
    }

    if (avatarUrl !== (currentUser?.avatar_url || '')) {
      req.avatar_url = avatarUrl || undefined;
    }

    if (bannerUrl !== (currentUser?.banner_url || '')) {
      req.banner_url = bannerUrl || undefined;
    }

    if (Object.keys(req).length === 0) {
      onClose();
      return;
    }

    setLoading(true);
    try {
      const updated = await updateProfile(req);
      // Update localStorage
      const stored = localStorage.getItem('user');
      if (stored) {
        const parsed = JSON.parse(stored);
        localStorage.setItem('user', JSON.stringify({
          ...parsed,
          username: updated.username,
          avatar_url: updated.avatar_url,
          banner_url: updated.banner_url,
        }));
      }
      onUpdate(updated);
      setSuccess('Profile updated successfully');
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : (err as { message?: string })?.message ?? 'Failed to update profile';
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="User Settings">
      <div className="user-settings-modal">
        <div className="settings-section">
          <h4>Profile Images</h4>

          <div className="form-group">
            <label className="form-label">Avatar</label>
            <div className="avatar-upload-section">
              <div className="avatar-preview">
                {avatarUrl ? (
                  <img src={avatarUrl} alt="Avatar" />
                ) : (
                  <span style={{ fontSize: 28, color: '#32CD32', fontWeight: 'bold' }}>
                    {(currentUser?.username || '?').charAt(0).toUpperCase()}
                  </span>
                )}
              </div>
              <div>
                <input
                  type="file"
                  accept="image/*"
                  ref={avatarFileInputRef}
                  style={{ display: 'none' }}
                  onChange={handleAvatarFileChange}
                />
                <button
                  className="upload-btn"
                  type="button"
                  disabled={avatarUploading || loading}
                  onClick={() => avatarFileInputRef.current?.click()}
                >
                  {avatarUploading ? 'Uploading...' : 'Change Avatar'}
                </button>
              </div>
            </div>
          </div>

          <div className="form-group">
            <label className="form-label">Banner</label>
            {bannerUrl && (
              <div className="banner-preview">
                <img src={bannerUrl} alt="Banner" />
              </div>
            )}
            <input
              type="file"
              accept="image/*"
              ref={bannerFileInputRef}
              style={{ display: 'none' }}
              onChange={handleBannerFileChange}
            />
            <button
              className="upload-btn"
              type="button"
              disabled={bannerUploading || loading}
              onClick={() => bannerFileInputRef.current?.click()}
            >
              {bannerUploading ? 'Uploading...' : bannerUrl ? 'Change Banner' : 'Upload Banner'}
            </button>
          </div>
        </div>

        <div className="settings-section">
          <h4>Account</h4>

          <div className="form-group">
            <label className="form-label">Username</label>
            <input
              className="form-input"
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="Enter username"
            />
          </div>

          <div className="form-group">
            <label className="form-label">Email</label>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <input
                className="form-input"
                type="email"
                value={currentUser?.email || ''}
                disabled
                style={{ flex: 1 }}
              />
              <button
                type="button"
                onClick={() => { setShowChangeEmail(v => !v); setEmailChangeMessage(null); setNewEmail(''); setEmailPassword(''); }}
                style={{ background: 'none', border: '1px solid #444', color: '#aaa', borderRadius: 4, padding: '6px 12px', cursor: 'pointer', fontSize: 12, whiteSpace: 'nowrap' }}
              >
                {showChangeEmail ? 'Cancel' : 'Change'}
              </button>
            </div>

            {/* Verification status */}
            {currentUser && currentUser.email_verified === false && !showChangeEmail && (
              <div style={{ marginTop: 8, padding: '8px 12px', background: '#2a1f00', border: '1px solid #664400', borderRadius: 6, display: 'flex', alignItems: 'center', gap: 12 }}>
                <span style={{ color: '#ffaa00', fontSize: 13 }}>Email not verified</span>
                <button
                  type="button"
                  disabled={resendLoading}
                  onClick={async () => {
                    setResendLoading(true);
                    setResendMessage('');
                    try {
                      await resendVerification();
                      setResendMessage('Verification email sent!');
                    } catch (err: unknown) {
                      setResendMessage((err as { message?: string })?.message || 'Failed to send email');
                    } finally {
                      setResendLoading(false);
                    }
                  }}
                  style={{ background: 'none', border: '1px solid #664400', color: '#ffaa00', borderRadius: 4, padding: '2px 10px', cursor: 'pointer', fontSize: 12 }}
                >
                  {resendLoading ? 'Sending...' : 'Resend'}
                </button>
                {resendMessage && (
                  <span style={{ fontSize: 12, color: resendMessage.includes('sent') ? '#32CD32' : '#ff4444' }}>{resendMessage}</span>
                )}
              </div>
            )}
            {currentUser?.email_verified && (
              <div style={{ marginTop: 6, fontSize: 12, color: '#32CD32' }}>✓ Email verified</div>
            )}

            {/* Change email form */}
            {showChangeEmail && (
              <div style={{ marginTop: 10, padding: '12px 14px', background: '#111', border: '1px solid #333', borderRadius: 6, display: 'flex', flexDirection: 'column', gap: 10 }}>
                <div>
                  <label style={{ fontSize: 12, color: '#aaa', display: 'block', marginBottom: 4 }}>New Email Address</label>
                  <input
                    className="form-input"
                    type="email"
                    value={newEmail}
                    onChange={e => setNewEmail(e.target.value)}
                    placeholder="new@example.com"
                    autoComplete="email"
                  />
                </div>
                <div>
                  <label style={{ fontSize: 12, color: '#aaa', display: 'block', marginBottom: 4 }}>Current Password</label>
                  <input
                    className="form-input"
                    type="password"
                    value={emailPassword}
                    onChange={e => setEmailPassword(e.target.value)}
                    placeholder="Confirm with your password"
                    autoComplete="current-password"
                  />
                </div>
                {emailChangeMessage && (
                  <span style={{ fontSize: 12, color: emailChangeMessage.ok ? '#32CD32' : '#ff4444' }}>{emailChangeMessage.text}</span>
                )}
                <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
                  <button
                    type="button"
                    disabled={emailChangeLoading || !newEmail.trim() || !emailPassword}
                    onClick={async () => {
                      setEmailChangeLoading(true);
                      setEmailChangeMessage(null);
                      try {
                        const updated = await changeEmail(newEmail.trim(), emailPassword);
                        const stored = localStorage.getItem('user');
                        if (stored) {
                          const parsed = JSON.parse(stored);
                          localStorage.setItem('user', JSON.stringify({ ...parsed, email: updated.email, email_verified: false }));
                        }
                        onUpdate(updated);
                        setEmailChangeMessage({ text: 'Email updated — check your inbox to verify the new address.', ok: true });
                        setNewEmail('');
                        setEmailPassword('');
                        setShowChangeEmail(false);
                      } catch (err: unknown) {
                        setEmailChangeMessage({ text: (err as { message?: string })?.message || 'Failed to change email', ok: false });
                      } finally {
                        setEmailChangeLoading(false);
                      }
                    }}
                    style={{ background: '#32CD32', border: 'none', color: '#000', borderRadius: 4, padding: '6px 16px', cursor: 'pointer', fontSize: 13, fontWeight: 600, opacity: emailChangeLoading || !newEmail.trim() || !emailPassword ? 0.5 : 1 }}
                  >
                    {emailChangeLoading ? 'Saving...' : 'Save Email'}
                  </button>
                </div>
                <p style={{ fontSize: 11, color: '#666', margin: 0 }}>Limited to 3 email changes and 3 resend attempts per day.</p>
              </div>
            )}

            {/* Show success from email change after form closes */}
            {!showChangeEmail && emailChangeMessage?.ok && (
              <div style={{ marginTop: 8, fontSize: 12, color: '#32CD32' }}>{emailChangeMessage.text}</div>
            )}
          </div>
        </div>

        <div className="settings-section">
          <h4>Change Password</h4>
          <p className="section-description">Leave blank to keep your current password.</p>

          <div className="form-group">
            <label className="form-label">Current Password</label>
            <input
              className="form-input"
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              placeholder="Enter current password"
              autoComplete="current-password"
            />
          </div>

          <div className="form-group">
            <label className="form-label">New Password</label>
            <input
              className="form-input"
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              placeholder="Enter new password"
              autoComplete="new-password"
            />
          </div>

          <div className="form-group">
            <label className="form-label">Confirm New Password</label>
            <input
              className="form-input"
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder="Confirm new password"
              autoComplete="new-password"
            />
          </div>
        </div>

        {error && <div className="modal-error">{error}</div>}
        {success && <div className="modal-success">{success}</div>}

        <div className="modal-actions">
          <Button variant="secondary" onClick={onClose}>Cancel</Button>
          <Button variant="primary" onClick={handleSave} loading={loading}>Save Changes</Button>
        </div>
      </div>
    </Modal>
  );
};
