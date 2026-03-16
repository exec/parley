import React, { useState, FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { Button } from '../components/ui/Button';
import './Auth.css';

export const ForgotPassword: React.FC = () => {
  const [email, setEmail] = useState('');
  const [error, setError] = useState('');
  const [submitted, setSubmitted] = useState(false);
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!email.trim()) {
      setError('Email is required');
      return;
    }
    setLoading(true);
    try {
      await fetch('/api/auth/forgot-password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email.trim() }),
      });
      setSubmitted(true);
    } catch {
      setError('Something went wrong. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="auth-page">
      <div className="auth-container">
        <div className="auth-card">
          <div className="auth-header">
            <h1 className="auth-title">Reset your password</h1>
            <p className="auth-subtitle">
              {submitted
                ? 'Check your email for a reset link.'
                : "Enter your email and we'll send you a reset link."}
            </p>
          </div>

          {!submitted ? (
            <form onSubmit={handleSubmit} className="auth-form" noValidate>
              {error && <div className="auth-error-banner">{error}</div>}

              <div className="input-wrapper">
                <label htmlFor="email" className="input-label">
                  Email
                </label>
                <input
                  id="email"
                  type="email"
                  className={`input ${error ? 'input-error' : ''}`}
                  value={email}
                  onChange={(e) => { setEmail(e.target.value); setError(''); }}
                  placeholder="Enter your email"
                  autoComplete="email"
                />
                {error && <span className="input-error-message">{error}</span>}
              </div>

              <Button type="submit" variant="primary" size="lg" loading={loading}>
                Send reset link
              </Button>
            </form>
          ) : (
            <div className="auth-success-banner">
              If an account with that email exists, you'll receive a password reset link shortly.
            </div>
          )}

          <div className="auth-footer">
            <Link to="/login" className="auth-link">Back to login</Link>
          </div>
        </div>
      </div>
    </div>
  );
};
