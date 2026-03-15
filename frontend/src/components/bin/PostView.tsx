import React, { useState, useEffect } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { BinPost, BinPostVersion, BinLineComment } from '../../api/types';
import { getPost, getVersions, getVersion, getLineComments } from '../../api/bin';
import { CodeBlock } from '../ui/CodeBlock';
import ShikiCodeBlock from '../ui/ShikiCodeBlock';
import './PostView.css';

interface PostViewProps {
  postId: string;
  onBack: () => void;
}

type TabKey = 'files' | 'comments' | 'linenotes';

function formatRelativeTime(dateStr: string): string {
  const date = new Date(dateStr);
  const now = Date.now();
  const diff = now - date.getTime();
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return 'just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return date.toLocaleDateString();
}

export const PostView: React.FC<PostViewProps> = ({ postId, onBack }) => {
  const [post, setPost] = useState<BinPost | null>(null);
  const [versions, setVersions] = useState<BinPostVersion[]>([]);
  const [selectedVersionId, setSelectedVersionId] = useState<string | null>(null);
  const [versionFiles, setVersionFiles] = useState<BinPostVersion['files']>(undefined);
  const [lineComments, setLineComments] = useState<BinLineComment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<TabKey>('files');
  const [activeFileIndex, setActiveFileIndex] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    Promise.all([
      getPost(postId),
      getVersions(postId),
      getLineComments(postId),
    ])
      .then(([fetchedPost, fetchedVersions, fetchedComments]) => {
        if (cancelled) return;
        setPost(fetchedPost);
        setVersions(fetchedVersions);
        setLineComments(fetchedComments);
        setActiveFileIndex(0);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to load post');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => { cancelled = true; };
  }, [postId]);

  // Load version files when a specific version is selected
  useEffect(() => {
    if (!selectedVersionId) {
      setVersionFiles(undefined);
      return;
    }
    let cancelled = false;
    getVersion(postId, selectedVersionId)
      .then((v) => {
        if (!cancelled) {
          setVersionFiles(v.files);
          setActiveFileIndex(0);
        }
      })
      .catch(() => {
        if (!cancelled) setVersionFiles(undefined);
      });
    return () => { cancelled = true; };
  }, [postId, selectedVersionId]);

  const currentFiles = versionFiles ?? post?.files ?? [];
  const activeFile = currentFiles[activeFileIndex] ?? null;

  const isViewingOldVersion = selectedVersionId !== null;
  const currentVersionObj = versions.find((v) => v.id === selectedVersionId);

  const highlightedLines = React.useMemo(() => {
    if (!activeFile) return new Set<number>();
    const fileId = activeFile.id;
    const lineNums = lineComments
      .filter((c) => c.file_id === fileId)
      .map((c) => c.line_number);
    return new Set(lineNums);
  }, [lineComments, activeFile]);

  if (loading) {
    return (
      <div className="post-view post-view--loading">
        <div className="bin-loading-spinner" />
        <span>Loading post...</span>
      </div>
    );
  }

  if (error || !post) {
    return (
      <div className="post-view post-view--error">
        <button className="post-view-back-btn" onClick={onBack}>&#8592; Back</button>
        <div className="post-view-error">{error ?? 'Post not found'}</div>
      </div>
    );
  }

  const authorInitials = (post.author_username || '?').slice(0, 2).toUpperCase();

  return (
    <div className="post-view">
      {/* ── Header ── */}
      <div className="post-view-header">
        <button className="post-view-back-btn" onClick={onBack}>
          &#8592; Back
        </button>

        <div className="post-view-header-main">
          <div className="post-view-title-row">
            <h1 className="post-view-title">{post.title}</h1>

            {versions.length > 1 && (
              <select
                className="post-view-version-select"
                value={selectedVersionId ?? ''}
                onChange={(e) => setSelectedVersionId(e.target.value || null)}
              >
                <option value="">Latest</option>
                {versions.map((v) => (
                  <option key={v.id} value={v.id}>
                    v{v.version}{v.description ? ` — ${v.description}` : ''}
                  </option>
                ))}
              </select>
            )}
          </div>

          <div className="post-view-meta">
            <div className="post-view-author">
              <div className="post-view-avatar">
                {post.author_avatar_url
                  ? <img src={post.author_avatar_url} alt={post.author_username} />
                  : <span>{authorInitials}</span>}
              </div>
              <span className="post-view-username">{post.author_username}</span>
            </div>
            <span className="post-view-time">{formatRelativeTime(post.created_at)}</span>

            {post.tags && post.tags.length > 0 && (
              <div className="post-view-tags">
                {post.tags.map((tag) => (
                  <span key={tag} className="post-view-tag-pill">{tag}</span>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* ── Old-version banner ── */}
      {isViewingOldVersion && (
        <div className="post-view-version-banner">
          Viewing v{currentVersionObj?.version ?? '?'}
          {currentVersionObj?.description ? ` — ${currentVersionObj.description}` : ''}
          {' '}
          <button
            className="post-view-version-banner-latest"
            onClick={() => setSelectedVersionId(null)}
          >
            Switch to latest
          </button>
        </div>
      )}

      {/* ── Description ── */}
      {post.description && (
        <div className="post-view-description">
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            components={{ code: ShikiCodeBlock }}
          >
            {post.description}
          </ReactMarkdown>
        </div>
      )}

      {/* ── Tabs ── */}
      <div className="post-view-tabs">
        <button
          className={`post-view-tab${activeTab === 'files' ? ' active' : ''}`}
          onClick={() => setActiveTab('files')}
        >
          Files{currentFiles.length > 0 ? ` (${currentFiles.length})` : ''}
        </button>
        <button
          className={`post-view-tab${activeTab === 'comments' ? ' active' : ''}`}
          onClick={() => setActiveTab('comments')}
        >
          Comments{post.comment_count > 0 ? ` (${post.comment_count})` : ''}
        </button>
        <button
          className={`post-view-tab${activeTab === 'linenotes' ? ' active' : ''}`}
          onClick={() => setActiveTab('linenotes')}
        >
          Line Notes{lineComments.length > 0 ? ` (${lineComments.length})` : ''}
        </button>
      </div>

      {/* ── Tab content ── */}
      <div className="post-view-tab-content">
        {/* Files tab */}
        {activeTab === 'files' && (
          <div className="post-view-files">
            {currentFiles.length > 1 && (
              <div className="post-view-file-tabs">
                {currentFiles.map((file, idx) => (
                  <button
                    key={file.id}
                    className={`post-view-file-tab${idx === activeFileIndex ? ' active' : ''}`}
                    onClick={() => setActiveFileIndex(idx)}
                  >
                    {file.filename || `File ${idx + 1}`}
                  </button>
                ))}
              </div>
            )}

            {activeFile ? (
              <CodeBlock
                content={activeFile.content}
                language={activeFile.language}
                filename={currentFiles.length === 1 ? activeFile.filename : undefined}
                showLineNumbers={true}
                highlightedLines={highlightedLines}
              />
            ) : (
              <div className="post-view-empty">No files attached.</div>
            )}
          </div>
        )}

        {/* Comments tab — wired in Task 23 */}
        {activeTab === 'comments' && (
          <div className="post-view-comments-placeholder">
            <span>Thread comments will appear here.</span>
          </div>
        )}

        {/* Line Notes tab */}
        {activeTab === 'linenotes' && (
          <div className="post-view-linenotes">
            {lineComments.length === 0 ? (
              <div className="post-view-empty">No line notes yet.</div>
            ) : (
              lineComments.map((comment) => {
                const commentInitials = (comment.author_username || '?').slice(0, 2).toUpperCase();
                const file = (versionFiles ?? post.files ?? []).find(
                  (f) => f.id === comment.file_id
                );
                return (
                  <div key={comment.id} className="post-view-linenote">
                    <div className="post-view-linenote-header">
                      <span className="post-view-linenote-badge">
                        {file ? `${file.filename} · ` : ''}Line {comment.line_number}
                      </span>
                      <div className="post-view-linenote-author">
                        <div className="post-view-linenote-avatar">
                          {comment.author_avatar_url
                            ? <img src={comment.author_avatar_url} alt={comment.author_username} />
                            : <span>{commentInitials}</span>}
                        </div>
                        <span className="post-view-linenote-username">{comment.author_username}</span>
                        <span className="post-view-linenote-time">{formatRelativeTime(comment.created_at)}</span>
                      </div>
                    </div>
                    <div className="post-view-linenote-content">{comment.content}</div>
                  </div>
                );
              })
            )}
          </div>
        )}
      </div>
    </div>
  );
};

export default PostView;
