import React, { useState, FormEvent } from 'react';
import { SITE_URL } from '../config';
import { Link, useNavigate, useLocation } from 'react-router-dom';
import { Button } from '../components/ui/Button';
import { apiClient } from '../api/client';
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
  const [method, setMethod] = useState<'email' | 'phone'>('email');
  const [username, setUsername] = useState('');
  const [email, setEmail] = useState('');
  const [phone, setPhone] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [smsConsent, setSmsConsent] = useState(false);
  const [preReleaseAck, setPreReleaseAck] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);
  // Set to true after account creation when no password was provided
  const [passkeySetupRequired, setPasskeySetupRequired] = useState(false);
  const [passkeyError, setPasskeyError] = useState('');
  const [passkeyLoading, setPasskeyLoading] = useState(false);

  const passkeyOnly = passkeySupported && password === '';

  const validate = (): boolean => {
    const e: Record<string, string> = {};
    if (!username.trim()) e.username = 'Username is required';
    else if (username.length < 2) e.username = 'Username must be at least 2 characters';
    else if (!/^[a-zA-Z0-9_]+$/.test(username)) e.username = 'Letters, numbers, and underscores only';

    if (method === 'email') {
      if (!email.trim()) e.email = 'Email is required';
      else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) e.email = 'Invalid email address';
    } else {
      if (!phone.trim()) e.phone = 'Phone number is required';
      else if (!/^\+?[1-9]\d{7,14}$/.test(phone.replace(/[\s\-()]/g, ''))) e.phone = 'Invalid phone number';
      if (!smsConsent) e.smsConsent = 'You must agree to receive SMS messages to continue';
    }

    if (password) {
      if (password.length < 8) e.password = 'Password must be at least 8 characters';
      if (!confirmPassword) e.confirmPassword = 'Please confirm your password';
      else if (password !== confirmPassword) e.confirmPassword = 'Passwords do not match';
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
      const body: Record<string, string> = { username };
      if (password) body.password = password;
      if (method === 'email') body.email = email;
      else body.phone = phone.replace(/[\s\-()]/g, '');

      const response = await fetch('/api/auth/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data.message || 'Registration failed');
      localStorage.setItem('token', data.token);
      localStorage.setItem('user', JSON.stringify(data.user));
      apiClient.setToken(data.token);

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
              <label className="input-label">Verification method</label>
              <div className="auth-method-toggle">
                <button
                  type="button"
                  className={`auth-method-btn${method === 'email' ? ' active' : ''}`}
                  onClick={() => { setMethod('email'); setSmsConsent(false); }}
                >Email</button>
                <button
                  type="button"
                  className={`auth-method-btn${method === 'phone' ? ' active' : ''}`}
                  onClick={() => setMethod('phone')}
                >Phone</button>
              </div>
              {method === 'email' ? (
                <>
                  <input
                    type="email"
                    className={`input ${errors.email ? 'input-error' : ''}`}
                    value={email}
                    onChange={e => setEmail(e.target.value)}
                    placeholder="Enter your email"
                    autoComplete="email"
                  />
                  {errors.email && <span className="input-error-message">{errors.email}</span>}
                </>
              ) : (
                <>
                  <input
                    type="tel"
                    className={`input ${errors.phone ? 'input-error' : ''}`}
                    value={phone}
                    onChange={e => setPhone(e.target.value)}
                    placeholder="+1 555 000 0000"
                    autoComplete="tel"
                  />
                  {errors.phone && <span className="input-error-message">{errors.phone}</span>}
                  <label className="auth-sms-consent">
                    <input
                      type="checkbox"
                      checked={smsConsent}
                      onChange={e => setSmsConsent(e.target.checked)}
                      className="auth-sms-checkbox"
                    />
                    <span className="auth-sms-text">
                      I agree to receive automated transactional SMS messages from Parley (up to 5 msgs/mo). Msg &amp; data rates may apply. Frequency may vary. Reply <strong>STOP</strong> to opt out or <strong>HELP</strong> for assistance. Your mobile number will not be sold or shared with third parties for promotional or marketing purposes.{' '}
                      <a href={`${SITE_URL}/privacy/`} target="_blank" rel="noopener noreferrer" className="auth-link">Terms &amp; Privacy Policy</a>.
                    </span>
                  </label>
                  {errors.smsConsent && <span className="input-error-message">{errors.smsConsent}</span>}
                </>
              )}
            </div>

            <div className="input-wrapper">
              <label className="input-label">Password</label>
              <input
                type="password"
                className={`input ${errors.password ? 'input-error' : ''}`}
                value={password}
                onChange={e => setPassword(e.target.value)}
                placeholder={passkeySupported ? 'Leave empty to set up passkey authentication' : 'Create a password'}
                autoComplete="new-password"
              />
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
                <input
                  type="password"
                  className={`input ${errors.confirmPassword ? 'input-error' : ''}`}
                  value={confirmPassword}
                  onChange={e => setConfirmPassword(e.target.value)}
                  placeholder="Confirm your password"
                  autoComplete="new-password"
                />
                {errors.confirmPassword && <span className="input-error-message">{errors.confirmPassword}</span>}
              </div>
            )}

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
            <Link to="/login" className="auth-link">Login</Link>
          </div>
        </div>
      </div>
    </div>
  );
};
