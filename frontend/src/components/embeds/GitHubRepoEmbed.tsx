import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { EmbedCard } from '../EmbedCard';
import { getRepo, type GitProvider, type GitRepo } from '../../api/git';
import './GitHubRepoEmbed.css';

interface Props {
  provider: GitProvider; // 'github' for V1
  owner: string;
  repo: string;
  /** Pinned ref from a tree/blob URL — Explore opens at this branch/tag/SHA. */
  ref?: string;
  /** Path captured from a tree/blob URL — Explore opens at this directory or file. */
  initialPath?: string;
  /** True when initialPath points at a file (a `blob/` URL was matched). */
  isFile?: boolean;
}

/**
 * Map GitHub primary languages to the dot colours used on github.com itself.
 * The full table is huge; this covers the long tail. Anything missing falls
 * back to the parley accent colour.
 */
const LANG_COLOR: Record<string, string> = {
  TypeScript: '#3178c6',
  JavaScript: '#f1e05a',
  Python: '#3572A5',
  Go: '#00ADD8',
  Rust: '#dea584',
  C: '#555555',
  'C++': '#f34b7d',
  'C#': '#178600',
  Java: '#b07219',
  Kotlin: '#A97BFF',
  Swift: '#F05138',
  Ruby: '#701516',
  PHP: '#4F5D95',
  Shell: '#89e051',
  HTML: '#e34c26',
  CSS: '#563d7c',
  SCSS: '#c6538c',
  Vue: '#41b883',
  Svelte: '#ff3e00',
  Lua: '#000080',
  Dart: '#00B4AB',
  Elixir: '#6e4a7e',
  Haskell: '#5e5086',
  Zig: '#ec915c',
};

function relativeTime(rfc3339: string): string {
  const t = new Date(rfc3339).getTime();
  if (!Number.isFinite(t)) return '';
  const seconds = Math.max(0, Math.floor((Date.now() - t) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo ago`;
  const years = Math.floor(days / 365);
  return `${years}y ago`;
}

function formatCount(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
  if (n >= 1_000) return (n / 1_000).toFixed(1).replace(/\.0$/, '') + 'k';
  return String(n);
}

export const GitHubRepoEmbed: React.FC<Props> = ({ provider, owner, repo, ref, initialPath, isFile }) => {
  const navigate = useNavigate();
  const [data, setData] = useState<GitRepo | null>(null);
  const [error, setError] = useState<'not_found' | 'rate_limited' | 'other' | null>(null);
  // isFile is currently informational only — the URL we navigate to encodes
  // path either way and the explorer resolves file vs dir at load time.
  void isFile;

  useEffect(() => {
    let cancelled = false;
    setData(null);
    setError(null);
    getRepo(provider, owner, repo)
      .then(r => { if (!cancelled) setData(r); })
      .catch(e => {
        if (cancelled) return;
        const code = (e as { code?: string })?.code;
        if (code === '404') setError('not_found');
        else if (code === '503') setError('rate_limited');
        else setError('other');
      });
    return () => { cancelled = true; };
  }, [provider, owner, repo]);

  // Degraded card on missing or rate-limited — minimal title + View on GitHub.
  if (error === 'not_found') {
    return (
      <EmbedCard
        icon={<GitHubMark />}
        title={`${owner}/${repo}`}
        subtitle="Not found"
        actions={
          <a className="gh-embed-button gh-embed-button--ghost"
             href={`https://github.com/${owner}/${repo}`} target="_blank" rel="noopener noreferrer">
            View on GitHub
          </a>
        }
      />
    );
  }
  if (error || !data) {
    return (
      <EmbedCard
        icon={<GitHubMark />}
        title={`${owner}/${repo}`}
        subtitle={error === 'rate_limited' ? 'Rate-limited — view directly' : 'Loading…'}
        actions={
          <a className="gh-embed-button gh-embed-button--ghost"
             href={`https://github.com/${owner}/${repo}`} target="_blank" rel="noopener noreferrer">
            View on GitHub
          </a>
        }
      />
    );
  }

  const langColor = data.language ? (LANG_COLOR[data.language] || 'var(--parley-accent, #32CD32)') : null;

  const icon = data.owner_avatar_url
    ? <img src={data.owner_avatar_url} alt="" />
    : <GitHubMark />;

  const onExplore = () => {
    const sp = new URLSearchParams();
    const effectiveRef = ref || data.default_branch;
    if (effectiveRef) sp.set('ref', effectiveRef);
    if (initialPath) sp.set('path', initialPath);
    const qs = sp.toString();
    navigate(`/explore/${provider}/${owner}/${repo}${qs ? `?${qs}` : ''}`);
  };

  return (
    <EmbedCard
      icon={icon}
      title={`${data.owner}/${data.name}`}
      subtitle={data.private ? '🔒 Private repository' : data.description || 'No description'}
      actions={
        <div className="gh-embed-actions-row">
          <a className="gh-embed-button gh-embed-button--ghost"
             href={data.html_url} target="_blank" rel="noopener noreferrer">
            View on GitHub
          </a>
          <button className="gh-embed-button gh-embed-button--primary" onClick={onExplore}>
            Explore →
          </button>
        </div>
      }
    >
      <div className="gh-embed-meta">
        {data.language && (
          <span className="gh-embed-meta-item" title={`Primary language: ${data.language}`}>
            <span className="gh-embed-lang-dot" style={{ background: langColor || undefined }} />
            {data.language}
          </span>
        )}
        <span className="gh-embed-meta-item" title="Stars">
          ★ {formatCount(data.stars)}
        </span>
        {data.forks > 0 && (
          <span className="gh-embed-meta-item" title="Forks">
            ⑂ {formatCount(data.forks)}
          </span>
        )}
        {data.pushed_at && (
          <span className="gh-embed-meta-item" title={`Last push: ${data.pushed_at}`}>
            pushed {relativeTime(data.pushed_at)}
          </span>
        )}
        {data.latest_release && (
          <span className="gh-embed-meta-item gh-embed-meta-tag" title="Latest release">
            {data.latest_release.tag_name}
          </span>
        )}
      </div>
    </EmbedCard>
  );
};

// GitHub Octocat-ish mark — minimal SVG so we don't ship the full asset.
const GitHubMark: React.FC = () => (
  <svg viewBox="0 0 24 24" width="28" height="28" aria-hidden="true">
    <path
      fill="#fff"
      d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.1.79-.25.79-.55 0-.27-.01-1.16-.02-2.11-3.2.69-3.87-1.36-3.87-1.36-.52-1.32-1.27-1.67-1.27-1.67-1.04-.71.08-.7.08-.7 1.15.08 1.76 1.18 1.76 1.18 1.02 1.74 2.68 1.24 3.34.95.1-.74.4-1.24.72-1.53-2.55-.29-5.24-1.28-5.24-5.7 0-1.26.45-2.29 1.18-3.1-.12-.29-.51-1.46.11-3.05 0 0 .97-.31 3.18 1.18a11.05 11.05 0 0 1 5.79 0c2.21-1.49 3.18-1.18 3.18-1.18.62 1.59.23 2.76.11 3.05.74.81 1.18 1.84 1.18 3.1 0 4.43-2.7 5.41-5.27 5.69.41.36.78 1.06.78 2.13 0 1.54-.01 2.78-.01 3.16 0 .31.21.66.8.55C20.21 21.39 23.5 17.08 23.5 12 23.5 5.65 18.35.5 12 .5z"
    />
  </svg>
);
