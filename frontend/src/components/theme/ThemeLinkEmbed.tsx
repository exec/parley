import React, { useEffect, useState } from 'react';
import { getPublicTheme, installTheme, UserTheme } from '../../api/themes';
import { useTheme } from '../../context/ThemeContext';
import './ThemeLinkEmbed.css';

interface Props { token: string; }

export const ThemeLinkEmbed: React.FC<Props> = ({ token }) => {
  const [theme, setTheme] = useState<UserTheme | null>(null);
  const [error, setError] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [installed, setInstalled] = useState(false);
  const themeCtx = useTheme();

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
      await themeCtx.setCustom(installed.id);
      setInstalled(true);
    } catch {
      setInstalling(false);
    }
  };

  if (error) return null; // silently hide bad links
  if (!theme) return <div className="theme-embed"><span className="theme-embed-loading">Loading theme…</span></div>;

  // Extract 4 colors from theme CSS by parsing CSS variable values
  const colors = extractThemeColors(theme.css);

  return (
    <div className="theme-embed">
      <div className="theme-embed-category">Custom Theme</div>
      <div className="theme-embed-title">{theme.name}</div>
      {theme.author_username && (
        <div className="theme-embed-author">by <strong>{theme.author_username}</strong></div>
      )}
      <div className="theme-embed-swatches">
        {colors.map((c, i) => <div key={i} className="theme-embed-swatch" style={{ background: c }} />)}
      </div>
      {installed
        ? <span className="theme-embed-applied">✓ Installed and applied!</span>
        : <button className="theme-embed-apply" onClick={handleApply} disabled={installing}>
            {installing ? 'Applying…' : 'Apply'}
          </button>
      }
    </div>
  );
};

/**
 * Parse CSS variables from raw CSS text. Looks for --bg-primary, --accent,
 * --bg-secondary, --text-primary in that order. Falls back to #888 for any missing.
 */
function extractThemeColors(css: string): string[] {
  const vars = ['--bg-primary', '--accent', '--bg-secondary', '--text-primary'];
  return vars.map(v => {
    const m = css.match(new RegExp(`${v}\\s*:\\s*([^;\\n]+)`));
    return m ? m[1].trim() : '#888';
  });
}
