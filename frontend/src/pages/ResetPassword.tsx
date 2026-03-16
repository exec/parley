import React, { useState, FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { Button } from '../components/ui/Button';
import './Auth.css';

export const ResetPassword: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const token = searchParams.get('token') ?? '';

  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState(false);

  if (!token) {
    return (
      <div className="auth-page">
        <div className="auth-container">
          <div className="auth-card">
            <div className="auth-header">
              <h1 className="auth-title">Invalid link</h1>
              <p className="auth-subtitle">
                This password reset link is invalid or has expired.
              </p>
            </div>
            <div className="auth-footer">
              <Link to="/forgot-password" className="auth-link">
                Request a new link
              </Link>
            </div>
          </div>
        </div>
      </div>
    );
  }

  if (success) {
    return (
      <div className="auth-page">
        <div className="auth-container">
          <div className="auth-card">
            <div className="auth-header">
              <h1 className="auth-title">Password reset!</h1>
              <p className="auth-subtitle">
                Your password has been updated. Redirecting to login…
              </p>
            </div>
            <div className="auth-footer">
              <Link to="/login" className="auth-link">Go to login</Link>
            </div>
          </div>
        </div>
      </div>
    );
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!password) { setError('Password is required'); return; }
    if (password.length < 8) { setError('Password must be at least 8 characters'); return; }
    if (password !== confirm) { setError('Passwords do not match'); return; }

    setLoading(true);
    try {
      const res = await fetch('/api/auth/reset-password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token, password }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.message || 'Reset failed');
      setSuccess(true);
      setTimeout(() => navigate('/login'), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Reset failed. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="auth-page">
      <div className="auth-container">
        <div className="auth-card">
          <div className="auth-header">
            <h1 className="auth-title">Choose a new password</h1>
            <p className="auth-subtitle">Must be at least 8 characters.</p>
          </div>

          <form onSubmit={handleSubmit} className="auth-form" noValidate>
            {error && <div className="auth-error-banner">{error}</div>}

            <div className="input-wrapper">
              <label htmlFor="password" className="input-label">
                New Password
              </label>
              <input
                id="password"
                type="password"
                className="input"
                value={password}
                onChange={(e) => { setPassword(e.target.value); setError(''); }}
                placeholder="Enter new password"
                autoComplete="new-password"
              />
            </div>

            <div className="input-wrapper">
              <label htmlFor="confirm" className="input-label">
                Confirm Password
              </label>
              <input
                id="confirm"
                type="password"
                className="input"
                value={confirm}
                onChange={(e) => { setConfirm(e.target.value); setError(''); }}
                placeholder="Confirm new password"
                autoComplete="new-password"
              />
            </div>

            <Button type="submit" variant="primary" size="lg" loading={loading}>
              Reset password
            </Button>
          </form>

          <div className="auth-footer">
            <Link to="/login" className="auth-link">Back to login</Link>
          </div>
        </div>
      </div>
    </div>
  );
};
