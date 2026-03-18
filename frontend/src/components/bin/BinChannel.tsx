import React, { useState, useEffect, useCallback } from 'react';
import { BinPost, BinChannelTag } from '../../api/types';
import { listPosts, getTags } from '../../api/bin';
import { PostListItem } from './PostListItem';
import { usePermissions } from '../../hooks/usePermissions';
import { PERM_CREATE_POSTS } from '../../lib/permissions';
import './BinChannel.css';

type SortOption = 'newest' | 'oldest' | 'recently_active';

interface BinChannelProps {
  channelId: string;
  serverId?: string;
  onOpenPost: (postId: string) => void;
  onNewPost: () => void;
}

export const BinChannel: React.FC<BinChannelProps> = ({ channelId, serverId, onOpenPost, onNewPost }) => {
  const { hasPerm: checkPerm } = usePermissions(serverId, channelId);
  const canCreatePosts = !serverId || checkPerm(PERM_CREATE_POSTS);
  const [posts, setPosts] = useState<BinPost[]>([]);
  const [tags, setTags] = useState<BinChannelTag[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedTags, setSelectedTags] = useState<Set<string>>(new Set());
  const [sort, setSort] = useState<SortOption>('newest');

  const fetchData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [postsResult, tagsResult] = await Promise.allSettled([
        listPosts(channelId),
        getTags(channelId),
      ]);
      if (postsResult.status === 'rejected') {
        const err = postsResult.reason;
        setError((err as any)?.message || 'Failed to load posts');
      } else {
        setPosts(postsResult.value);
      }
      setTags(tagsResult.status === 'fulfilled' ? tagsResult.value : []);
    } finally {
      setLoading(false);
    }
  }, [channelId]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const toggleTag = (tagName: string) => {
    setSelectedTags(prev => {
      const next = new Set(prev);
      if (next.has(tagName)) {
        next.delete(tagName);
      } else {
        next.add(tagName);
      }
      return next;
    });
  };

  const filteredPosts = React.useMemo(() => {
    let result = posts;

    if (selectedTags.size > 0) {
      result = result.filter(p =>
        p.tags && p.tags.some(t => selectedTags.has(t))
      );
    }

    result = [...result].sort((a, b) => {
      if (sort === 'newest') {
        return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
      } else if (sort === 'oldest') {
        return new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
      } else {
        // recently_active: sort by updated_at
        return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime();
      }
    });

    return result;
  }, [posts, selectedTags, sort]);

  return (
    <div className="bin-channel">
      <div className="bin-channel-header">
        <div className="bin-channel-title">
          <span className="bin-channel-icon">&lt;/&gt;</span>
          Bin
        </div>
        {canCreatePosts && (
          <button className="bin-new-post-btn" onClick={onNewPost}>
            + New Post
          </button>
        )}
      </div>

      <div className="bin-filter-bar">
        <span className="bin-filter-label">Tags:</span>
        <div className="bin-filter-tags">
          {tags.length === 0 && !loading && (
            <span style={{ fontSize: 12, color: 'var(--parley-text-dim)' }}>None</span>
          )}
          {tags.map(tag => (
            <button
              key={tag.id}
              className={`bin-tag-pill${selectedTags.has(tag.name) ? ' active' : ''}`}
              style={{
                borderColor: tag.color,
                color: tag.color,
              }}
              onClick={() => toggleTag(tag.name)}
            >
              {tag.name}
            </button>
          ))}
        </div>

        <select
          className="bin-sort-select"
          value={sort}
          onChange={e => setSort(e.target.value as SortOption)}
        >
          <option value="newest">Newest</option>
          <option value="oldest">Oldest</option>
          <option value="recently_active">Recently Active</option>
        </select>
      </div>

      {loading && (
        <div className="bin-loading-state">
          <div className="bin-loading-spinner" />
          <span>Loading posts...</span>
        </div>
      )}

      {!loading && error && (
        <div className="bin-error-state">{error}</div>
      )}

      {!loading && !error && (
        <div className="bin-post-list">
          {filteredPosts.length === 0 ? (
            <div className="bin-empty-state">
              <div className="bin-empty-icon">&lt;/&gt;</div>
              <div className="bin-empty-label">
                {selectedTags.size > 0 ? 'No matching posts' : 'No posts yet'}
              </div>
              <div className="bin-empty-hint">
                {selectedTags.size > 0
                  ? 'No posts match the selected tags.'
                  : 'Be the first to share some code!'}
              </div>
            </div>
          ) : (
            filteredPosts.map(post => (
              <PostListItem
                key={post.id}
                post={post}
                tags={tags}
                onClick={() => onOpenPost(post.id)}
              />
            ))
          )}
        </div>
      )}
    </div>
  );
};

export default BinChannel;
