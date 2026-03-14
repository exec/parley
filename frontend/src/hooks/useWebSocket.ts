import { useEffect, useRef, useCallback } from 'react';
import { Message, DmMessage } from '../api/types';

interface WSMessage {
  type: string;
  payload: unknown;
}

interface UseWebSocketOptions {
  onMessage: (msg: Message) => void;
  onDmMessage?: (msg: DmMessage) => void;
  onServerMemberJoin?: (serverId: string, userId: string) => void;
  activeChannelId: string | null;
  extraChannelIds?: string[]; // Additional channels to subscribe to for notifications
}

export function useWebSocket({ onMessage, onDmMessage, onServerMemberJoin, activeChannelId, extraChannelIds = [] }: UseWebSocketOptions) {
  const wsRef = useRef<WebSocket | null>(null);
  const subscribedChannelsRef = useRef<Set<string>>(new Set());
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const activeChannelIdRef = useRef<string | null>(activeChannelId);
  const extraChannelIdsRef = useRef<string[]>(extraChannelIds);

  // Keep refs to the latest callbacks so ws.onmessage never captures stale closures
  const onMessageRef = useRef(onMessage);
  const onDmMessageRef = useRef(onDmMessage);
  const onServerMemberJoinRef = useRef(onServerMemberJoin);
  useEffect(() => { onMessageRef.current = onMessage; }, [onMessage]);
  useEffect(() => { onDmMessageRef.current = onDmMessage; }, [onDmMessage]);
  useEffect(() => { onServerMemberJoinRef.current = onServerMemberJoin; }, [onServerMemberJoin]);

  const connect = useCallback(() => {
    const token = localStorage.getItem('token');
    if (!token) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws?token=${encodeURIComponent(token)}`;

    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      // Subscribe to all channels we care about
      const channelsToSubscribe = new Set<string>();
      const currentActiveChannel = activeChannelIdRef.current;
      if (currentActiveChannel) {
        channelsToSubscribe.add(currentActiveChannel);
      }
      extraChannelIdsRef.current.forEach(ch => channelsToSubscribe.add(ch));

      channelsToSubscribe.forEach(channelId => {
        ws.send(JSON.stringify({
          type: 'CHANNEL_SUBSCRIBE',
          payload: { channel_id: channelId },
        }));
        subscribedChannelsRef.current.add(channelId);
      });
    };

    ws.onmessage = (event) => {
      try {
        const wsMsg: WSMessage = JSON.parse(event.data);
        if (wsMsg.type === 'MESSAGE_CREATE') {
          onMessageRef.current(wsMsg.payload as Message);
        } else if (wsMsg.type === 'dm_message' && onDmMessageRef.current) {
          onDmMessageRef.current(wsMsg.payload as DmMessage);
        } else if (wsMsg.type === 'server_member_join' && onServerMemberJoinRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const payload = wsMsg.payload as { server_id: string; user_id: string };
            if (payload.server_id && payload.user_id) {
              onServerMemberJoinRef.current(payload.server_id, payload.user_id);
            }
          }
        }
      } catch (err) {
        console.error('[WebSocket] Failed to parse message:', err);
      }
    };

    ws.onclose = () => {
      wsRef.current = null;
      subscribedChannelsRef.current.clear();
      // Reconnect after 3 seconds
      reconnectTimeoutRef.current = setTimeout(connect, 3000);
    };

    ws.onerror = () => {
      ws.close();
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    connect();
    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (wsRef.current) {
        wsRef.current.onclose = null; // prevent reconnect on intentional close
        wsRef.current.close();
      }
    };
  }, [connect]);

  // Update ref when activeChannelId changes
  useEffect(() => {
    activeChannelIdRef.current = activeChannelId;
  }, [activeChannelId]);

  // Update extraChannelIds ref when it changes
  useEffect(() => {
    extraChannelIdsRef.current = extraChannelIds;
  }, [extraChannelIds]);

  // Subscribe/unsubscribe when active channel or extra channels change
  useEffect(() => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;

    const oldChannels = new Set(subscribedChannelsRef.current);
    const newChannels = new Set<string>();

    const currentActiveChannel = activeChannelIdRef.current;
    if (currentActiveChannel) {
      newChannels.add(currentActiveChannel);
    }
    extraChannelIdsRef.current.forEach(ch => newChannels.add(ch));

    // Unsubscribe from channels that are no longer needed
    oldChannels.forEach(channelId => {
      if (!newChannels.has(channelId)) {
        ws.send(JSON.stringify({
          type: 'CHANNEL_UNSUBSCRIBE',
          payload: { channel_id: channelId },
        }));
        subscribedChannelsRef.current.delete(channelId);
      }
    });

    // Subscribe to new channels
    newChannels.forEach(channelId => {
      if (!oldChannels.has(channelId)) {
        ws.send(JSON.stringify({
          type: 'CHANNEL_SUBSCRIBE',
          payload: { channel_id: channelId },
        }));
        subscribedChannelsRef.current.add(channelId);
      }
    });
  }, [activeChannelId, extraChannelIds]);
}