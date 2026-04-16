import { useCallback, useEffect, useRef, useState } from 'react';
import { BotCommand } from '../api/types';
import { listServerCommands } from '../api/slashCommands';

const CACHE_TTL_MS = 30_000;

interface CacheEntry {
  commands: BotCommand[];
  fetchedAt: number;
  inflight?: Promise<BotCommand[]>;
}

// Module-scope cache: survives channel switches within the same server.
const cache = new Map<string, CacheEntry>();

export interface UseSlashCommandsResult {
  commands: BotCommand[];
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
}

/**
 * Fetch + cache slash commands for a server. Stale-while-revalidate:
 * returns cached value immediately and re-fetches silently when the cache is older
 * than CACHE_TTL_MS. `refresh()` forces a fetch regardless of cache age.
 */
export function useSlashCommands(serverID: string | undefined): UseSlashCommandsResult {
  const [commands, setCommands] = useState<BotCommand[]>(() => {
    if (!serverID) return [];
    return cache.get(serverID)?.commands ?? [];
  });
  const [loading, setLoading] = useState<boolean>(() => {
    if (!serverID) return false;
    return !cache.has(serverID);
  });
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const fetchCommands = useCallback(async (sid: string, force: boolean): Promise<void> => {
    const existing = cache.get(sid);
    const fresh = existing && (Date.now() - existing.fetchedAt) < CACHE_TTL_MS;
    if (existing && fresh && !force) {
      // Cache hit, nothing to do — just make sure state reflects it.
      if (mountedRef.current) {
        setCommands(existing.commands);
        setLoading(false);
        setError(null);
      }
      return;
    }

    // Share in-flight requests across concurrent callers for the same server.
    if (existing?.inflight && !force) {
      try {
        const result = await existing.inflight;
        if (mountedRef.current) {
          setCommands(result);
          setLoading(false);
          setError(null);
        }
      } catch (err) {
        if (mountedRef.current) {
          setError(err instanceof Error ? err.message : 'Failed to load commands');
          setLoading(false);
        }
      }
      return;
    }

    // Kick off the fetch. We stash the promise on the cache entry so other
    // hook mounts can piggy-back rather than issuing duplicate requests.
    const promise = listServerCommands(sid);
    const entry: CacheEntry = existing
      ? { ...existing, inflight: promise }
      : { commands: [], fetchedAt: 0, inflight: promise };
    cache.set(sid, entry);

    // If we had no cache at all, surface the loading state.
    if (!existing) {
      if (mountedRef.current) setLoading(true);
    }

    try {
      const result = await promise;
      cache.set(sid, { commands: result, fetchedAt: Date.now() });
      if (mountedRef.current) {
        setCommands(result);
        setLoading(false);
        setError(null);
      }
    } catch (err) {
      // Drop the inflight marker so a later attempt can retry.
      const current = cache.get(sid);
      if (current?.inflight === promise) {
        if (current.fetchedAt === 0) {
          cache.delete(sid);
        } else {
          cache.set(sid, { commands: current.commands, fetchedAt: current.fetchedAt });
        }
      }
      if (mountedRef.current) {
        setError(err instanceof Error ? err.message : 'Failed to load commands');
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    if (!serverID) {
      setCommands([]);
      setLoading(false);
      setError(null);
      return;
    }
    const existing = cache.get(serverID);
    if (existing) {
      // Show the cached commands immediately, then revalidate if stale.
      setCommands(existing.commands);
      setLoading(false);
      setError(null);
      const isStale = (Date.now() - existing.fetchedAt) >= CACHE_TTL_MS;
      if (isStale) {
        void fetchCommands(serverID, false);
      }
    } else {
      setCommands([]);
      setLoading(true);
      setError(null);
      void fetchCommands(serverID, false);
    }
  }, [serverID, fetchCommands]);

  const refresh = useCallback(async () => {
    if (!serverID) return;
    await fetchCommands(serverID, true);
  }, [serverID, fetchCommands]);

  return { commands, loading, error, refresh };
}

// ----- Slash-command detection helpers -----

export interface SlashCommandMatch {
  /** text after the leading '/', up to the cursor (may contain no spaces for v1) */
  query: string;
  /** always 0 in v1 — we only match '/' at the very start of the textarea */
  position: number;
}

/**
 * Detect an active slash-command query at the cursor position.
 * v1 is intentionally start-only: the textarea must begin with '/' and the cursor
 * must sit within the command name (no whitespace between '/' and cursor).
 * This avoids colliding with mid-sentence slashes (URLs, "and/or", etc.).
 */
export function detectSlashCommand(text: string, cursorPos: number): SlashCommandMatch | null {
  if (!text.startsWith('/')) return null;
  const before = text.slice(1, cursorPos);
  // Once the user types a space after the command name they've moved past the
  // autocomplete region; the option-picker UI takes over from here.
  if (/\s/.test(before)) return null;
  return { query: before, position: 0 };
}
