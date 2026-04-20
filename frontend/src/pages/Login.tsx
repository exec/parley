import React, { useState, FormEvent, useEffect, useRef } from 'react';
import { Link, useNavigate, useLocation } from 'react-router-dom';
import { Eye, EyeOff } from 'lucide-react';
import { Button } from '../components/ui/Button';
import { loginWithPasskey } from '../api/passkeys';
import { exchangeDesktopCode } from '../api/desktopAuth';
import { apiClient } from '../api/client';
import { isTauri, randomState, openInBrowser, onDeepLink } from '../lib/tauri';
import './Auth.css';

// URL the Tauri app opens in the system browser for the passkey handoff. In
// dev this defaults to the Vite dev server (same origin as the webview) so
// Safari can hit `/auth/desktop` and proxy `/api` locally. Prod Tauri builds
// must set VITE_SITE_URL to the deployed Parley URL before `npm run build`.
const DESKTOP_SITE_URL =
  (import.meta.env.VITE_SITE_URL as string) ||
  (typeof window !== 'undefined' ? window.location.origin : '');

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
  const [browserLoading, setBrowserLoading] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const stateRef = useRef<string>('');
  const inTauri = isTauri();

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

  // Tauri-only: open the site in the default browser for a passkey login, then
  // receive a one-time exchange code back via a parley:// deep link.
  const handleBrowserLogin = async () => {
    setErrors({});
    setBrowserLoading(true);
    stateRef.current = randomState();
    try {
      const url = `${DESKTOP_SITE_URL}/auth/desktop?state=${encodeURIComponent(stateRef.current)}`;
      await openInBrowser(url);
    } catch (err) {
      setErrors({ general: err instanceof Error ? err.message : 'Failed to open browser.' });
      setBrowserLoading(false);
    }
  };

  useEffect(() => {
    if (!inTauri) return;
    let cancelled = false;
    let unlisten: (() => void) | null = null;
    onDeepLink((url) => {
      if (cancelled) return;
      try {
        const parsed = new URL(url);
        if (parsed.host !== 'auth' && parsed.pathname !== '/auth') return;
        const code = parsed.searchParams.get('code');
        const state = parsed.searchParams.get('state');
        if (!code || !state || state !== stateRef.current) {
          setErrors({ general: 'Invalid desktop login response.' });
          setBrowserLoading(false);
          return;
        }
        exchangeDesktopCode(code, state)
          .then((data) => {
            localStorage.setItem('token', data.token);
            localStorage.setItem('user', JSON.stringify(data.user));
            apiClient.setToken(data.token);
            navigate(redirectTo);
          })
          .catch((err) => {
            setErrors({ general: err instanceof Error ? err.message : 'Desktop login failed.' });
          })
          .finally(() => setBrowserLoading(false));
      } catch {
        setErrors({ general: 'Invalid desktop login response.' });
        setBrowserLoading(false);
      }
    }).then((fn) => { unlisten = fn; });
    return () => {
      cancelled = true;
      if (unlisten) unlisten();
    };
  }, [inTauri, navigate, redirectTo]);

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
              <div className="auth-password-wrapper">
                <input
                  id="password"
                  name="password"
                  type={showPassword ? 'text' : 'password'}
                  className={`input ${errors.password ? 'input-error' : ''}`}
                  value={formData.password}
                  onChange={handleChange}
                  placeholder="Enter your password"
                  autoComplete="current-password"
                />
                <button
                  type="button"
                  className="auth-password-toggle"
                  onClick={() => setShowPassword((v) => !v)}
                  aria-label={showPassword ? 'Hide password' : 'Show password'}
                  aria-pressed={showPassword}
                  tabIndex={0}
                >
                  {showPassword ? <EyeOff size={18} aria-hidden="true" /> : <Eye size={18} aria-hidden="true" />}
                </button>
              </div>
              {errors.password && (
                <span className="input-error-message">{errors.password}</span>
              )}
            </div>

            <Button type="submit" variant="primary" size="lg" loading={loading}>
              Login
            </Button>
          </form>

          {inTauri ? (
            <Button
              type="button"
              variant="secondary"
              size="lg"
              loading={browserLoading}
              onClick={handleBrowserLogin}
              style={{ width: '100%', marginTop: 12 }}
            >
              🔑 Sign in with passkey (via browser)
            </Button>
          ) : (
            typeof window !== 'undefined' && window.PublicKeyCredential && (
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
            )
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