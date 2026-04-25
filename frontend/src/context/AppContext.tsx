import React, { createContext, useContext, useEffect, useState, useCallback, useRef } from 'react';
import { User, Server, Channel, Message, ServerMember, DmChannel, DmMessage, Reaction, FriendUser, FriendRequest, FriendRequestsResponse, AppNotification } from '../api/types';

// Pure helper: applies a single reaction add/remove to an array of messages.
// Works for both Message[] and DmMessage[] since both have id and reactions fields.
function applyReactionToList<T extends { id: string | number; reactions?: Reaction[] }>(
  list: T[],
  update: { message_id: string; user_id: string; emoji: string; added: boolean }
): T[] {
  return list.map(msg => {
    if (String(msg.id) !== String(update.message_id)) return msg;
    const reactions: Reaction[] = [...(msg.reactions ?? [])];
    const idx = reactions.findIndex(r => r.emoji === update.emoji);
    if (update.added) {
      if (idx >= 0) {
        const r = reactions[idx];
        if (!r.user_ids.includes(update.user_id)) {
          reactions[idx] = { ...r, count: r.count + 1, user_ids: [...r.user_ids, update.user_id] };
        }
      } else {
        reactions.push({ emoji: update.emoji, count: 1, user_ids: [update.user_id] });
      }
    } else {
      if (idx >= 0) {
        const newUserIds = reactions[idx].user_ids.filter(uid => uid !== update.user_id);
        if (newUserIds.length === 0) {
          reactions.splice(idx, 1);
        } else {
          reactions[idx] = { ...reactions[idx], count: newUserIds.length, user_ids: newUserIds };
        }
      }
    }
    return { ...msg, reactions };
  });
}
import { apiClient } from '../api/client';
import * as serversApi from '../api/servers';
import * as channelsApi from '../api/channels';
import * as messagesApi from '../api/messages';
import * as dmsApi from '../api/dms';
import * as friendsApi from '../api/friends';
import * as notificationsApi from '../api/notifications';
import { getCurrentUser } from '../api/auth';
import { ReactionUpdate, MemberRoleUpdate, UserUpdate, BotStatusUpdate } from '../hooks/useWebSocket';
import { resetThemeOnLogout } from './ThemeContext';

interface AppState {
  currentUser: User | null;
  servers: Server[];
  activeServer: Server | null;
  channels: Channel[];
  activeChannel: Channel | null;
  messages: Message[];
  hasMoreMessages: boolean;
  members: ServerMember[];
  isLoadingServers: boolean;
  isLoadingChannels: boolean;
  isLoadingMessages: boolean;
  dmChannels: DmChannel[];
  activeDmChannel: DmChannel | null;
  dmMessages: DmMessage[];
  hasMoreDmMessages: boolean;
  isLoadingDms: boolean;
  friends: FriendUser[];
  friendRequests: FriendRequestsResponse;
  pendingRequestCount: number;
  pendingJumpMessageId: string | null;
  notifications: AppNotification[];
  unreadNotificationCount: number;
}

interface AppActions {
  selectServer: (serverId: string, channelId?: string) => Promise<void>;
  selectChannel: (channelId: string) => Promise<void>;
  selectChannelAround: (channelId: string, aroundId: string) => Promise<void>;
  clearJumpTarget: () => void;
  createServer: (name: string) => Promise<void>;
  updateServer: (server: Server) => void;
  deleteServer: (serverId: string) => void;
  leaveServer: (serverId: string) => Promise<void>;
  createChannel: (name: string, type: number, topic?: string) => Promise<void>;
  deleteChannel: (channelId: string) => Promise<void>;
  reorderChannels: (orders: channelsApi.ChannelOrder[]) => Promise<void>;
  sendMessage: (content: string, attachmentUrl?: string, attachmentName?: string, attachmentType?: string, parentId?: string) => Promise<void>;
  editMessage: (messageId: string, content: string) => Promise<void>;
  deleteMessage: (messageId: string) => Promise<void>;
  receiveMessage: (msg: Message) => void;
  receiveMessageUpdate: (msg: Message) => void;
  receiveMessageDelete: (messageId: string, channelId: string) => void;
  toggleReaction: (messageId: string, emoji: string) => Promise<void>;
  applyReactionUpdate: (update: ReactionUpdate) => void;
  logout: () => void;
  loadDmChannels: () => Promise<void>;
  openDmChannel: (userId: string) => Promise<void>;
  selectDmChannel: (channelId: string) => Promise<void>;
  sendDmMessage: (content: string, attachmentUrl?: string, attachmentName?: string, attachmentType?: string, parentId?: string) => Promise<void>;
  receiveDmMessage: (msg: DmMessage) => void;
  deleteDmMessage: (dmChannelId: string, messageId: string) => Promise<void>;
  applyDmReactionUpdate: (update: ReactionUpdate) => void;
  receiveDmMessageDelete: (messageId: string) => void;
  receiveDmChannelCreate: (channel: DmChannel) => void;
  applyDmChannelUpdate: (update: { channel_id: string; name?: string; avatar_url?: string }) => void;
  addServer: (server: Server) => void;
  updateCurrentUser: (user: User) => void;
  loadServers: () => Promise<void>;
  // WS event handlers for server structure changes
  receiveChannelCreate: (channel: Channel) => void;
  receiveChannelUpdate: (channel: Channel) => void;
  receiveChannelDelete: (channelId: string, serverId: string) => void;
  receiveMemberLeave: (serverId: string, userId: string) => void;
  receiveMemberRemoved: (serverId: string, userId: string) => void; // for kick or ban of current user
  receiveMemberRoleUpdate: (update: MemberRoleUpdate) => void;
  receiveBotStatusUpdate: (update: BotStatusUpdate) => void;
  receiveUserUpdate: (update: UserUpdate) => void;
  reloadMembers: (serverId: string) => Promise<void>;
  reloadChannels: (serverId: string) => Promise<void>;
  loadMoreMessages: () => Promise<void>;
  loadMoreDmMessages: () => Promise<void>;
  loadFriends: () => Promise<void>;
  sendFriendRequest: (username: string) => Promise<void>;
  acceptFriendRequest: (requestId: string) => Promise<void>;
  declineOrCancelRequest: (requestId: string) => Promise<void>;
  removeFriend: (userId: string) => Promise<void>;
  receiveFriendRequest: (req: FriendRequest) => void;
  receiveFriendAccept: (user: FriendUser) => void;
  receiveFriendRemove: (userId: string) => void;
  receiveNotification: (notif: AppNotification) => void;
  markNotificationRead: (id: string) => Promise<void>;
  markAllNotificationsRead: () => Promise<void>;
  loadNotifications: () => Promise<void>;
}

const AppContext = createContext<(AppState & AppActions) | null>(null);

export function AppProvider({ children }: { children: React.ReactNode }) {
  const [currentUser, setCurrentUser] = useState<User | null>(() => {
    try {
      const stored = localStorage.getItem('user');
      if (!stored) return null;
      const u = JSON.parse(stored);
      return {
        id: u.id || u.ID || '',
        username: u.username || u.Username || '',
        email: u.email || u.Email || '',
        avatar_url: u.avatar_url || '',
        banner_url: u.banner_url || '',
        display_name: u.display_name || undefined,
        bio: u.bio || undefined,
        email_verified: u.email_verified ?? undefined,
        // phone_number and phone_verified are intentionally excluded from localStorage
        // to avoid persisting sensitive data. They are fetched on-demand in settings.
      };
    } catch {
      return null;
    }
  });
  const [servers, setServers] = useState<Server[]>([]);
  const [activeServer, setActiveServer] = useState<Server | null>(null);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [activeChannel, setActiveChannel] = useState<Channel | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [hasMoreMessages, setHasMoreMessages] = useState(false);
  const [members, setMembers] = useState<ServerMember[]>([]);
  const [isLoadingServers, setIsLoadingServers] = useState(false);
  const [isLoadingChannels, setIsLoadingChannels] = useState(false);
  const [isLoadingMessages, setIsLoadingMessages] = useState(false);

  const [dmChannels, setDmChannels] = useState<DmChannel[]>([]);
  const [activeDmChannel, setActiveDmChannel] = useState<DmChannel | null>(null);
  const activeChannelRef = useRef<Channel | null>(null);
  useEffect(() => { activeChannelRef.current = activeChannel; }, [activeChannel]);
  const activeDmChannelRef = useRef<DmChannel | null>(null);
  useEffect(() => { activeDmChannelRef.current = activeDmChannel; }, [activeDmChannel]);
  const [dmMessages, setDmMessages] = useState<DmMessage[]>([]);
  const [hasMoreDmMessages, setHasMoreDmMessages] = useState(false);
  const [isLoadingDms, setIsLoadingDms] = useState(false);
  const [friends, setFriends] = useState<FriendUser[]>([]);
  const [friendRequests, setFriendRequests] = useState<FriendRequestsResponse>({ incoming: [], outgoing: [] });

  const pendingRequestCount = friendRequests.incoming.length;
  const [pendingJumpMessageId, setPendingJumpMessageId] = useState<string | null>(null);
  const [notifications, setNotifications] = useState<AppNotification[]>([]);
  const unreadNotificationCount = notifications.filter(n => !n.read).length;

  useEffect(() => {
    const token = localStorage.getItem('token');
    if (token) {
      apiClient.setToken(token);
    }
  }, []);

  // Refresh currentUser from the server on startup so stale localStorage data
  // (e.g. missing display_name from an old session) is always corrected.
  useEffect(() => {
    const token = localStorage.getItem('token');
    if (!token) return;
    getCurrentUser()
      .then(fresh => {
        setCurrentUser(fresh);
        const { phone_number: _p, phone_verified: _pv, ...safe } = fresh as User & { phone_number?: string; phone_verified?: boolean };
        localStorage.setItem('user', JSON.stringify(safe));
      })
      .catch(() => { /* silently ignore — stale data is better than crashing */ });
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!currentUser) return;
    setIsLoadingServers(true);
    serversApi.getServers()
      .then(data => setServers(data ?? []))
      .catch(console.error)
      .finally(() => setIsLoadingServers(false));

    dmsApi.getDmChannels()
      .then(data => setDmChannels(data ?? []))
      .catch(console.error);
    friendsApi.getFriends()
      .then(data => setFriends(data ?? []))
      .catch(console.error);
    friendsApi.getFriendRequests()
      .then(data => setFriendRequests(data ?? { incoming: [], outgoing: [] }))
      .catch(console.error);
  }, [currentUser?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const loadServers = useCallback(async () => {
    if (!currentUser) return;
    setIsLoadingServers(true);
    try {
      const data = await serversApi.getServers();
      setServers(data ?? []);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoadingServers(false);
    }
  }, [currentUser]);

  const selectServer = useCallback(async (serverId: string, channelId?: string) => {
    setActiveDmChannel(null);
    setDmMessages([]);

    // Special signal to go home (deselect everything)
    if (serverId === '__none__') {
      setActiveServer(null);
      setChannels([]);
      setMembers([]);
      setActiveChannel(null);
      setMessages([]);
      return;
    }

    const srv = servers.find(s => s.id === serverId);
    if (!srv) return;
    setActiveServer(srv);
    setActiveChannel(null);
    setMessages([]);
    setIsLoadingChannels(true);
    try {
      const [chs, mems] = await Promise.all([
        channelsApi.getChannels(serverId),
        serversApi.getMembers(serverId),
      ]);
      setChannels(chs ?? []);
      setMembers(mems ?? []);
      const resolvedChannelId = channelId || localStorage.getItem(`parley_last_channel_${serverId}`) || undefined;
      const target = (resolvedChannelId && chs?.find(c => c.id === resolvedChannelId)) || chs?.find(c => c.type === 0);
      if (target) {
        setActiveChannel(target);
        setIsLoadingMessages(true);
        const msgs = await messagesApi.getMessages(target.id, { limit: 50 });
        setMessages(msgs ?? []);
        setHasMoreMessages((msgs?.length ?? 0) >= 50);
        setIsLoadingMessages(false);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoadingChannels(false);
    }
  }, [servers]);

  const selectChannel = useCallback(async (channelId: string) => {
    setActiveDmChannel(null);
    setDmMessages([]);

    const ch = channels.find(c => c.id === channelId);
    if (!ch) return;
    setActiveChannel(ch);
    if (activeServer) localStorage.setItem(`parley_last_channel_${activeServer.id}`, channelId);
    setHasMoreMessages(false);
    setIsLoadingMessages(true);
    try {
      const msgs = await messagesApi.getMessages(channelId, { limit: 50 });
      setMessages(msgs ?? []);
      setHasMoreMessages((msgs?.length ?? 0) >= 50);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoadingMessages(false);
    }
  }, [channels, activeServer]);

  const selectChannelAround = useCallback(async (channelId: string, aroundId: string) => {
    setActiveDmChannel(null);
    setDmMessages([]);

    const ch = channels.find(c => c.id === channelId);
    if (!ch) return;
    setActiveChannel(ch);
    if (activeServer) localStorage.setItem(`parley_last_channel_${activeServer.id}`, channelId);
    setHasMoreMessages(false);
    setIsLoadingMessages(true);
    try {
      const msgs = await messagesApi.getMessages(channelId, { limit: 50, around: aroundId });
      setMessages(msgs ?? []);
      setHasMoreMessages(false); // around-loaded pages don't support load-more for now
      setPendingJumpMessageId(aroundId);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoadingMessages(false);
    }
  }, [channels, activeServer]);

  const clearJumpTarget = useCallback(() => {
    setPendingJumpMessageId(null);
  }, []);

  const createServer = useCallback(async (name: string) => {
    const srv = await serversApi.createServer(name);
    setServers(prev => [...prev, srv]);
    setActiveServer(srv);
    setActiveChannel(null);
    setMessages([]);
    const [chs, mems] = await Promise.all([
      channelsApi.getChannels(srv.id),
      serversApi.getMembers(srv.id),
    ]);
    setChannels(chs ?? []);
    setMembers(mems ?? []);
  }, []);

  const updateServer = useCallback((updated: Server) => {
    setServers(prev => prev.map(s => s.id === updated.id ? updated : s));
    setActiveServer(prev => prev?.id === updated.id ? updated : prev);
  }, []);

  const deleteServer = useCallback((serverId: string) => {
    setServers(prev => prev.filter(s => s.id !== serverId));
    setActiveServer(null);
    setChannels([]);
    setMembers([]);
    setActiveChannel(null);
    setMessages([]);
  }, []);

  const leaveServer = useCallback(async (serverId: string) => {
    // Call the leave server API
    await serversApi.leaveServer(serverId);
    // Remove the server from local state
    setServers(prev => prev.filter(s => s.id !== serverId));
    // If we're in the left server, go home
    if (activeServer?.id === serverId) {
      setActiveServer(null);
      setChannels([]);
      setMembers([]);
      setActiveChannel(null);
      setMessages([]);
    }
  }, [activeServer]);

  const addServer = useCallback((srv: Server) => {
    setServers(prev => {
      if (prev.find(s => s.id === srv.id)) return prev;
      return [...prev, srv];
    });
  }, []);

  const createChannel = useCallback(async (name: string, type: number, topic?: string) => {
    if (!activeServer) return;
    await channelsApi.createChannel(activeServer.id, name, type, topic);
    // Channel will be added via CHANNEL_CREATE WebSocket event
  }, [activeServer]);

  const reorderChannels = useCallback(async (orders: channelsApi.ChannelOrder[]) => {
    if (!activeServer) return;
    // Optimistic update
    setChannels(prev => {
      const updated = [...prev];
      orders.forEach(o => {
        const idx = updated.findIndex(c => c.id === o.id);
        if (idx !== -1) {
          updated[idx] = { ...updated[idx], position: o.position, parent_id: o.parent_id ?? undefined };
        }
      });
      return updated.sort((a, b) => a.position - b.position);
    });
    await channelsApi.reorderChannels(activeServer.id, orders);
  }, [activeServer]);

  const deleteChannel = useCallback(async (channelId: string) => {
    await channelsApi.deleteChannel(channelId);
    setChannels(prev => prev.filter(c => c.id !== channelId));
    if (activeChannel?.id === channelId) {
      setActiveChannel(null);
      setMessages([]);
    }
  }, [activeChannel]);

  const sendMessage = useCallback(async (content: string, attachmentUrl?: string, attachmentName?: string, attachmentType?: string, parentId?: string) => {
    if (!activeChannel || !currentUser) return;
    const nonce = crypto.randomUUID();

    // Optimistic: add immediately with a temporary pending flag so the sender
    // sees their message right away without waiting for the WS echo.
    const optimistic: Message = {
      id: `pending-${nonce}`,
      channel_id: activeChannel.id,
      author_id: currentUser.id,
      author_username: currentUser.username,
      author_display_name: currentUser.display_name || undefined,
      content,
      nonce,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
      reactions: [],
      pending: true,
      attachment_url: attachmentUrl,
      attachment_name: attachmentName,
      attachment_type: attachmentType,
      parent_id: parentId,
    };
    setMessages(prev => [...prev, optimistic]);

    try {
      const confirmed = await messagesApi.sendMessage(activeChannel.id, content, nonce, attachmentUrl, attachmentName, attachmentType, parentId);
      // Confirm immediately from the HTTP 201 response — don't wait for WS echo.
      // receiveMessage handles the WS echo as a no-op (nonce already replaced,
      // duplicate id check prevents double-add).
      setMessages(prev => {
        const idx = prev.findIndex(m => m.nonce === nonce);
        if (idx >= 0) {
          const next = [...prev];
          next[idx] = confirmed;
          return next;
        }
        // WS echo arrived first — guard against duplicate id
        if (prev.some(m => m.id === confirmed.id)) return prev;
        return [...prev, confirmed];
      });
    } catch (err) {
      // Remove the optimistic message on failure.
      setMessages(prev => prev.filter(m => m.nonce !== nonce));
      throw err;
    }
  }, [activeChannel, currentUser]);

  const editMessage = useCallback(async (messageId: string, content: string) => {
    const updated = await messagesApi.editMessage(messageId, content);
    setMessages(prev => prev.map(m => m.id === messageId ? updated : m));
  }, []);

  const deleteMessage = useCallback(async (messageId: string) => {
    await messagesApi.deleteMessage(messageId);
    setMessages(prev => prev.filter(m => m.id !== messageId));
  }, []);

  const receiveMessage = useCallback((msg: Message) => {
    const ch = activeChannelRef.current;
    if (ch !== null && msg.channel_id === ch.id) {
      setMessages(prev => {
        // If we have an optimistic entry for this nonce, replace it.
        if (msg.nonce) {
          const idx = prev.findIndex(m => m.nonce === msg.nonce);
          if (idx >= 0) {
            const next = [...prev];
            next[idx] = msg;
            return next;
          }
        }
        // Guard against duplicate real IDs (e.g. WS delivered twice).
        if (prev.some(m => m.id === msg.id)) return prev;
        return [...prev, msg];
      });
    }
  }, []);

  const loadDmChannels = useCallback(async () => {
    setIsLoadingDms(true);
    try {
      const channels = await dmsApi.getDmChannels();
      setDmChannels(channels ?? []);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoadingDms(false);
    }
  }, []);

  const openDmChannel = useCallback(async (userId: string) => {
    try {
      const channel = await dmsApi.openDmChannel(userId);
      setDmChannels(prev => {
        if (prev.find(c => c.id === channel.id)) return prev;
        return [channel, ...prev];
      });
      setActiveDmChannel(channel);
      setActiveServer(null);
      setActiveChannel(null);
      setMessages([]);
      setIsLoadingDms(true);
      const msgs = await dmsApi.getDmMessages(channel.id);
      setDmMessages(msgs ?? []);
      setHasMoreDmMessages((msgs?.length ?? 0) >= 50);
      setIsLoadingDms(false);
    } catch (err) {
      console.error(err);
    }
  }, []);

  const selectDmChannel = useCallback(async (channelId: string) => {
    const channel = dmChannels.find(c => c.id === channelId);
    if (!channel) return;

    setActiveDmChannel(channel);
    setActiveServer(null);
    setActiveChannel(null);
    setMessages([]);
    setDmMessages([]);
    setHasMoreDmMessages(false);

    setIsLoadingDms(true);
    try {
      const msgs = await dmsApi.getDmMessages(channel.id);
      setDmMessages(msgs ?? []);
      setHasMoreDmMessages((msgs?.length ?? 0) >= 50);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoadingDms(false);
    }
  }, [dmChannels]);

  const sendDmMessage = useCallback(async (content: string, attachmentUrl?: string, attachmentName?: string, attachmentType?: string, parentId?: string) => {
    if (!activeDmChannel) return;
    const msg = await dmsApi.sendDmMessage(activeDmChannel.id, content, attachmentUrl, attachmentName, attachmentType, parentId);
    setDmMessages(prev => {
      if (prev.some(m => m.id === msg.id)) return prev;
      return [...prev, msg];
    });
    // Bump this DM to the top of the list — the WS broadcast also reaches the
    // sender (BroadcastToChannel sends to all subscribers) which would also
    // trigger the reorder via receiveDmMessage, but doing it here too makes the
    // ordering update synchronous with the send instead of waiting on the
    // round-trip.
    const sentChannelId = activeDmChannel.id;
    setDmChannels(prev => {
      const idx = prev.findIndex(c => String(c.id) === String(sentChannelId));
      if (idx <= 0) return prev;
      const next = prev.slice();
      const [bumped] = next.splice(idx, 1);
      return [bumped, ...next];
    });
  }, [activeDmChannel]);

  const deleteDmMessage = useCallback(async (dmChannelId: string, messageId: string) => {
    await dmsApi.deleteDmMessage(dmChannelId, messageId);
    setDmMessages(prev => prev.filter(m => m.id !== messageId));
  }, []);

  const receiveDmMessage = useCallback((msg: DmMessage) => {
    const adc = activeDmChannelRef.current;
    // Use string coercion so numeric IDs from JSON match string IDs from state
    if (adc && String(msg.dm_channel_id) === String(adc.id)) {
      setDmMessages(prev => {
        if (prev.some(m => String(m.id) === String(msg.id))) return prev;
        return [...prev, msg];
      });
    }
    // Bump the channel to the top of the DM list. The backend orders by
    // last-activity on initial fetch; this keeps that ordering live as
    // new messages stream in (sent or received).
    setDmChannels(prev => {
      const idx = prev.findIndex(c => String(c.id) === String(msg.dm_channel_id));
      if (idx <= 0) return prev; // already at top, or not in our list
      const next = prev.slice();
      const [bumped] = next.splice(idx, 1);
      return [bumped, ...next];
    });
  }, []);

  const logout = useCallback(() => {
    resetThemeOnLogout();
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    apiClient.setToken(null);
    window.location.href = '/login';
  }, []);

  const updateCurrentUser = useCallback((user: User) => {
    setCurrentUser(user);
    // Omit phone fields from localStorage to avoid persisting sensitive contact info.
    const { phone_number: _p, phone_verified: _pv, ...safeUser } = user;
    localStorage.setItem('user', JSON.stringify(safeUser));
  }, []);

  const receiveMessageUpdate = useCallback((msg: Message) => {
    setMessages(prev => prev.map(m => m.id === msg.id ? { ...msg, reactions: m.reactions } : m));
  }, []);

  const receiveMessageDelete = useCallback((messageId: string, _channelId: string) => {
    setMessages(prev => prev.filter(m => m.id !== messageId));
  }, []);

  const applyReactionUpdate = useCallback((update: ReactionUpdate) => {
    setMessages(prev => applyReactionToList(prev, update));
  }, []);

  const applyDmReactionUpdate = useCallback((update: ReactionUpdate) => {
    setDmMessages(prev => applyReactionToList(prev, update));
  }, []);

  const receiveDmMessageDelete = useCallback((messageId: string) => {
    setDmMessages(prev => prev.filter(m => m.id !== messageId));
  }, []);

  const receiveDmChannelCreate = useCallback((channel: DmChannel) => {
    setDmChannels(prev => {
      if (prev.some(c => String(c.id) === String(channel.id))) return prev;
      return [channel, ...prev];
    });
  }, []);

  const applyDmChannelUpdate = useCallback((update: { channel_id: string; name?: string; avatar_url?: string }) => {
    const id = String(update.channel_id);
    const patch = (c: DmChannel): DmChannel => {
      if (String(c.id) !== id) return c;
      // Only mutate fields that arrived in the payload — null/undefined means
      // "unchanged" (avoiding accidental clears).
      const next: DmChannel = { ...c };
      if (update.name !== undefined) next.name = update.name;
      if (update.avatar_url !== undefined) next.avatar_url = update.avatar_url;
      return next;
    };
    setDmChannels(prev => prev.map(patch));
    setActiveDmChannel(prev => (prev ? patch(prev) : prev));
  }, []);

  const toggleReaction = useCallback(async (messageId: string, emoji: string) => {
    await messagesApi.toggleReaction(messageId, emoji);
  }, []);

  const receiveChannelCreate = useCallback((channel: Channel) => {
    setChannels(prev => {
      if (prev.some(c => c.id === channel.id)) return prev;
      return [...prev, channel];
    });
  }, []);

  const receiveChannelUpdate = useCallback((channel: Channel) => {
    setChannels(prev => prev.map(c => c.id === channel.id ? channel : c));
    setActiveChannel(prev => prev?.id === channel.id ? channel : prev);
  }, []);

  const receiveChannelDelete = useCallback((channelId: string, _serverId: string) => {
    setChannels(prev => prev.filter(c => c.id !== channelId));
    setActiveChannel(prev => prev?.id === channelId ? null : prev);
    setMessages(prev => prev.filter(() => true)); // keep messages, UI handles null channel
  }, []);

  const reloadMembers = useCallback(async (serverId: string) => {
    try {
      const mems = await serversApi.getMembers(serverId);
      setMembers(mems ?? []);
    } catch (err) {
      console.error('Failed to reload members:', err);
    }
  }, []);

  const reloadChannels = useCallback(async (serverId: string) => {
    try {
      const chs = await channelsApi.getChannels(serverId);
      setChannels(chs ?? []);
      // If active channel is no longer accessible, clear it
      setActiveChannel(prev => {
        if (!prev) return prev;
        return (chs ?? []).some(c => c.id === prev.id) ? prev : null;
      });
    } catch (err) {
      console.error('Failed to reload channels:', err);
    }
  }, []);

  const receiveMemberLeave = useCallback((serverId: string, userId: string) => {
    // Update members list if the user is in the active server
    setMembers(prev => prev.filter(m => !(m.server_id === serverId && m.user_id === userId)));
  }, []);

  // Called when the current user is kicked or banned
  const receiveMemberRemoved = useCallback((serverId: string, userId: string) => {
    setMembers(prev => prev.filter(m => !(m.server_id === serverId && m.user_id === userId)));
    setServers(prev => prev.filter(s => s.id !== serverId));
    setActiveServer(prev => {
      if (prev?.id === serverId) {
        setChannels([]);
        setMembers([]);
        setActiveChannel(null);
        setMessages([]);
      }
      return prev?.id === serverId ? null : prev;
    });
  }, []);

  const receiveMemberRoleUpdate = useCallback((update: MemberRoleUpdate) => {
    setMembers(prev => prev.map(m =>
      m.user_id === update.user_id && m.server_id === update.server_id
        ? { ...m, roles: update.roles }
        : m
    ));
  }, []);

  const receiveBotStatusUpdate = useCallback((update: BotStatusUpdate) => {
    setMembers(prev => prev.map(m =>
      m.user_id === update.bot_user_id && m.server_id === update.server_id
        ? { ...m, bot_degraded: update.degraded }
        : m
    ));
  }, []);

  const receiveUserUpdate = useCallback((update: UserUpdate) => {
    setMembers(prev => prev.map(m => {
      if (m.user_id !== update.user_id) return m;
      const next = { ...m };
      if (update.username !== undefined) next.username = update.username;
      if (update.avatar_url !== undefined) next.avatar_url = update.avatar_url;
      if (update.banner_url !== undefined) next.banner_url = update.banner_url;
      if (update.display_name !== undefined) next.display_name = update.display_name;
      if (update.bio !== undefined) next.bio = update.bio;
      if (update.status_type !== undefined) next.status_type = update.status_type as typeof next.status_type;
      if (update.status_text !== undefined) next.status_text = update.status_text;
      return next;
    }));
    setCurrentUser(prev => {
      if (!prev || prev.id !== update.user_id) return prev;
      const next = { ...prev };
      if (update.username !== undefined) next.username = update.username;
      if (update.avatar_url !== undefined) next.avatar_url = update.avatar_url;
      if (update.banner_url !== undefined) next.banner_url = update.banner_url;
      if (update.display_name !== undefined) next.display_name = update.display_name;
      if (update.bio !== undefined) next.bio = update.bio;
      if (update.status_type !== undefined) next.status_type = update.status_type as typeof next.status_type;
      if (update.status_text !== undefined) next.status_text = update.status_text;
      // Keep localStorage in sync so page reloads reflect WS-driven updates.
      const { phone_number: _p, phone_verified: _pv, ...safe } = next as typeof next & { phone_number?: string; phone_verified?: boolean };
      localStorage.setItem('user', JSON.stringify(safe));
      return next;
    });
    setMessages(prev => prev.map(m => {
      if (m.author_id !== update.user_id) return m;
      const next = { ...m };
      if (update.username !== undefined) next.author_username = update.username;
      if (update.avatar_url !== undefined) next.author_avatar_url = update.avatar_url;
      if (update.display_name !== undefined) next.author_display_name = update.display_name;
      return next;
    }));
  }, []);

  const loadMoreMessages = useCallback(async () => {
    if (!activeChannel || isLoadingMessages) return;
    setIsLoadingMessages(true);
    try {
      const oldest = messages[0];
      const msgs = await messagesApi.getMessages(activeChannel.id, { limit: 50, before: oldest?.id });
      if (!msgs || msgs.length === 0) {
        setHasMoreMessages(false);
        return;
      }
      setMessages(prev => [...msgs, ...prev]);
      setHasMoreMessages(msgs.length >= 50);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoadingMessages(false);
    }
  }, [activeChannel, isLoadingMessages, messages]);

  const loadMoreDmMessages = useCallback(async () => {
    if (!activeDmChannel || isLoadingDms) return;
    setIsLoadingDms(true);
    try {
      const oldest = dmMessages[0];
      const msgs = await dmsApi.getDmMessages(activeDmChannel.id, 50, oldest?.id);
      if (!msgs || msgs.length === 0) {
        setHasMoreDmMessages(false);
        return;
      }
      setDmMessages(prev => [...msgs, ...prev]);
      setHasMoreDmMessages(msgs.length >= 50);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoadingDms(false);
    }
  }, [activeDmChannel, isLoadingDms, dmMessages]);

  const loadFriends = useCallback(async () => {
    try {
      const [f, reqs] = await Promise.all([
        friendsApi.getFriends(),
        friendsApi.getFriendRequests(),
      ]);
      setFriends(f ?? []);
      setFriendRequests(reqs ?? { incoming: [], outgoing: [] });
    } catch (err) {
      console.error('loadFriends:', err);
    }
  }, []);

  const sendFriendRequest = useCallback(async (username: string) => {
    const req = await friendsApi.sendFriendRequest(username);
    setFriendRequests(prev => ({ ...prev, outgoing: [...prev.outgoing, req] }));
  }, []);

  const acceptFriendRequest = useCallback(async (requestId: string) => {
    const newFriend = await friendsApi.acceptFriendRequest(requestId);
    setFriendRequests(prev => ({
      ...prev,
      incoming: prev.incoming.filter(r => r.id !== requestId),
    }));
    setFriends(prev => {
      if (prev.some(f => f.id === newFriend.id)) return prev;
      return [...prev, newFriend];
    });
  }, []);

  const declineOrCancelRequest = useCallback(async (requestId: string) => {
    await friendsApi.declineOrCancelRequest(requestId);
    setFriendRequests(prev => ({
      incoming: prev.incoming.filter(r => r.id !== requestId),
      outgoing: prev.outgoing.filter(r => r.id !== requestId),
    }));
  }, []);

  const removeFriend = useCallback(async (userId: string) => {
    await friendsApi.removeFriend(userId);
    setFriends(prev => prev.filter(f => f.id !== userId));
  }, []);

  // WS event handlers
  const receiveFriendRequest = useCallback((req: FriendRequest) => {
    setFriendRequests(prev => {
      if (prev.incoming.some(r => r.id === req.id)) return prev;
      return { ...prev, incoming: [req, ...prev.incoming] };
    });
  }, []);

  const receiveFriendAccept = useCallback((user: FriendUser) => {
    // Add to friends
    setFriends(prev => {
      if (prev.some(f => f.id === user.id)) return prev;
      return [...prev, user];
    });
    // Remove from both incoming and outgoing (handles all-session cases)
    setFriendRequests(prev => ({
      incoming: prev.incoming.filter(r => r.user.id !== user.id),
      outgoing: prev.outgoing.filter(r => r.user.id !== user.id),
    }));
  }, []);

  const receiveFriendRemove = useCallback((userId: string) => {
    setFriends(prev => prev.filter(f => f.id !== userId));
  }, []);

  const loadNotifications = useCallback(async () => {
    try {
      const data = await notificationsApi.getNotifications();
      setNotifications(data ?? []);
    } catch { /* ignore */ }
  }, []);

  const receiveNotification = useCallback((notif: AppNotification) => {
    setNotifications(prev => [notif, ...prev]);
  }, []);

  const markNotificationRead = useCallback(async (id: string) => {
    await notificationsApi.markRead(id);
    setNotifications(prev => prev.map(n => n.id === id ? { ...n, read: true } : n));
  }, []);

  const markAllNotificationsRead = useCallback(async () => {
    await notificationsApi.markAllRead();
    setNotifications(prev => prev.map(n => ({ ...n, read: true })));
  }, []);

  return (
    <AppContext.Provider value={{
      currentUser,
      servers,
      activeServer,
      channels,
      activeChannel,
      messages,
      hasMoreMessages,
      members,
      isLoadingServers,
      isLoadingChannels,
      isLoadingMessages,
      dmChannels,
      activeDmChannel,
      dmMessages,
      hasMoreDmMessages,
      isLoadingDms,
      selectServer,
      selectChannel,
      selectChannelAround,
      clearJumpTarget,
      createServer,
      updateServer,
      deleteServer,
      leaveServer,
      addServer,
      createChannel,
      deleteChannel,
      reorderChannels,
      sendMessage,
      editMessage,
      deleteMessage,
      receiveMessage,
      receiveMessageUpdate,
      receiveMessageDelete,
      toggleReaction,
      applyReactionUpdate,
      logout,
      loadDmChannels,
      openDmChannel,
      selectDmChannel,
      sendDmMessage,
      receiveDmMessage,
      deleteDmMessage,
      applyDmReactionUpdate,
      receiveDmMessageDelete,
      receiveDmChannelCreate,
      applyDmChannelUpdate,
      updateCurrentUser,
      loadServers,
      receiveChannelCreate,
      receiveChannelUpdate,
      receiveChannelDelete,
      receiveMemberLeave,
      receiveMemberRemoved,
      receiveMemberRoleUpdate,
      receiveBotStatusUpdate,
      receiveUserUpdate,
      reloadMembers,
      reloadChannels,
      loadMoreMessages,
      loadMoreDmMessages,
      friends,
      friendRequests,
      pendingRequestCount,
      pendingJumpMessageId,
      loadFriends,
      sendFriendRequest,
      acceptFriendRequest,
      declineOrCancelRequest,
      removeFriend,
      receiveFriendRequest,
      receiveFriendAccept,
      receiveFriendRemove,
      notifications,
      unreadNotificationCount,
      receiveNotification,
      markNotificationRead,
      markAllNotificationsRead,
      loadNotifications,
    }}>
      {children}
    </AppContext.Provider>
  );
}

export function useApp() {
  const ctx = useContext(AppContext);
  if (!ctx) throw new Error('useApp must be used within AppProvider');
  return ctx;
}
