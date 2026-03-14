import { useEffect, useRef, useCallback } from 'react';
import { Message } from '../api/types';

interface WSMessage {
  type: string;
  payload: unknown;
}

interface UseWebSocketOptions {
  onMessage: (msg: Message) => void;
  activeChannelId: string | null;
}

export function useWebSocket({ onMessage, activeChannelId }: UseWebSocketOptions) {
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
        if (wsMsg.type === 'MESSAGE_CREATE') {
          onMessage(wsMsg.payload as Message);
        }
      } catch {
        // ignore parse errors
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
