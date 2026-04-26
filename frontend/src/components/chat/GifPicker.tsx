import React, { useState, useEffect, useRef, useCallback } from 'react';
import { createPortal } from 'react-dom';
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
  const anchorRef = useRef<HTMLSpanElement>(null); // invisible inline anchor we put inside the parent
  const pickerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [pos, setPos] = useState<{ left: number; bottom: number; width: number } | null>(null);

  const recomputePos = useCallback(() => {
    const anchor = anchorRef.current?.parentElement;
    if (!anchor) return;
    const rect = anchor.getBoundingClientRect();
    setPos({
      left: rect.left,
      bottom: window.innerHeight - rect.top + 8,
      width: rect.width,
    });
  }, []);

  // Recompute on mount + window resize/scroll. ResizeObserver handles the
  // common "user resized split" case.
  useEffect(() => {
    recomputePos();
    const onResize = () => recomputePos();
    window.addEventListener('resize', onResize);
    window.addEventListener('scroll', onResize, true);
    const anchor = anchorRef.current?.parentElement;
    let ro: ResizeObserver | null = null;
    if (anchor && typeof ResizeObserver !== 'undefined') {
      ro = new ResizeObserver(recomputePos);
      ro.observe(anchor);
    }
    return () => {
      window.removeEventListener('resize', onResize);
      window.removeEventListener('scroll', onResize, true);
      ro?.disconnect();
    };
  }, [recomputePos]);

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
      if (pickerRef.current && !pickerRef.current.contains(e.target as Node)) {
        // Don't close on clicks within the original parent (where the GIF
        // toggle button lives) so the toggle still works as expected.
        const anchor = anchorRef.current?.parentElement;
        if (anchor && anchor.contains(e.target as Node)) return;
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

  const portal = pos
    ? createPortal(
        <div
          ref={pickerRef}
          className="gif-picker"
          style={{
            position: 'fixed',
            left: pos.left,
            right: 'auto',
            bottom: pos.bottom,
            width: pos.width,
          }}
        >
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
        </div>,
        document.body,
      )
    : null;

  return (
    <>
      <span ref={anchorRef} style={{ display: 'none' }} aria-hidden="true" />
      {portal}
    </>
  );
};
