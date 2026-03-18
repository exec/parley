import React, { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { verifyEmail } from '../api/auth';

export const VerifyEmail: React.FC = () => {
  const [searchParams] = useSearchParams();
  const [status, setStatus] = useState<'loading' | 'success' | 'error'>('loading');
  const [message, setMessage] = useState('');

  useEffect(() => {
    const token = searchParams.get('token');
    if (!token) {
      setStatus('error');
      setMessage('No verification token provided.');
      return;
    }

    verifyEmail(token)
      .then(() => {
        setStatus('success');
        setMessage('Your email has been verified successfully!');
        // Update stored user so settings reflects verified state immediately
        const stored = localStorage.getItem('user');
        if (stored) {
          try {
            const u = JSON.parse(stored);
            u.email_verified = true;
            localStorage.setItem('user', JSON.stringify(u));
          } catch { /* ignore */ }
        }
      })
      .catch((err: { message?: string }) => {
        setStatus('error');
        setMessage(err?.message || 'Verification failed. The link may be invalid or expired.');
      });
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div style={{
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      minHeight: '100vh',
      background: '#0a0a0a',
      color: '#fff',
      fontFamily: 'monospace',
      gap: 24,
      padding: 24,
    }}>
      <h1 style={{ color: 'var(--parley-accent)', fontSize: 28, margin: 0 }}>Parley</h1>

      {status === 'loading' && (
        <p style={{ color: '#aaa' }}>Verifying your email...</p>
      )}

      {status === 'success' && (
        <>
          <p style={{ color: 'var(--parley-accent)', fontSize: 18 }}>{message}</p>
          <Link
            to="/"
            style={{ color: 'var(--parley-accent)', textDecoration: 'underline' }}
          >
            Go to Parley
          </Link>
        </>
      )}

      {status === 'error' && (
        <>
          <p style={{ color: '#ff4444', fontSize: 18 }}>{message}</p>
          <Link
            to="/login"
            style={{ color: 'var(--parley-accent)', textDecoration: 'underline' }}
          >
            Back to Login
          </Link>
        </>
      )}
    </div>
  );
};
