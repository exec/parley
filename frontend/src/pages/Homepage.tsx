import { useState } from 'react';
import { DmChannel } from '../api/types';
import { searchUsers } from '../api/users';
import { PublicUser } from '../api/types';

interface HomepageProps {
  onCreateServer: () => void;
  dmChannels: DmChannel[];
  onSelectDm: (channelId: string) => void;
  onOpenDm: (userId: string) => void;
  currentUserId?: string;
}

export function Homepage({ onCreateServer, dmChannels, onSelectDm, onOpenDm, currentUserId }: HomepageProps) {
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<PublicUser[]>([]);

  const handleSearch = async (query: string) => {
    setSearchQuery(query);
    if (query.length < 2) {
      setSearchResults([]);
      return;
    }
    try {
      const users = await searchUsers(query);
      // Filter out current user
      setSearchResults(users.filter(u => u.id !== currentUserId));
    } catch (err) {
      console.error(err);
    }
  };

  return (
    <div className="homepage-layout">
      <div className="homepage-sidebar">
        <div className="homepage-header">
          <h1 className="homepage-logo">Parley</h1>
        </div>

        <div className="homepage-dm-section">
          <div className="homepage-section-title">Direct Messages</div>
          <div className="dm-list">
            {dmChannels.map(channel => (
              <div
                key={channel.id}
                className="dm-item"
                onClick={() => onSelectDm(channel.id)}
              >
                <div className="dm-avatar">
                  {channel.other_username.charAt(0).toUpperCase()}
                </div>
                <span className="dm-name">{channel.other_username}</span>
              </div>
            ))}
            {dmChannels.length === 0 && (
              <div className="no-dms">No direct messages yet</div>
            )}
          </div>
        </div>

        <div className="homepage-user-section">
          <div className="user-account">
            <div className="user-avatar-small">U</div>
            <span className="user-username">User</span>
          </div>
        </div>
      </div>

      <div className="homepage-main">
        <div className="welcome-screen">
          <h1 className="welcome-title">Welcome to Parley!</h1>
          <p className="welcome-subtitle">Connect with friends and communities</p>

          <div className="search-container">
            <input
              type="text"
              className="user-search-input"
              placeholder="Find people to chat with..."
              value={searchQuery}
              onChange={(e) => handleSearch(e.target.value)}
            />
            {searchResults.length > 0 && (
              <div className="search-results">
                {searchResults.map(user => (
                  <div
                    key={user.id}
                    className="search-result-item"
                    onClick={() => {
                      onOpenDm(user.id);
                      setSearchQuery('');
                      setSearchResults([]);
                    }}
                  >
                    <div className="result-avatar">
                      {user.username.charAt(0).toUpperCase()}
                    </div>
                    <span className="result-username">{user.username}</span>
                  </div>
                ))}
              </div>
            )}
          </div>

          <button className="create-server-cta" onClick={onCreateServer}>
            Create a Server
          </button>
        </div>
      </div>
    </div>
  );
}