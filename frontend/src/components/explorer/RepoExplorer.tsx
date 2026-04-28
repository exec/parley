import React, { useEffect, useState, useCallback, useRef } from 'react';
import { CodeBlock } from '../ui/CodeBlock';
import { languageFromFilename } from '../../lib/shiki';
import {
  getRepo, getTree, getBlob, getBranches, decodeBlobText,
  type GitProvider, type GitRepo, type GitTreeEntry, type GitBlob, type GitBranch,
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
  const [activeRef, setActiveRef] = useState<string>(target.ref || '');
  const [dir, setDir] = useState<DirState>({ path: '', entries: [], loading: true, error: null });
  const [file, setFile] = useState<BlobState | null>(null);

  // Branch dropdown state
  const [branchMenuOpen, setBranchMenuOpen] = useState(false);
  const [branches, setBranches] = useState<GitBranch[] | null>(null);
  const [branchesLoading, setBranchesLoading] = useState(false);
  const branchBtnRef = useRef<HTMLButtonElement | null>(null);

  // Load repo metadata once.
  useEffect(() => {
    let cancelled = false;
    setMetaError(null);
    setMeta(null);
    getRepo(target.provider, target.owner, target.repo)
      .then(r => {
        if (cancelled) return;
        setMeta(r);
        // If the caller didn't pin a ref, lock onto the default branch now
        // so subsequent tree/blob requests get a stable name.
        if (!target.ref && !activeRef) setActiveRef(r.default_branch);
      })
      .catch(e => { if (!cancelled) setMetaError((e as Error)?.message || 'failed to load repo'); });
    return () => { cancelled = true; };
  // activeRef intentionally NOT in deps — we set it once on first metadata.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target.provider, target.owner, target.repo, target.ref]);

  const ref = activeRef || meta?.default_branch || '';

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

  // Close branch menu on outside click.
  useEffect(() => {
    if (!branchMenuOpen) return;
    const onDoc = (e: MouseEvent) => {
      const t = e.target as Node;
      const menu = document.querySelector('.explorer-branch-menu');
      if (branchBtnRef.current?.contains(t)) return;
      if (menu && menu.contains(t)) return;
      setBranchMenuOpen(false);
    };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, [branchMenuOpen]);

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

  const openBranchMenu = () => {
    setBranchMenuOpen(true);
    if (!branches && !branchesLoading) {
      setBranchesLoading(true);
      getBranches(target.provider, target.owner, target.repo)
        .then(bs => setBranches(bs))
        .catch(() => setBranches([]))
        .finally(() => setBranchesLoading(false));
    }
  };

  const switchBranch = (name: string) => {
    setBranchMenuOpen(false);
    if (name === ref) return;
    setActiveRef(name);
    setFile(null);          // viewing file from old ref no longer makes sense
    // dir reload triggers via the loadDir useCallback dep on `ref`
  };

  // Breadcrumbs live in the tree pane header now.
  const treePaneBreadcrumbs = (() => {
    const segments = dir.path === '' ? [] : dir.path.split('/');
    return (
      <div className="explorer-breadcrumbs">
        <button className="explorer-crumb" onClick={() => loadDir('')}>{target.repo}</button>
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
      <div className="explorer-header">
        <button className="explorer-close" onClick={onClose} aria-label="Close explorer">←</button>
        {meta?.owner_avatar_url && <img className="explorer-avatar" src={meta.owner_avatar_url} alt="" />}
        <span className="explorer-title-text">{target.owner}/{target.repo}</span>
        {ref && (
          <span className="explorer-ref-wrap">
            <button
              ref={branchBtnRef}
              className="explorer-ref-btn"
              onClick={() => (branchMenuOpen ? setBranchMenuOpen(false) : openBranchMenu())}
              title="Switch branch"
              aria-haspopup="listbox"
              aria-expanded={branchMenuOpen}
            >
              <span className="explorer-ref-icon">⎇</span>
              <span className="explorer-ref-name">{ref}</span>
              <span className="explorer-ref-caret">▾</span>
            </button>
            {branchMenuOpen && (
              <div className="explorer-branch-menu" role="listbox">
                {branchesLoading && <div className="explorer-branch-loading">Loading branches…</div>}
                {!branchesLoading && branches?.length === 0 && (
                  <div className="explorer-branch-loading">No branches found.</div>
                )}
                {!branchesLoading && branches?.map(b => (
                  <button
                    key={b.name}
                    className={`explorer-branch-item${b.name === ref ? ' is-active' : ''}`}
                    onClick={() => switchBranch(b.name)}
                    role="option"
                    aria-selected={b.name === ref}
                  >
                    <span className="explorer-branch-name">{b.name}</span>
                    {b.is_default && <span className="explorer-branch-default">default</span>}
                  </button>
                ))}
              </div>
            )}
          </span>
        )}
        <span className="explorer-spacer" />
        <a className="explorer-link" href={`https://github.com/${target.owner}/${target.repo}`} target="_blank" rel="noopener noreferrer">
          Open on GitHub ↗
        </a>
      </div>

      {metaError && <div className="explorer-banner explorer-banner--error">Failed to load repository metadata: {metaError}</div>}

      <div className="explorer-body">
        <div className="explorer-main">
          <div className="explorer-file">{fileContent}</div>
        </div>

        <aside className="explorer-tree">
          {/* Breadcrumbs scoped to the tree pane (right side): clicking any
           * segment navigates to that directory. The repo name is the first
           * crumb so the tree pane has its own "where am I" affordance. */}
          <div className="explorer-tree-header">{treePaneBreadcrumbs}</div>
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
