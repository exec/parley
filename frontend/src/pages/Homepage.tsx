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
        <div className="homepage-logo-mark">
          <img src="/favicon.svg" alt="Parley" className="homepage-logo-svg" />
        </div>
        <h1 className="homepage-title">Welcome back{currentUser?.display_name || currentUser?.username ? `, ${currentUser.display_name || currentUser.username}` : ''}</h1>
        <p className="homepage-subtitle">
          Search for someone to message, or start a new server.
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
          {isSearching ? (
            <div className="homepage-search-spinner" />
          ) : (
            <svg className="homepage-search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="11" cy="11" r="8" /><path d="m21 21-4.35-4.35" />
            </svg>
          )}
          {searchResults.length > 0 && (
            <div className="homepage-search-results">
              {searchResults.map(user => (
                <div
                  key={user.id}
                  className="homepage-search-result"
                  onClick={() => handleStartDm(user.id)}
                >
                  <div className="homepage-result-avatar">
                    {(user.display_name || user.username).charAt(0).toUpperCase()}
                  </div>
                  <div className="homepage-result-info">
                    <span className="homepage-result-name">{user.display_name || user.username}</span>
                    <span className="homepage-result-action">Open direct message</span>
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
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="15" height="15">
            <path d="M12 5v14M5 12h14"/>
          </svg>
          Create a Server
        </button>
      </div>
    </div>
  );
}
