import React, { useState, useRef } from 'react';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { User } from '../../api/types';
import { updateProfile } from '../../api/auth';
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
            <input
              className="form-input"
              type="email"
              value={currentUser?.email || ''}
              disabled
              title="Email cannot be changed"
            />
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
