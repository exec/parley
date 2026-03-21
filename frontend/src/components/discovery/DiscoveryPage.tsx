import React, { useState, useEffect, useCallback, useRef } from 'react';
import { PublicServer, ServerCategory } from '../../api/types';
import { discoverServers, getServerCategories } from '../../api/discovery';
import './DiscoveryPage.css';

interface Props {
  currentUserId?: string;
  joinedServerIds: Set<string>;
  onJoin: (vanityUrl: string) => void;
}

const PAGE_SIZE = 24;

export const DiscoveryPage: React.FC<Props> = ({ currentUserId: _currentUserId, joinedServerIds, onJoin }) => {
  const [categories, setCategories] = useState<ServerCategory[]>([]);
  const [servers, setServers] = useState<PublicServer[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [selectedCategory, setSelectedCategory] = useState<number | null>(null);
  const [query, setQuery] = useState('');
  const [loading, setLoading] = useState(false);

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Load categories once on mount
  useEffect(() => {
    getServerCategories().then(setCategories).catch(() => {});
  }, []);

  const loadServers = useCallback(async (q: string, catId: number | null, pg: number) => {
    setLoading(true);
    try {
      const result = await discoverServers({ page: pg, categoryId: catId ?? undefined, q: q || undefined });
      setServers(result.servers ?? []);
      setTotal(result.total ?? 0);
    } catch {
      setServers([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, []);

  // Fetch on page/category change (immediate)
  useEffect(() => {
    loadServers(query, selectedCategory, page);
  }, [page, selectedCategory]); // eslint-disable-line

  // Debounce search input
  const handleSearch = (val: string) => {
    setQuery(val);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      setPage(1);
      loadServers(val, selectedCategory, 1);
    }, 300);
  };

  const handleCategorySelect = (id: number | null) => {
    setSelectedCategory(id);
    setPage(1);
  };

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="discovery-page">
      <div className="discovery-header">
        <h1 className="discovery-title">Discover Servers</h1>
        <input
          className="discovery-search"
          type="text"
          placeholder="Search servers..."
          value={query}
          onChange={e => handleSearch(e.target.value)}
          maxLength={100}
        />
      </div>

      <div className="discovery-filters">
        <button
          className={`discovery-cat-pill${selectedCategory === null ? ' active' : ''}`}
          onClick={() => handleCategorySelect(null)}
        >
          All
        </button>
        {categories.map(cat => (
          <button
            key={cat.id}
            className={`discovery-cat-pill${selectedCategory === cat.id ? ' active' : ''}`}
            onClick={() => handleCategorySelect(cat.id)}
          >
            {cat.name}
          </button>
        ))}
      </div>

      {loading ? (
        <div className="discovery-loading">Loading...</div>
      ) : servers.length === 0 ? (
        <div className="discovery-empty">No servers found.</div>
      ) : (
        <div className="discovery-grid">
          {servers.map(server => {
            const isJoined = joinedServerIds.has(server.id);
            return (
              <div key={server.id} className="discovery-card">
                <div className="discovery-card-icon">
                  {server.icon_url ? (
                    <img src={server.icon_url} alt={server.name} />
                  ) : (
                    <span>{server.name.charAt(0).toUpperCase()}</span>
                  )}
                </div>
                <div className="discovery-card-body">
                  <div className="discovery-card-name">{server.name}</div>
                  {server.description && (
                    <div className="discovery-card-desc">{server.description}</div>
                  )}
                  <div className="discovery-card-meta">
                    <span>{server.member_count.toLocaleString()} members</span>
                    {server.categories.map(c => (
                      <span key={c.id} className="discovery-card-cat">{c.name}</span>
                    ))}
                  </div>
                </div>
                <button
                  className={`discovery-join-btn${isJoined ? ' joined' : ''}`}
                  onClick={() => !isJoined && onJoin(server.vanity_url)}
                  disabled={isJoined}
                >
                  {isJoined ? 'Joined' : 'Join'}
                </button>
              </div>
            );
          })}
        </div>
      )}

      {totalPages > 1 && (
        <div className="discovery-pagination">
          <button disabled={page <= 1} onClick={() => setPage(p => p - 1)}>Prev</button>
          <span>Page {page} of {totalPages}</span>
          <button disabled={page >= totalPages} onClick={() => setPage(p => p + 1)}>Next</button>
        </div>
      )}
    </div>
  );
};
