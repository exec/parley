import React from 'react';
import { BinPost, BinChannelTag } from '../../api/types';
import './PostListItem.css';

interface PostListItemProps {
  post: BinPost;
  tags: BinChannelTag[];
  onClick: () => void;
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

export const PostListItem: React.FC<PostListItemProps> = ({ post, tags, onClick }) => {
  const tagMap = React.useMemo(() => {
    const m: Record<string, BinChannelTag> = {};
    tags.forEach(t => { m[t.name] = t; });
    return m;
  }, [tags]);

  const initials = (post.author_username || '?').slice(0, 2).toUpperCase();

  return (
    <div className="post-list-item" onClick={onClick} role="button" tabIndex={0}
      onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') onClick(); }}>

      <div className="post-list-item-top">
        <span className="post-list-item-title">{post.title}</span>
        <span className="post-list-item-time">{formatRelativeTime(post.updated_at)}</span>
      </div>

      {post.description && (
        <div className="post-list-item-desc">{post.description}</div>
      )}

      {post.tags && post.tags.length > 0 && (
        <div className="post-list-item-tags">
          {post.tags.map(tagName => {
            const tag = tagMap[tagName];
            const color = tag?.color || '#32CD32';
            return (
              <span
                key={tagName}
                className="post-list-item-tag"
                style={{ borderColor: color, color }}
              >
                {tagName}
              </span>
            );
          })}
        </div>
      )}

      <div className="post-list-item-bottom">
        <div className="post-list-item-author">
          <div className="post-list-item-avatar">
            {post.author_avatar_url
              ? <img src={post.author_avatar_url} alt={post.author_username} />
              : initials
            }
          </div>
          <span className="post-list-item-username">{post.author_username}</span>
        </div>

        <div className="post-list-item-stats">
          <span className="post-stat">
            <span className="post-stat-icon">&#128196;</span>
            {post.files?.length ?? 0} {(post.files?.length ?? 0) === 1 ? 'file' : 'files'}
          </span>
          <span className="post-stat">
            <span className="post-stat-icon">&#128172;</span>
            {post.comment_count ?? 0}
          </span>
        </div>
      </div>
    </div>
  );
};

export default PostListItem;
