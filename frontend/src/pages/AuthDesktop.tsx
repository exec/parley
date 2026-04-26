import { useEffect, useState, FormEvent } from 'react';
import { useNavigate, useLocation, Link } from 'react-router-dom';
import { Button } from '../components/ui/Button';
import { apiClient, IS_DESKTOP } from '../api/client';
import { loginWithPasskey } from '../api/passkeys';
import { issueDesktopCode } from '../api/desktopAuth';
import './Auth.css';

type Status = 'login' | 'issuing' | 'ready' | 'error';

export const AuthDesktop: React.FC = () => {
  const { search } = useLocation();
  const navigate = useNavigate();
  const state = new URLSearchParams(search).get('state') || '';

  const [status, setStatus] = useState<Status>('login');
  const [deepLink, setDeepLink] = useState<string>('');
  const [error, setError] = useState<string>('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [passkeyLoading, setPasskeyLoading] = useState(false);

  const issueAndRedirect = async () => {
    if (!state) {
      setError('Missing desktop handoff state. Relaunch sign-in from the app.');
      setStatus('error');
      return;
    }
    setStatus('issuing');
    setError('');
    try {
      const { code } = await issueDesktopCode(state);
      const url = `parley://auth?code=${encodeURIComponent(code)}&state=${encodeURIComponent(state)}`;
      setDeepLink(url);
      setStatus('ready');
      window.location.href = url;
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to hand off to desktop app.');
      setStatus('error');
    }
  };

  useEffect(() => {
    // This page is loaded in the user's browser — if they already have a
    // valid session here (cached user blob from a previous web login),
    // skip the form and immediately issue the desktop handoff code.
    if (localStorage.getItem('user')) {
      issueAndRedirect();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handlePassword = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      const input = email.trim();
      const isPhone = /^\+?[\d\s\-()]+$/.test(input) && input.replace(/\D/g, '').length >= 8;
      const body = isPhone
        ? { phone: input.replace(/[\s\-()]/g, ''), password }
        : { email: input, password };
      const resp = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify(body),
      });
      const data = await resp.json().catch(() => ({}));
      if (!resp.ok) throw new Error(data.message || 'Login failed');
      localStorage.setItem('user', JSON.stringify(data.user));
      // This page is browser-side (not the desktop shell). Web users now
      // get an HttpOnly session cookie via /auth/login; only the desktop
      // build needs the token in localStorage for its Bearer header.
      if (IS_DESKTOP) {
        localStorage.setItem('token', data.token);
        apiClient.setToken(data.token);
      }
      await issueAndRedirect();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed.');
    } finally {
      setLoading(false);
    }
  };

  const handlePasskey = async () => {
    setError('');
    setPasskeyLoading(true);
    try {
      const data = await loginWithPasskey();
      localStorage.setItem('user', JSON.stringify(data.user));
      if (IS_DESKTOP) {
        localStorage.setItem('token', data.token);
        apiClient.setToken(data.token);
      }
      await issueAndRedirect();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Passkey authentication failed.');
    } finally {
      setPasskeyLoading(false);
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
            <h1 className="auth-title">Sign in to desktop app</h1>
            <p className="auth-subtitle">
              {status === 'login' && 'Authenticate here, then the Parley app will pick up where you left off.'}
              {status === 'issuing' && 'Preparing your session…'}
              {status === 'ready' && 'Opening the Parley desktop app…'}
              {status === 'error' && 'Something went wrong.'}
            </p>
          </div>

          {error && <div className="auth-error-banner">{error}</div>}

          {status === 'login' && (
            <>
              <form onSubmit={handlePassword} className="auth-form" noValidate>
                <div className="input-wrapper">
                  <label htmlFor="email" className="input-label">Email or Phone</label>
                  <input
                    id="email"
                    name="email"
                    type="text"
                    className="input"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="Email or phone number"
                    autoComplete="email"
                  />
                </div>
                <div className="input-wrapper">
                  <label htmlFor="password" className="input-label">Password</label>
                  <input
                    id="password"
                    name="password"
                    type="password"
                    className="input"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="Enter your password"
                    autoComplete="current-password"
                  />
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
                  onClick={handlePasskey}
                  style={{ width: '100%', marginTop: 12 }}
                >
                  🔑 Sign in with passkey
                </Button>
              )}

              <div className="auth-footer">
                <span className="auth-footer-text">Need an account?</span>{' '}
                <Link to="/register" className="auth-link">Register</Link>
              </div>
            </>
          )}

          {status === 'ready' && deepLink && (
            <>
              <p style={{ textAlign: 'center', marginTop: 8 }}>
                If the app didn't open automatically, click below.
              </p>
              <Button
                type="button"
                variant="primary"
                size="lg"
                onClick={() => { window.location.href = deepLink; }}
                style={{ width: '100%', marginTop: 12 }}
              >
                Open Parley desktop app
              </Button>
            </>
          )}

          {status === 'error' && (
            <Button
              type="button"
              variant="primary"
              size="lg"
              onClick={() => navigate('/')}
              style={{ width: '100%', marginTop: 12 }}
            >
              Back to Parley
            </Button>
          )}
        </div>
      </div>
    </div>
  );
};
