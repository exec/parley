import React, { useEffect, useState } from 'react';
import { getPublicTheme, installTheme, UserTheme } from '../../api/themes';
import { useTheme } from '../../context/ThemeContext';
import { useApp } from '../../context/AppContext';
import { buildEmbedPreviewHTML } from '../../lib/themePreview';
import './ThemeLinkEmbed.css';

interface Props { token: string; }

export const ThemeLinkEmbed: React.FC<Props> = ({ token }) => {
  const [theme, setTheme] = useState<UserTheme | null>(null);
  const [error, setError] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [installed, setInstalled] = useState(false);
  const themeCtx = useTheme();
  const { currentUser } = useApp();

  useEffect(() => {
    getPublicTheme(token)
      .then(setTheme)
      .catch(() => setError(true));
  }, [token]);

  const handleApply = async () => {
    if (!localStorage.getItem('token')) {
      window.location.href = `/login?redirect=${encodeURIComponent(window.location.pathname)}`;
      return;
    }
    setInstalling(true);
    try {
      const installed = await installTheme(token);
      await themeCtx.setCustom(installed.id, installed);
      setInstalled(true);
    } catch {
      setInstalling(false);
    }
  };

  if (error) return null;
  if (!theme) return <div className="theme-embed"><span className="theme-embed-loading">Loading theme…</span></div>;

  const displayName = currentUser?.display_name || currentUser?.username || 'You';
  const avatarUrl = currentUser?.avatar_url || null;
  const previewSrc = buildEmbedPreviewHTML(theme.base_theme, theme.css, displayName, avatarUrl);

  return (
    <div className="theme-embed">
      <div className="theme-embed-category">Custom Theme</div>
      <div className="theme-embed-title">{theme.name}</div>
      {theme.author_username && (
        <div className="theme-embed-author">by <strong>{theme.author_username}</strong></div>
      )}
      <iframe
        className="theme-embed-preview"
        srcDoc={previewSrc}
        sandbox="allow-same-origin"
        title="Theme preview"
      />
      {installed
        ? <span className="theme-embed-applied">✓ Installed and applied!</span>
        : <button className="theme-embed-apply" onClick={handleApply} disabled={installing}>
            {installing ? 'Applying…' : 'Apply'}
          </button>
      }
    </div>
  );
};
