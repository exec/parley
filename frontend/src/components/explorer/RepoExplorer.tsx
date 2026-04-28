import React, { useEffect, useState, useCallback, useRef } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { CodeBlock } from '../ui/CodeBlock';
import { languageFromFilename } from '../../lib/shiki';
import {
  getRepo, getTree, getBlob, getBranches, decodeBlobText,
  type GitProvider, type GitRepo, type GitTreeEntry, type GitBlob, type GitBranch,
} from '../../api/git';
import './RepoExplorer.css';

interface Props {
  provider: GitProvider;
  owner: string;
  repo: string;
  /** Where to navigate when the user closes the explorer. Defaults to /. */
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

export const RepoExplorer: React.FC<Props> = ({ provider, owner, repo, onClose }) => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  const urlRef = searchParams.get('ref') || '';
  const urlPath = searchParams.get('path') || '';

  const [meta, setMeta] = useState<GitRepo | null>(null);
  const [metaError, setMetaError] = useState<string | null>(null);

  const [dir, setDir] = useState<DirState>({ path: '', entries: [], loading: true, error: null });
  const [file, setFile] = useState<BlobState | null>(null);

  // Branch dropdown state
  const [branchMenuOpen, setBranchMenuOpen] = useState(false);
  const [branches, setBranches] = useState<GitBranch[] | null>(null);
  const [branchesLoading, setBranchesLoading] = useState(false);
  const branchBtnRef = useRef<HTMLButtonElement | null>(null);

  // Effective ref: URL wins over the repo's default branch.
  const ref = urlRef || meta?.default_branch || '';

  // Helper: rebuild the search-params object so we can navigate to "same
  // base path, updated query" without reaching for window.location.
  const updateQuery = useCallback((next: { ref?: string | null; path?: string | null }) => {
    const sp = new URLSearchParams(searchParams);
    if ('ref'  in next) { next.ref  ? sp.set('ref',  next.ref!)  : sp.delete('ref'); }
    if ('path' in next) { next.path ? sp.set('path', next.path!) : sp.delete('path'); }
    navigate({ search: sp.toString() ? `?${sp}` : '' }, { replace: true });
  }, [navigate, searchParams]);

  // Load repo metadata once per (provider, owner, repo).
  useEffect(() => {
    let cancelled = false;
    setMetaError(null);
    setMeta(null);
    getRepo(provider, owner, repo)
      .then(r => { if (!cancelled) setMeta(r); })
      .catch(e => { if (!cancelled) setMetaError((e as Error)?.message || 'failed to load repo'); });
    return () => { cancelled = true; };
  }, [provider, owner, repo]);

  // Resolve URL `path` against ref. The path may be either a file or a dir;
  // we try the tree endpoint first, fall back to the blob endpoint when the
  // backend says "this path is a file".
  useEffect(() => {
    if (!ref) return;
    let cancelled = false;
    // Empty path → just load root tree, clear any open file.
    if (!urlPath) {
      setFile(null);
      setDir({ path: '', entries: [], loading: true, error: null });
      getTree(provider, owner, repo, ref, '')
        .then(entries => { if (!cancelled) setDir({ path: '', entries: [...entries].sort(compareEntries), loading: false, error: null }); })
        .catch(e => { if (!cancelled) setDir({ path: '', entries: [], loading: false, error: (e as Error)?.message || 'failed to load tree' }); });
      return () => { cancelled = true; };
    }
    // Non-empty path: try as directory. If the backend answers with a 400
    // (path is a file), interpret as a file path and fetch the blob.
    setDir({ path: urlPath, entries: [], loading: true, error: null });
    getTree(provider, owner, repo, ref, urlPath)
      .then(entries => {
        if (cancelled) return;
        setDir({ path: urlPath, entries: [...entries].sort(compareEntries), loading: false, error: null });
        setFile(null);
      })
      .catch(() => {
        // Treat any tree error as "probably a file" — fall back to blob.
        if (cancelled) return;
        const dirOfFile = parentOf(urlPath);
        setDir({ path: dirOfFile, entries: [], loading: true, error: null });
        // Load the parent dir for tree pane, plus the file itself in parallel.
        Promise.all([
          getTree(provider, owner, repo, ref, dirOfFile).catch(() => [] as GitTreeEntry[]),
          getBlob(provider, owner, repo, ref, urlPath),
        ]).then(([entries, blob]) => {
          if (cancelled) return;
          setDir({ path: dirOfFile, entries: [...entries].sort(compareEntries), loading: false, error: null });
          setFile({ path: urlPath, blob, loading: false, error: null });
        }).catch(e => {
          if (cancelled) return;
          setFile({ path: urlPath, blob: null, loading: false, error: (e as Error)?.message || 'failed to load file' });
        });
      });
    return () => { cancelled = true; };
  }, [provider, owner, repo, ref, urlPath]);

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
    if (e.type === 'dir') { updateQuery({ path: e.path }); return; }
    if (e.type === 'file') { updateQuery({ path: e.path }); return; }
    // submodule / symlink — no in-app browse
  };

  const goToDir = (path: string) => updateQuery({ path: path || null });

  const openBranchMenu = () => {
    setBranchMenuOpen(true);
    if (!branches && !branchesLoading) {
      setBranchesLoading(true);
      getBranches(provider, owner, repo)
        .then(bs => setBranches(bs))
        .catch(() => setBranches([]))
        .finally(() => setBranchesLoading(false));
    }
  };

  const switchBranch = (name: string) => {
    setBranchMenuOpen(false);
    if (name === ref) return;
    // Branch switch invalidates the current path (it may not exist on the
    // new ref). Drop path; user can re-navigate from the root tree.
    updateQuery({ ref: name, path: null });
  };

  // Breadcrumbs live in the tree pane header; first crumb is the repo name.
  const treePaneBreadcrumbs = (() => {
    const segments = dir.path === '' ? [] : dir.path.split('/');
    return (
      <div className="explorer-breadcrumbs">
        <button className="explorer-crumb" onClick={() => goToDir('')}>{repo}</button>
        {segments.map((seg, i) => {
          const sub = segments.slice(0, i + 1).join('/');
          return (
            <React.Fragment key={sub}>
              <span className="explorer-crumb-sep">/</span>
              <button className="explorer-crumb" onClick={() => goToDir(sub)}>{seg}</button>
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
          <div className="explorer-empty-title">{meta?.description || `${owner}/${repo}`}</div>
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
        <span className="explorer-title-text">{owner}/{repo}</span>
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
        <a className="explorer-link" href={`https://github.com/${owner}/${repo}`} target="_blank" rel="noopener noreferrer">
          Open on GitHub ↗
        </a>
      </div>

      {metaError && <div className="explorer-banner explorer-banner--error">Failed to load repository metadata: {metaError}</div>}

      <div className="explorer-body">
        <div className="explorer-main">
          <div className="explorer-file">{fileContent}</div>
        </div>

        <aside className="explorer-tree">
          <div className="explorer-tree-header">{treePaneBreadcrumbs}</div>
          {dir.loading && <div className="explorer-tree-loading">Loading…</div>}
          {dir.error && <div className="explorer-tree-error">{dir.error}</div>}
          {!dir.loading && !dir.error && (
            <ul className="explorer-tree-list">
              {dir.path !== '' && (
                <li>
                  <button className="explorer-tree-item explorer-tree-item--up" onClick={() => goToDir(parentOf(dir.path))}>
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
