import React, { useEffect, useState } from 'react';
import { getPublicTheme, installTheme, UserTheme } from '../../api/themes';
import { useTheme } from '../../context/ThemeContext';
import { useIdentity } from '../../context/AppContext';
import { buildEmbedPreviewHTML } from '../../lib/themePreview';
import { EmbedCard } from '../EmbedCard';
import './ThemeLinkEmbed.css';

interface Props { token: string; }

export const ThemeLinkEmbed: React.FC<Props> = ({ token }) => {
  const [theme, setTheme] = useState<UserTheme | null>(null);
  const [invalid, setInvalid] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [installed, setInstalled] = useState(false);
  const themeCtx = useTheme();
  const { currentUser } = useIdentity();

  useEffect(() => {
    getPublicTheme(token)
      .then(setTheme)
      .catch(() => setInvalid(true));
  }, [token]);

  const handleApply = async () => {
    if (invalid) return; // frontend guard
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
      setInvalid(true);
      setInstalling(false);
    }
  };

  if (invalid) return (
    <EmbedCard
      title="Invalid Theme"
      subtitle="This theme link is no longer valid."
      actions={<button className="theme-embed-apply" disabled>Apply</button>}
    />
  );

  if (!theme) return (
    <EmbedCard
      title=""
      actions={<span className="theme-embed-loading">Loading theme…</span>}
    />
  );

  const displayName = currentUser?.display_name || currentUser?.username || 'You';
  const avatarUrl = currentUser?.avatar_url || null;
  const previewSrc = buildEmbedPreviewHTML(theme.base_theme, theme.css, displayName, avatarUrl);

  // Prefer the stored background_url; fall back to extracting it from the CSS
  // (handles older themes that embedded the bg directly in their CSS).
  const bgUrl = theme.background_url
    ?? theme.css.match(/background-image\s*:\s*url\(\s*["']?([^"')]+)["']?\s*\)/)?.[1]
    ?? null;
  // Extract glass vars from within the glass block specifically — some themes declare
  // solid fallback values for these vars BEFORE the glass block, so searching the full
  // CSS would return the wrong (opaque) values on the first match.
  const glassBlock = theme.css.match(/\/\* bg-glass-start \*\/([\s\S]*?)\/\* bg-glass-end \*\//)?.[1] ?? '';
  const panelBg = glassBlock.match(/--parley-panel-bg\s*:\s*([^;}\n]+)/)?.[1]?.trim() ?? 'rgba(0,0,0,0.55)';
  const panelBlur = glassBlock.match(/--parley-panel-blur\s*:\s*([^;}\n]+)/)?.[1]?.trim() ?? '0px';

  const card = (
    <EmbedCard
      title={theme.name}
      subtitle={theme.author_username ? `by ${theme.author_username}` : undefined}
      preview={
        <iframe
          className="theme-embed-preview"
          srcDoc={previewSrc}
          sandbox="allow-same-origin"
          title="Theme preview"
        />
      }
      actions={
        installed
          ? <span className="theme-embed-applied">✓ Installed and applied!</span>
          : <button className="theme-embed-apply" onClick={handleApply} disabled={installing}>
              {installing ? 'Applying…' : 'Apply'}
            </button>
      }
      frostedBg={bgUrl ? { color: panelBg, blur: panelBlur } : undefined}
    />
  );

  if (!bgUrl) return card;
  return (
    <div className="theme-embed-bg-wrap" style={{ backgroundImage: `url(${bgUrl})` }}>
      {card}
    </div>
  );
};
