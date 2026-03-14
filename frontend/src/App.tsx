import { Routes, Route, Navigate, useNavigate, useLocation } from 'react-router-dom';
import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { Login } from './pages/Login';
import { Register } from './pages/Register';
import { InvitePage } from './pages/InvitePage';
import { VerifyEmail } from './pages/VerifyEmail';
import { Impersonate } from './pages/Impersonate';
import { AppProvider, useApp } from './context/AppContext';
import { useWebSocket } from './hooks/useWebSocket';
import { DmMessage } from './api/types';
import MainLayout from './components/layout/MainLayout';
import ChannelList from './components/layout/ChannelList';
import DmPanel from './components/layout/DmPanel';
import UserSidebar from './components/layout/UserSidebar';
import { ChatWindow } from './components/chat/ChatWindow';
import { DmChat } from './components/chat/DmChat';
import { Homepage } from './pages/Homepage';
import { CreateServerModal } from './components/modals/CreateServerModal';
import { CreateChannelModal } from './components/modals/CreateChannelModal';
import { ManageRolesModal } from './components/modals/ManageRolesModal';
import { UserProfileModal } from './components/modals/UserProfileModal';
import { ServerSettingsModal } from './components/modals/ServerSettingsModal';
import { UserSettingsModal } from './components/modals/UserSettingsModal';
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
    logout,
    dmChannels,
    activeDmChannel,
    dmMessages,
    isLoadingDms,
    selectDmChannel,
    sendDmMessage,
    openDmChannel,
    updateCurrentUser,
    loadServers,
  } = useApp();

  const navigate = useNavigate();
  const location = useLocation();
  const didRestoreFromUrl = useRef(false);

  const [showCreateServer, setShowCreateServer] = useState(false);
  const [showCreateChannel, setShowCreateChannel] = useState(false);
  const [showManageRoles, setShowManageRoles] = useState(false);
  const [manageRolesFocusUserId, setManageRolesFocusUserId] = useState<string | undefined>(undefined);
  const [showVoiceModal, setShowVoiceModal] = useState(false);
  const [showProfile, setShowProfile] = useState(false);
  const [profileUserId, setProfileUserId] = useState<string | null>(null);
  const [showServerSettings, setShowServerSettings] = useState(false);
  const [showUserSettings, setShowUserSettings] = useState(false);

  // Typing indicators: channelId → list of typing users
  const [typingUsers, setTypingUsers] = useState<Record<string, { userId: string; username: string }[]>>({});
  const typingTimeoutsRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());
  const lastTypingSentRef = useRef<number>(0);

  // Unread counts: channelId (or dmChannelId) → unread message count
  const [unreadCounts, setUnreadCounts] = useState<Record<string, number>>({});

  // Online presence: set of user IDs currently connected via WebSocket
  const [onlineUsers, setOnlineUsers] = useState<Set<string>>(new Set());

  // Determine current view
  const view: View = activeDmChannel ? 'dm' : activeServer ? 'server' : 'homepage';

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
      navigate('/', { replace: true });
    }
  }, [activeDmChannel?.id, activeServer?.id, activeChannel?.id]); // eslint-disable-line react-hooks/exhaustive-deps

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

  const handleServerMemberJoin = useCallback((_serverId: string, _userId: string) => {
    // When a member joins, reload servers to show the new server for the joining user
    loadServers();
  }, [loadServers]);

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
    // Clear typing indicator when a message arrives from that user
    clearTypingUser(msg.channel_id, msg.author_id);
    receiveMessage(msg);
    // Track unread for channels we're not currently viewing
    if (msg.channel_id !== activeChannel?.id) {
      setUnreadCounts(prev => ({ ...prev, [msg.channel_id]: (prev[msg.channel_id] ?? 0) + 1 }));
    }
  }, [receiveMessage, clearTypingUser, activeChannel?.id]);

  const handleReceiveDmMessage = useCallback((msg: DmMessage) => {
    receiveDmMessage(msg);
    if (msg.dm_channel_id !== activeDmChannel?.id) {
      setUnreadCounts(prev => ({ ...prev, [msg.dm_channel_id]: (prev[msg.dm_channel_id] ?? 0) + 1 }));
    }
  }, [receiveDmMessage, activeDmChannel?.id]);

  // Get all channel IDs from all servers to subscribe to for notifications
  const allChannelIds = servers.flatMap(server =>
    channels.filter(c => c.server_id === server.id).map(c => c.id)
  );

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
    setOnlineUsers(prev => {
      const next = new Set(prev);
      userIds.forEach(id => next.add(id));
      return next;
    });
  }, []);

  const { sendTyping } = useWebSocket({
    onMessage: handleReceiveMessage,
    onDmMessage: handleReceiveDmMessage,
    onServerMemberJoin: handleServerMemberJoin,
    onTyping: handleTyping,
    onUserOnline: handleUserOnline,
    onUserOffline: handleUserOffline,
    onPresenceSnapshot: handlePresenceSnapshot,
    onMessageUpdate: receiveMessageUpdate,
    onMessageDelete: receiveMessageDelete,
    onReactionUpdate: applyReactionUpdate,
    activeChannelId: activeChannel?.id ?? null,
    extraChannelIds: allChannelIds,
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

  const handleGoHome = () => {
    selectServer('__none__');
  };

  // Build left panel based on view
  const leftPanel = view === 'server' ? (
    <ChannelList
      serverName={activeServer?.name ?? ''}
      channels={channels}
      activeChannelId={activeChannel?.id ?? null}
      onChannelSelect={selectChannel}
      onCreateChannel={() => setShowCreateChannel(true)}
      onDeleteChannel={deleteChannel}
      onManageRoles={() => setShowManageRoles(true)}
      onServerSettings={() => setShowServerSettings(true)}
      onLeaveServer={() => leaveServer(activeServer?.id ?? '')}
      owner_id={activeServer?.owner_id}
      currentUser={currentUser ?? undefined}
      onLogout={logout}
      onOpenSettings={() => setShowUserSettings(true)}
      onVoiceChannelClick={() => setShowVoiceModal(true)}
      channelUnreadCounts={unreadCounts}
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
    />
  );

  // Build right panel
  const rightPanel = view === 'server' ? (
    <UserSidebar
      members={members}
      ownerId={activeServer?.owner_id}
      currentUserId={currentUser?.id}
      onViewProfile={handleViewProfile}
      onSendMessage={openDmChannel}
      onlineUserIds={onlineUsers}
      currentUserIsOwner={currentUser?.id === activeServer?.owner_id}
      onManageRoles={(userId) => {
        setManageRolesFocusUserId(userId);
        setShowManageRoles(true);
      }}
    />
  ) : undefined;

  // Build main content
  let mainContent: React.ReactNode;
  if (view === 'homepage') {
    mainContent = (
      <Homepage
        currentUser={currentUser}
        onCreateServer={() => setShowCreateServer(true)}
        onOpenDm={openDmChannel}
      />
    );
  } else if (view === 'dm') {
    mainContent = (
      <DmChat
        channel={activeDmChannel!}
        messages={dmMessages}
        currentUserId={currentUser?.id}
        onSendMessage={sendDmMessage}
        isLoading={isLoadingDms}
      />
    );
  } else if (activeChannel) {
    mainContent = (
      <ChatWindow
        channel={activeChannel}
        messages={messages}
        currentUserId={currentUser?.id}
        onSendMessage={sendMessage}
        onEdit={(msg) => editMessage(msg.id, msg.content)}
        onDelete={deleteMessage}
        onReact={toggleReaction}
        onViewProfile={handleViewProfile}
        onSendMessageToUser={(userId) => openDmChannel(userId)}
        isLoading={isLoadingMessages}
        typingUsers={typingUsers[activeChannel.id] ?? []}
        onTyping={handleSendTyping}
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
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh', background: '#000', color: '#32CD32' }}>
        Loading...
      </div>
    );
  }

  return (
    <>
      <MainLayout
        servers={servers}
        activeServerId={activeServer?.id ?? null}
        onServerSelect={selectServer}
        onCreateServer={() => setShowCreateServer(true)}
        onHomepage={handleGoHome}
        leftPanel={leftPanel}
        rightPanel={rightPanel}
        serverUnreadCounts={serverUnreadCounts}
      >
        {mainContent}
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
      <ManageRolesModal
        isOpen={showManageRoles}
        onClose={() => { setShowManageRoles(false); setManageRolesFocusUserId(undefined); }}
        serverId={activeServer?.id ?? ''}
        members={members}
        focusUserId={manageRolesFocusUserId}
      />
      <UserProfileModal
        isOpen={showProfile}
        onClose={() => setShowProfile(false)}
        userId={profileUserId}
        currentUserId={currentUser?.id}
        onStartDm={openDmChannel}
      />
      <ServerSettingsModal
        isOpen={showServerSettings}
        onClose={() => setShowServerSettings(false)}
        server={activeServer}
        onUpdate={updateServer}
        onDelete={() => deleteServer(activeServer?.id ?? '')}
        onCreateInvite={() => {}}
      />

      <UserSettingsModal
        isOpen={showUserSettings}
        onClose={() => setShowUserSettings(false)}
        currentUser={currentUser}
        onUpdate={updateCurrentUser}
      />

      {showVoiceModal && (
        <div className="modal-overlay" onClick={() => setShowVoiceModal(false)}>
          <div className="modal-content" onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h2 className="modal-title">Voice Channels</h2>
              <button className="modal-close" onClick={() => setShowVoiceModal(false)}>&times;</button>
            </div>
            <div className="modal-body">
              <p style={{ color: 'var(--text-secondary)', textAlign: 'center', padding: '20px 0' }}>
                Voice channels are coming soon!
              </p>
            </div>
          </div>
        </div>
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
    <AppProvider>
      <ErrorBoundary>
        <MainApp />
      </ErrorBoundary>
    </AppProvider>
  </ProtectedRoute>
);

function App() {
  return (
    <Routes>
      <Route path="/login" element={<AuthRoute><Login /></AuthRoute>} />
      <Route path="/register" element={<AuthRoute><Register /></AuthRoute>} />
      <Route
        path="/invite/:code"
        element={
          <ProtectedRoute>
            <AppProvider>
              <InvitePage />
            </AppProvider>
          </ProtectedRoute>
        }
      />
      <Route path="/verify-email" element={<VerifyEmail />} />
      <Route path="/impersonate" element={<Impersonate />} />
      {/* Channel routes — all handled by MainApp which syncs URL with state */}
      <Route path="/" element={ProtectedApp} />
      <Route path="/channels/*" element={ProtectedApp} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

export default App;
