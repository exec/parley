import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { getInvite, joinServerByInvite } from '../api/servers';
import { Server } from '../api/types';
import './InvitePage.css';

export function InvitePage() {
  const { code } = useParams<{ code: string }>();
  const navigate = useNavigate();
  const [status, setStatus] = useState<'loading' | 'preview' | 'joining' | 'success' | 'error'>('loading');
  const [server, setServer] = useState<Server | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!code) {
      setStatus('error');
      setError('Invalid invite link.');
      return;
    }

    const token = localStorage.getItem('token');
    if (!token) {
      navigate(`/login?redirect=/invite/${code}`);
      return;
    }

    // Preview only — do not join yet
    getInvite(code)
      .then(({ server: srv }) => {
        setServer(srv);
        setStatus('preview');
      })
      .catch(err => {
        setError(err?.message || 'This invite is invalid or has expired.');
        setStatus('error');
      });
  }, [code, navigate]);

  const handleJoin = () => {
    if (!code) return;
    setStatus('joining');
    joinServerByInvite(code)
      .then(srv => {
        setServer(srv);
        setStatus('success');
      })
      .catch(err => {
        setError(err?.message || 'Failed to join server.');
        setStatus('error');
      });
  };

  return (
    <div className="invite-page">
      <div className="invite-card">
        {status === 'loading' && (
          <>
            <div className="invite-spinner" />
            <p className="invite-text">Loading invite...</p>
          </>
        )}

        {status === 'preview' && server && (
          <>
            <div className="invite-server-icon">
              {server.name.charAt(0).toUpperCase()}
            </div>
            <h1 className="invite-title">You've been invited to join</h1>
            <p className="invite-server-name">{server.name}</p>
            <button className="invite-btn" onClick={handleJoin}>
              Accept Invite
            </button>
            <button className="invite-btn invite-btn--secondary" onClick={() => navigate('/')}>
              Decline
            </button>
          </>
        )}

        {status === 'joining' && (
          <>
            <div className="invite-spinner" />
            <p className="invite-text">Joining server...</p>
          </>
        )}

        {status === 'success' && server && (
          <>
            <div className="invite-server-icon">
              {server.name.charAt(0).toUpperCase()}
            </div>
            <h1 className="invite-title">You joined!</h1>
            <p className="invite-server-name">{server.name}</p>
            <button
              className="invite-btn"
              onClick={() => navigate('/')}
            >
              Open Parley
            </button>
          </>
        )}

        {status === 'error' && (
          <>
            <div className="invite-error-icon">✕</div>
            <h1 className="invite-title">Invalid Invite</h1>
            <p className="invite-error-text">{error}</p>
            <button className="invite-btn" onClick={() => navigate('/')}>
              Go Home
            </button>
          </>
        )}
      </div>
    </div>
  );
}
