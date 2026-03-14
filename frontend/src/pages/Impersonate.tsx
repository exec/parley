import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';

export const Impersonate: React.FC = () => {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const [error, setError] = useState('');

  useEffect(() => {
    const token = params.get('token');
    if (!token) {
      setError('No token provided');
      return;
    }
    // Verify token works by calling /api/auth/me
    fetch('/api/auth/me', { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.json())
      .then(user => {
        if (!user || !user.id) throw new Error('Invalid token');
        localStorage.setItem('token', token);
        localStorage.setItem('user', JSON.stringify(user));
        navigate('/', { replace: true });
        window.location.reload();
      })
      .catch(() => setError('Invalid or expired impersonation token'));
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  if (error) return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh', background: '#000', color: '#ff4444', fontFamily: 'monospace' }}>
      {error}
    </div>
  );
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh', background: '#000', color: '#32CD32', fontFamily: 'monospace' }}>
      Authenticating...
    </div>
  );
};
