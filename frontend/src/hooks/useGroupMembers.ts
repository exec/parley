import { useEffect, useState, useCallback } from 'react';
import { apiClient } from '../api/client';
import type { DmChannelMember } from '../api/types';

export interface UseGroupMembersResult {
  members: DmChannelMember[];
  loading: boolean;
  refetch: () => Promise<void>;
}

export function useGroupMembers(dmChannelId: string | null): UseGroupMembersResult {
  const [members, setMembers] = useState<DmChannelMember[]>([]);
  const [loading, setLoading] = useState(false);

  const fetchOnce = useCallback(async () => {
    if (!dmChannelId) {
      setMembers([]);
      return;
    }
    setLoading(true);
    try {
      const list = await apiClient.get<DmChannelMember[]>(`/dms/${dmChannelId}/members`);
      setMembers(list);
    } finally {
      setLoading(false);
    }
  }, [dmChannelId]);

  useEffect(() => {
    void fetchOnce();
  }, [fetchOnce]);

  // Refetch when WS reports a member change for this channel.
  useEffect(() => {
    if (!dmChannelId) return;
    const handler = (e: Event) => {
      const detail = (e as CustomEvent<{ channel_id: string }>).detail;
      if (detail?.channel_id === dmChannelId) void fetchOnce();
    };
    window.addEventListener('parley:dm_member_change', handler);
    return () => window.removeEventListener('parley:dm_member_change', handler);
  }, [dmChannelId, fetchOnce]);

  return { members, loading, refetch: fetchOnce };
}
