import React, { useEffect, useState, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { getThemeRepo, installTheme, featureTheme, RepoTheme, UserTheme } from '../api/themes';
import { apiClient } from '../api/client';
import { useTheme } from '../context/ThemeContext';
import './ThemeRepoPage.css';

// Helper: extract a CSS variable value from a CSS string
function extractCSSVar(css: string, varName: string): string {
  const re = new RegExp(varName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '\\s*:\\s*([^;\\n}]+)');
  const m = css.match(re);
  return m ? m[1].trim() : '';
}

const SWATCH_VARS = ['--parley-bg', '--parley-bg-secondary', '--parley-accent', '--parley-sidebar', '--parley-text'];

function ThemeCard({ theme, isAdmin, onFeatureToggle, existingTheme }: {
  theme: RepoTheme;
  isAdmin: boolean;
  onFeatureToggle: (id: number, featured: boolean) => void;
  existingTheme: UserTheme | undefined;
}) {
  const navigate = useNavigate();
  const themeCtx = useTheme();
  const [installing, setInstalling] = useState(false);
  const [installError, setInstallError] = useState('');
  const [featuring, setFeaturing] = useState(false);

  const swatches = SWATCH_VARS.map(v => extractCSSVar(theme.css, v)).filter(c => c !== '');

  const handleInstall = async () => {
    if (!localStorage.getItem('token')) {
      navigate('/login');
      return;
    }
    // Already installed — just apply it
    if (existingTheme) {
      await themeCtx.setCustom(existingTheme.id, existingTheme);
      navigate('/');
      return;
    }
    if (!theme.share_token) return;
    setInstalling(true);
    setInstallError('');
    try {
      const installed = await installTheme(theme.share_token);
      themeCtx.setCustom(installed.id, installed);
    } catch {
      setInstallError('Install failed');
    } finally {
      setInstalling(false);
    }
  };

  const handleFeature = async () => {
    setFeaturing(true);
    try {
      await featureTheme(theme.id, !theme.is_featured);
      onFeatureToggle(theme.id, !theme.is_featured);
    } catch {
      // silently fail
    } finally {
      setFeaturing(false);
    }
  };

  const authorLabel = theme.author_display_name || theme.author_username || 'Unknown';

  return (
    <div className="theme-card">
      {swatches.length > 0 && (
        <div className="theme-card-swatches">
          {swatches.map((color, i) => (
            <div key={i} className="theme-card-swatch" style={{ background: color }} />
          ))}
        </div>
      )}
      <div className="theme-card-body">
        <div className="theme-card-name" title={theme.name}>{theme.name}</div>
        <div className="theme-card-author">by {authorLabel}</div>
        {theme.is_featured && (
          <div className="theme-card-badges">
            <span className="theme-card-featured-badge">⭐ Featured</span>
          </div>
        )}
        <div className="theme-card-actions">
          {existingTheme ? (
            <button className="theme-card-install-btn" onClick={handleInstall}>
              Apply (already installed)
            </button>
          ) : (
            <button
              className="theme-card-install-btn"
              onClick={handleInstall}
              disabled={installing || !theme.share_token}
            >
              {installing ? 'Installing…' : 'Install'}
            </button>
          )}
          {installError && <div style={{ fontSize: '11px', color: 'var(--parley-danger, #f04747)' }}>{installError}</div>}
          {isAdmin && (
            <button
              className={`theme-card-feature-btn${theme.is_featured ? ' featured' : ''}`}
              onClick={handleFeature}
              disabled={featuring}
            >
              {theme.is_featured ? '★ Unfeature' : '☆ Feature'}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

export const ThemeRepoPage: React.FC = () => {
  const navigate = useNavigate();
  const { customThemes } = useTheme();
  const [themes, setThemes] = useState<RepoTheme[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');
  const [isAdmin, setIsAdmin] = useState(false);
  const LIMIT = 24;

  // Build a map of share_token → UserTheme for already-installed themes
  const installedByToken = new Map(
    customThemes.filter(t => t.share_token).map(t => [t.share_token!, t])
  );

  // Check admin status
  useEffect(() => {
    const token = localStorage.getItem('token');
    if (!token) return;
    apiClient.get<{ badges?: number }>('/auth/me').then(u => {
      if (u.badges && (u.badges & 2) !== 0) setIsAdmin(true);
    }).catch(() => {});
  }, []);

  const load = useCallback(async (p: number, append: boolean) => {
    if (p === 1) setLoading(true);
    else setLoadingMore(true);
    try {
      const data = await getThemeRepo(p, LIMIT);
      setThemes(prev => append ? [...prev, ...data.themes] : data.themes);
      setTotal(data.total);
      setPage(p);
    } catch {
      setError('Failed to load themes');
    } finally {
      setLoading(false);
      setLoadingMore(false);
    }
  }, []);

  useEffect(() => { load(1, false); }, [load]);

  const handleFeatureToggle = (id: number, featured: boolean) => {
    setThemes(prev => prev.map(t => t.id === id ? { ...t, is_featured: featured } : t));
  };

  const handleLoadMore = () => {
    load(page + 1, true);
  };

  const featuredThemes = themes.filter(t => t.is_featured);

  return (
    <div className="theme-repo-page">
      <div className="theme-repo-header">
        <a
          className="theme-repo-back"
          href="/"
          onClick={e => { e.preventDefault(); navigate('/'); }}
        >
          ← Parley
        </a>
        <div className="theme-repo-title">Theme Repository</div>
      </div>

      <div className="theme-repo-content">
        {loading && (
          <div className="theme-repo-loading">Loading themes…</div>
        )}

        {!loading && error && (
          <div className="theme-repo-empty">{error}</div>
        )}

        {!loading && !error && themes.length === 0 && (
          <div className="theme-repo-empty">No published themes yet. Be the first to share one!</div>
        )}

        {!loading && !error && themes.length > 0 && (
          <>
            {featuredThemes.length > 0 && (
              <section style={{ marginBottom: '32px' }}>
                <div className="theme-repo-section-title">Featured</div>
                <div className="theme-repo-grid theme-repo-grid--featured">
                  {featuredThemes.map(theme => (
                    <ThemeCard
                      key={theme.id}
                      theme={theme}
                      isAdmin={isAdmin}
                      onFeatureToggle={handleFeatureToggle}
                      existingTheme={installedByToken.get(theme.share_token ?? '')}
                    />
                  ))}
                </div>
              </section>
            )}

            <section>
              <div className="theme-repo-section-title">All Themes</div>
              <div className="theme-repo-grid">
                {themes.map(theme => (
                  <ThemeCard
                    key={theme.id}
                    theme={theme}
                    isAdmin={isAdmin}
                    onFeatureToggle={handleFeatureToggle}
                    existingTheme={installedByToken.get(theme.share_token ?? '')}
                  />
                ))}
              </div>

              {themes.length < total && (
                <div className="theme-repo-load-more">
                  <button
                    className="theme-repo-load-more-btn"
                    onClick={handleLoadMore}
                    disabled={loadingMore}
                  >
                    {loadingMore ? 'Loading…' : 'Load more'}
                  </button>
                </div>
              )}
            </section>
          </>
        )}
      </div>
    </div>
  );
};
