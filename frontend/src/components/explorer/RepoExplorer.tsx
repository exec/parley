import React, { useEffect, useState, useCallback } from 'react';
import { CodeBlock } from '../ui/CodeBlock';
import { languageFromFilename } from '../../lib/shiki';
import {
  getRepo, getTree, getBlob, decodeBlobText,
  type GitProvider, type GitRepo, type GitTreeEntry, type GitBlob,
} from '../../api/git';
import './RepoExplorer.css';

export interface ExplorerTarget {
  provider: GitProvider;
  owner: string;
  repo: string;
  ref?: string;          // empty/missing → default branch
  initialPath?: string;  // file to auto-open
}

interface Props {
  target: ExplorerTarget;
  onClose: () => void;
}

interface DirState {
  path: string;
  entries: GitTreeEntry[];
  loading: boolean;
  error: string | null;
}

interface BlobState {
  path: string;
  blob: GitBlob | null;
  loading: boolean;
  error: string | null;
}

function compareEntries(a: GitTreeEntry, b: GitTreeEntry): number {
  // Dirs first, then alpha by name (case-insensitive).
  if (a.type === 'dir' && b.type !== 'dir') return -1;
  if (b.type === 'dir' && a.type !== 'dir') return 1;
  return a.name.toLowerCase().localeCompare(b.name.toLowerCase());
}

function parentOf(path: string): string {
  const i = path.lastIndexOf('/');
  return i === -1 ? '' : path.slice(0, i);
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export const RepoExplorer: React.FC<Props> = ({ target, onClose }) => {
  const [meta, setMeta] = useState<GitRepo | null>(null);
  const [metaError, setMetaError] = useState<string | null>(null);
  const [dir, setDir] = useState<DirState>({ path: '', entries: [], loading: true, error: null });
  const [file, setFile] = useState<BlobState | null>(null);

  // Load repo metadata once.
  useEffect(() => {
    let cancelled = false;
    setMetaError(null);
    setMeta(null);
    getRepo(target.provider, target.owner, target.repo)
      .then(r => { if (!cancelled) setMeta(r); })
      .catch(e => { if (!cancelled) setMetaError((e as Error)?.message || 'failed to load repo'); });
    return () => { cancelled = true; };
  }, [target.provider, target.owner, target.repo]);

  const ref = target.ref || meta?.default_branch || '';

  // Load directory listing whenever path or ref changes.
  const loadDir = useCallback((path: string) => {
    if (!ref) return;
    setDir({ path, entries: [], loading: true, error: null });
    let cancelled = false;
    getTree(target.provider, target.owner, target.repo, ref, path)
      .then(entries => {
        if (cancelled) return;
        setDir({ path, entries: [...entries].sort(compareEntries), loading: false, error: null });
      })
      .catch(e => {
        if (cancelled) return;
        setDir({ path, entries: [], loading: false, error: (e as Error)?.message || 'failed to load tree' });
      });
    return () => { cancelled = true; };
  }, [target.provider, target.owner, target.repo, ref]);

  useEffect(() => { loadDir(''); }, [loadDir]);

  const openFile = useCallback((path: string) => {
    if (!ref) return;
    setFile({ path, blob: null, loading: true, error: null });
    let cancelled = false;
    getBlob(target.provider, target.owner, target.repo, ref, path)
      .then(b => {
        if (cancelled) return;
        setFile({ path, blob: b, loading: false, error: null });
      })
      .catch(e => {
        if (cancelled) return;
        setFile({ path, blob: null, loading: false, error: (e as Error)?.message || 'failed to load file' });
      });
    return () => { cancelled = true; };
  }, [target.provider, target.owner, target.repo, ref]);

  // Auto-open initial path.
  useEffect(() => {
    if (target.initialPath && ref) openFile(target.initialPath);
  }, [target.initialPath, ref, openFile]);

  // Esc closes.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const handleEntryClick = (e: GitTreeEntry) => {
    if (e.type === 'dir') {
      loadDir(e.path);
      return;
    }
    if (e.type === 'file') {
      openFile(e.path);
      return;
    }
    // submodule / symlink — no in-app browse
  };

  const breadcrumbs = (() => {
    const segments = dir.path === '' ? [] : dir.path.split('/');
    return (
      <div className="explorer-breadcrumbs">
        <button className="explorer-crumb" onClick={() => loadDir('')}>{target.owner}/{target.repo}</button>
        {segments.map((seg, i) => {
          const sub = segments.slice(0, i + 1).join('/');
          return (
            <React.Fragment key={sub}>
              <span className="explorer-crumb-sep">/</span>
              <button className="explorer-crumb" onClick={() => loadDir(sub)}>{seg}</button>
            </React.Fragment>
          );
        })}
      </div>
    );
  })();

  const fileContent = (() => {
    if (!file) {
      return (
        <div className="explorer-empty">
          <div className="explorer-empty-title">{meta?.description || `${target.owner}/${target.repo}`}</div>
          <div className="explorer-empty-hint">Select a file from the tree to view its contents.</div>
        </div>
      );
    }
    if (file.loading) return <div className="explorer-loading">Loading {file.path}…</div>;
    if (file.error || !file.blob) return <div className="explorer-error">Could not load {file.path}: {file.error || 'unknown error'}</div>;
    if (file.blob.truncated) {
      return (
        <div className="explorer-error">
          <p><strong>{file.path}</strong> ({formatBytes(file.blob.size)}) is too large to display inline.</p>
          <a href={file.blob.html_url} target="_blank" rel="noopener noreferrer">View raw on GitHub →</a>
        </div>
      );
    }
    if (file.blob.content_type === 'binary') {
      return (
        <div className="explorer-error">
          <p><strong>{file.path}</strong> is a binary file ({formatBytes(file.blob.size)}).</p>
          <a href={file.blob.html_url} target="_blank" rel="noopener noreferrer">View raw on GitHub →</a>
        </div>
      );
    }
    const text = decodeBlobText(file.blob);
    return (
      <CodeBlock
        content={text}
        language={languageFromFilename(file.path) || 'plaintext'}
        filename={file.path}
        showLineNumbers
      />
    );
  })();

  return (
    <div className="explorer-root">
      {/* Header: close + avatar (visual anchor) + ref + external link.
       * Owner/repo text intentionally omitted — the breadcrumbs render
       * `owner/repo` as their first crumb so we don't show it twice. */}
      <div className="explorer-header">
        <button className="explorer-close" onClick={onClose} aria-label="Close explorer">←</button>
        {meta?.owner_avatar_url && <img className="explorer-avatar" src={meta.owner_avatar_url} alt="" />}
        {ref && <span className="explorer-ref">{ref}</span>}
        <span className="explorer-spacer" />
        <a className="explorer-link" href={`https://github.com/${target.owner}/${target.repo}`} target="_blank" rel="noopener noreferrer">
          Open on GitHub ↗
        </a>
      </div>

      {metaError && <div className="explorer-banner explorer-banner--error">Failed to load repository metadata: {metaError}</div>}

      <div className="explorer-body">
        <div className="explorer-main">
          <div className="explorer-toolbar">{breadcrumbs}</div>
          <div className="explorer-file">{fileContent}</div>
        </div>

        <aside className="explorer-tree">
          {/* Static label — the current path lives in the breadcrumbs (left).
           * Repeating it here was visually noisy. */}
          <div className="explorer-tree-header">Files</div>
          {dir.loading && <div className="explorer-tree-loading">Loading…</div>}
          {dir.error && <div className="explorer-tree-error">{dir.error}</div>}
          {!dir.loading && !dir.error && (
            <ul className="explorer-tree-list">
              {dir.path !== '' && (
                <li>
                  <button className="explorer-tree-item explorer-tree-item--up" onClick={() => loadDir(parentOf(dir.path))}>
                    <span className="explorer-tree-icon">↶</span> ..
                  </button>
                </li>
              )}
              {dir.entries.map(e => (
                <li key={e.path}>
                  <button
                    className={`explorer-tree-item explorer-tree-item--${e.type}${file?.path === e.path ? ' is-active' : ''}`}
                    onClick={() => handleEntryClick(e)}
                    title={e.type === 'file' ? `${e.name} (${formatBytes(e.size)})` : e.name}
                    disabled={e.type === 'symlink' || e.type === 'submodule'}
                  >
                    <span className="explorer-tree-icon">
                      {e.type === 'dir' ? '📁' : e.type === 'file' ? '📄' : '↪'}
                    </span>
                    <span className="explorer-tree-name">{e.name}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </aside>
      </div>
    </div>
  );
};
