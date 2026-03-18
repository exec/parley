import React, { useState, FormEvent } from 'react';
import { Link, useNavigate, useLocation } from 'react-router-dom';
import { Button } from '../components/ui/Button';
import { loginWithPasskey } from '../api/passkeys';
import { apiClient } from '../api/client';
import './Auth.css';

interface LoginFormData {
  email: string;
  password: string;
}

interface LoginErrors {
  email?: string;
  password?: string;
  general?: string;
}

export const Login: React.FC = () => {
  const navigate = useNavigate();
  const { search } = useLocation();
  const redirectTo = (() => {
    const p = new URLSearchParams(search).get('redirect') || '/';
    return p.startsWith('/') && !p.startsWith('//') ? p : '/';
  })();
  const [formData, setFormData] = useState<LoginFormData>({
    email: '',
    password: '',
  });
  const [errors, setErrors] = useState<LoginErrors>({});
  const [loading, setLoading] = useState(false);
  const [passkeyLoading, setPasskeyLoading] = useState(false);

  const validate = (): boolean => {
    const newErrors: LoginErrors = {};

    if (!formData.email.trim()) {
      newErrors.email = 'Email or phone is required';
    }

    if (!formData.password) {
      newErrors.password = 'Password is required';
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setErrors({});

    if (!validate()) {
      return;
    }

    setLoading(true);

    try {
      const input = formData.email.trim();
      const isPhone = /^\+?[\d\s\-()]+$/.test(input) && input.replace(/\D/g, '').length >= 8;
      const body = isPhone
        ? { phone: input.replace(/[\s\-()]/g, ''), password: formData.password }
        : { email: input, password: formData.password };

      const response = await fetch('/api/auth/login', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(body),
      });

      const data = await response.json().catch(() => ({}));

      if (!response.ok) {
        throw new Error(data.message || 'Login failed');
      }

      localStorage.setItem('token', data.token);
      localStorage.setItem('user', JSON.stringify(data.user));
      apiClient.setToken(data.token);

      navigate(redirectTo);
    } catch (err) {
      setErrors({
        general: err instanceof Error ? err.message : 'Login failed. Please try again.',
      });
    } finally {
      setLoading(false);
    }
  };

  const handlePasskeyLogin = async () => {
    setErrors({});
    setPasskeyLoading(true);
    try {
      const data = await loginWithPasskey();
      localStorage.setItem('token', data.token);
      localStorage.setItem('user', JSON.stringify(data.user));
      apiClient.setToken(data.token);
      navigate(redirectTo);
    } catch (err) {
      setErrors({
        general: err instanceof Error ? err.message : 'Passkey authentication failed.',
      });
    } finally {
      setPasskeyLoading(false);
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, value } = e.target;
    setFormData((prev) => ({ ...prev, [name]: value }));
    if (errors[name as keyof LoginErrors]) {
      setErrors((prev) => ({ ...prev, [name]: undefined }));
    }
  };

  return (
    <div className="auth-page">
      <nav className="auth-nav">
        <a href="/" className="auth-nav-brand">Parley</a>
      </nav>
      <div className="auth-container">
        <div className="auth-card">
          <div className="auth-header">
            <h1 className="auth-title">Welcome back!</h1>
            <p className="auth-subtitle">We're so excited to see you again!</p>
          </div>

          <form onSubmit={handleSubmit} className="auth-form" noValidate>
            {errors.general && (
              <div className="auth-error-banner">{errors.general}</div>
            )}

            <div className="input-wrapper">
              <label htmlFor="email" className="input-label">
                Email or Phone
              </label>
              <input
                id="email"
                name="email"
                type="text"
                className={`input ${errors.email ? 'input-error' : ''}`}
                value={formData.email}
                onChange={handleChange}
                placeholder="Email or phone number"
                autoComplete="email"
              />
              {errors.email && (
                <span className="input-error-message">{errors.email}</span>
              )}
            </div>

            <div className="input-wrapper">
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
                <label htmlFor="password" className="input-label">
                  Password
                </label>
                <Link to="/forgot-password" className="auth-forgot-link">
                  Forgot password?
                </Link>
              </div>
              <input
                id="password"
                name="password"
                type="password"
                className={`input ${errors.password ? 'input-error' : ''}`}
                value={formData.password}
                onChange={handleChange}
                placeholder="Enter your password"
                autoComplete="current-password"
              />
              {errors.password && (
                <span className="input-error-message">{errors.password}</span>
              )}
            </div>

            <Button type="submit" variant="primary" size="lg" loading={loading}>
              Login
            </Button>
          </form>

          {typeof window !== 'undefined' && window.PublicKeyCredential && (
            <Button
              type="button"
              variant="secondary"
              size="lg"
              loading={passkeyLoading}
              onClick={handlePasskeyLogin}
              style={{ width: '100%', marginTop: 12 }}
            >
              🔑 Sign in with passkey
            </Button>
          )}

          <div className="auth-footer">
            <span className="auth-footer-text">Need an account?</span>{' '}
            <Link to="/register" className="auth-link">
              Register
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
};