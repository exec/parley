import { useState, useEffect, useRef } from 'react';
import { getMyPermissions } from '../api/servers';
import { getMyChannelPermissions } from '../api/overwrites';
import { hasPerm, PERM_ADMINISTRATOR } from '../lib/permissions';

interface PermissionsResult {
  serverPerms: bigint;
  channelPerms: bigint;
  hasPerm: (perm: bigint) => boolean;
  loading: boolean;
}

// Simple in-memory cache to avoid redundant fetches within a session
const serverPermCache = new Map<string, bigint>();
const channelPermCache = new Map<string, bigint>();

// Listeners notified when the cache is invalidated so all hook instances re-fetch
const invalidationListeners = new Set<() => void>();

export function usePermissions(serverId?: string, channelId?: string): PermissionsResult {
  const [serverPerms, setServerPerms] = useState<bigint>(0n);
  const [channelPerms, setChannelPerms] = useState<bigint>(0n);
  const [loading, setLoading] = useState(false);
  // Bumped by invalidatePermCache to force the effect to re-run
  const [invalidateCount, setInvalidateCount] = useState(0);

  const prevServerIdRef = useRef<string | undefined>(undefined);
  const prevChannelIdRef = useRef<string | undefined>(undefined);

  // Subscribe to cache invalidation events for the lifetime of this hook instance
  useEffect(() => {
    const listener = () => {
      // Reset refs so the fetch effect treats this as a new server/channel
      prevServerIdRef.current = undefined;
      prevChannelIdRef.current = undefined;
      setInvalidateCount(c => c + 1);
    };
    invalidationListeners.add(listener);
    return () => { invalidationListeners.delete(listener); };
  }, []);

  useEffect(() => {
    if (!serverId) {
      setServerPerms(0n);
      setChannelPerms(0n);
      return;
    }

    const serverChanged = serverId !== prevServerIdRef.current;
    const channelChanged = channelId !== prevChannelIdRef.current;

    if (!serverChanged && !channelChanged) return;

    prevServerIdRef.current = serverId;
    prevChannelIdRef.current = channelId;

    let cancelled = false;
    setLoading(true);

    const fetchPerms = async () => {
      try {
        // Fetch server perms (possibly cached)
        let sPerms: bigint;
        if (serverPermCache.has(serverId)) {
          sPerms = serverPermCache.get(serverId)!;
        } else {
          const n = await getMyPermissions(serverId);
          sPerms = BigInt(n);
          serverPermCache.set(serverId, sPerms);
        }

        if (cancelled) return;
        setServerPerms(sPerms);

        // If admin at server level, channel perms are effectively all
        if (hasPerm(sPerms, PERM_ADMINISTRATOR)) {
          const allPerms = (1n << 42n) - 1n;
          setChannelPerms(allPerms);
          return;
        }

        // Fetch channel-specific perms if channelId provided
        if (channelId) {
          let cPerms: bigint;
          if (channelPermCache.has(channelId)) {
            cPerms = channelPermCache.get(channelId)!;
          } else {
            cPerms = await getMyChannelPermissions(channelId);
            channelPermCache.set(channelId, cPerms);
          }
          if (!cancelled) setChannelPerms(cPerms);
        } else {
          if (!cancelled) setChannelPerms(sPerms);
        }
      } catch {
        // On error, fall back to no permissions (safe default)
        if (!cancelled) {
          setServerPerms(0n);
          setChannelPerms(0n);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    fetchPerms();
    return () => { cancelled = true; };
  }, [serverId, channelId, invalidateCount]); // eslint-disable-line react-hooks/exhaustive-deps

  const checkPerm = (perm: bigint): boolean => {
    const effectivePerms = channelId ? channelPerms : serverPerms;
    return hasPerm(effectivePerms, perm);
  };

  return { serverPerms, channelPerms, hasPerm: checkPerm, loading };
}

/** Invalidate the permission caches for a server and/or channel, then notify all hook instances. */
export function invalidatePermCache(serverId?: string, channelId?: string) {
  if (serverId) serverPermCache.delete(serverId);
  if (channelId) channelPermCache.delete(channelId);
  invalidationListeners.forEach(fn => fn());
}
