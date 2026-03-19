import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useViewportAdjust } from '../../hooks/useViewportAdjust';
import { apiClient } from '../../api/client';
import './GifPicker.css';

interface GifResult {
  id: string;
  title: string;
  images: {
    fixed_width_small: { url: string };
    original: { url: string };
  };
}

interface GifPickerProps {
  onSelect: (url: string, title: string) => void;
  onClose: () => void;
}

export const GifPicker: React.FC<GifPickerProps> = ({ onSelect, onClose }) => {
  const [query, setQuery] = useState('');
  const [gifs, setGifs] = useState<GifResult[]>([]);
  const [loading, setLoading] = useState(false);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useViewportAdjust(containerRef, []);

  const fetchGifs = useCallback(async (q: string) => {
    setLoading(true);
    try {
      const params = new URLSearchParams({ limit: '24', rating: 'pg-13' });
      let endpoint: string;
      if (q.trim()) {
        params.set('q', q.trim());
        endpoint = `/giphy/search?${params.toString()}`;
      } else {
        endpoint = `/giphy/trending?${params.toString()}`;
      }
      const json = await apiClient.get<{ data: GifResult[] }>(endpoint);
      setGifs(json.data ?? []);
    } catch {
      setGifs([]);
    } finally {
      setLoading(false);
    }
  }, []);

  // Load trending on mount
  useEffect(() => {
    fetchGifs('');
    inputRef.current?.focus();
  }, [fetchGifs]);

  // Debounce search
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => fetchGifs(query), 400);
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current); };
  }, [query, fetchGifs]);

  // Close on outside click
  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        onClose();
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [onClose]);

  // Close on Escape
  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [onClose]);

  return (
    <div ref={containerRef} className="gif-picker">
      <div className="gif-picker-header">
        <input
          ref={inputRef}
          className="gif-picker-search"
          type="text"
          placeholder="Search GIFs…"
          value={query}
          onChange={e => setQuery(e.target.value)}
        />
        <span className="gif-picker-powered">via GIPHY</span>
      </div>

      <div className="gif-picker-grid">
        {loading && <div className="gif-picker-loading">Loading…</div>}
        {!loading && gifs.length === 0 && (
          <div className="gif-picker-empty">No results</div>
        )}
        {!loading && gifs.map(gif => (
          <button
            key={gif.id}
            className="gif-picker-item"
            onClick={() => onSelect(gif.images.original.url, gif.title)}
            title={gif.title}
          >
            <img
              src={gif.images.fixed_width_small.url}
              alt={gif.title}
              loading="lazy"
            />
          </button>
        ))}
      </div>
    </div>
  );
};
