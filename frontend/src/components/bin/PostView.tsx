import React, { useState, useEffect, useCallback, useRef } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { BinPost, BinPostVersion, BinLineComment, Message } from '../../api/types';
import { getPost, getVersions, getVersion, getLineComments, deletePost, updateLineComment, deleteLineComment } from '../../api/bin';
import { getMessages, sendMessage as apiSendMessage, editMessage, deleteMessage, toggleReaction } from '../../api/messages';
import { CodeBlock } from '../ui/CodeBlock';
import ShikiCodeBlock from '../ui/ShikiCodeBlock';
import { MessageList } from '../chat/MessageList';
import { MessageInput } from '../chat/MessageInput';
import { LineCommentForm } from './LineCommentForm';
import './PostView.css';

interface PostViewProps {
  postId: string;
  onBack: () => void;
  currentUserId?: string;
}

type TabKey = 'files' | 'comments' | 'linenotes';

interface ActiveLineComment {
  lineNumber: number;
  fileId: string;
  versionId: string;
}

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

export const PostView: React.FC<PostViewProps> = ({ postId, onBack, currentUserId }) => {
  const [post, setPost] = useState<BinPost | null>(null);
  const [versions, setVersions] = useState<BinPostVersion[]>([]);
  const [selectedVersionId, setSelectedVersionId] = useState<string | null>(null);
  const [versionFiles, setVersionFiles] = useState<BinPostVersion['files']>(undefined);
  const [lineComments, setLineComments] = useState<BinLineComment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<TabKey>('files');
  const [activeFileIndex, setActiveFileIndex] = useState(0);

  // Line comment form state
  const [activeLineComment, setActiveLineComment] = useState<ActiveLineComment | null>(null);

  // Thread channel messages state
  const [threadMessages, setThreadMessages] = useState<Message[]>([]);
  const [threadLoading, setThreadLoading] = useState(false);
  const threadChannelIdRef = useRef<string | null>(null);

  // Reply-to state for thread comments
  const [replyTo, setReplyTo] = useState<Message | null>(null);

  // Line note edit state: commentId -> draft content (null = not editing)
  const [editingLineNote, setEditingLineNote] = useState<Record<string, string | null>>({});

  const refreshLineComments = useCallback(async () => {
    try {
      const comments = await getLineComments(postId);
      setLineComments(comments);
    } catch {
      // non-fatal
    }
  }, [postId]);

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
        threadChannelIdRef.current = fetchedPost.thread_channel_id;
        setActiveFileIndex(0);
        // Auto-select the latest version so line comments always reference
        // bin_post_version_files IDs (required by the FK constraint).
        if (fetchedVersions.length > 0) {
          setSelectedVersionId(fetchedVersions[0].id);
        }
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to load post');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => { cancelled = true; };
  }, [postId]);

  // Load thread channel messages when Comments tab is selected
  useEffect(() => {
    if (activeTab !== 'comments') return;
    const channelId = threadChannelIdRef.current;
    if (!channelId) return;

    let cancelled = false;
    setThreadLoading(true);
    getMessages(channelId, { limit: 100 })
      .then((msgs) => {
        if (!cancelled) setThreadMessages(msgs);
      })
      .catch(() => {
        // non-fatal
      })
      .finally(() => {
        if (!cancelled) setThreadLoading(false);
      });

    return () => { cancelled = true; };
  }, [activeTab]);

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

  const currentFiles = versionFiles ?? [];
  const activeFile = currentFiles[activeFileIndex] ?? null;

  // "old version" means viewing something other than the latest (versions[0])
  const isViewingOldVersion = selectedVersionId !== null && selectedVersionId !== versions[0]?.id;
  const currentVersionObj = versions.find((v) => v.id === selectedVersionId);

  const highlightedLines = React.useMemo(() => {
    if (!activeFile) return new Set<number>();
    const fileId = activeFile.id;
    const lineNums = lineComments
      .filter((c) => c.file_id === fileId)
      .map((c) => c.line_number);
    return new Set(lineNums);
  }, [lineComments, activeFile]);

  const handleLineClick = useCallback(
    (lineNumber: number) => {
      if (!activeFile || !selectedVersionId) return;
      setActiveLineComment({
        lineNumber,
        fileId: activeFile.id,
        versionId: selectedVersionId,
      });
    },
    [activeFile, selectedVersionId, versions]
  );

  const handleLineCommentCreated = useCallback(
    (_comment: BinLineComment) => {
      // Refresh the full list so we always have server-canonical data
      refreshLineComments();
      setActiveLineComment(null);
    },
    [refreshLineComments]
  );

  const handleSendThreadMessage = useCallback(
    async (content: string, attachmentUrl?: string, attachmentName?: string, attachmentType?: string, parentId?: string) => {
      const channelId = threadChannelIdRef.current;
      if (!channelId || (!content.trim() && !attachmentUrl)) return;
      const nonce = crypto.randomUUID();
      try {
        const confirmed = await apiSendMessage(channelId, content, nonce, attachmentUrl, attachmentName, attachmentType, parentId);
        setThreadMessages((prev) => {
          if (prev.some((m) => m.id === confirmed.id)) return prev;
          return [...prev, confirmed];
        });
        setReplyTo(null);
      } catch {
        // swallow — user can retry
      }
    },
    []
  );

  // Thread message action handlers
  const handleEditThreadMessage = useCallback(
    async (message: Message) => {
      const newContent = window.prompt('Edit message:', message.content);
      if (newContent === null || newContent === message.content) return;
      try {
        const updated = await editMessage(message.id, newContent);
        setThreadMessages((prev) =>
          prev.map((m) => (m.id === updated.id ? updated : m))
        );
      } catch {
        // swallow
      }
    },
    []
  );

  const handleDeleteThreadMessage = useCallback(
    async (messageId: string) => {
      if (!window.confirm('Delete this message?')) return;
      try {
        await deleteMessage(messageId);
        setThreadMessages((prev) => prev.filter((m) => m.id !== messageId));
      } catch {
        // swallow
      }
    },
    []
  );

  const handleReactThreadMessage = useCallback(
    async (messageId: string, emoji: string) => {
      try {
        await toggleReaction(messageId, emoji);
        // Refresh messages to get updated reaction counts
        const channelId = threadChannelIdRef.current;
        if (channelId) {
          const msgs = await getMessages(channelId, { limit: 100 });
          setThreadMessages(msgs);
        }
      } catch {
        // swallow
      }
    },
    []
  );

  // Delete post handler
  const handleDeletePost = useCallback(async () => {
    if (!window.confirm('Are you sure you want to delete this post? This action cannot be undone.')) return;
    try {
      await deletePost(postId);
      onBack();
    } catch {
      // swallow
    }
  }, [postId, onBack]);

  // Line note edit/delete handlers
  const handleStartEditLineNote = useCallback((commentId: string, currentContent: string) => {
    setEditingLineNote((prev) => ({ ...prev, [commentId]: currentContent }));
  }, []);

  const handleCancelEditLineNote = useCallback((commentId: string) => {
    setEditingLineNote((prev) => ({ ...prev, [commentId]: null }));
  }, []);

  const handleSaveEditLineNote = useCallback(async (commentId: string) => {
    const newContent = editingLineNote[commentId];
    if (newContent === null || newContent === undefined) return;
    try {
      await updateLineComment(commentId, newContent);
      await refreshLineComments();
      setEditingLineNote((prev) => ({ ...prev, [commentId]: null }));
    } catch {
      // swallow
    }
  }, [editingLineNote, refreshLineComments]);

  const handleDeleteLineNote = useCallback(async (commentId: string) => {
    if (!window.confirm('Delete this line note?')) return;
    try {
      await deleteLineComment(commentId);
      await refreshLineComments();
    } catch {
      // swallow
    }
  }, [refreshLineComments]);

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
  const isPostAuthor = currentUserId !== undefined && String(currentUserId) === String(post.author_id);

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
                {versions.map((v, i) => (
                  <option key={v.id} value={v.id}>
                    v{v.version}{i === 0 ? ' (Latest)' : ''}{v.description ? ` — ${v.description}` : ''}
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

        {isPostAuthor && (
          <button
            className="post-view-delete-btn"
            onClick={handleDeletePost}
            title="Delete post"
          >
            Delete
          </button>
        )}
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

            <div className="post-view-files-body">
              {activeFile ? (
                <>
                  <CodeBlock
                    content={activeFile.content}
                    language={activeFile.language}
                    filename={currentFiles.length === 1 ? activeFile.filename : undefined}
                    showLineNumbers={true}
                    highlightedLines={highlightedLines}
                    onLineClick={handleLineClick}
                  />
                  {activeLineComment && activeLineComment.fileId === activeFile.id && (
                    <LineCommentForm
                      postId={postId}
                      versionId={activeLineComment.versionId}
                      fileId={activeLineComment.fileId}
                      lineNumber={activeLineComment.lineNumber}
                      onCreated={handleLineCommentCreated}
                      onCancel={() => setActiveLineComment(null)}
                    />
                  )}
                </>
              ) : (
                <div className="post-view-empty">No files attached.</div>
              )}
            </div>
          </div>
        )}

        {/* Comments tab — thread channel messages */}
        {activeTab === 'comments' && (
          <div className="post-view-comments">
            {threadLoading ? (
              <div className="post-view-comments-loading">
                <div className="bin-loading-spinner" />
              </div>
            ) : (
              <>
                <MessageList
                  messages={threadMessages}
                  allMessages={threadMessages}
                  currentUserId={currentUserId}
                  onEdit={handleEditThreadMessage}
                  onDelete={handleDeleteThreadMessage}
                  onReact={handleReactThreadMessage}
                  onReply={(msg) => setReplyTo(msg)}
                />
                <MessageInput
                  channelName="thread"
                  onSendMessage={handleSendThreadMessage}
                  replyTo={replyTo}
                  serverId={undefined}
                  channelId={threadChannelIdRef.current || undefined}
                />
              </>
            )}
          </div>
        )}

        {/* Line Notes tab */}
        {activeTab === 'linenotes' && (
          <div className="post-view-linenotes">
            {lineComments.length === 0 ? (
              <div className="post-view-empty">No line notes yet. Click a line number in the Files tab to add one.</div>
            ) : (
              lineComments.map((comment) => {
                const commentInitials = (comment.author_username || '?').slice(0, 2).toUpperCase();
                const file = (versionFiles ?? post.files ?? []).find(
                  (f) => f.id === comment.file_id
                );
                const isNoteAuthor = currentUserId !== undefined && String(currentUserId) === String(comment.author_id);
                const draftContent = editingLineNote[comment.id];
                const isEditing = draftContent !== null && draftContent !== undefined;

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
                      {isNoteAuthor && (
                        <div className="post-view-linenote-actions">
                          {!isEditing && (
                            <button
                              className="post-view-linenote-action-btn"
                              onClick={() => handleStartEditLineNote(comment.id, comment.content)}
                              title="Edit"
                            >
                              Edit
                            </button>
                          )}
                          <button
                            className="post-view-linenote-action-btn post-view-linenote-action-btn--delete"
                            onClick={() => handleDeleteLineNote(comment.id)}
                            title="Delete"
                          >
                            Delete
                          </button>
                        </div>
                      )}
                    </div>
                    {isEditing ? (
                      <div className="post-view-linenote-edit">
                        <textarea
                          className="post-view-linenote-edit-textarea"
                          value={draftContent}
                          onChange={(e) =>
                            setEditingLineNote((prev) => ({ ...prev, [comment.id]: e.target.value }))
                          }
                          rows={3}
                        />
                        <div className="post-view-linenote-edit-actions">
                          <button
                            className="post-view-linenote-action-btn"
                            onClick={() => handleSaveEditLineNote(comment.id)}
                          >
                            Save
                          </button>
                          <button
                            className="post-view-linenote-action-btn"
                            onClick={() => handleCancelEditLineNote(comment.id)}
                          >
                            Cancel
                          </button>
                        </div>
                      </div>
                    ) : (
                      <div className="post-view-linenote-content">{comment.content}</div>
                    )}
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
