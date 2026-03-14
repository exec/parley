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
}

export function useWebSocket({ onMessage, onDmMessage, onServerMemberJoin, activeChannelId }: UseWebSocketOptions) {
  const wsRef = useRef<WebSocket | null>(null);
  const subscribedChannelRef = useRef<string | null>(null);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const connect = useCallback(() => {
    const token = localStorage.getItem('token');
    if (!token) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws?token=${encodeURIComponent(token)}`;

    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      // Subscribe to active channel if we have one
      if (activeChannelId) {
        ws.send(JSON.stringify({
          type: 'CHANNEL_SUBSCRIBE',
          payload: { channel_id: activeChannelId },
        }));
        subscribedChannelRef.current = activeChannelId;
      }
    };

    ws.onmessage = (event) => {
      try {
        const wsMsg: WSMessage = JSON.parse(event.data);
        console.log('[WebSocket] Received:', wsMsg.type, wsMsg.payload);
        if (wsMsg.type === 'MESSAGE_CREATE') {
          console.log('[WebSocket] Calling onMessage for MESSAGE_CREATE');
          onMessage(wsMsg.payload as Message);
        } else if (wsMsg.type === 'dm_message' && onDmMessage) {
          onDmMessage(wsMsg.payload as DmMessage);
        } else if (wsMsg.type === 'server_member_join' && onServerMemberJoin) {
          // Parse server member join event
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const payload = wsMsg.payload as { server_id: string; user_id: string };
            if (payload.server_id && payload.user_id) {
              onServerMemberJoin(payload.server_id, payload.user_id);
            }
          }
        }
      } catch (err) {
        console.error('[WebSocket] Failed to parse message:', err);
      }
    };

    ws.onclose = () => {
      wsRef.current = null;
      subscribedChannelRef.current = null;
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

  // Subscribe/unsubscribe when active channel changes
  useEffect(() => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;

    // Unsubscribe from previous channel
    if (subscribedChannelRef.current && subscribedChannelRef.current !== activeChannelId) {
      ws.send(JSON.stringify({
        type: 'CHANNEL_UNSUBSCRIBE',
        payload: { channel_id: subscribedChannelRef.current },
      }));
    }

    // Subscribe to new channel
    if (activeChannelId) {
      ws.send(JSON.stringify({
        type: 'CHANNEL_SUBSCRIBE',
        payload: { channel_id: activeChannelId },
      }));
      subscribedChannelRef.current = activeChannelId;
    } else {
      subscribedChannelRef.current = null;
    }
  }, [activeChannelId]);
}