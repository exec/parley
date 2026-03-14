import { useState } from 'react';
import { PublicUser, User } from '../api/types';
import { searchUsers } from '../api/users';
import './Homepage.css';

interface HomepageProps {
  currentUser?: User | null;
  onCreateServer: () => void;
  onOpenDm: (userId: string) => void;
}

export function Homepage({ currentUser, onCreateServer, onOpenDm }: HomepageProps) {
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<PublicUser[]>([]);
  const [isSearching, setIsSearching] = useState(false);

  const handleSearch = async (query: string) => {
    setSearchQuery(query);
    if (query.length < 2) {
      setSearchResults([]);
      return;
    }
    setIsSearching(true);
    try {
      const users = await searchUsers(query);
      setSearchResults((users ?? []).filter(u => u.id !== currentUser?.id));
    } catch (err) {
      console.error(err);
    } finally {
      setIsSearching(false);
    }
  };

  const handleStartDm = (userId: string) => {
    onOpenDm(userId);
    setSearchQuery('');
    setSearchResults([]);
  };

  return (
    <div className="homepage">
      <div className="homepage-hero">
        <div className="homepage-logo-mark">P</div>
        <h1 className="homepage-title">Welcome to Parley</h1>
        <p className="homepage-subtitle">
          Find a friend and start chatting, or create a new server.
        </p>
      </div>

      <div className="homepage-search-section">
        <label className="homepage-search-label">Find People</label>
        <div className="homepage-search-wrapper">
          <input
            type="text"
            className="homepage-search-input"
            placeholder="Search by username..."
            value={searchQuery}
            onChange={e => handleSearch(e.target.value)}
            autoComplete="off"
          />
          {isSearching && <div className="homepage-search-spinner" />}
          {searchResults.length > 0 && (
            <div className="homepage-search-results">
              {searchResults.map(user => (
                <div
                  key={user.id}
                  className="homepage-search-result"
                  onClick={() => handleStartDm(user.id)}
                >
                  <div className="homepage-result-avatar">
                    {user.username.charAt(0).toUpperCase()}
                  </div>
                  <div className="homepage-result-info">
                    <span className="homepage-result-name">{user.username}</span>
                    <span className="homepage-result-action">Send a message</span>
                  </div>
                </div>
              ))}
            </div>
          )}
          {searchQuery.length >= 2 && searchResults.length === 0 && !isSearching && (
            <div className="homepage-search-results">
              <div className="homepage-no-results">No users found for "{searchQuery}"</div>
            </div>
          )}
        </div>
      </div>

      <div className="homepage-divider">
        <span>or</span>
      </div>

      <div className="homepage-actions">
        <button className="homepage-create-btn" onClick={onCreateServer}>
          <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20">
            <path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z"/>
          </svg>
          Create a Server
        </button>
      </div>
    </div>
  );
}
