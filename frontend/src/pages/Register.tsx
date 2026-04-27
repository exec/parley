import React, { useState, useEffect, FormEvent } from 'react';
import { Link, useNavigate, useLocation } from 'react-router-dom';
import { Eye, EyeOff } from 'lucide-react';
import { Button } from '../components/ui/Button';
import { apiClient, IS_DESKTOP as inTauri } from '../api/client';
import { registerPasskey } from '../api/passkeys';
import './Auth.css';

const passkeySupported = typeof window !== 'undefined' && !!window.PublicKeyCredential;

export const Register: React.FC = () => {
  const navigate = useNavigate();
  const { search } = useLocation();
  const redirectTo = (() => {
    const p = new URLSearchParams(search).get('redirect') || '/';
    return p.startsWith('/') && !p.startsWith('//') ? p : '/';
  })();
  // Parley is invite-only during early launch. Phone/SMS signup is disabled
  // while the SMS provider is nonfunctional; the code is kept in commented
  // branches below so it can be restored when that changes.
  // const [method, setMethod] = useState<'email' | 'phone'>('email');
  const [username, setUsername] = useState('');
  const [email, setEmail] = useState('');
  // const [phone, setPhone] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  // const [smsConsent, setSmsConsent] = useState(false);
  const [inviteCode, setInviteCode] = useState('');
  // DOB is computed client-side and discarded; only `is_adult: true` is sent
  // to the server. See PRIVACY.md §11.
  const [dobMonth, setDobMonth] = useState('');
  const [dobDay, setDobDay] = useState('');
  const [dobYear, setDobYear] = useState('');
  const [preReleaseAck, setPreReleaseAck] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);
  // Set to true after account creation when no password was provided
  const [passkeySetupRequired, setPasskeySetupRequired] = useState(false);
  // Prefill invite code from ?invite=... (e.g. a shared link)
  useEffect(() => {
    const fromQuery = new URLSearchParams(search).get('invite');
    if (fromQuery) setInviteCode(fromQuery);
  }, [search]);
  const [passkeyError, setPasskeyError] = useState('');
  const [passkeyLoading, setPasskeyLoading] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirmPassword, setShowConfirmPassword] = useState(false);

  const passkeyOnly = passkeySupported && password === '';

  const validate = (): boolean => {
    const e: Record<string, string> = {};
    if (!username.trim()) e.username = 'Username is required';
    else if (username.length < 2) e.username = 'Username must be at least 2 characters';
    else if (!/^[a-zA-Z0-9_]+$/.test(username)) e.username = 'Letters, numbers, and underscores only';

    if (!inviteCode.trim()) e.inviteCode = 'An invite code is required';

    if (!email.trim()) e.email = 'Email is required';
    else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) e.email = 'Invalid email address';

    // Phone/SMS signup is disabled while SMS is nonfunctional. Preserved for
    // re-enablement:
    // if (method === 'phone') {
    //   if (!phone.trim()) e.phone = 'Phone number is required';
    //   else if (!/^\+?[1-9]\d{7,14}$/.test(phone.replace(/[\s\-()]/g, ''))) e.phone = 'Invalid phone number';
    //   if (!smsConsent) e.smsConsent = 'You must agree to receive SMS messages to continue';
    // }

    if (password) {
      if (password.length < 8) e.password = 'Password must be at least 8 characters';
      if (!confirmPassword) e.confirmPassword = 'Please confirm your password';
      else if (password !== confirmPassword) e.confirmPassword = 'Passwords do not match';
    }
    if (!dobYear || !dobMonth || !dobDay) {
      e.dob = 'Please enter your date of birth';
    } else {
      const dob = new Date(parseInt(dobYear, 10), parseInt(dobMonth, 10) - 1, parseInt(dobDay, 10));
      const cutoff = new Date();
      cutoff.setFullYear(cutoff.getFullYear() - 18);
      if (Number.isNaN(dob.getTime()) || dob > cutoff) e.dob = 'You must be 18 or older to use Parley';
    }
    if (!preReleaseAck) e.preReleaseAck = 'You must acknowledge the pre-release disclaimer to continue';
    setErrors(e);
    return Object.keys(e).length === 0;
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setErrors({});
    if (!validate()) return;
    setLoading(true);
    try {
      // is_adult is the only thing transmitted from the DOB check —
      // the date itself stays in the browser. See PRIVACY.md §11.
      const body: Record<string, string | boolean> = { username, email, invite_code: inviteCode.trim(), is_adult: true };
      if (password) body.password = password;
      // Phone signup disabled — see note at top of file.
      // if (method === 'phone') body.phone = phone.replace(/[\s\-()]/g, '');

      const response = await fetch('/api/auth/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify(body),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data.message || 'Registration failed');
      localStorage.setItem('user', JSON.stringify(data.user));
      if (inTauri) {
        localStorage.setItem('token', data.token);
        apiClient.setToken(data.token);
      }

      if (!password) {
        // Passkey-only account: must complete setup before entering the app.
        setPasskeySetupRequired(true);
        setLoading(false);
        // Immediately kick off passkey creation.
        handlePasskeySetup();
        return;
      }

      navigate(redirectTo);
    } catch (err) {
      setErrors({ general: err instanceof Error ? err.message : 'Registration failed. Please try again.' });
    } finally {
      setLoading(false);
    }
  };

  const handlePasskeySetup = async () => {
    setPasskeyError('');
    setPasskeyLoading(true);
    try {
      await registerPasskey('My Passkey');
      navigate(redirectTo);
    } catch (err) {
      setPasskeyError(
        err instanceof Error && err.message
          ? err.message
          : 'Passkey setup failed or was cancelled. Please try again.'
      );
    } finally {
      setPasskeyLoading(false);
    }
  };

  // ── Passkey setup gate ────────────────────────────────────────────────────
  if (passkeySetupRequired) {
    return (
      <div className="auth-page">
        <nav className="auth-nav">
          <a href="/" className="auth-nav-brand">Parley</a>
        </nav>
        <div className="auth-container">
          <div className="auth-card">
            <div className="auth-header">
              <h1 className="auth-title">Set up your passkey</h1>
              <p className="auth-subtitle">
                Your account was created. Complete passkey setup to continue.
              </p>
            </div>
            <div className="auth-form">
              {passkeyError && <div className="auth-error-banner">{passkeyError}</div>}
              <p style={{ fontSize: 14, color: 'var(--parley-text-muted)', marginBottom: 16, lineHeight: 1.5 }}>
                Your browser will prompt you to save a passkey using Touch ID, Face ID, Windows Hello, or a security key. This replaces your password and is required to log in.
              </p>
              <Button
                type="button"
                variant="primary"
                size="lg"
                loading={passkeyLoading}
                onClick={handlePasskeySetup}
                style={{ width: '100%' }}
              >
                🔑 Set up passkey
              </Button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // ── Registration form ─────────────────────────────────────────────────────
  return (
    <div className="auth-page">
      <nav className="auth-nav">
        <a href="/" className="auth-nav-brand">Parley</a>
      </nav>
      <div className="auth-container">
        <div className="auth-card">
          <div className="auth-header">
            <h1 className="auth-title">Create an account</h1>
            <p className="auth-subtitle">Join the community and start chatting!</p>
          </div>
          <form onSubmit={handleSubmit} className="auth-form" noValidate>
            {errors.general && <div className="auth-error-banner">{errors.general}</div>}

            <div className="input-wrapper">
              <label className="input-label">Username</label>
              <input
                type="text"
                className={`input ${errors.username ? 'input-error' : ''}`}
                value={username}
                onChange={e => setUsername(e.target.value)}
                placeholder="Create a username"
                autoComplete="username"
              />
              {errors.username && <span className="input-error-message">{errors.username}</span>}
            </div>

            <div className="input-wrapper">
              <label className="input-label">Invite code</label>
              <input
                type="text"
                className={`input ${errors.inviteCode ? 'input-error' : ''}`}
                value={inviteCode}
                onChange={e => setInviteCode(e.target.value)}
                placeholder="Paste your invite code"
                autoComplete="off"
                autoCapitalize="off"
                autoCorrect="off"
                spellCheck={false}
              />
              {errors.inviteCode && <span className="input-error-message">{errors.inviteCode}</span>}
              <span className="input-hint">
                Parley is invite-only. Ask someone with an unused code to share one.
              </span>
            </div>

            <div className="input-wrapper">
              <label className="input-label">Email</label>
              <input
                type="email"
                className={`input ${errors.email ? 'input-error' : ''}`}
                value={email}
                onChange={e => setEmail(e.target.value)}
                placeholder="Enter your email"
                autoComplete="email"
              />
              {errors.email && <span className="input-error-message">{errors.email}</span>}
            </div>

            {/* Phone + SMS signup disabled while SMS is nonfunctional. Kept in
                history so it can be reinstated when the provider comes back.
                See commented blocks in validate() / handleSubmit() above. */}

            <div className="input-wrapper">
              <label className="input-label">Password</label>
              <div className="auth-password-wrapper">
                <input
                  type={showPassword ? 'text' : 'password'}
                  className={`input ${errors.password ? 'input-error' : ''}`}
                  value={password}
                  onChange={e => setPassword(e.target.value)}
                  placeholder={passkeySupported ? 'Leave empty to set up passkey authentication' : 'Create a password'}
                  autoComplete="new-password"
                />
                <button
                  type="button"
                  className="auth-password-toggle"
                  onClick={() => setShowPassword(v => !v)}
                  aria-label={showPassword ? 'Hide password' : 'Show password'}
                  aria-pressed={showPassword}
                  tabIndex={0}
                >
                  {showPassword ? <EyeOff size={18} aria-hidden="true" /> : <Eye size={18} aria-hidden="true" />}
                </button>
              </div>
              {errors.password && <span className="input-error-message">{errors.password}</span>}
              {passkeyOnly && (
                <span className="input-hint">
                  🔑 You'll be prompted to save a passkey after creating your account.
                </span>
              )}
            </div>

            {password && (
              <div className="input-wrapper">
                <label className="input-label">Confirm Password</label>
                <div className="auth-password-wrapper">
                  <input
                    type={showConfirmPassword ? 'text' : 'password'}
                    className={`input ${errors.confirmPassword ? 'input-error' : ''}`}
                    value={confirmPassword}
                    onChange={e => setConfirmPassword(e.target.value)}
                    placeholder="Confirm your password"
                    autoComplete="new-password"
                  />
                  <button
                    type="button"
                    className="auth-password-toggle"
                    onClick={() => setShowConfirmPassword(v => !v)}
                    aria-label={showConfirmPassword ? 'Hide password' : 'Show password'}
                    aria-pressed={showConfirmPassword}
                    tabIndex={0}
                  >
                    {showConfirmPassword ? <EyeOff size={18} aria-hidden="true" /> : <Eye size={18} aria-hidden="true" />}
                  </button>
                </div>
                {errors.confirmPassword && <span className="input-error-message">{errors.confirmPassword}</span>}
              </div>
            )}

            <div className="input-wrapper">
              <label className="input-label">Date of birth</label>
              <div style={{ display: 'flex', gap: 8 }}>
                <select
                  className={`input ${errors.dob ? 'input-error' : ''}`}
                  value={dobMonth}
                  onChange={e => setDobMonth(e.target.value)}
                  aria-label="Month"
                  style={{ flex: 1.4 }}
                >
                  <option value="">Month</option>
                  {[
                    'January', 'February', 'March', 'April', 'May', 'June',
                    'July', 'August', 'September', 'October', 'November', 'December',
                  ].map((name, i) => (
                    <option key={i + 1} value={String(i + 1)}>{name}</option>
                  ))}
                </select>
                <select
                  className={`input ${errors.dob ? 'input-error' : ''}`}
                  value={dobDay}
                  onChange={e => setDobDay(e.target.value)}
                  aria-label="Day"
                  style={{ flex: 1 }}
                >
                  <option value="">Day</option>
                  {Array.from({ length: 31 }, (_, i) => i + 1).map(d => (
                    <option key={d} value={String(d)}>{d}</option>
                  ))}
                </select>
                <select
                  className={`input ${errors.dob ? 'input-error' : ''}`}
                  value={dobYear}
                  onChange={e => setDobYear(e.target.value)}
                  aria-label="Year"
                  style={{ flex: 1.2 }}
                >
                  <option value="">Year</option>
                  {(() => {
                    const now = new Date().getFullYear();
                    return Array.from({ length: 101 }, (_, i) => now - i).map(y => (
                      <option key={y} value={String(y)}>{y}</option>
                    ));
                  })()}
                </select>
              </div>
              {errors.dob && <span className="input-error-message">{errors.dob}</span>}
              <span className="input-hint">
                Parley is for adults (18+). Your date of birth is checked locally and never sent to our servers.
              </span>
            </div>

            <div className="input-wrapper">
              <label style={{ display: 'flex', alignItems: 'flex-start', gap: 8, cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={preReleaseAck}
                  onChange={e => setPreReleaseAck(e.target.checked)}
                  style={{ marginTop: 2, flexShrink: 0, accentColor: '#00b4d8' }}
                />
                <span style={{ fontSize: 11, color: '#888', lineHeight: 1.5 }}>
                  I understand that Parley is <strong style={{ color: '#ccc' }}>pre-release software</strong> provided "as is" with no warranty of any kind. This service may contain bugs, experience data loss, or be discontinued at any time. <strong style={{ color: '#ccc' }}>Do not upload sensitive, confidential, or personally identifying information.</strong> Data stored here is not guaranteed to be secure or persistent. By creating an account you accept all risk associated with using pre-release software.
                </span>
              </label>
              {errors.preReleaseAck && <span className="input-error-message">{errors.preReleaseAck}</span>}
            </div>

            <Button type="submit" variant="primary" size="lg" loading={loading}>
              {passkeyOnly ? 'Continue' : 'Register'}
            </Button>
          </form>
          <div className="auth-footer">
            <span className="auth-footer-text">Already have an account?</span>{' '}
            <Link to={`/login${search}`} className="auth-link">Login</Link>
          </div>
        </div>
      </div>
    </div>
  );
};
