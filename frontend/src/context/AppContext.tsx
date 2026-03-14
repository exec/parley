import React, { createContext, useContext, useEffect, useState, useCallback } from 'react';
import { User, Server, Channel, Message, ServerMember, DmChannel, DmMessage } from '../api/types';
import { apiClient } from '../api/client';
import * as serversApi from '../api/servers';
import * as channelsApi from '../api/channels';
import * as messagesApi from '../api/messages';
import * as dmsApi from '../api/dms';

interface AppState {
  currentUser: User | null;
  servers: Server[];
  activeServer: Server | null;
  channels: Channel[];
  activeChannel: Channel | null;
  messages: Message[];
  members: ServerMember[];
  isLoadingServers: boolean;
  isLoadingChannels: boolean;
  isLoadingMessages: boolean;
  dmChannels: DmChannel[];
  activeDmChannel: DmChannel | null;
  dmMessages: DmMessage[];
  isLoadingDms: boolean;
}

interface AppActions {
  selectServer: (serverId: string) => Promise<void>;
  selectChannel: (channelId: string) => Promise<void>;
  createServer: (name: string) => Promise<void>;
  updateServer: (server: Server) => void;
  deleteServer: (serverId: string) => void;
  leaveServer: (serverId: string) => Promise<void>;
  createChannel: (name: string, type: number) => Promise<void>;
  deleteChannel: (channelId: string) => Promise<void>;
  sendMessage: (content: string) => Promise<void>;
  editMessage: (messageId: string, content: string) => Promise<void>;
  deleteMessage: (messageId: string) => Promise<void>;
  receiveMessage: (msg: Message) => void;
  logout: () => void;
  loadDmChannels: () => Promise<void>;
  openDmChannel: (userId: string) => Promise<void>;
  selectDmChannel: (channelId: string) => Promise<void>;
  sendDmMessage: (content: string) => Promise<void>;
  receiveDmMessage: (msg: DmMessage) => void;
  addServer: (server: Server) => void;
  updateCurrentUser: (user: User) => void;
  loadServers: () => Promise<void>;
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
  const [members, setMembers] = useState<ServerMember[]>([]);
  const [isLoadingServers, setIsLoadingServers] = useState(false);
  const [isLoadingChannels, setIsLoadingChannels] = useState(false);
  const [isLoadingMessages, setIsLoadingMessages] = useState(false);

  const [dmChannels, setDmChannels] = useState<DmChannel[]>([]);
  const [activeDmChannel, setActiveDmChannel] = useState<DmChannel | null>(null);
  const [dmMessages, setDmMessages] = useState<DmMessage[]>([]);
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

  const selectServer = useCallback(async (serverId: string) => {
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
      const firstText = chs?.find(c => c.type === 0);
      if (firstText) {
        setActiveChannel(firstText);
        setIsLoadingMessages(true);
        const msgs = await messagesApi.getMessages(firstText.id);
        setMessages(msgs ?? []);
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
    setIsLoadingMessages(true);
    try {
      const msgs = await messagesApi.getMessages(channelId);
      setMessages(msgs ?? []);
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

  const createChannel = useCallback(async (name: string, type: number) => {
    if (!activeServer) return;
    const ch = await channelsApi.createChannel(activeServer.id, name, type);
    setChannels(prev => [...prev, ch]);
  }, [activeServer]);

  const deleteChannel = useCallback(async (channelId: string) => {
    await channelsApi.deleteChannel(channelId);
    setChannels(prev => prev.filter(c => c.id !== channelId));
    if (activeChannel?.id === channelId) {
      setActiveChannel(null);
      setMessages([]);
    }
  }, [activeChannel]);

  const sendMessage = useCallback(async (content: string) => {
    if (!activeChannel) return;
    const msg = await messagesApi.sendMessage(activeChannel.id, content);
    setMessages(prev => [...prev, msg]);
  }, [activeChannel]);

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

    setIsLoadingDms(true);
    try {
      const msgs = await dmsApi.getDmMessages(channel.id);
      setDmMessages(msgs ?? []);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoadingDms(false);
    }
  }, [dmChannels]);

  const sendDmMessage = useCallback(async (content: string) => {
    if (!activeDmChannel) return;
    const msg = await dmsApi.sendDmMessage(activeDmChannel.id, content);
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

  return (
    <AppContext.Provider value={{
      currentUser,
      servers,
      activeServer,
      channels,
      activeChannel,
      messages,
      members,
      isLoadingServers,
      isLoadingChannels,
      isLoadingMessages,
      dmChannels,
      activeDmChannel,
      dmMessages,
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
      logout,
      loadDmChannels,
      openDmChannel,
      selectDmChannel,
      sendDmMessage,
      receiveDmMessage,
      updateCurrentUser,
      loadServers,
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
