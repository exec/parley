import React, { useState, FormEvent } from 'react';
import { Link, useNavigate, useLocation } from 'react-router-dom';
import { Button } from '../components/ui/Button';
import { apiClient } from '../api/client';
import './Auth.css';

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
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);

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

    if (!password) e.password = 'Password is required';
    else if (password.length < 8) e.password = 'Password must be at least 8 characters';
    if (!confirmPassword) e.confirmPassword = 'Please confirm your password';
    else if (password !== confirmPassword) e.confirmPassword = 'Passwords do not match';
    setErrors(e);
    return Object.keys(e).length === 0;
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setErrors({});
    if (!validate()) return;
    setLoading(true);
    try {
      const body: Record<string, string> = { username, password };
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
      navigate(redirectTo);
    } catch (err) {
      setErrors({ general: err instanceof Error ? err.message : 'Registration failed. Please try again.' });
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="auth-page">
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
              <div style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
                <button
                  type="button"
                  onClick={() => { setMethod('email'); setSmsConsent(false); }}
                  style={{
                    flex: 1, padding: '8px 0', borderRadius: 4, border: '1px solid',
                    borderColor: method === 'email' ? '#32CD32' : '#444',
                    background: method === 'email' ? 'rgba(50,205,50,0.1)' : '#111',
                    color: method === 'email' ? '#32CD32' : '#aaa',
                    cursor: 'pointer', fontSize: 13, fontWeight: method === 'email' ? 600 : 400,
                  }}
                >Email</button>
                <button
                  type="button"
                  onClick={() => setMethod('phone')}
                  style={{
                    flex: 1, padding: '8px 0', borderRadius: 4, border: '1px solid',
                    borderColor: method === 'phone' ? '#32CD32' : '#444',
                    background: method === 'phone' ? 'rgba(50,205,50,0.1)' : '#111',
                    color: method === 'phone' ? '#32CD32' : '#aaa',
                    cursor: 'pointer', fontSize: 13, fontWeight: method === 'phone' ? 600 : 400,
                  }}
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
                  <label style={{ display: 'flex', alignItems: 'flex-start', gap: 8, marginTop: 10, cursor: 'pointer' }}>
                    <input
                      type="checkbox"
                      checked={smsConsent}
                      onChange={e => setSmsConsent(e.target.checked)}
                      style={{ marginTop: 2, flexShrink: 0, accentColor: '#32CD32' }}
                    />
                    <span style={{ fontSize: 11, color: '#888', lineHeight: 1.5 }}>
                      I agree to receive automated transactional SMS messages from Parley (up to 5 msgs/mo). Msg &amp; data rates may apply. Frequency may vary. Reply <strong>STOP</strong> to opt out or <strong>HELP</strong> for assistance. Your mobile number will not be sold or shared with third parties for promotional or marketing purposes.{' '}
                      <a href="https://parley.x86-64.com/privacy/" target="_blank" rel="noopener noreferrer" style={{ color: '#32CD32' }}>Terms &amp; Privacy Policy</a>.
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
                placeholder="Create a password"
                autoComplete="new-password"
              />
              {errors.password && <span className="input-error-message">{errors.password}</span>}
            </div>

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

            <Button type="submit" variant="primary" size="lg" loading={loading}>
              Register
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
