import { useCallback, useEffect, useState } from 'react';
import * as readStateApi from '../api/readState';
import type { ChannelKind } from '../api/types';

// Per (kind, channelId) read-state. Listens to the parley:channel_read_state
// CustomEvent (dispatched by App.tsx from the WS update event) for multi-tab sync.
export function useChannelReadState(kind: ChannelKind, channelId: string | null): {
  lastReadMessageId: string | null;
  markRead: (messageId: string) => void;
} {
  const [lastReadMessageId, setLastReadMessageId] = useState<string | null>(null);

  useEffect(() => {
    if (!channelId) return;
    const handler = (e: Event) => {
      const detail = (e as CustomEvent<{channel_kind: ChannelKind; channel_id: string; last_read_message_id: string}>).detail;
      if (detail.channel_kind === kind && detail.channel_id === channelId) {
        setLastReadMessageId(detail.last_read_message_id);
      }
    };
    window.addEventListener('parley:channel_read_state', handler);
    return () => window.removeEventListener('parley:channel_read_state', handler);
  }, [kind, channelId]);

  const markRead = useCallback((messageId: string) => {
    if (!channelId) return;
    setLastReadMessageId(messageId); // optimistic
    readStateApi.markRead(kind, channelId, messageId).catch((err) => {
      console.error('markRead failed', err);
    });
  }, [kind, channelId]);

  return { lastReadMessageId, markRead };
}
