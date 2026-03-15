import { useEffect, useRef, useCallback } from 'react';
import { Message, DmMessage, Channel, Server, Role } from '../api/types';

interface WSMessage {
  type: string;
  payload: unknown;
}

export interface ReactionUpdate {
  message_id: string;
  channel_id: string;
  user_id: string;
  emoji: string;
  added: boolean;
}

export interface MemberRoleUpdate {
  server_id: string;
  user_id: string;
  roles: Role[];
}

export interface UserUpdate {
  user_id: string;
  username: string;
  avatar_url: string;
}

export interface VoiceStateUpdate {
  channel_id: string;
  user_id: string;
  username: string;
  avatar_url?: string;
  action: 'join' | 'leave';
}

interface UseWebSocketOptions {
  onMessage: (msg: Message) => void;
  onDmMessage?: (msg: DmMessage) => void;
  onServerMemberJoin?: (serverId: string, userId: string) => void;
  onServerMemberLeave?: (serverId: string, userId: string) => void;
  onServerMemberKick?: (serverId: string, userId: string) => void;
  onServerMemberBan?: (serverId: string, userId: string) => void;
  onTyping?: (userId: string, username: string, channelId: string) => void;
  onUserOnline?: (userId: string) => void;
  onUserOffline?: (userId: string) => void;
  onPresenceSnapshot?: (userIds: string[]) => void;
  onMessageUpdate?: (msg: Message) => void;
  onMessageDelete?: (messageId: string, channelId: string) => void;
  onReactionUpdate?: (update: ReactionUpdate) => void;
  onChannelCreate?: (channel: Channel) => void;
  onChannelUpdate?: (channel: Channel) => void;
  onChannelDelete?: (channelId: string, serverId: string) => void;
  onServerUpdate?: (server: Server) => void;
  onServerDelete?: (serverId: string) => void;
  onMemberRoleUpdate?: (update: MemberRoleUpdate) => void;
  onUserUpdate?: (update: UserUpdate) => void;
  onVoiceStateUpdate?: (update: VoiceStateUpdate) => void;
  activeChannelId: string | null;
  extraChannelIds?: string[]; // Additional channels to subscribe to for notifications
}

export function useWebSocket({ onMessage, onDmMessage, onServerMemberJoin, onServerMemberLeave, onServerMemberKick, onServerMemberBan, onTyping, onUserOnline, onUserOffline, onPresenceSnapshot, onMessageUpdate, onMessageDelete, onReactionUpdate, onChannelCreate, onChannelUpdate, onChannelDelete, onServerUpdate, onServerDelete, onMemberRoleUpdate, onUserUpdate, onVoiceStateUpdate, activeChannelId, extraChannelIds = [] }: UseWebSocketOptions) {
  const wsRef = useRef<WebSocket | null>(null);
  const subscribedChannelsRef = useRef<Set<string>>(new Set());
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectAttemptsRef = useRef(0);
  const activeChannelIdRef = useRef<string | null>(activeChannelId);
  const extraChannelIdsRef = useRef<string[]>(extraChannelIds);

  // Keep refs to the latest callbacks so ws.onmessage never captures stale closures
  const onMessageRef = useRef(onMessage);
  const onDmMessageRef = useRef(onDmMessage);
  const onServerMemberJoinRef = useRef(onServerMemberJoin);
  const onServerMemberLeaveRef = useRef(onServerMemberLeave);
  const onServerMemberKickRef = useRef(onServerMemberKick);
  const onServerMemberBanRef = useRef(onServerMemberBan);
  const onTypingRef = useRef(onTyping);
  const onUserOnlineRef = useRef(onUserOnline);
  const onUserOfflineRef = useRef(onUserOffline);
  const onPresenceSnapshotRef = useRef(onPresenceSnapshot);
  const onMessageUpdateRef = useRef(onMessageUpdate);
  const onMessageDeleteRef = useRef(onMessageDelete);
  const onReactionUpdateRef = useRef(onReactionUpdate);
  const onChannelCreateRef = useRef(onChannelCreate);
  const onChannelUpdateRef = useRef(onChannelUpdate);
  const onChannelDeleteRef = useRef(onChannelDelete);
  const onServerUpdateRef = useRef(onServerUpdate);
  const onServerDeleteRef = useRef(onServerDelete);
  const onMemberRoleUpdateRef = useRef(onMemberRoleUpdate);
  const onUserUpdateRef = useRef(onUserUpdate);
  const onVoiceStateUpdateRef = useRef(onVoiceStateUpdate);
  useEffect(() => { onMessageRef.current = onMessage; }, [onMessage]);
  useEffect(() => { onDmMessageRef.current = onDmMessage; }, [onDmMessage]);
  useEffect(() => { onServerMemberJoinRef.current = onServerMemberJoin; }, [onServerMemberJoin]);
  useEffect(() => { onServerMemberLeaveRef.current = onServerMemberLeave; }, [onServerMemberLeave]);
  useEffect(() => { onServerMemberKickRef.current = onServerMemberKick; }, [onServerMemberKick]);
  useEffect(() => { onServerMemberBanRef.current = onServerMemberBan; }, [onServerMemberBan]);
  useEffect(() => { onTypingRef.current = onTyping; }, [onTyping]);
  useEffect(() => { onUserOnlineRef.current = onUserOnline; }, [onUserOnline]);
  useEffect(() => { onUserOfflineRef.current = onUserOffline; }, [onUserOffline]);
  useEffect(() => { onPresenceSnapshotRef.current = onPresenceSnapshot; }, [onPresenceSnapshot]);
  useEffect(() => { onMessageUpdateRef.current = onMessageUpdate; }, [onMessageUpdate]);
  useEffect(() => { onMessageDeleteRef.current = onMessageDelete; }, [onMessageDelete]);
  useEffect(() => { onReactionUpdateRef.current = onReactionUpdate; }, [onReactionUpdate]);
  useEffect(() => { onChannelCreateRef.current = onChannelCreate; }, [onChannelCreate]);
  useEffect(() => { onChannelUpdateRef.current = onChannelUpdate; }, [onChannelUpdate]);
  useEffect(() => { onChannelDeleteRef.current = onChannelDelete; }, [onChannelDelete]);
  useEffect(() => { onServerUpdateRef.current = onServerUpdate; }, [onServerUpdate]);
  useEffect(() => { onServerDeleteRef.current = onServerDelete; }, [onServerDelete]);
  useEffect(() => { onMemberRoleUpdateRef.current = onMemberRoleUpdate; }, [onMemberRoleUpdate]);
  useEffect(() => { onUserUpdateRef.current = onUserUpdate; }, [onUserUpdate]);
  useEffect(() => { onVoiceStateUpdateRef.current = onVoiceStateUpdate; }, [onVoiceStateUpdate]);

  const sendTyping = useCallback((channelId: string, username: string) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({
      type: 'TYPING',
      payload: { channel_id: channelId, username },
    }));
  }, []);

  const connect = useCallback(() => {
    const token = localStorage.getItem('token');
    if (!token) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws?token=${encodeURIComponent(token)}`;

    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      reconnectAttemptsRef.current = 0; // reset backoff on successful connection
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
        } else if (wsMsg.type === 'SERVER_MEMBER_JOIN' && onServerMemberJoinRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const payload = wsMsg.payload as { server_id: string; user_id: string };
            if (payload.server_id && payload.user_id) {
              onServerMemberJoinRef.current(payload.server_id, payload.user_id);
            }
          }
        } else if (wsMsg.type === 'USER_TYPING' && onTypingRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const payload = wsMsg.payload as { user_id: string; username: string; channel_id: string };
            if (payload.user_id && payload.channel_id) {
              onTypingRef.current(payload.user_id, payload.username ?? '', payload.channel_id);
            }
          }
        } else if (wsMsg.type === 'USER_ONLINE' && onUserOnlineRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const payload = wsMsg.payload as { user_id: string };
            if (payload.user_id) onUserOnlineRef.current(payload.user_id);
          }
        } else if (wsMsg.type === 'USER_OFFLINE' && onUserOfflineRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const payload = wsMsg.payload as { user_id: string };
            if (payload.user_id) onUserOfflineRef.current(payload.user_id);
          }
        } else if (wsMsg.type === 'PRESENCE_SNAPSHOT' && onPresenceSnapshotRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const payload = wsMsg.payload as { user_ids: string[] };
            if (Array.isArray(payload.user_ids)) onPresenceSnapshotRef.current(payload.user_ids);
          }
        } else if (wsMsg.type === 'MESSAGE_UPDATE' && onMessageUpdateRef.current) {
          onMessageUpdateRef.current(wsMsg.payload as Message);
        } else if (wsMsg.type === 'MESSAGE_DELETE' && onMessageDeleteRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const payload = wsMsg.payload as { id: string; channel_id: string };
            if (payload.id) onMessageDeleteRef.current(payload.id, payload.channel_id ?? '');
          }
        } else if ((wsMsg.type === 'REACTION_ADD' || wsMsg.type === 'REACTION_REMOVE') && onReactionUpdateRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const payload = wsMsg.payload as { message_id: string; channel_id: string; user_id: string; emoji: string };
            if (payload.message_id && payload.emoji) {
              onReactionUpdateRef.current({ ...payload, added: wsMsg.type === 'REACTION_ADD' });
            }
          }
        } else if (wsMsg.type === 'SERVER_MEMBER_LEAVE' && onServerMemberLeaveRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const p = wsMsg.payload as { server_id: string; user_id: string };
            if (p.server_id && p.user_id) onServerMemberLeaveRef.current(p.server_id, p.user_id);
          }
        } else if (wsMsg.type === 'SERVER_MEMBER_KICK' && onServerMemberKickRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const p = wsMsg.payload as { server_id: string; user_id: string };
            if (p.server_id && p.user_id) onServerMemberKickRef.current(p.server_id, p.user_id);
          }
        } else if (wsMsg.type === 'SERVER_MEMBER_BAN' && onServerMemberBanRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const p = wsMsg.payload as { server_id: string; user_id: string };
            if (p.server_id && p.user_id) onServerMemberBanRef.current(p.server_id, p.user_id);
          }
        } else if (wsMsg.type === 'CHANNEL_CREATE' && onChannelCreateRef.current) {
          onChannelCreateRef.current(wsMsg.payload as Channel);
        } else if (wsMsg.type === 'CHANNEL_UPDATE' && onChannelUpdateRef.current) {
          onChannelUpdateRef.current(wsMsg.payload as Channel);
        } else if (wsMsg.type === 'CHANNEL_DELETE' && onChannelDeleteRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const p = wsMsg.payload as { channel_id: string; server_id: string };
            if (p.channel_id) onChannelDeleteRef.current(p.channel_id, p.server_id ?? '');
          }
        } else if (wsMsg.type === 'SERVER_UPDATE' && onServerUpdateRef.current) {
          onServerUpdateRef.current(wsMsg.payload as Server);
        } else if (wsMsg.type === 'SERVER_DELETE' && onServerDeleteRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const p = wsMsg.payload as { server_id: string };
            if (p.server_id) onServerDeleteRef.current(p.server_id);
          }
        } else if (wsMsg.type === 'MEMBER_ROLE_UPDATE' && onMemberRoleUpdateRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            onMemberRoleUpdateRef.current(wsMsg.payload as MemberRoleUpdate);
          }
        } else if (wsMsg.type === 'USER_UPDATE' && onUserUpdateRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            onUserUpdateRef.current(wsMsg.payload as UserUpdate);
          }
        } else if (wsMsg.type === 'VOICE_STATE_UPDATE' && onVoiceStateUpdateRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            onVoiceStateUpdateRef.current(wsMsg.payload as VoiceStateUpdate);
          }
        }
      } catch (err) {
        console.error('[WebSocket] Failed to parse message:', err);
      }
    };

    ws.onclose = () => {
      wsRef.current = null;
      subscribedChannelsRef.current.clear();
      // Exponential backoff: 1s, 2s, 4s, 8s, 16s, capped at 30s, plus up to 1s jitter
      const attempt = reconnectAttemptsRef.current;
      reconnectAttemptsRef.current = attempt + 1;
      const base = Math.min(1000 * Math.pow(2, attempt), 30000);
      const delay = base + Math.random() * 1000;
      reconnectTimeoutRef.current = setTimeout(connect, delay);
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
  // eslint-disable-next-line react-hooks/exhaustive-deps
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

  return { sendTyping };
}