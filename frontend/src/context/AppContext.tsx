import React, { createContext, useContext, useEffect, useState, useCallback } from 'react';
import { User, Server, Channel, Message, ServerMember } from '../api/types';
import { apiClient } from '../api/client';
import * as serversApi from '../api/servers';
import * as channelsApi from '../api/channels';
import * as messagesApi from '../api/messages';

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
}

const AppContext = createContext<(AppState & AppActions) | null>(null);

export function AppProvider({ children }: { children: React.ReactNode }) {
  const [currentUser] = useState<User | null>(() => {
    try {
      const stored = localStorage.getItem('user');
      return stored ? JSON.parse(stored) : null;
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
      .then(setServers)
      .catch(console.error)
      .finally(() => setIsLoadingServers(false));
  }, [currentUser]);

  const selectServer = useCallback(async (serverId: string) => {
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
      setChannels(chs);
      setMembers(mems);
      // Auto-select first text channel
      const firstText = chs.find(c => c.type === 0);
      if (firstText) {
        setActiveChannel(firstText);
        setIsLoadingMessages(true);
        const msgs = await messagesApi.getMessages(firstText.id);
        setMessages(msgs);
        setIsLoadingMessages(false);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoadingChannels(false);
    }
  }, [servers]);

  const selectChannel = useCallback(async (channelId: string) => {
    const ch = channels.find(c => c.id === channelId);
    if (!ch) return;
    setActiveChannel(ch);
    setIsLoadingMessages(true);
    try {
      const msgs = await messagesApi.getMessages(channelId);
      setMessages(msgs);
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
      selectServer,
      selectChannel,
      createServer,
      createChannel,
      deleteChannel,
      sendMessage,
      receiveMessage,
      logout,
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
