import React, { useState, useEffect, useRef } from 'react';
import { searchUsers } from '../../api/users';
import type { PublicUser } from '../../api/types';
import './UserMultiPicker.css';

interface Props {
  selected: PublicUser[];
  onChange: (next: PublicUser[]) => void;
  excludeUserIds?: string[];
  maxCount?: number;
  placeholder?: string;
}

export const UserMultiPicker: React.FC<Props> = ({
  selected,
  onChange,
  excludeUserIds = [],
  maxCount = 99,
  placeholder = 'Search users…',
}) => {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<PublicUser[]>([]);
  const [loading, setLoading] = useState(false);
  const debounceRef = useRef<number | null>(null);

  useEffect(() => {
    if (debounceRef.current) window.clearTimeout(debounceRef.current);
    if (!query.trim()) { setResults([]); return; }
    setLoading(true);
    debounceRef.current = window.setTimeout(async () => {
      try {
        const list = await searchUsers(query.trim());
        const excluded = new Set([...excludeUserIds, ...selected.map(u => u.id)]);
        setResults(list.filter(u => !excluded.has(u.id)).slice(0, 10));
      } finally {
        setLoading(false);
      }
    }, 300);
    return () => {
      if (debounceRef.current) window.clearTimeout(debounceRef.current);
    };
  }, [query, selected, excludeUserIds]);

  const add = (u: PublicUser) => {
    if (selected.length >= maxCount) return;
    onChange([...selected, u]);
    setQuery('');
    setResults([]);
  };

  const remove = (userId: string) => {
    onChange(selected.filter(u => u.id !== userId));
  };

  return (
    <div className="user-multi-picker">
      {selected.length > 0 && (
        <div className="ump-chips">
          {selected.map(u => (
            <span key={u.id} className="ump-chip">
              {u.display_name || u.username}
              <button type="button" onClick={() => remove(u.id)} aria-label={`Remove ${u.username}`}>×</button>
            </span>
          ))}
        </div>
      )}
      <input
        type="text"
        className="form-input"
        placeholder={selected.length >= maxCount ? `Max ${maxCount} users selected` : placeholder}
        value={query}
        onChange={e => setQuery(e.target.value)}
        disabled={selected.length >= maxCount}
      />
      {results.length > 0 && (
        <ul className="ump-results">
          {results.map(u => (
            <li key={u.id} className="ump-result-row" onClick={() => add(u)}>
              <div className="ump-avatar">
                {u.avatar_url
                  ? <img src={u.avatar_url} alt="" />
                  : (u.display_name || u.username || '?').charAt(0).toUpperCase()}
              </div>
              <span className="ump-name">{u.display_name || u.username}</span>
              <span className="ump-username">@{u.username}</span>
            </li>
          ))}
        </ul>
      )}
      {loading && <div className="ump-loading">Searching…</div>}
    </div>
  );
};
