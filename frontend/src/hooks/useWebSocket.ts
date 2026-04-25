import { useEffect, useRef, useCallback, MutableRefObject } from 'react';

// Keeps a ref in sync with the latest value of a callback so event handlers
// never capture stale closures. Equivalent to the ref+useEffect pattern but
// expressed as a single call per callback.
function useLatest<T>(value: T): MutableRefObject<T> {
  const ref = useRef<T>(value);
  useEffect(() => { ref.current = value; }, [value]);
  return ref;
}
import { Message, DmMessage, Channel, Server, Role, FriendUser, FriendRequest, AppNotification, DmChannel } from '../api/types';
import { getWsTicket } from '../api/auth';

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
  username?: string;
  avatar_url?: string;
  banner_url?: string;
  display_name?: string;
  bio?: string;
  status_type?: string;
  status_text?: string;
}

export interface VoiceStateUpdate {
  channel_id: string;
  user_id: string;
  username: string;
  avatar_url?: string;
  action: 'join' | 'leave';
}

export interface VoiceForceMuteEvent {
  channel_id: string;
  muted: boolean;
}

export interface BinPostEvent {
  post_id: string;
  channel_id: string;
}

export interface ChannelOverwriteUpdateEvent {
  channel_id: string;
  overwrites: unknown[];
}

export interface RoleUpdateEvent {
  id: string;
  server_id: string;
  name: string;
  color: string;
  permissions: number;
  hoist: boolean;
  position: number;
}

export interface RoleDeleteEvent {
  role_id: string;
  server_id: string;
}

export interface BotStatusUpdate {
  server_id: string;
  bot_user_id: string;
  degraded: boolean;
}

export interface DmChannelCreateEvent {
  channel: DmChannel;
  message: DmMessage;
}

export interface SoundboardPlayEvent {
  channel_id: string;
  user_id: string;
  sound_id: string;
  sound_name: string;
  emoji: string;
  duration_ms: number;
}

interface UseWebSocketOptions {
  onMessage: (msg: Message) => void;
  onDmMessage?: (msg: DmMessage) => void;
  onServerMemberJoin?: (serverId: string, userId: string) => void;
  onServerMemberLeave?: (serverId: string, userId: string) => void;
  onServerMemberKick?: (serverId: string, userId: string) => void;
  onServerMemberBan?: (serverId: string, userId: string) => void;
  onTyping?: (userId: string, username: string, channelId: string, expiresAt?: string) => void;
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
  onVoiceForceMute?: (event: VoiceForceMuteEvent) => void;
  onVoiceForceDisconnect?: () => void;
  onBinPostCreate?: (event: BinPostEvent) => void;
  onBinPostUpdate?: (event: BinPostEvent) => void;
  onBinPostDelete?: (event: BinPostEvent) => void;
  onChannelOverwriteUpdate?: (event: ChannelOverwriteUpdateEvent) => void;
  onRoleUpdate?: (event: RoleUpdateEvent) => void;
  onRoleDelete?: (event: RoleDeleteEvent) => void;
  onBotStatusUpdate?: (update: BotStatusUpdate) => void;
  onDmMessageDelete?: (messageId: string, dmChannelId: string) => void;
  onDmChannelCreate?: (event: DmChannelCreateEvent) => void;
  onDmReactionUpdate?: (update: { message_id: string; dm_channel_id: string; user_id: string; emoji: string; added: boolean }) => void;
  onFriendRequest?: (req: FriendRequest) => void;
  onFriendAccept?: (user: FriendUser) => void;
  onFriendRemove?: (userId: string) => void;
  onUserStatusUpdate?: (userId: string, statusType: string, statusText: string) => void;
  onSoundboardPlay?: (event: SoundboardPlayEvent) => void;
  onNotification?: (notif: AppNotification) => void;
  onChannelReadStateUpdate?: (data: { channel_kind: 1 | 2; channel_id: string; last_read_message_id: string }) => void;
  onChannelNotificationUpdate?: (data: { channel_kind: 1 | 2; channel_id: string; notification_setting: 0 | 1 | 2 }) => void;
  activeChannelId: string | null;
  extraChannelIds?: string[]; // Additional channels to subscribe to for notifications
  onConnect?: () => void; // Called on every successful (re)connect
}

export function useWebSocket({ onMessage, onDmMessage, onServerMemberJoin, onServerMemberLeave, onServerMemberKick, onServerMemberBan, onTyping, onUserOnline, onUserOffline, onPresenceSnapshot, onMessageUpdate, onMessageDelete, onReactionUpdate, onChannelCreate, onChannelUpdate, onChannelDelete, onServerUpdate, onServerDelete, onMemberRoleUpdate, onUserUpdate, onVoiceStateUpdate, onVoiceForceMute, onVoiceForceDisconnect, onBinPostCreate, onBinPostUpdate, onBinPostDelete, onChannelOverwriteUpdate, onRoleUpdate, onRoleDelete, onBotStatusUpdate, onDmMessageDelete, onDmChannelCreate, onDmReactionUpdate, onFriendRequest, onFriendAccept, onFriendRemove, onUserStatusUpdate, onSoundboardPlay, onNotification, onChannelReadStateUpdate, onChannelNotificationUpdate, onConnect, activeChannelId, extraChannelIds = [] }: UseWebSocketOptions) {
  const wsRef = useRef<WebSocket | null>(null);
  const subscribedChannelsRef = useRef<Set<string>>(new Set());
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectAttemptsRef = useRef(0);
  const activeChannelIdRef = useRef<string | null>(activeChannelId);
  const extraChannelIdsRef = useRef<string[]>(extraChannelIds);

  // Keep refs to the latest callbacks so ws.onmessage never captures stale closures
  const onMessageRef = useLatest(onMessage);
  const onDmMessageRef = useLatest(onDmMessage);
  const onServerMemberJoinRef = useLatest(onServerMemberJoin);
  const onServerMemberLeaveRef = useLatest(onServerMemberLeave);
  const onServerMemberKickRef = useLatest(onServerMemberKick);
  const onServerMemberBanRef = useLatest(onServerMemberBan);
  const onTypingRef = useLatest(onTyping);
  const onUserOnlineRef = useLatest(onUserOnline);
  const onUserOfflineRef = useLatest(onUserOffline);
  const onPresenceSnapshotRef = useLatest(onPresenceSnapshot);
  const onMessageUpdateRef = useLatest(onMessageUpdate);
  const onMessageDeleteRef = useLatest(onMessageDelete);
  const onReactionUpdateRef = useLatest(onReactionUpdate);
  const onChannelCreateRef = useLatest(onChannelCreate);
  const onChannelUpdateRef = useLatest(onChannelUpdate);
  const onChannelDeleteRef = useLatest(onChannelDelete);
  const onServerUpdateRef = useLatest(onServerUpdate);
  const onServerDeleteRef = useLatest(onServerDelete);
  const onMemberRoleUpdateRef = useLatest(onMemberRoleUpdate);
  const onUserUpdateRef = useLatest(onUserUpdate);
  const onVoiceStateUpdateRef = useLatest(onVoiceStateUpdate);
  const onVoiceForceMuteRef = useLatest(onVoiceForceMute);
  const onVoiceForceDisconnectRef = useLatest(onVoiceForceDisconnect);
  const onBinPostCreateRef = useLatest(onBinPostCreate);
  const onBinPostUpdateRef = useLatest(onBinPostUpdate);
  const onBinPostDeleteRef = useLatest(onBinPostDelete);
  const onChannelOverwriteUpdateRef = useLatest(onChannelOverwriteUpdate);
  const onRoleUpdateRef = useLatest(onRoleUpdate);
  const onRoleDeleteRef = useLatest(onRoleDelete);
  const onBotStatusUpdateRef = useLatest(onBotStatusUpdate);
  const onDmMessageDeleteRef = useLatest(onDmMessageDelete);
  const onDmChannelCreateRef = useLatest(onDmChannelCreate);
  const onDmReactionUpdateRef = useLatest(onDmReactionUpdate);
  const onFriendRequestRef = useLatest(onFriendRequest);
  const onFriendAcceptRef = useLatest(onFriendAccept);
  const onFriendRemoveRef = useLatest(onFriendRemove);
  const onUserStatusUpdateRef = useLatest(onUserStatusUpdate);
  const onSoundboardPlayRef = useLatest(onSoundboardPlay);
  const onNotificationRef = useLatest(onNotification);
  const onChannelReadStateUpdateRef = useLatest(onChannelReadStateUpdate);
  const onChannelNotificationUpdateRef = useLatest(onChannelNotificationUpdate);
  const onConnectRef = useLatest(onConnect);

  const sendTyping = useCallback((channelId: string, username: string) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({
      type: 'TYPING',
      payload: { channel_id: channelId, username },
    }));
  }, []);

  const connect = useCallback(async () => {
    const token = localStorage.getItem('token');
    if (!token) return;

    // Exchange the long-lived JWT for a short-lived single-use ticket so the
    // JWT never appears in nginx access logs. If the ticket fetch fails, bail
    // out and let the reconnection logic retry — never put the JWT in the URL.
    let ticket: string;
    try {
      ticket = await getWsTicket();
    } catch {
      console.warn('[WebSocket] Failed to fetch WS ticket, will retry on reconnect');
      // Trigger reconnection via onclose by simulating a failed attempt
      reconnectAttemptsRef.current += 1;
      const base = Math.min(1000 * Math.pow(2, reconnectAttemptsRef.current), 30000);
      const delay = base + Math.random() * 1000;
      reconnectTimeoutRef.current = setTimeout(connect, delay);
      return;
    }

    const siteUrl = (import.meta.env.VITE_SITE_URL as string) || '';
    let wsUrl: string;
    if (siteUrl) {
      const u = new URL(siteUrl);
      const protocol = u.protocol === 'https:' ? 'wss:' : 'ws:';
      wsUrl = `${protocol}//${u.host}/ws?ticket=${encodeURIComponent(ticket)}`;
    } else {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      wsUrl = `${protocol}//${window.location.host}/ws?ticket=${encodeURIComponent(ticket)}`;
    }

    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      reconnectAttemptsRef.current = 0; // reset backoff on successful connection
      onConnectRef.current?.();
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
            const payload = wsMsg.payload as { user_id: string; username: string; channel_id: string; expires_at?: string };
            if (payload.user_id && payload.channel_id) {
              onTypingRef.current(payload.user_id, payload.username ?? '', payload.channel_id, payload.expires_at);
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
        } else if (wsMsg.type === 'VOICE_FORCE_MUTE') {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null && onVoiceForceMuteRef.current) {
            onVoiceForceMuteRef.current(wsMsg.payload as VoiceForceMuteEvent);
          }
        } else if (wsMsg.type === 'VOICE_FORCE_DISCONNECT') {
          if (onVoiceForceDisconnectRef.current) {
            onVoiceForceDisconnectRef.current();
          }
        } else if (wsMsg.type === 'BIN_POST_CREATE') {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null && onBinPostCreateRef.current) {
            onBinPostCreateRef.current(wsMsg.payload as BinPostEvent);
          }
        } else if (wsMsg.type === 'BIN_POST_UPDATE') {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null && onBinPostUpdateRef.current) {
            onBinPostUpdateRef.current(wsMsg.payload as BinPostEvent);
          }
        } else if (wsMsg.type === 'BIN_POST_DELETE') {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null && onBinPostDeleteRef.current) {
            onBinPostDeleteRef.current(wsMsg.payload as BinPostEvent);
          }
        } else if (wsMsg.type === 'CHANNEL_OVERWRITE_UPDATE') {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null && onChannelOverwriteUpdateRef.current) {
            onChannelOverwriteUpdateRef.current(wsMsg.payload as ChannelOverwriteUpdateEvent);
          }
        } else if (wsMsg.type === 'ROLE_UPDATE') {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null && onRoleUpdateRef.current) {
            onRoleUpdateRef.current(wsMsg.payload as RoleUpdateEvent);
          }
        } else if (wsMsg.type === 'ROLE_DELETE') {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null && onRoleDeleteRef.current) {
            onRoleDeleteRef.current(wsMsg.payload as RoleDeleteEvent);
          }
        } else if (wsMsg.type === 'BOT_STATUS_UPDATE') {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null && onBotStatusUpdateRef.current) {
            onBotStatusUpdateRef.current(wsMsg.payload as BotStatusUpdate);
          }
        } else if (wsMsg.type === 'dm_message_delete' && onDmMessageDeleteRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const p = wsMsg.payload as { message_id: string; dm_channel_id: string };
            if (p.message_id) onDmMessageDeleteRef.current(p.message_id, p.dm_channel_id ?? '');
          }
        } else if ((wsMsg.type === 'dm_reaction_add' || wsMsg.type === 'dm_reaction_remove') && onDmReactionUpdateRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const p = wsMsg.payload as { message_id: string; dm_channel_id: string; user_id: string; emoji: string };
            if (p.message_id && p.emoji) {
              onDmReactionUpdateRef.current({ ...p, added: wsMsg.type === 'dm_reaction_add' });
            }
          }
        } else if (wsMsg.type === 'FRIEND_REQUEST' && onFriendRequestRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const p = wsMsg.payload as { request: FriendRequest };
            if (p.request) onFriendRequestRef.current(p.request);
          }
        } else if (wsMsg.type === 'FRIEND_ACCEPT' && onFriendAcceptRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const p = wsMsg.payload as { user: FriendUser };
            if (p.user) onFriendAcceptRef.current(p.user);
          }
        } else if (wsMsg.type === 'FRIEND_REMOVE' && onFriendRemoveRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const p = wsMsg.payload as { user_id: string };
            if (p.user_id) onFriendRemoveRef.current(p.user_id);
          }
        } else if (wsMsg.type === 'USER_STATUS_UPDATE' && onUserStatusUpdateRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const p = wsMsg.payload as { user_id: string; status_type: string; status_text: string };
            if (p.user_id) onUserStatusUpdateRef.current(p.user_id, p.status_type ?? '', p.status_text ?? '');
          }
        } else if (wsMsg.type === 'SOUNDBOARD_PLAY' && onSoundboardPlayRef.current) {
          onSoundboardPlayRef.current(wsMsg.payload as SoundboardPlayEvent);
        } else if (wsMsg.type === 'NOTIFICATION_CREATE' && onNotificationRef.current) {
          onNotificationRef.current(wsMsg.payload as AppNotification);
        } else if (wsMsg.type === 'DM_CHANNEL_CREATE' && onDmChannelCreateRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const p = wsMsg.payload as DmChannelCreateEvent;
            if (p.channel) onDmChannelCreateRef.current(p);
          }
        } else if (wsMsg.type === 'CHANNEL_READ_STATE_UPDATE' && onChannelReadStateUpdateRef.current) {
          onChannelReadStateUpdateRef.current(wsMsg.payload as { channel_kind: 1 | 2; channel_id: string; last_read_message_id: string });
        } else if (wsMsg.type === 'CHANNEL_NOTIFICATION_UPDATE' && onChannelNotificationUpdateRef.current) {
          onChannelNotificationUpdateRef.current(wsMsg.payload as { channel_kind: 1 | 2; channel_id: string; notification_setting: 0 | 1 | 2 });
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