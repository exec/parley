import React, { useEffect, useState } from 'react';
import { APP_VERSION, APP_COMMIT } from '../../config';
import { isTauri, openInBrowser, copyToClipboard } from '../../lib/tauri';

const REPO_URL = 'https://github.com/exec/parley';

type Platform = 'macOS' | 'Windows' | 'Linux' | 'Web';

function detectPlatform(): Platform {
  if (typeof document !== 'undefined') {
    const p = document.documentElement.dataset.tauriPlatform;
    if (p === 'macos') return 'macOS';
    if (p === 'windows') return 'Windows';
    if (p === 'linux') return 'Linux';
  }
  return 'Web';
}

export const AboutTab: React.FC = () => {
  const [tauriVersion, setTauriVersion] = useState<string | null>(null);
  const platform = detectPlatform();
  const copied = useCopyState();

  // Prefer the runtime-reported version from Tauri when available — if the
  // installed app ever drifts from the bundled web build, this surfaces it.
  useEffect(() => {
    if (!isTauri()) return;
    import('@tauri-apps/api/app')
      .then(({ getVersion }) => getVersion().then(setTauriVersion))
      .catch(() => {/* non-critical */});
  }, []);

  const version = tauriVersion ?? APP_VERSION;
  const buildLine = APP_COMMIT ? `${version} (${APP_COMMIT})` : version;
  const debugReport = [
    `Parley ${version}`,
    APP_COMMIT ? `Commit: ${APP_COMMIT}` : null,
    `Platform: ${platform}`,
    `User agent: ${typeof navigator !== 'undefined' ? navigator.userAgent : 'n/a'}`,
  ].filter(Boolean).join('\n');

  return (
    <>
      <h2 className="settings-page-title">About</h2>

      <div className="settings-section" style={{ display: 'flex', gap: 20, alignItems: 'center' }}>
        <img
          src="/favicon.svg"
          alt="Parley"
          width={72}
          height={72}
          style={{ borderRadius: 16, flexShrink: 0 }}
        />
        <div>
          <div style={{ fontSize: 22, fontWeight: 700, color: 'var(--parley-text, #ddd)' }}>Parley</div>
          <div style={{ fontSize: 13, color: 'var(--parley-text-muted, #888)', marginTop: 2 }}>
            {buildLine} &middot; {platform}
          </div>
        </div>
      </div>

      <div className="settings-section">
        <div className="settings-section-title">Links</div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          <LinkRow label="Source code" href={REPO_URL} />
          <LinkRow label="Releases" href={`${REPO_URL}/releases`} />
          <LinkRow label="Report an issue" href={`${REPO_URL}/issues/new`} />
        </div>
      </div>

      <div className="settings-section">
        <div className="settings-section-title">Debug info</div>
        <div className="settings-form-hint" style={{ marginBottom: 8 }}>
          Copy this when reporting a bug so the build, platform, and webview are clear.
        </div>
        <pre
          style={{
            fontSize: 12,
            background: 'var(--parley-input, #1a1a1a)',
            border: '1px solid var(--parley-border, #2a2d32)',
            borderRadius: 6,
            padding: '10px 12px',
            color: 'var(--parley-text-muted, #aaa)',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            margin: 0,
          }}
        >
          {debugReport}
        </pre>
        <button
          className="settings-btn settings-btn-ghost"
          style={{ marginTop: 8 }}
          onClick={() => copied.copy(debugReport)}
        >
          {copied.done ? 'Copied!' : 'Copy debug info'}
        </button>
      </div>

      <div className="settings-section">
        <div className="settings-form-hint">
          Parley is open source. &copy; {new Date().getFullYear()} Dylan Hart.
        </div>
      </div>
    </>
  );
};

const LinkRow: React.FC<{ label: string; href: string }> = ({ label, href }) => {
  const handleClick = (e: React.MouseEvent) => {
    // In the desktop shell, open URLs in the system browser rather than
    // inside the webview (which would navigate away from the app).
    if (isTauri()) {
      e.preventDefault();
      openInBrowser(href);
    }
  };
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      onClick={handleClick}
      style={{ fontSize: 13, color: 'var(--parley-accent, #8af)', textDecoration: 'none' }}
    >
      {label} &rarr;
    </a>
  );
};

function useCopyState() {
  const [done, setDone] = useState(false);
  return {
    done,
    copy: async (text: string) => {
      try {
        await copyToClipboard(text);
        setDone(true);
        setTimeout(() => setDone(false), 2000);
      } catch {/* ignore */}
    },
  };
}
