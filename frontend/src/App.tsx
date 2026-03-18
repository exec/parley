import { Routes, Route, Navigate, useNavigate, useLocation } from 'react-router-dom';
import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { Login } from './pages/Login';
import { Register } from './pages/Register';
import { InvitePage } from './pages/InvitePage';
import { VerifyEmail } from './pages/VerifyEmail';
import { Impersonate } from './pages/Impersonate';
import { ForgotPassword } from './pages/ForgotPassword';
import { ResetPassword } from './pages/ResetPassword';
import { AppProvider, useApp } from './context/AppContext';
import { ThemeProvider } from './context/ThemeContext';
import { Landing } from './pages/Landing';
import { SharedThemePage } from './pages/SharedThemePage';
import { ThemeRepoPage } from './pages/ThemeRepoPage';
import { BotInvitePage } from './pages/BotInvitePage';
import { useWebSocket, MemberRoleUpdate, UserUpdate, VoiceStateUpdate, VoiceForceMuteEvent, RoleUpdateEvent, RoleDeleteEvent, BotStatusUpdate } from './hooks/useWebSocket';
import { VoiceChannel } from './components/voice/VoiceChannel';

import { DmMessage, Message, BinChannelTag, ServerMember } from './api/types';
import * as serversApi from './api/servers';
import * as channelsApi from './api/channels';
import * as dmsApi from './api/dms';
import { getVoiceParticipants, muteVoiceParticipant } from './api/voice';
import { getTags } from './api/bin';
import MainLayout from './components/layout/MainLayout';
import ChannelList from './components/layout/ChannelList';
import DmPanel from './components/layout/DmPanel';
import FriendsView from './components/layout/FriendsView';
import UserSidebar from './components/layout/UserSidebar';
import MiniProfile from './components/layout/MiniProfile';
import { ChatWindow } from './components/chat/ChatWindow';
import { BinChannel } from './components/bin/BinChannel';
import { PostView } from './components/bin/PostView';
import { CreatePostModal } from './components/bin/CreatePostModal';
import { Homepage } from './pages/Homepage';
import { CreateServerModal } from './components/modals/CreateServerModal';
import { CreateChannelModal } from './components/modals/CreateChannelModal';
import { UserProfileModal } from './components/modals/UserProfileModal';
import { AssignRolesModal } from './components/modals/AssignRolesModal';
import { UserSettings } from './components/settings/UserSettings';
import { ServerSettings } from './components/settings/ServerSettings';
import { NotificationSettingsModal, getNotifPref } from './components/modals/NotificationSettingsModal';
import { ChannelSettingsModal } from './components/modals/ChannelSettingsModal';
import { useNotifications } from './hooks/useNotifications';
import { invalidatePermCache, clearAllPermCaches } from './hooks/usePermissions';
import { useVoiceConnection } from './hooks/useVoiceConnection';
import { ErrorBoundary } from './components/ErrorBoundary';

type View = 'homepage' | 'server' | 'dm';

function MainApp() {
  const {
    currentUser,
    servers,
    activeServer,
    channels,
    activeChannel,
    messages,
    members,
    isLoadingServers,
    isLoadingMessages,
    selectServer,
    selectChannel,
    createServer,
    updateServer,
    deleteServer,
    leaveServer,
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
    receiveDmMessage,
    deleteDmMessage,
    applyDmReactionUpdate,
    receiveDmMessageDelete,
    logout,
    dmChannels,
    activeDmChannel,
    dmMessages,
    hasMoreDmMessages,
    isLoadingDms,
    selectDmChannel,
    sendDmMessage,
    openDmChannel,
    updateCurrentUser,
    loadServers,
    hasMoreMessages,
    loadMoreMessages,
    loadMoreDmMessages,
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
    reorderChannels,
    friends,
    friendRequests,
    pendingRequestCount,
    sendFriendRequest,
    acceptFriendRequest,
    declineOrCancelRequest,
    removeFriend,
    receiveFriendRequest,
    receiveFriendAccept,
    receiveFriendRemove,
  } = useApp();

  const navigate = useNavigate();
  const location = useLocation();
  const didRestoreFromUrl = useRef(false);

  const [showCreateServer, setShowCreateServer] = useState(false);
  const [showCreateChannel, setShowCreateChannel] = useState(false);
  const [showProfile, setShowProfile] = useState(false);
  const [profileUserId, setProfileUserId] = useState<string | null>(null);
  const [vcMiniProfile, setVcMiniProfile] = useState<{ member: ServerMember; position: { top: number; left: number } } | null>(null);
  const [showServerSettings, setShowServerSettings] = useState(false);
  const [serverSettingsInitialTab, setServerSettingsInitialTab] = useState<'overview' | 'roles' | 'danger'>('overview');
  const [showUserSettings, setShowUserSettings] = useState(false);
  const [vcChatOpen, setVcChatOpen] = useState(false);
  // Assign roles modal (from context menu on a specific user)
  const [showAssignRoles, setShowAssignRoles] = useState(false);
  const [assignRolesUserId, setAssignRolesUserId] = useState('');
  const [assignRolesUsername, setAssignRolesUsername] = useState('');

  // Channel settings modal — snapshot name/parentId at open time so WS events
  // don't cause ChannelPermissions to re-fetch while the modal is open
  const [showChannelSettings, setShowChannelSettings] = useState(false);
  const [channelSettingsId, setChannelSettingsId] = useState('');
  const [channelSettingsName, setChannelSettingsName] = useState('');
  const [channelSettingsParentId, setChannelSettingsParentId] = useState<string | undefined>(undefined);

  // Notification settings modal
  const [showNotifSettings, setShowNotifSettings] = useState(false);
  const [notifSettingsServerId, setNotifSettingsServerId] = useState('');

  const { requestPermission, notify } = useNotifications();

  // Request notification permission once on mount (Chromium allows this without a gesture)
  useEffect(() => { requestPermission(); }, [requestPermission]);

  // Voice state: channelId → list of participants
  const [voiceParticipants, setVoiceParticipants] = useState<Record<string, { user_id: string; username: string; avatar_url?: string }[]>>({});
  const [activeVoiceChannel, setActiveVoiceChannel] = useState<string | null>(null);

  const handleVcLeave = useCallback(() => {
    if (currentUser && activeVoiceChannel) {
      setVoiceParticipants(prev => {
        const filtered = (prev[activeVoiceChannel] ?? []).filter(p => p.user_id !== currentUser.id);
        return { ...prev, [activeVoiceChannel]: filtered };
      });
    }
    setActiveVoiceChannel(null);
  }, [currentUser, activeVoiceChannel]);

  const {
    connected: vcConnected,
    connecting: vcConnecting,
    error: vcError,
    muted: vcMuted,
    deafened: vcDeafened,
    videoEnabled: vcVideoEnabled,
    screenSharing: vcScreenSharing,
    activeSpeakers: vcActiveSpeakers,
    participants: vcParticipants,
    localParticipant: vcLocalParticipant,

    toggleMute: vcToggleMute,
    forceMute: vcForceMute,
    toggleDeafen: vcToggleDeafen,
    toggleVideo: vcToggleVideo,
    toggleScreenShare: vcToggleScreenShare,
    disconnect: vcDisconnect,
    retry: vcRetry,
  } = useVoiceConnection(activeVoiceChannel, handleVcLeave);

  // Reply-to state for nested replies
  const [replyTo, setReplyTo] = useState<Message | null>(null);

  // Sidebar visibility — open by default on desktop, closed on mobile
  const [showMembers, setShowMembers] = useState(() => window.innerWidth > 768);
  const [showChannelList, setShowChannelList] = useState(false); // mobile drawer, starts closed

  // Bin channel state
  const [activePostId, setActivePostId] = useState<string | null>(null);
  const [showCreatePost, setShowCreatePost] = useState(false);
  const [binTags, setBinTags] = useState<BinChannelTag[]>([]);

  // Typing indicators: channelId → list of typing users
  const [typingUsers, setTypingUsers] = useState<Record<string, { userId: string; username: string }[]>>({});
  const typingTimeoutsRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());
  const lastTypingSentRef = useRef<number>(0);

  // Unread counts: channelId (or dmChannelId) → unread message count
  const [unreadCounts, setUnreadCounts] = useState<Record<string, number>>({});

  // Online presence: set of user IDs currently connected via WebSocket
  const [onlineUsers, setOnlineUsers] = useState<Set<string>>(new Set());

  // Friends view toggle
  const [activeFriendsView, setActiveFriendsView] = useState(false);

  // Determine current view
  const view: View = activeDmChannel ? 'dm' : activeServer ? 'server' : 'homepage';

  // Compute effective permissions from the current user's roles in the active server
  const isServerOwner = currentUser?.id === activeServer?.owner_id;
  const currentMember = members.find(m => m.user_id === currentUser?.id);
  const effectivePermissions = isServerOwner
    ? ~0 // all bits set
    : (currentMember?.roles ?? []).reduce((acc, role) => acc | (role.permissions ?? 0), 0);
  // Bit 0 = Administrator, Bit 3 = ManageChannels, Bit 4 = KickMembers, Bit 5 = BanMembers
  const canManageChannels = isServerOwner || (effectivePermissions & (1 | 8)) !== 0;
  const canKickMembers = isServerOwner || (effectivePermissions & (1 | 16)) !== 0;
  const canBanMembers = isServerOwner || (effectivePermissions & (1 | 32)) !== 0;
  // PermMuteMembers = 1 << 34 — too large for bitwise int32, use BigInt comparison
  const canMuteMembers = isServerOwner || (BigInt(effectivePermissions) & (BigInt(1) << BigInt(34))) !== BigInt(0);
  const canKickFromVoice = isServerOwner || (BigInt(effectivePermissions) & (BigInt(1) << BigInt(36))) !== BigInt(0);

  // Restore state from URL once servers are loaded
  useEffect(() => {
    if (isLoadingServers || didRestoreFromUrl.current || servers.length === 0) return;
    didRestoreFromUrl.current = true;

    const path = location.pathname;
    // /channels/@me/:dmId
    const dmMatch = path.match(/^\/channels\/@me\/([^/]+)$/);
    if (dmMatch) {
      selectDmChannel(dmMatch[1]);
      return;
    }
    // /channels/:serverId/:channelId or /channels/:serverId
    const serverMatch = path.match(/^\/channels\/([^/]+)(?:\/([^/]+))?$/);
    if (serverMatch) {
      const [, serverId, channelId] = serverMatch;
      selectServer(serverId, channelId || undefined);
    }
  }, [isLoadingServers, servers.length]); // eslint-disable-line react-hooks/exhaustive-deps

  // Update URL when active state changes
  useEffect(() => {
    if (!didRestoreFromUrl.current) return;
    if (activeDmChannel) {
      navigate(`/channels/@me/${activeDmChannel.id}`, { replace: true });
    } else if (activeServer && activeChannel) {
      navigate(`/channels/${activeServer.id}/${activeChannel.id}`, { replace: true });
    } else if (activeServer) {
      navigate(`/channels/${activeServer.id}`, { replace: true });
    } else {
      navigate('/channels/@me', { replace: true });
    }
  }, [activeDmChannel?.id, activeServer?.id, activeChannel?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  // Reset bin state when channel changes
  useEffect(() => {
    setActivePostId(null);
    setShowCreatePost(false);
    setBinTags([]);
    if (activeChannel?.type === 2) {
      getTags(activeChannel.id)
        .then(tags => setBinTags(tags))
        .catch(console.error);
    }
  }, [activeChannel?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  // Clear unread count when the active channel or DM channel changes
  useEffect(() => {
    if (!activeChannel) return;
    setUnreadCounts(prev => {
      if (!prev[activeChannel.id]) return prev;
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      const { [activeChannel.id]: _cleared, ...rest } = prev;
      return rest;
    });
  }, [activeChannel?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!activeDmChannel) return;
    setUnreadCounts(prev => {
      if (!prev[activeDmChannel.id]) return prev;
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      const { [activeDmChannel.id]: _cleared, ...rest } = prev;
      return rest;
    });
  }, [activeDmChannel?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  // Reset friends view when navigating to a server or DM channel
  useEffect(() => {
    if (activeServer || activeDmChannel) {
      setActiveFriendsView(false);
    }
  }, [activeServer, activeDmChannel]);

  const handleServerMemberJoin = useCallback((serverId: string, _userId: string) => {
    loadServers();
    if (activeServer?.id === serverId) {
      reloadMembers(serverId);
    }
  }, [loadServers, reloadMembers, activeServer?.id]);

  const handleMemberLeave = useCallback((serverId: string, userId: string) => {
    receiveMemberLeave(serverId, userId);
  }, [receiveMemberLeave]);

  const handleMemberKick = useCallback((serverId: string, userId: string) => {
    if (currentUser && userId === currentUser.id) {
      // Current user was kicked — remove server from state
      receiveMemberRemoved(serverId, userId);
    } else {
      receiveMemberLeave(serverId, userId);
    }
  }, [currentUser, receiveMemberRemoved, receiveMemberLeave]);

  const handleMemberBan = useCallback((serverId: string, userId: string) => {
    if (currentUser && userId === currentUser.id) {
      receiveMemberRemoved(serverId, userId);
    } else {
      receiveMemberLeave(serverId, userId);
    }
  }, [currentUser, receiveMemberRemoved, receiveMemberLeave]);

  const handleServerUpdate = useCallback((server: Parameters<typeof updateServer>[0]) => {
    updateServer(server);
  }, [updateServer]);

  const handleServerDelete = useCallback((serverId: string) => {
    deleteServer(serverId);
  }, [deleteServer]);

  const handleBotStatusUpdate = useCallback((update: BotStatusUpdate) => {
    receiveBotStatusUpdate(update);
  }, [receiveBotStatusUpdate]);

  const handleMemberRoleUpdate = useCallback((update: MemberRoleUpdate) => {
    receiveMemberRoleUpdate(update);
    // Bust the permission cache for the affected server so usePermissions re-fetches
    if (update.server_id) invalidatePermCache(update.server_id);
    // If it's the current user's roles that changed, reload channels — their
    // visible channel set may have changed (permissions added/removed).
    if (update.user_id === currentUser?.id && update.server_id) {
      reloadChannels(update.server_id);
    }
  }, [receiveMemberRoleUpdate, currentUser?.id, reloadChannels]);

  const handleRoleUpdate = useCallback((event: RoleUpdateEvent) => {
    if (event.server_id) invalidatePermCache(event.server_id);
  }, []);

  const handleRoleDelete = useCallback((event: RoleDeleteEvent) => {
    if (event.server_id) invalidatePermCache(event.server_id);
  }, []);

  const handleUserUpdate = useCallback((update: UserUpdate) => {
    receiveUserUpdate(update);
  }, [receiveUserUpdate]);

  // Fetch initial voice presence whenever the channel list changes (server switch / reload)
  useEffect(() => {
    const voiceChannels = channels.filter(c => c.type === 1);
    if (voiceChannels.length === 0) return;
    Promise.all(
      voiceChannels.map(ch =>
        getVoiceParticipants(ch.id)
          .then(ps => ({ channelId: ch.id, participants: ps }))
          .catch(() => ({ channelId: ch.id, participants: [] }))
      )
    ).then(results => {
      setVoiceParticipants(prev => {
        const next = { ...prev };
        results.forEach(({ channelId, participants }) => { next[channelId] = participants; });
        return next;
      });
    });
  }, [channels]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleVoiceForceMute = useCallback((_event: VoiceForceMuteEvent) => {
    vcForceMute();
  }, [vcForceMute]);

  const handleVoiceForceDisconnect = useCallback(() => {
    vcDisconnect();
  }, [vcDisconnect]);

  const handleVoiceStateUpdate = useCallback((update: VoiceStateUpdate) => {
    setVoiceParticipants(prev => {
      const list = prev[update.channel_id] ?? [];
      if (update.action === 'join') {
        if (list.some(p => p.user_id === update.user_id)) return prev;
        return { ...prev, [update.channel_id]: [...list, { user_id: update.user_id, username: update.username, avatar_url: update.avatar_url }] };
      } else {
        const filtered = list.filter(p => p.user_id !== update.user_id);
        return { ...prev, [update.channel_id]: filtered };
      }
    });
  }, []);

  const clearTypingUser = useCallback((channelId: string, userId: string) => {
    const key = `${channelId}:${userId}`;
    const existing = typingTimeoutsRef.current.get(key);
    if (existing) {
      clearTimeout(existing);
      typingTimeoutsRef.current.delete(key);
    }
    setTypingUsers(prev => {
      const list = prev[channelId] ?? [];
      if (!list.some(t => t.userId === userId)) return prev;
      const filtered = list.filter(t => t.userId !== userId);
      if (filtered.length === 0) {
        // eslint-disable-next-line @typescript-eslint/no-unused-vars
        const { [channelId]: _removed, ...rest } = prev;
        return rest;
      }
      return { ...prev, [channelId]: filtered };
    });
  }, []);

  const handleTyping = useCallback((userId: string, username: string, channelId: string) => {
    if (userId === currentUser?.id) return; // don't show self typing
    const key = `${channelId}:${userId}`;

    // Reset auto-expire timeout
    const existing = typingTimeoutsRef.current.get(key);
    if (existing) clearTimeout(existing);

    setTypingUsers(prev => {
      const list = prev[channelId] ?? [];
      if (list.some(t => t.userId === userId)) return prev; // already in list
      return { ...prev, [channelId]: [...list, { userId, username }] };
    });

    const timeout = setTimeout(() => {
      setTypingUsers(prev => {
        const list = prev[channelId] ?? [];
        const filtered = list.filter(t => t.userId !== userId);
        if (filtered.length === 0) {
          // eslint-disable-next-line @typescript-eslint/no-unused-vars
          const { [channelId]: _removed, ...rest } = prev;
          return rest;
        }
        return { ...prev, [channelId]: filtered };
      });
      typingTimeoutsRef.current.delete(key);
    }, 3000);

    typingTimeoutsRef.current.set(key, timeout);
  }, [currentUser?.id]);

  const handleReceiveMessage = useCallback((msg: Parameters<typeof receiveMessage>[0]) => {
    clearTypingUser(msg.channel_id, msg.author_id);
    receiveMessage(msg);
    if (msg.channel_id !== activeChannel?.id) {
      // Apply notification preference for this server
      const pref = getNotifPref(activeServer?.id ?? '');
      let shouldCount = true;
      if (pref === 'never') {
        shouldCount = false;
      } else if (pref === 'tags') {
        shouldCount = msg.content.includes('@everyone') ||
          msg.content.includes('@here') ||
          msg.content.includes(`<@${currentUser?.id}>`);
      } else if (pref === 'direct') {
        shouldCount = msg.content.includes(`<@${currentUser?.id}>`);
      }
      if (shouldCount) {
        setUnreadCounts(prev => ({ ...prev, [msg.channel_id]: (prev[msg.channel_id] ?? 0) + 1 }));
        const channelName = channels.find(c => c.id === msg.channel_id)?.name ?? 'a channel';
        notify(`#${channelName}`, `${msg.author_username}: ${msg.content}`, msg.author_avatar_url);
      }
    }
  }, [receiveMessage, clearTypingUser, activeChannel?.id, activeServer?.id, currentUser?.id, channels, notify]);

  const handleReceiveDmMessage = useCallback((msg: DmMessage) => {
    receiveDmMessage(msg);
    if (msg.dm_channel_id !== activeDmChannel?.id) {
      setUnreadCounts(prev => ({ ...prev, [msg.dm_channel_id]: (prev[msg.dm_channel_id] ?? 0) + 1 }));
      notify(`${msg.author_username}`, msg.content, msg.author_avatar_url);
    }
  }, [receiveDmMessage, activeDmChannel?.id, notify]);

  const handleDmDelete = useCallback(async (messageId: string) => {
    if (!activeDmChannel) return;
    await deleteDmMessage(activeDmChannel.id, messageId);
  }, [deleteDmMessage, activeDmChannel]);

  const handleDmReact = useCallback(async (messageId: string, emoji: string) => {
    if (!activeDmChannel) return;
    try {
      await dmsApi.toggleDmReaction(activeDmChannel.id, messageId, emoji);
    } catch (err) {
      console.error('Failed to toggle DM reaction:', err);
    }
  }, [activeDmChannel]);

  // userid → display name map for rendering mention tokens in messages
  const memberMap = useMemo(
    () => new Map(members.map(m => [m.user_id, m.display_name || m.username])),
    [members],
  );

  const channelMap = useMemo(
    () => new Map(channels.map(c => [c.id, c.name])),
    [channels],
  );

  // Channel IDs for the active server (for unread notifications)
  const allChannelIds = channels.map(c => c.id);

  // Virtual server channels for server-level events (membership, channels, etc.)
  const serverVirtualChannelIds = useMemo(() =>
    servers.map(s => `server:${s.id}`),
  [servers]);

  // Virtual DM channels — subscribe on load so events arrive for all DM channels
  const dmVirtualChannelIds = useMemo(() =>
    dmChannels.map(ch => `dm:${ch.id}`),
  [dmChannels]);

  const extraChannelIds = useMemo(() =>
    [...allChannelIds, ...serverVirtualChannelIds, ...dmVirtualChannelIds],
  [allChannelIds, serverVirtualChannelIds, dmVirtualChannelIds]);

  const handleUserOnline = useCallback((userId: string) => {
    setOnlineUsers(prev => {
      if (prev.has(userId)) return prev;
      const next = new Set(prev);
      next.add(userId);
      return next;
    });
  }, []);

  const handleUserOffline = useCallback((userId: string) => {
    setOnlineUsers(prev => {
      if (!prev.has(userId)) return prev;
      const next = new Set(prev);
      next.delete(userId);
      return next;
    });
    // Clear any typing indicators for this user across all channels
    setTypingUsers(prev => {
      let changed = false;
      const updated: Record<string, { userId: string; username: string }[]> = {};
      for (const [channelId, users] of Object.entries(prev)) {
        const filtered = users.filter(u => u.userId !== userId);
        if (filtered.length !== users.length) changed = true;
        if (filtered.length > 0) updated[channelId] = filtered;
      }
      return changed ? updated : prev;
    });
    // Cancel pending typing timeouts for this user
    for (const [key, timeout] of typingTimeoutsRef.current.entries()) {
      if (key.endsWith(`:${userId}`)) {
        clearTimeout(timeout);
        typingTimeoutsRef.current.delete(key);
      }
    }
  }, []);

  const handlePresenceSnapshot = useCallback((userIds: string[]) => {
    setOnlineUsers(new Set(userIds));
  }, []);

  const { sendTyping } = useWebSocket({
    onMessage: handleReceiveMessage,
    onDmMessage: handleReceiveDmMessage,
    onServerMemberJoin: handleServerMemberJoin,
    onServerMemberLeave: handleMemberLeave,
    onServerMemberKick: handleMemberKick,
    onServerMemberBan: handleMemberBan,
    onTyping: handleTyping,
    onUserOnline: handleUserOnline,
    onUserOffline: handleUserOffline,
    onPresenceSnapshot: handlePresenceSnapshot,
    onMessageUpdate: receiveMessageUpdate,
    onMessageDelete: receiveMessageDelete,
    onReactionUpdate: applyReactionUpdate,
    onChannelCreate: receiveChannelCreate,
    onChannelUpdate: receiveChannelUpdate,
    onChannelDelete: receiveChannelDelete,
    onServerUpdate: handleServerUpdate,
    onServerDelete: handleServerDelete,
    onMemberRoleUpdate: handleMemberRoleUpdate,
    onBotStatusUpdate: handleBotStatusUpdate,
    onRoleUpdate: handleRoleUpdate,
    onRoleDelete: handleRoleDelete,
    onConnect: clearAllPermCaches,
    onUserUpdate: handleUserUpdate,
    onVoiceStateUpdate: handleVoiceStateUpdate,
    onVoiceForceMute: handleVoiceForceMute,
    onVoiceForceDisconnect: handleVoiceForceDisconnect,
    onFriendRequest: receiveFriendRequest,
    onFriendAccept: receiveFriendAccept,
    onFriendRemove: receiveFriendRemove,
    onDmMessageDelete: useCallback((messageId: string) => {
      receiveDmMessageDelete(messageId);
    }, [receiveDmMessageDelete]),
    onDmReactionUpdate: useCallback((update: { message_id: string; dm_channel_id: string; user_id: string; emoji: string; added: boolean }) => {
      applyDmReactionUpdate({
        message_id: update.message_id,
        channel_id: update.dm_channel_id,
        user_id: update.user_id,
        emoji: update.emoji,
        added: update.added,
      });
    }, [applyDmReactionUpdate]),
    activeChannelId: activeChannel?.id ?? null,
    extraChannelIds,
  });

  const handleSendTyping = useCallback(() => {
    if (!activeChannel || !currentUser) return;
    const now = Date.now();
    if (now - lastTypingSentRef.current < 2000) return; // throttle: at most once per 2s
    lastTypingSentRef.current = now;
    sendTyping(activeChannel.id, currentUser.username);
  }, [activeChannel, currentUser, sendTyping]);

  // Aggregate unread counts per server (from the active server's channel list)
  const serverUnreadCounts = useMemo(() => {
    const result: Record<string, number> = {};
    channels.forEach(ch => {
      const count = unreadCounts[ch.id];
      if (count) {
        result[ch.server_id] = (result[ch.server_id] ?? 0) + count;
      }
    });
    return result;
  }, [channels, unreadCounts]);

  const handleViewProfile = (userId: string) => {
    setProfileUserId(userId);
    setShowProfile(true);
  };

  const handleVcParticipantClick = (userId: string, clientX: number, clientY: number) => {
    const member = members.find(m => m.user_id === userId);
    if (!member) return;
    const left = Math.min(clientX + 10, window.innerWidth - 295);
    const top = Math.min(clientY, window.innerHeight - 330);
    setVcMiniProfile({ member, position: { top, left } });
  };

  const handleGoHome = () => {
    selectServer('__none__');
  };

  const handleOpenFriends = () => {
    setActiveFriendsView(true);
    selectServer('__none__');
  };

  const handleFriendMessage = async (userId: string) => {
    await openDmChannel(userId);
    setActiveFriendsView(false);
  };

  // Build left panel based on view
  const leftPanel = view === 'server' ? (
    <ChannelList
      serverName={activeServer?.name ?? ''}
      channels={channels}
      activeChannelId={activeChannel?.id ?? null}
      onChannelSelect={(id) => { selectChannel(id); if (window.innerWidth <= 768) setShowChannelList(false); }}
      onCreateChannel={() => setShowCreateChannel(true)}
      onDeleteChannel={deleteChannel}
      onManageRoles={() => { setServerSettingsInitialTab('roles'); setShowServerSettings(true); }}
      onServerSettings={() => { setServerSettingsInitialTab('overview'); setShowServerSettings(true); }}
      onLeaveServer={() => leaveServer(activeServer?.id ?? '')}
      owner_id={activeServer?.owner_id}
      currentUser={currentUser ?? undefined}
      onLogout={logout}
      onOpenSettings={() => setShowUserSettings(true)}
      onVoiceChannelClick={(channelId) => {
        setActiveVoiceChannel(channelId);
        selectChannel(channelId); // load messages for the VC chat panel
        if (currentUser) {
          setVoiceParticipants(prev => {
            const list = prev[channelId] ?? [];
            if (list.some(p => p.user_id === currentUser.id)) return prev;
            return { ...prev, [channelId]: [...list, { user_id: currentUser.id, username: currentUser.display_name || currentUser.username, avatar_url: currentUser.avatar_url }] };
          });
        }
      }}
      voiceParticipants={voiceParticipants}
      activeVoiceChannelId={activeVoiceChannel}
      channelUnreadCounts={unreadCounts}
      canManageChannels={canManageChannels}
      canMuteMembers={canMuteMembers}
      canKickFromVoice={canKickFromVoice}
      onRenameChannel={async (channelId, newName) => {
        const ch = channels.find(c => c.id === channelId);
        if (!ch) return;
        const updated = await channelsApi.updateChannel(channelId, newName, ch.topic);
        receiveChannelUpdate(updated);
      }}
      onMarkChannelRead={(channelId) => setUnreadCounts(prev => ({ ...prev, [channelId]: 0 }))}
      onChannelSettings={(channelId) => {
        const ch = channels.find(c => c.id === channelId);
        setChannelSettingsId(channelId);
        setChannelSettingsName(ch?.name ?? '');
        setChannelSettingsParentId(ch?.parent_id);
        setShowChannelSettings(true);
      }}
      isOpen={showChannelList}
      vcConnected={vcConnected}
      vcMuted={vcMuted}
      vcDeafened={vcDeafened}
      vcVideoEnabled={vcVideoEnabled}
      vcScreenSharing={vcScreenSharing}
      onVcMuteToggle={vcToggleMute}
      onVcDeafenToggle={vcToggleDeafen}
      onVcVideoToggle={() => vcToggleVideo().catch(console.error)}
      onVcScreenShareToggle={() => vcToggleScreenShare().catch(console.error)}
      onVcLeave={vcDisconnect}
      onVcNavigate={() => { if (activeVoiceChannel) selectChannel(activeVoiceChannel); }}
      onReorderChannels={reorderChannels}
      onVcParticipantClick={handleVcParticipantClick}
    />
  ) : (
    <DmPanel
      dmChannels={dmChannels}
      activeDmChannelId={activeDmChannel?.id ?? null}
      currentUser={currentUser}
      onSelectDm={selectDmChannel}
      onLogout={logout}
      onOpenSettings={() => setShowUserSettings(true)}
      dmUnreadCounts={unreadCounts}
      onlineUserIds={onlineUsers}
    />
  );

  // VC channel - is the user currently viewing the voice channel page?
  const vcChannel = activeVoiceChannel ? channels.find(c => c.id === activeVoiceChannel) ?? null : null;
  const isViewingVC = !!(vcChannel && activeChannel?.id === activeVoiceChannel && !activeDmChannel);

  // Build right panel
  let rightPanel: React.ReactNode;
  if (view === 'server') {
    if (isViewingVC && vcChatOpen && vcChannel && currentUser) {
      rightPanel = (
        <div className="vc-chat-sidebar">
          <ChatWindow
            channel={vcChannel}
            messages={messages}
            currentUserId={currentUser.id}
            members={members}
            channels={channels}
            memberMap={memberMap}
            channelMap={channelMap}
            onSendMessage={sendMessage}
            onEdit={(msg) => editMessage(msg.id, msg.content)}
            onDelete={deleteMessage}
            onReact={toggleReaction}
            onReply={(msg) => setReplyTo(msg)}
            onViewProfile={handleViewProfile}
            onSendMessageToUser={(userId) => openDmChannel(userId)}
            onLoadMore={loadMoreMessages}
            hasMore={hasMoreMessages}
            isLoading={isLoadingMessages}
            typingUsers={typingUsers[vcChannel.id] ?? []}
            onTyping={handleSendTyping}
            canManageChannels={canManageChannels}
            replyTo={replyTo}
            onClearReply={() => setReplyTo(null)}
            onlineUserIds={onlineUsers}
            onNavigateToChannel={(channelId) => {
              selectChannel(channelId);
            }}
          />
        </div>
      );
    } else {
      rightPanel = (
        <UserSidebar
          members={members}
          ownerId={activeServer?.owner_id}
          currentUserId={currentUser?.id}
          onViewProfile={handleViewProfile}
          onSendMessage={openDmChannel}
          onlineUserIds={onlineUsers}
          currentUserIsOwner={isServerOwner}
          canKickMembers={canKickMembers}
          canBanMembers={canBanMembers}
          onManageRoles={(userId) => {
            const m = members.find(mem => mem.user_id === userId);
            setAssignRolesUserId(userId);
            setAssignRolesUsername(m?.username || userId);
            setShowAssignRoles(true);
          }}
          onKick={activeServer ? (userId) => {
            serversApi.kickMember(activeServer.id, userId).catch(console.error);
          } : undefined}
          onBan={activeServer ? (userId) => {
            serversApi.banMember(activeServer.id, userId).catch(console.error);
          } : undefined}
          isOpen={showMembers}
        />
      );
    }
  }

  // Build main content
  let mainContent: React.ReactNode;
  const AtMeTopbar = ({ friendsActive }: { friendsActive: boolean }) => (
    <div className="at-me-topbar">
      <button
        className={`at-me-topbar-friends-btn${friendsActive ? ' active' : ''}`}
        onClick={friendsActive ? handleGoHome : handleOpenFriends}
      >
        <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
          <path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z"/>
        </svg>
        Friends
        {pendingRequestCount > 0 && (
          <span className="at-me-topbar-friends-badge">{pendingRequestCount > 99 ? '99+' : pendingRequestCount}</span>
        )}
      </button>
    </div>
  );

  if (activeFriendsView) {
    mainContent = (
      <div className="at-me-layout">
        <AtMeTopbar friendsActive={true} />
        <div className="at-me-content">
          <FriendsView
            friends={friends}
            friendRequests={friendRequests}
            onlineUserIds={onlineUsers}
            currentUserId={currentUser?.id ?? ''}
            onMessage={handleFriendMessage}
            onAccept={acceptFriendRequest}
            onDeclineOrCancel={declineOrCancelRequest}
            onRemove={removeFriend}
            onSendRequest={sendFriendRequest}
          />
        </div>
      </div>
    );
  } else if (activeChannel?.type === 2) {
    mainContent = activePostId ? (
      <PostView postId={activePostId} onBack={() => setActivePostId(null)} currentUserId={currentUser?.id} />
    ) : (
      <BinChannel
        channelId={activeChannel.id}
        serverId={activeServer?.id}
        onOpenPost={setActivePostId}
        onNewPost={() => setShowCreatePost(true)}
        currentUserId={currentUser?.id}
      />
    );
  } else if (isViewingVC && vcChannel && currentUser) {
    mainContent = (
      <VoiceChannel
        channel={vcChannel}
        currentUser={{ id: currentUser.id, username: currentUser.display_name || currentUser.username, avatar_url: currentUser.avatar_url }}
        participants={vcParticipants}
        localParticipant={vcLocalParticipant}
        voiceParticipants={Object.fromEntries(
          (voiceParticipants[activeVoiceChannel!] ?? []).map(p => [p.user_id, p])
        )}
        activeSpeakers={vcActiveSpeakers}
        connected={vcConnected}
        connecting={vcConnecting}
        error={vcError}
        muted={vcMuted}
        deafened={vcDeafened}
        videoEnabled={vcVideoEnabled}
        screenSharing={vcScreenSharing}
        onToggleMute={vcToggleMute}
        onToggleDeafen={vcToggleDeafen}
        onToggleVideo={vcToggleVideo}
        onToggleScreenShare={vcToggleScreenShare}
        onLeave={vcDisconnect}
        onRetry={vcRetry}
        canMuteMembers={canMuteMembers}
        canKickFromVoice={canKickFromVoice}
        onMuteParticipant={async (userId) => { try { await muteVoiceParticipant(activeVoiceChannel!, userId); } catch(e) { console.error(e); } }}
        vcChatOpen={vcChatOpen}
        onToggleVcChat={() => setVcChatOpen(v => !v)}
        onParticipantClick={(userId, e) => handleVcParticipantClick(userId, e.clientX, e.clientY)}
      />
    );
  } else if (view === 'homepage') {
    mainContent = (
      <div className="at-me-layout">
        <AtMeTopbar friendsActive={false} />
        <div className="at-me-content">
          <Homepage
            currentUser={currentUser}
            onCreateServer={() => setShowCreateServer(true)}
            onOpenDm={openDmChannel}
          />
        </div>
      </div>
    );
  } else if (view === 'dm' && activeDmChannel) {
    const dmChannel = {
      id: activeDmChannel.id,
      server_id: '',
      name: activeDmChannel.other_display_name || activeDmChannel.other_username,
      type: 0,
      position: 0,
      created_at: activeDmChannel.created_at,
      updated_at: activeDmChannel.created_at,
    };
    const dmAsMessages: Message[] = dmMessages.map(dm => ({
      id: dm.id,
      channel_id: dm.dm_channel_id,
      author_id: dm.author_id,
      author_username: dm.author_username,
      author_display_name: dm.author_display_name,
      author_avatar_url: dm.author_avatar_url,
      content: dm.content,
      created_at: dm.created_at,
      updated_at: dm.updated_at,
      attachment_url: dm.attachment_url,
      attachment_name: dm.attachment_name,
      attachment_type: dm.attachment_type,
      parent_id: dm.parent_id,
      parent_author_username: dm.parent_author_username,
      parent_author_display_name: dm.parent_author_display_name,
      reactions: dm.reactions ?? [],
    }));
    const dmMembers = [
      // Other participant — use what DmChannel provides
      {
        id: activeDmChannel.other_user_id,
        server_id: '',
        user_id: activeDmChannel.other_user_id,
        username: activeDmChannel.other_username,
        display_name: activeDmChannel.other_display_name,
        avatar_url: activeDmChannel.other_avatar_url,
        joined_at: '',
      },
      // Current user — full profile available
      ...(currentUser ? [{
        id: currentUser.id,
        server_id: '',
        user_id: currentUser.id,
        username: currentUser.username,
        display_name: currentUser.display_name,
        avatar_url: currentUser.avatar_url,
        banner_url: currentUser.banner_url,
        bio: currentUser.bio,
        badges: currentUser.badges,
        joined_at: '',
      }] : []),
    ];
    mainContent = (
      <ChatWindow
        channel={dmChannel}
        messages={dmAsMessages}
        currentUserId={currentUser?.id}
        members={dmMembers}
        onSendMessage={sendDmMessage}
        onDelete={handleDmDelete}
        onReact={handleDmReact}
        onReply={(msg) => setReplyTo(msg)}
        onClearReply={() => setReplyTo(null)}
        replyTo={replyTo}
        onLoadMore={loadMoreDmMessages}
        hasMore={hasMoreDmMessages}
        isLoading={isLoadingDms}
        onViewProfile={handleViewProfile}
        headerPrefix="@"
        headerAvatar={activeDmChannel.other_avatar_url}
        isOnline={onlineUsers.has(activeDmChannel.other_user_id)}
        onlineUserIds={onlineUsers}
        hideRoles={true}
      />
    );
  } else if (activeChannel) {
    mainContent = (
      <ChatWindow
        channel={activeChannel}
        messages={messages}
        currentUserId={currentUser?.id}
        members={members}
        channels={channels}
        memberMap={memberMap}
        channelMap={channelMap}
        onSendMessage={sendMessage}
        onEdit={(msg) => editMessage(msg.id, msg.content)}
        onDelete={deleteMessage}
        onReact={toggleReaction}
        onReply={(msg) => setReplyTo(msg)}
        onViewProfile={handleViewProfile}
        onSendMessageToUser={(userId) => openDmChannel(userId)}
        onLoadMore={loadMoreMessages}
        hasMore={hasMoreMessages}
        isLoading={isLoadingMessages}
        typingUsers={typingUsers[activeChannel.id] ?? []}
        onTyping={handleSendTyping}
        canManageChannels={canManageChannels}
        replyTo={replyTo}
        onClearReply={() => setReplyTo(null)}
        onNavigateToChannel={(channelId) => {
          selectChannel(channelId);
        }}
        showMembers={showMembers}
        onToggleMembers={() => setShowMembers(m => !m)}
        onToggleChannelList={() => setShowChannelList(c => !c)}
        onUpdateTopic={async (channelId, topic) => {
          const updated = await channelsApi.updateChannel(channelId, activeChannel.name, topic);
          receiveChannelUpdate(updated);
        }}
        onlineUserIds={onlineUsers}
      />
    );
  } else if (activeServer) {
    mainContent = (
      <div className="no-channel-selected">
        <p>Select a channel to start chatting</p>
      </div>
    );
  } else {
    mainContent = null;
  }

  if (isLoadingServers) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh', background: 'var(--parley-bg-primary)', color: 'var(--parley-accent)' }}>
        Loading...
      </div>
    );
  }

  return (
    <>
      <MainLayout
        servers={servers}
        activeServerId={activeServer?.id ?? null}
        currentUserId={currentUser?.id}
        onServerSelect={selectServer}
        onCreateServer={() => setShowCreateServer(true)}
        onHomepage={handleGoHome}
        leftPanel={leftPanel}
        rightPanel={rightPanel}
        serverUnreadCounts={serverUnreadCounts}
        onMarkServerRead={(serverId) => {
          setUnreadCounts(prev => {
            const next = { ...prev };
            channels.forEach(ch => { if (ch.server_id === serverId) delete next[ch.id]; });
            return next;
          });
        }}
        onNotificationSettings={(serverId) => {
          setNotifSettingsServerId(serverId);
          setShowNotifSettings(true);
        }}
        onServerSettings={(serverId) => {
          selectServer(serverId);
          setServerSettingsInitialTab('overview');
          setShowServerSettings(true);
        }}
        onLeaveServer={(serverId) => leaveServer(serverId)}
      >
        {mainContent}
        {/* Mobile backdrop — closes drawers when tapping outside */}
        {(showChannelList || showMembers) && (
          <div
            className="mobile-backdrop"
            onClick={() => { setShowChannelList(false); setShowMembers(false); }}
          />
        )}
      </MainLayout>


      <CreateServerModal
        isOpen={showCreateServer}
        onClose={() => setShowCreateServer(false)}
        onCreate={createServer}
      />
      <CreateChannelModal
        isOpen={showCreateChannel}
        onClose={() => setShowCreateChannel(false)}
        onCreate={createChannel}
      />
      <UserProfileModal
        isOpen={showProfile}
        onClose={() => setShowProfile(false)}
        userId={profileUserId}
        currentUserId={currentUser?.id}
        onStartDm={openDmChannel}
        isOnline={profileUserId ? onlineUsers.has(profileUserId) : undefined}
      />
      {vcMiniProfile && (
        <MiniProfile
          member={vcMiniProfile.member}
          isCurrentUser={vcMiniProfile.member.user_id === currentUser?.id}
          isOnline={onlineUsers.has(vcMiniProfile.member.user_id)}
          position={vcMiniProfile.position}
          onClose={() => setVcMiniProfile(null)}
          onSendMessage={openDmChannel}
          onViewProfile={(userId) => { setVcMiniProfile(null); handleViewProfile(userId); }}
        />
      )}
      <AssignRolesModal
        isOpen={showAssignRoles}
        onClose={() => setShowAssignRoles(false)}
        serverId={activeServer?.id ?? ''}
        userId={assignRolesUserId}
        username={assignRolesUsername}
      />
      <UserSettings
        isOpen={showUserSettings}
        onClose={() => setShowUserSettings(false)}
        currentUser={currentUser}
        onUpdate={updateCurrentUser}
      />
      <ServerSettings
        isOpen={showServerSettings}
        onClose={() => setShowServerSettings(false)}
        server={activeServer}
        members={members}
        onUpdate={updateServer}
        onDelete={() => deleteServer(activeServer?.id ?? '')}
        onCreateInvite={() => {}}
        initialTab={serverSettingsInitialTab}
      />

      <NotificationSettingsModal
        isOpen={showNotifSettings}
        onClose={() => setShowNotifSettings(false)}
        serverId={notifSettingsServerId}
        serverName={servers.find(s => s.id === notifSettingsServerId)?.name ?? ''}
      />

      {showChannelSettings && activeServer && (
        <ChannelSettingsModal
          isOpen={showChannelSettings}
          onClose={() => setShowChannelSettings(false)}
          channelId={channelSettingsId}
          channelName={channelSettingsName}
          serverId={activeServer.id}
          parentId={channelSettingsParentId}
        />
      )}

      {showCreatePost && activeChannel?.type === 2 && (
        <CreatePostModal
          isOpen={showCreatePost}
          channelId={activeChannel.id}
          availableTags={binTags}
          onClose={() => setShowCreatePost(false)}
          onCreated={(postId) => {
            setShowCreatePost(false);
            setActivePostId(postId);
          }}
        />
      )}

    </>
  );
}

const ProtectedRoute: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const token = localStorage.getItem('token');
  if (!token) return <Navigate to="/login" replace />;
  return <>{children}</>;
};

const AuthRoute: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const token = localStorage.getItem('token');
  if (token) return <Navigate to="/" replace />;
  return <>{children}</>;
};

const ProtectedApp = (
  <ProtectedRoute>
    <ThemeProvider>
      <AppProvider>
        <ErrorBoundary>
          <MainApp />
        </ErrorBoundary>
      </AppProvider>
    </ThemeProvider>
  </ProtectedRoute>
);

const HomeRoute: React.FC = () => {
  const token = localStorage.getItem('token');
  if (!token) return <Landing />;
  return <Navigate to="/channels/@me" replace />;
};

function App() {
  return (
    <Routes>
      <Route path="/login" element={<AuthRoute><Login /></AuthRoute>} />
      <Route path="/register" element={<AuthRoute><Register /></AuthRoute>} />
      <Route
        path="/invite/:code"
        element={
          <ProtectedRoute>
            <ThemeProvider>
              <AppProvider>
                <InvitePage />
              </AppProvider>
            </ThemeProvider>
          </ProtectedRoute>
        }
      />
      <Route path="/verify-email" element={<VerifyEmail />} />
      <Route path="/forgot-password" element={<ForgotPassword />} />
      <Route path="/reset-password" element={<ResetPassword />} />
      <Route path="/impersonate" element={<Impersonate />} />
      {/* Channel routes — all handled by MainApp which syncs URL with state */}
      <Route path="/" element={<HomeRoute />} />
      <Route path="/channels/*" element={ProtectedApp} />
      <Route path="/theme/:token" element={<ThemeProvider><SharedThemePage /></ThemeProvider>} />
      <Route path="/themes" element={<ThemeProvider><ThemeRepoPage /></ThemeProvider>} />
      <Route path="/bots/invite/:token" element={<BotInvitePage />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

export default App;
