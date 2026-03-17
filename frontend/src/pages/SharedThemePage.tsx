import React, { useEffect, useRef, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { getPublicTheme, installTheme, UserTheme } from '../api/themes';
import { useTheme } from '../context/ThemeContext';
import './SharedThemePage.css';

export const SharedThemePage: React.FC = () => {
  const { token } = useParams<{ token: string }>();
  const navigate = useNavigate();
  const themeCtx = useTheme();
  const [theme, setTheme] = useState<UserTheme | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [installing, setInstalling] = useState(false);
  const [installed, setInstalled] = useState(false);
  const prevTheme = useRef(localStorage.getItem('parley-theme') || 'rory');
  const prevCSS = useRef(localStorage.getItem('parley-custom-css') || undefined);

  useEffect(() => {
    if (!token) return;
    getPublicTheme(token).then(t => {
      setTheme(t);
      themeCtx.applyTheme('custom', t.css);
    }).catch(() => setError('Theme not found or link is invalid.'));
  }, [token]); // eslint-disable-line

  const handleInstall = async () => {
    if (!localStorage.getItem('token')) {
      navigate(`/login?redirect=/theme/${token}`); return;
    }
    setInstalling(true);
    try { await installTheme(token!); setInstalled(true); }
    catch { setError('Install failed.'); }
    finally { setInstalling(false); }
  };

  const handleDiscard = () => {
    themeCtx.applyTheme(prevTheme.current, prevCSS.current);
    navigate('/');
  };

  if (error) return <div className="shared-theme-page"><div className="shared-theme-card"><div className="shared-theme-error">{error}</div></div></div>;
  if (!theme) return <div className="shared-theme-page"><div className="shared-theme-card"><div className="shared-theme-loading">Loading theme…</div></div></div>;

  const palette = ['--bg-primary','--bg-secondary','--accent','--text-primary'].map(
    v => getComputedStyle(document.body).getPropertyValue(v).trim() || '#888'
  );

  return (
    <div className="shared-theme-page">
      <div className="shared-theme-card">
        <div className="shared-theme-title">{theme.name}</div>
        <div className="shared-theme-subtitle">Someone shared a theme with you</div>
        <div className="shared-theme-palette">
          {palette.map((c,i) => <div key={i} className="shared-theme-palette-chip" style={{background:c}} />)}
        </div>
        <div className="shared-theme-actions">
          {installed
            ? <div style={{color:'var(--discord-success)',fontWeight:600}}>✓ Installed! Find it in Settings → Appearance.</div>
            : <button className="shared-theme-install" onClick={handleInstall} disabled={installing}>{installing?'Installing…':'Install Theme'}</button>
          }
          <button className="shared-theme-discard" onClick={handleDiscard}>Discard &amp; go home</button>
        </div>
      </div>
    </div>
  );
};
