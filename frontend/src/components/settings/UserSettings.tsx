import React, { useState, useRef, useEffect, useCallback } from 'react';
import { User } from '../../api/types';
import { updateProfile, resendVerification, changeEmail, verifyPhone, resendPhone, changePhone } from '../../api/auth';
import { uploadFile } from '../../api/upload';
import { DeveloperTab } from './DeveloperTab';
import './Settings.css';

type Tab = 'account' | 'profile' | 'developer';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  currentUser: User | null;
  onUpdate: (user: User) => void;
}

export const UserSettings: React.FC<Props> = ({ isOpen, onClose, currentUser, onUpdate }) => {
  const [activeTab, setActiveTab] = useState<Tab>('account');

  // Profile fields
  const [username, setUsername] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [avatarUrl, setAvatarUrl] = useState('');
  const [bannerUrl, setBannerUrl] = useState('');
  const [bio, setBio] = useState('');

  // Password fields
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');

  // Email change
  const [showChangeEmail, setShowChangeEmail] = useState(false);
  const [newEmail, setNewEmail] = useState('');
  const [emailPassword, setEmailPassword] = useState('');
  const [emailChangeLoading, setEmailChangeLoading] = useState(false);
  const [emailChangeMsg, setEmailChangeMsg] = useState<{ text: string; ok: boolean } | null>(null);
  const [resendLoading, setResendLoading] = useState(false);
  const [resendMsg, setResendMsg] = useState('');

  // Phone change
  const [showChangePhone, setShowChangePhone] = useState(false);
  const [newPhone, setNewPhone] = useState('');
  const [phonePassword, setPhonePassword] = useState('');
  const [phoneChangeLoading, setPhoneChangeLoading] = useState(false);
  const [phoneChangeMsg, setPhoneChangeMsg] = useState<{ text: string; ok: boolean } | null>(null);
  const [phoneVerifyCode, setPhoneVerifyCode] = useState('');
  const [phoneVerifyLoading, setPhoneVerifyLoading] = useState(false);
  const [phoneResendLoading, setPhoneResendLoading] = useState(false);
  const [phoneResendMsg, setPhoneResendMsg] = useState('');
  const [smsConsent, setSmsConsent] = useState(false);

  // Upload states
  const [avatarUploading, setAvatarUploading] = useState(false);
  const [bannerUploading, setBannerUploading] = useState(false);

  // Submit state
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [unsavedConfirm, setUnsavedConfirm] = useState(false);

  const avatarInputRef = useRef<HTMLInputElement>(null);
  const bannerInputRef = useRef<HTMLInputElement>(null);

  // Reset state when opened
  useEffect(() => {
    if (isOpen && currentUser) {
      setUsername(currentUser.username || '');
      setDisplayName(currentUser.display_name || '');
      setAvatarUrl(currentUser.avatar_url || '');
      setBannerUrl(currentUser.banner_url || '');
      setBio(currentUser.bio || '');
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
      setError('');
      setSuccess('');
      setShowChangeEmail(false);
      setNewEmail('');
      setEmailPassword('');
      setEmailChangeMsg(null);
      setShowChangePhone(false);
      setNewPhone('');
      setPhonePassword('');
      setPhoneChangeMsg(null);
      setPhoneVerifyCode('');
      setSmsConsent(false);
      setResendMsg('');
      setPhoneResendMsg('');
      setUnsavedConfirm(false);
      setActiveTab('account');
    }
  }, [isOpen, currentUser]);

  // ESC to close
  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') attemptClose();
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [isOpen, username, avatarUrl, bannerUrl, newPassword]); // eslint-disable-line react-hooks/exhaustive-deps

  const hasChanges = useCallback(() => {
    if (!currentUser) return false;
    return (
      username !== currentUser.username ||
      displayName !== (currentUser.display_name || '') ||
      avatarUrl !== (currentUser.avatar_url || '') ||
      bannerUrl !== (currentUser.banner_url || '') ||
      bio !== (currentUser.bio || '') ||
      !!newPassword
    );
  }, [currentUser, username, displayName, avatarUrl, bannerUrl, bio, newPassword]);

  const handleReset = useCallback(() => {
    if (!currentUser) return;
    setUsername(currentUser.username || '');
    setDisplayName(currentUser.display_name || '');
    setAvatarUrl(currentUser.avatar_url || '');
    setBannerUrl(currentUser.banner_url || '');
    setBio(currentUser.bio || '');
    setNewPassword('');
    setCurrentPassword('');
    setConfirmPassword('');
    setError('');
    setSuccess('');
  }, [currentUser]);

  const attemptClose = () => {
    if (hasChanges()) {
      setUnsavedConfirm(true);
    } else {
      onClose();
    }
  };

  const handleAvatarUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
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
      if (avatarInputRef.current) avatarInputRef.current.value = '';
    }
  };

  const handleBannerUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
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
      if (bannerInputRef.current) bannerInputRef.current.value = '';
    }
  };

  const handleSave = async () => {
    setError('');
    setSuccess('');
    if (!username.trim()) { setError('Username is required'); return; }
    if (newPassword && newPassword !== confirmPassword) { setError('New passwords do not match'); return; }
    if (newPassword && newPassword.length < 6) { setError('New password must be at least 6 characters'); return; }

    const req: Record<string, string | undefined> = {};
    if (username.trim() !== currentUser?.username) req.username = username.trim();
    if (displayName !== (currentUser?.display_name || '')) req.display_name = displayName;
    if (newPassword) { req.current_password = currentPassword; req.new_password = newPassword; }
    if (avatarUrl !== (currentUser?.avatar_url || '')) req.avatar_url = avatarUrl || undefined;
    if (bannerUrl !== (currentUser?.banner_url || '')) req.banner_url = bannerUrl || undefined;
    if (bio !== (currentUser?.bio || '')) req.bio = bio;

    if (Object.keys(req).length === 0) { onClose(); return; }

    setLoading(true);
    try {
      const updated = await updateProfile(req);
      const stored = localStorage.getItem('user');
      if (stored) {
        const parsed = JSON.parse(stored);
        localStorage.setItem('user', JSON.stringify({ ...parsed, username: updated.username, display_name: updated.display_name, avatar_url: updated.avatar_url, banner_url: updated.banner_url, bio: updated.bio }));
      }
      onUpdate(updated);
      setSuccess('Profile updated successfully');
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to update profile');
    } finally {
      setLoading(false);
    }
  };

  if (!isOpen) return null;

  return (
    <div className="settings-overlay">
      {/* Sidebar */}
      <div className="settings-sidebar">
        <div className="settings-sidebar-group">
          <div className="settings-sidebar-group-label">User Settings</div>
          <button className={`settings-nav-item${activeTab === 'account' ? ' active' : ''}`} onClick={() => setActiveTab('account')}>
            My Account
          </button>
          <button className={`settings-nav-item${activeTab === 'profile' ? ' active' : ''}`} onClick={() => setActiveTab('profile')}>
            Profile
          </button>
          <button className={`settings-nav-item${activeTab === 'developer' ? ' active' : ''}`} onClick={() => setActiveTab('developer')}>
            Developer
          </button>
        </div>
        <div className="settings-nav-divider" />
        <div className="settings-sidebar-spacer" />
        <div className="settings-sidebar-group">
          <div className="settings-nav-divider" />
          <button className="settings-nav-item danger" onClick={attemptClose}>
            Close Settings
          </button>
        </div>
      </div>

      {/* Main content */}
      <div className="settings-main">
        <div className="settings-content">
          {activeTab === 'account' && <AccountTab
            currentUser={currentUser}
            username={username} setUsername={setUsername}
            currentPassword={currentPassword} setCurrentPassword={setCurrentPassword}
            newPassword={newPassword} setNewPassword={setNewPassword}
            confirmPassword={confirmPassword} setConfirmPassword={setConfirmPassword}
            showChangeEmail={showChangeEmail} setShowChangeEmail={setShowChangeEmail}
            newEmail={newEmail} setNewEmail={setNewEmail}
            emailPassword={emailPassword} setEmailPassword={setEmailPassword}
            emailChangeLoading={emailChangeLoading} setEmailChangeLoading={setEmailChangeLoading}
            emailChangeMsg={emailChangeMsg} setEmailChangeMsg={setEmailChangeMsg}
            resendLoading={resendLoading} setResendLoading={setResendLoading}
            resendMsg={resendMsg} setResendMsg={setResendMsg}
            showChangePhone={showChangePhone} setShowChangePhone={setShowChangePhone}
            newPhone={newPhone} setNewPhone={setNewPhone}
            phonePassword={phonePassword} setPhonePassword={setPhonePassword}
            phoneChangeLoading={phoneChangeLoading} setPhoneChangeLoading={setPhoneChangeLoading}
            phoneChangeMsg={phoneChangeMsg} setPhoneChangeMsg={setPhoneChangeMsg}
            phoneVerifyCode={phoneVerifyCode} setPhoneVerifyCode={setPhoneVerifyCode}
            phoneVerifyLoading={phoneVerifyLoading} setPhoneVerifyLoading={setPhoneVerifyLoading}
            phoneResendLoading={phoneResendLoading} setPhoneResendLoading={setPhoneResendLoading}
            phoneResendMsg={phoneResendMsg} setPhoneResendMsg={setPhoneResendMsg}
            smsConsent={smsConsent} setSmsConsent={setSmsConsent}
            onUpdate={onUpdate}
            loading={loading}
            error={error} setError={setError}
            success={success}
            onSave={handleSave}
          />}
          {activeTab === 'profile' && <ProfileTab
            currentUser={currentUser}
            displayName={displayName} setDisplayName={setDisplayName}
            avatarUrl={avatarUrl} setAvatarUrl={setAvatarUrl}
            bannerUrl={bannerUrl} setBannerUrl={setBannerUrl}
            bio={bio} setBio={setBio}
            avatarUploading={avatarUploading}
            bannerUploading={bannerUploading}
            avatarInputRef={avatarInputRef}
            bannerInputRef={bannerInputRef}
            onAvatarUpload={handleAvatarUpload}
            onBannerUpload={handleBannerUpload}
            username={username}
            loading={loading}
            error={error}
            success={success}
            onSave={handleSave}
            onReset={handleReset}
            hasChanges={hasChanges()}
          />}
          {activeTab === 'developer' && <DeveloperTab />}
        </div>
      </div>

      {/* Close button */}
      <div className="settings-close-wrap">
        <button className="settings-close-btn" onClick={attemptClose} title="Close (ESC)">×</button>
        <span className="settings-close-hint">ESC</span>
      </div>

      {/* Unsaved changes confirmation */}
      {unsavedConfirm && (
        <div style={{ position: 'fixed', inset: 0, zIndex: 1200, display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(0,0,0,0.7)' }}>
          <div style={{ background: '#111', border: '1px solid #2a2d32', borderRadius: 8, padding: 28, maxWidth: 400, width: '90%' }}>
            <div style={{ fontSize: 16, fontWeight: 700, color: '#ddd', marginBottom: 8 }}>Unsaved Changes</div>
            <div style={{ fontSize: 13, color: '#777', marginBottom: 24 }}>You have unsaved changes. What would you like to do?</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              <button className="settings-btn settings-btn-primary" style={{ justifyContent: 'center' }} onClick={() => { setUnsavedConfirm(false); handleSave().then(() => onClose()).catch(() => {}); }}>
                Save Changes
              </button>
              <button className="settings-btn settings-btn-danger" style={{ justifyContent: 'center' }} onClick={() => { handleReset(); setUnsavedConfirm(false); onClose(); }}>
                Abandon Changes
              </button>
              <button className="settings-btn settings-btn-ghost" style={{ justifyContent: 'center' }} onClick={() => setUnsavedConfirm(false)}>
                Keep Editing
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Unsaved changes bar */}
      {hasChanges() && !unsavedConfirm && (
        <div className="settings-unsaved-bar">
          <span className="settings-unsaved-text">You have unsaved changes</span>
          <button className="settings-btn settings-btn-ghost" onClick={handleReset}>Reset</button>
          <button className="settings-btn settings-btn-primary" onClick={handleSave} disabled={loading}>
            {loading ? 'Saving...' : 'Save Changes'}
          </button>
        </div>
      )}
    </div>
  );
};

/* ---- Account Tab ---- */

interface AccountTabProps {
  currentUser: User | null;
  username: string; setUsername: (v: string) => void;
  currentPassword: string; setCurrentPassword: (v: string) => void;
  newPassword: string; setNewPassword: (v: string) => void;
  confirmPassword: string; setConfirmPassword: (v: string) => void;
  showChangeEmail: boolean; setShowChangeEmail: (v: boolean) => void;
  newEmail: string; setNewEmail: (v: string) => void;
  emailPassword: string; setEmailPassword: (v: string) => void;
  emailChangeLoading: boolean; setEmailChangeLoading: (v: boolean) => void;
  emailChangeMsg: { text: string; ok: boolean } | null; setEmailChangeMsg: (v: { text: string; ok: boolean } | null) => void;
  resendLoading: boolean; setResendLoading: (v: boolean) => void;
  resendMsg: string; setResendMsg: (v: string) => void;
  showChangePhone: boolean; setShowChangePhone: (v: boolean) => void;
  newPhone: string; setNewPhone: (v: string) => void;
  phonePassword: string; setPhonePassword: (v: string) => void;
  phoneChangeLoading: boolean; setPhoneChangeLoading: (v: boolean) => void;
  phoneChangeMsg: { text: string; ok: boolean } | null; setPhoneChangeMsg: (v: { text: string; ok: boolean } | null) => void;
  phoneVerifyCode: string; setPhoneVerifyCode: (v: string) => void;
  phoneVerifyLoading: boolean; setPhoneVerifyLoading: (v: boolean) => void;
  phoneResendLoading: boolean; setPhoneResendLoading: (v: boolean) => void;
  phoneResendMsg: string; setPhoneResendMsg: (v: string) => void;
  smsConsent: boolean; setSmsConsent: (v: boolean) => void;
  onUpdate: (user: User) => void;
  loading: boolean;
  error: string; setError: (v: string) => void;
  success: string;
  onSave: () => void;
}

const AccountTab: React.FC<AccountTabProps> = (p) => {
  return (
    <>
      <h2 className="settings-page-title">My Account</h2>

      {p.error && <div className="settings-error">{p.error}</div>}
      {p.success && <div className="settings-success">{p.success}</div>}

      <div className="settings-section">
        <div className="settings-section-title">Username</div>
        <div className="settings-form-group">
          <input
            className="settings-form-input"
            type="text"
            value={p.username}
            onChange={e => p.setUsername(e.target.value)}
            placeholder="Username"
          />
        </div>
        <button className="settings-btn settings-btn-primary" onClick={p.onSave} disabled={p.loading}>
          {p.loading ? 'Saving...' : 'Save Username'}
        </button>
      </div>

      <div className="settings-section">
        <div className="settings-section-title">Email Address</div>
        <div className="settings-row">
          <div className="settings-row-info">
            <div className="settings-row-label">Email</div>
            <div className="settings-row-value">{p.currentUser?.email || '—'}</div>
          </div>
          {p.currentUser?.email_verified
            ? <span className="settings-row-badge verified">✓ Verified</span>
            : <span className="settings-row-badge unverified">Unverified</span>
          }
          <button
            className="settings-btn settings-btn-ghost"
            style={{ marginLeft: 12 }}
            onClick={() => { p.setShowChangeEmail(!p.showChangeEmail); p.setEmailChangeMsg(null); p.setNewEmail(''); p.setEmailPassword(''); }}
          >
            {p.showChangeEmail ? 'Cancel' : 'Change'}
          </button>
        </div>

        {p.currentUser && p.currentUser.email_verified === false && !p.showChangeEmail && (
          <div className="settings-verify-banner">
            <span className="settings-verify-text">Verification email not yet confirmed</span>
            <button
              className="settings-btn settings-btn-ghost"
              disabled={p.resendLoading}
              onClick={async () => {
                p.setResendLoading(true); p.setResendMsg('');
                try { await resendVerification(); p.setResendMsg('Sent!'); }
                catch (e: unknown) { p.setResendMsg((e as {message?:string})?.message || 'Failed'); }
                finally { p.setResendLoading(false); }
              }}
            >
              {p.resendLoading ? 'Sending...' : 'Resend'}
            </button>
            {p.resendMsg && <span style={{ fontSize: 12, color: p.resendMsg === 'Sent!' ? '#44cc44' : '#ff6666' }}>{p.resendMsg}</span>}
          </div>
        )}

        {p.showChangeEmail && (
          <div className="settings-inline-form">
            <div>
              <label className="settings-inline-label">New Email Address</label>
              <input className="settings-form-input" type="email" value={p.newEmail} onChange={e => p.setNewEmail(e.target.value)} placeholder="new@example.com" autoComplete="email" />
            </div>
            <div>
              <label className="settings-inline-label">Current Password</label>
              <input className="settings-form-input" type="password" value={p.emailPassword} onChange={e => p.setEmailPassword(e.target.value)} placeholder="Confirm with your password" autoComplete="current-password" />
            </div>
            {p.emailChangeMsg && <span style={{ fontSize: 12, color: p.emailChangeMsg.ok ? '#44cc44' : '#ff6666' }}>{p.emailChangeMsg.text}</span>}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
              <button
                className="settings-btn settings-btn-primary"
                disabled={p.emailChangeLoading || !p.newEmail.trim() || !p.emailPassword}
                onClick={async () => {
                  p.setEmailChangeLoading(true); p.setEmailChangeMsg(null);
                  try {
                    const updated = await changeEmail(p.newEmail.trim(), p.emailPassword);
                    const stored = localStorage.getItem('user');
                    if (stored) localStorage.setItem('user', JSON.stringify({ ...JSON.parse(stored), email: updated.email, email_verified: false }));
                    p.onUpdate(updated);
                    p.setEmailChangeMsg({ text: 'Email updated — check your inbox to verify.', ok: true });
                    p.setNewEmail(''); p.setEmailPassword(''); p.setShowChangeEmail(false);
                  } catch (e: unknown) { p.setEmailChangeMsg({ text: (e as {message?:string})?.message || 'Failed', ok: false }); }
                  finally { p.setEmailChangeLoading(false); }
                }}
              >
                {p.emailChangeLoading ? 'Saving...' : 'Save Email'}
              </button>
            </div>
            <p style={{ fontSize: 11, color: '#555', margin: 0 }}>Limited to 3 email changes per day.</p>
          </div>
        )}
        {!p.showChangeEmail && p.emailChangeMsg?.ok && (
          <div className="settings-verified-note">{p.emailChangeMsg.text}</div>
        )}
      </div>

      <div className="settings-section">
        <div className="settings-section-title">Phone Number</div>
        <div className="settings-row">
          <div className="settings-row-info">
            <div className="settings-row-label">Phone</div>
            <div className="settings-row-value">{p.currentUser?.phone_number || 'No phone number'}</div>
          </div>
          {p.currentUser?.phone_number && (
            p.currentUser.phone_verified
              ? <span className="settings-row-badge verified">✓ Verified</span>
              : <span className="settings-row-badge unverified">Unverified</span>
          )}
          <button
            className="settings-btn settings-btn-ghost"
            style={{ marginLeft: 12 }}
            onClick={() => { p.setShowChangePhone(!p.showChangePhone); p.setPhoneChangeMsg(null); p.setNewPhone(''); p.setPhonePassword(''); p.setSmsConsent(false); }}
          >
            {p.showChangePhone ? 'Cancel' : p.currentUser?.phone_number ? 'Change' : 'Add'}
          </button>
        </div>

        {p.currentUser?.phone_number && !p.currentUser.phone_verified && !p.showChangePhone && (
          <div style={{ marginTop: 8, padding: '10px 14px', background: '#2a1f00', border: '1px solid #664400', borderRadius: 6 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 8 }}>
              <span style={{ fontSize: 13, color: '#ffaa00' }}>Phone not yet verified</span>
              <button className="settings-btn settings-btn-ghost" disabled={p.phoneResendLoading}
                onClick={async () => {
                  p.setPhoneResendLoading(true); p.setPhoneResendMsg('');
                  try { await resendPhone(); p.setPhoneResendMsg('Code sent!'); }
                  catch (e: unknown) { p.setPhoneResendMsg((e as {message?:string})?.message || 'Failed'); }
                  finally { p.setPhoneResendLoading(false); }
                }}>
                {p.phoneResendLoading ? 'Sending...' : 'Resend code'}
              </button>
              {p.phoneResendMsg && <span style={{ fontSize: 12, color: p.phoneResendMsg.includes('sent') ? '#44cc44' : '#ff6666' }}>{p.phoneResendMsg}</span>}
            </div>
            <div className="settings-verify-input-row">
              <input className="settings-form-input" type="text" inputMode="numeric" maxLength={6}
                value={p.phoneVerifyCode} onChange={e => p.setPhoneVerifyCode(e.target.value.replace(/\D/g, ''))}
                placeholder="6-digit code" style={{ letterSpacing: 4 }} />
              <button className="settings-btn settings-btn-primary"
                disabled={p.phoneVerifyLoading || p.phoneVerifyCode.length !== 6}
                onClick={async () => {
                  p.setPhoneVerifyLoading(true);
                  try {
                    await verifyPhone(p.phoneVerifyCode);
                    p.onUpdate({ ...p.currentUser!, phone_verified: true });
                    const stored = localStorage.getItem('user');
                    if (stored) localStorage.setItem('user', JSON.stringify({ ...JSON.parse(stored), phone_verified: true }));
                    p.setPhoneVerifyCode('');
                  } catch (e: unknown) { p.setPhoneResendMsg((e as {message?:string})?.message || 'Invalid code'); }
                  finally { p.setPhoneVerifyLoading(false); }
                }}>
                {p.phoneVerifyLoading ? 'Verifying...' : 'Verify'}
              </button>
            </div>
          </div>
        )}

        {p.showChangePhone && (
          <div className="settings-inline-form">
            <div>
              <label className="settings-inline-label">Phone Number</label>
              <input className="settings-form-input" type="tel" value={p.newPhone} onChange={e => p.setNewPhone(e.target.value)} placeholder="+1 555 000 0000" autoComplete="tel" />
            </div>
            <div>
              <label className="settings-inline-label">Current Password</label>
              <input className="settings-form-input" type="password" value={p.phonePassword} onChange={e => p.setPhonePassword(e.target.value)} placeholder="Confirm with your password" autoComplete="current-password" />
            </div>
            <label style={{ display: 'flex', alignItems: 'flex-start', gap: 8, cursor: 'pointer' }}>
              <input type="checkbox" checked={p.smsConsent} onChange={e => p.setSmsConsent(e.target.checked)} style={{ marginTop: 2, flexShrink: 0, accentColor: '#32CD32' }} />
              <span style={{ fontSize: 11, color: '#555', lineHeight: 1.5 }}>
                I agree to receive automated transactional SMS messages (up to 5/mo). Msg &amp; data rates may apply. Reply <strong style={{ color: '#777' }}>STOP</strong> to opt out.{' '}
                <a href="https://parley.x86-64.com/privacy/" target="_blank" rel="noopener noreferrer" style={{ color: '#32CD32' }}>Privacy Policy</a>.
              </span>
            </label>
            {p.phoneChangeMsg && <span style={{ fontSize: 12, color: p.phoneChangeMsg.ok ? '#44cc44' : '#ff6666' }}>{p.phoneChangeMsg.text}</span>}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
              <button className="settings-btn settings-btn-primary"
                disabled={p.phoneChangeLoading || !p.newPhone.trim() || !p.phonePassword || !p.smsConsent}
                onClick={async () => {
                  p.setPhoneChangeLoading(true); p.setPhoneChangeMsg(null);
                  try {
                    const updated = await changePhone(p.newPhone.trim().replace(/[\s\-()]/g, ''), p.phonePassword);
                    const stored = localStorage.getItem('user');
                    if (stored) localStorage.setItem('user', JSON.stringify({ ...JSON.parse(stored), phone_number: updated.phone_number, phone_verified: false }));
                    p.onUpdate(updated);
                    p.setPhoneChangeMsg({ text: 'Phone updated — enter the code we just sent to verify.', ok: true });
                    p.setNewPhone(''); p.setPhonePassword(''); p.setShowChangePhone(false);
                  } catch (e: unknown) { p.setPhoneChangeMsg({ text: (e as {message?:string})?.message || 'Failed', ok: false }); }
                  finally { p.setPhoneChangeLoading(false); }
                }}>
                {p.phoneChangeLoading ? 'Saving...' : 'Save Phone'}
              </button>
            </div>
          </div>
        )}
        {!p.showChangePhone && p.phoneChangeMsg?.ok && (
          <div className="settings-verified-note">{p.phoneChangeMsg.text}</div>
        )}
      </div>

      <div className="settings-section">
        <div className="settings-section-title">Change Password</div>
        <div className="settings-form-group">
          <label className="settings-form-label">Current Password</label>
          <input className="settings-form-input" type="password" value={p.currentPassword} onChange={e => p.setCurrentPassword(e.target.value)} placeholder="Current password" autoComplete="current-password" />
        </div>
        <div className="settings-form-group">
          <label className="settings-form-label">New Password</label>
          <input className="settings-form-input" type="password" value={p.newPassword} onChange={e => p.setNewPassword(e.target.value)} placeholder="New password (min. 6 characters)" autoComplete="new-password" />
        </div>
        <div className="settings-form-group">
          <label className="settings-form-label">Confirm New Password</label>
          <input className="settings-form-input" type="password" value={p.confirmPassword} onChange={e => p.setConfirmPassword(e.target.value)} placeholder="Confirm new password" autoComplete="new-password" />
        </div>
        {p.newPassword && (
          <button className="settings-btn settings-btn-primary" onClick={p.onSave} disabled={p.loading}>
            {p.loading ? 'Saving...' : 'Update Password'}
          </button>
        )}
      </div>
    </>
  );
};

/* ---- Profile Tab ---- */

interface ProfileTabProps {
  currentUser: User | null;
  displayName: string; setDisplayName: (v: string) => void;
  avatarUrl: string; setAvatarUrl: (v: string) => void;
  bannerUrl: string; setBannerUrl: (v: string) => void;
  bio: string; setBio: (v: string) => void;
  avatarUploading: boolean;
  bannerUploading: boolean;
  avatarInputRef: React.RefObject<HTMLInputElement>;
  bannerInputRef: React.RefObject<HTMLInputElement>;
  onAvatarUpload: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onBannerUpload: (e: React.ChangeEvent<HTMLInputElement>) => void;
  username: string;
  loading: boolean;
  error: string;
  success: string;
  onSave: () => void;
  onReset: () => void;
  hasChanges: boolean;
}

const BIO_MAX = 1000;

const ProfileTab: React.FC<ProfileTabProps> = (p) => {
  return (
    <>
      <h2 className="settings-page-title">Profile</h2>

      {p.error && <div className="settings-error">{p.error}</div>}
      {p.success && <div className="settings-success">{p.success}</div>}

      <div className="settings-profile-split">
        <div className="settings-profile-form">
          <div className="settings-section">
            <div className="settings-section-title">Display Name</div>
            <input
              className="settings-form-input"
              type="text"
              value={p.displayName}
              onChange={e => p.setDisplayName(e.target.value.slice(0, 100))}
              placeholder={`${p.currentUser?.username || 'username'} (leave blank to use username)`}
              maxLength={100}
              disabled={p.loading}
            />
            <div className="settings-form-hint">This is the name shown in chat. Your username is still used for @mentions and your profile tag.</div>
          </div>

          <div className="settings-section">
            <div className="settings-section-title">About Me</div>
            <textarea
              className="settings-form-input settings-bio-input"
              value={p.bio}
              onChange={e => p.setBio(e.target.value.slice(0, BIO_MAX))}
              placeholder="Tell people a bit about yourself... (markdown supported)"
              rows={4}
              disabled={p.loading}
            />
            <div className="settings-form-hint" style={{ textAlign: 'right', marginTop: 4 }}>
              {p.bio.length} / {BIO_MAX}
            </div>
          </div>

          <div className="settings-section">
            <div className="settings-section-title">Avatar</div>
            <div className="settings-upload-row">
              <div className="settings-avatar-preview">
                {p.avatarUrl
                  ? <img src={p.avatarUrl} alt="Avatar" />
                  : <span>{(p.username || p.currentUser?.username || '?').charAt(0).toUpperCase()}</span>
                }
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                <input type="file" accept="image/*" ref={p.avatarInputRef} style={{ display: 'none' }} onChange={p.onAvatarUpload} />
                <button className="settings-upload-btn" disabled={p.avatarUploading || p.loading} onClick={() => p.avatarInputRef.current?.click()}>
                  {p.avatarUploading ? 'Uploading...' : 'Change Avatar'}
                </button>
                {p.avatarUrl && (
                  <button className="settings-upload-remove-btn" onClick={() => p.setAvatarUrl('')}>Remove Avatar</button>
                )}
              </div>
            </div>
          </div>

          <div className="settings-section">
            <div className="settings-section-title">Banner</div>
            {p.bannerUrl && (
              <div className="settings-banner-preview" style={{ backgroundImage: `url(${p.bannerUrl})` }} />
            )}
            <input type="file" accept="image/*" ref={p.bannerInputRef} style={{ display: 'none' }} onChange={p.onBannerUpload} />
            <div style={{ display: 'flex', gap: 8 }}>
              <button className="settings-upload-btn" disabled={p.bannerUploading || p.loading} onClick={() => p.bannerInputRef.current?.click()}>
                {p.bannerUploading ? 'Uploading...' : p.bannerUrl ? 'Change Banner' : 'Upload Banner'}
              </button>
              {p.bannerUrl && (
                <button className="settings-upload-remove-btn" onClick={() => p.setBannerUrl('')}>Remove</button>
              )}
            </div>
          </div>

          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, paddingTop: 8 }}>
            {p.hasChanges && (
              <button className="settings-btn settings-btn-ghost" onClick={p.onReset} disabled={p.loading}>
                Reset
              </button>
            )}
            <button className="settings-btn settings-btn-primary" onClick={p.onSave} disabled={p.loading}>
              {p.loading ? 'Saving...' : 'Save Changes'}
            </button>
          </div>
        </div>

        {/* Live preview */}
        <div className="settings-profile-preview">
          <div className="settings-preview-label">Preview</div>
          <div className="settings-preview-card">
            <div
              className="settings-preview-banner"
              style={{ backgroundImage: p.bannerUrl ? `url(${p.bannerUrl})` : undefined }}
            />
            <div className="settings-preview-avatar-row">
              <div className="settings-preview-avatar">
                {p.avatarUrl
                  ? <img src={p.avatarUrl} alt="Avatar" />
                  : <span>{(p.username || p.currentUser?.username || '?').charAt(0).toUpperCase()}</span>
                }
              </div>
            </div>
            <div className="settings-preview-body">
              <div className="settings-preview-name">{p.username || p.currentUser?.username || 'username'}</div>
              <div className="settings-preview-sub">@{(p.username || p.currentUser?.username || 'username').toLowerCase()}</div>
            </div>
          </div>
        </div>
      </div>
    </>
  );
};
