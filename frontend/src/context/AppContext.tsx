import React, { createContext, useContext, useEffect, useState, useCallback } from 'react';
import { User, Server, Channel, Message, ServerMember, DmChannel, DmMessage, Reaction } from '../api/types';
import { apiClient } from '../api/client';
import * as serversApi from '../api/servers';
import * as channelsApi from '../api/channels';
import * as messagesApi from '../api/messages';
import * as dmsApi from '../api/dms';
import { ReactionUpdate, MemberRoleUpdate, UserUpdate } from '../hooks/useWebSocket';

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
}

interface AppActions {
  selectServer: (serverId: string, channelId?: string) => Promise<void>;
  selectChannel: (channelId: string) => Promise<void>;
  createServer: (name: string) => Promise<void>;
  updateServer: (server: Server) => void;
  deleteServer: (serverId: string) => void;
  leaveServer: (serverId: string) => Promise<void>;
  createChannel: (name: string, type: number, topic?: string) => Promise<void>;
  deleteChannel: (channelId: string) => Promise<void>;
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
  sendDmMessage: (content: string, attachmentUrl?: string, attachmentName?: string, attachmentType?: string) => Promise<void>;
  receiveDmMessage: (msg: DmMessage) => void;
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
  receiveUserUpdate: (update: UserUpdate) => void;
  reloadMembers: (serverId: string) => Promise<void>;
  loadMoreMessages: () => Promise<void>;
  loadMoreDmMessages: () => Promise<void>;
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
        email_verified: u.email_verified ?? undefined,
        phone_number: u.phone_number || '',
        phone_verified: u.phone_verified ?? undefined,
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
  const [dmMessages, setDmMessages] = useState<DmMessage[]>([]);
  const [hasMoreDmMessages, setHasMoreDmMessages] = useState(false);
  const [isLoadingDms, setIsLoadingDms] = useState(false);

  useEffect(() => {
    const token = localStorage.getItem('token');
    if (token) {
      apiClient.setToken(token);
    }
  }, []);

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
  }, [currentUser]);

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
      const target = (channelId && chs?.find(c => c.id === channelId)) || chs?.find(c => c.type === 0);
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
  }, [channels]);

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
    const ch = activeChannel;
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
  }, [activeChannel]);

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

  const sendDmMessage = useCallback(async (content: string, attachmentUrl?: string, attachmentName?: string, attachmentType?: string) => {
    if (!activeDmChannel) return;
    const msg = await dmsApi.sendDmMessage(activeDmChannel.id, content, attachmentUrl, attachmentName, attachmentType);
    setDmMessages(prev => [...prev, msg]);
  }, [activeDmChannel]);

  const receiveDmMessage = useCallback((msg: DmMessage) => {
    if (activeDmChannel && msg.dm_channel_id === activeDmChannel.id) {
      setDmMessages(prev => {
        if (prev.some(m => m.id === msg.id)) return prev;
        return [...prev, msg];
      });
    }
  }, [activeDmChannel]);

  const logout = useCallback(() => {
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    apiClient.setToken(null);
    window.location.href = '/login';
  }, []);

  const updateCurrentUser = useCallback((user: User) => {
    setCurrentUser(user);
    localStorage.setItem('user', JSON.stringify(user));
  }, []);

  const receiveMessageUpdate = useCallback((msg: Message) => {
    setMessages(prev => prev.map(m => m.id === msg.id ? { ...msg, reactions: m.reactions } : m));
  }, []);

  const receiveMessageDelete = useCallback((messageId: string, _channelId: string) => {
    setMessages(prev => prev.filter(m => m.id !== messageId));
  }, []);

  const applyReactionUpdate = useCallback((update: ReactionUpdate) => {
    setMessages(prev => prev.map(msg => {
      if (msg.id !== update.message_id) return msg;
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
    }));
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

  const receiveUserUpdate = useCallback((update: UserUpdate) => {
    setMembers(prev => prev.map(m =>
      m.user_id === update.user_id
        ? { ...m, username: update.username }
        : m
    ));
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
      createServer,
      updateServer,
      deleteServer,
      leaveServer,
      addServer,
      createChannel,
      deleteChannel,
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
      updateCurrentUser,
      loadServers,
      receiveChannelCreate,
      receiveChannelUpdate,
      receiveChannelDelete,
      receiveMemberLeave,
      receiveMemberRemoved,
      receiveMemberRoleUpdate,
      receiveUserUpdate,
      reloadMembers,
      loadMoreMessages,
      loadMoreDmMessages,
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
