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
  // DM state
  dmChannels: DmChannel[];
  activeDmChannel: DmChannel | null;
  dmMessages: DmMessage[];
  isLoadingDms: boolean;
}

interface AppActions {
  selectServer: (serverId: string) => Promise<void>;
  selectChannel: (channelId: string) => Promise<void>;
  createServer: (name: string) => Promise<void>;
  createChannel: (name: string, type: number) => Promise<void>;
  deleteChannel: (channelId: string) => Promise<void>;
  sendMessage: (content: string) => Promise<void>;
  receiveMessage: (msg: Message) => void;
  logout: () => void;
  // DM actions
  loadDmChannels: () => Promise<void>;
  openDmChannel: (userId: string) => Promise<void>;
  selectDmChannel: (channelId: string) => Promise<void>;
  sendDmMessage: (content: string) => Promise<void>;
  receiveDmMessage: (msg: DmMessage) => void;
}

const AppContext = createContext<(AppState & AppActions) | null>(null);

export function AppProvider({ children }: { children: React.ReactNode }) {
  const [currentUser] = useState<User | null>(() => {
    try {
      const stored = localStorage.getItem('user');
      if (!stored) return null;
      const u = JSON.parse(stored);
      // Handle old PascalCase format (stored before JSON tags were added)
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

  // DM state
  const [dmChannels, setDmChannels] = useState<DmChannel[]>([]);
  const [activeDmChannel, setActiveDmChannel] = useState<DmChannel | null>(null);
  const [dmMessages, setDmMessages] = useState<DmMessage[]>([]);
  const [isLoadingDms, setIsLoadingDms] = useState(false);

  // Initialize token from localStorage
  useEffect(() => {
    const token = localStorage.getItem('token');
    if (token) {
      apiClient.setToken(token);
    }
  }, []);

  // Load servers on mount
  useEffect(() => {
    if (!currentUser) return;
    setIsLoadingServers(true);
    serversApi.getServers()
      .then(data => setServers(data ?? []))
      .catch(console.error)
      .finally(() => setIsLoadingServers(false));

    // Also load DM channels
    dmsApi.getDmChannels()
      .then(data => setDmChannels(data ?? []))
      .catch(console.error);
  }, [currentUser]);

  const selectServer = useCallback(async (serverId: string) => {
    // Clear DM state when selecting a server
    setActiveDmChannel(null);
    setDmMessages([]);

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
      // Auto-select first text channel
      const firstText = chs.find(c => c.type === 0);
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
    // Clear DM state when selecting a channel
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
    // Auto-select the new server
    setActiveServer(srv);
    setChannels([]);
    setMembers([]);
    setActiveChannel(null);
    setMessages([]);
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

  const receiveMessage = useCallback((msg: Message) => {
    if (activeChannel && msg.channel_id === activeChannel.id) {
      setMessages(prev => {
        // Avoid duplicates (message may have already been added optimistically)
        if (prev.some(m => m.id === msg.id)) return prev;
        return [...prev, msg];
      });
    }
  }, [activeChannel]);

  // DM Actions
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

  const openDmChannelAction = useCallback(async (userId: string) => {
    try {
      const channel = await dmsApi.openDmChannel(userId);
      // Check if we already have this channel
      setDmChannels(prev => {
        const exists = prev.find(c => c.id === channel.id);
        if (exists) return prev;
        return [channel, ...prev];
      });
      setActiveDmChannel(channel);
      setActiveServer(null);
      setActiveChannel(null);
      // Load messages
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
      createChannel,
      deleteChannel,
      sendMessage,
      receiveMessage,
      logout,
      loadDmChannels,
      openDmChannel: openDmChannelAction,
      selectDmChannel,
      sendDmMessage,
      receiveDmMessage,
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