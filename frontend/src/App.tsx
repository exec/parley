import { Routes, Route, Navigate, useNavigate, useLocation } from 'react-router-dom';
import { useState, useEffect, useRef, useCallback } from 'react';
import { Login } from './pages/Login';
import { Register } from './pages/Register';
import { InvitePage } from './pages/InvitePage';
import { AppProvider, useApp } from './context/AppContext';
import { useWebSocket } from './hooks/useWebSocket';
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
  const [showVoiceModal, setShowVoiceModal] = useState(false);
  const [showProfile, setShowProfile] = useState(false);
  const [profileUserId, setProfileUserId] = useState<string | null>(null);
  const [showServerSettings, setShowServerSettings] = useState(false);
  const [showUserSettings, setShowUserSettings] = useState(false);

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
      selectServer(serverId).then(() => {
        if (channelId) selectChannel(channelId);
      });
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

  const handleServerMemberJoin = useCallback((_serverId: string, _userId: string) => {
    // When a member joins, reload servers to show the new server for the joining user
    loadServers();
  }, [loadServers]);

  // Get all channel IDs from all servers to subscribe to for notifications
  const allChannelIds = servers.flatMap(server =>
    channels.filter(c => c.server_id === server.id).map(c => c.id)
  );

  useWebSocket({
    onMessage: receiveMessage,
    onDmMessage: receiveDmMessage,
    onServerMemberJoin: handleServerMemberJoin,
    activeChannelId: activeChannel?.id ?? null,
    extraChannelIds: allChannelIds,
  });

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
    />
  ) : (
    <DmPanel
      dmChannels={dmChannels}
      activeDmChannelId={activeDmChannel?.id ?? null}
      currentUser={currentUser}
      onSelectDm={selectDmChannel}
      onLogout={logout}
      onOpenSettings={() => setShowUserSettings(true)}
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
        onViewProfile={handleViewProfile}
        onSendMessageToUser={(userId) => openDmChannel(userId)}
        isLoading={isLoadingMessages}
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
        onClose={() => setShowManageRoles(false)}
        members={members}
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
      {/* Channel routes — all handled by MainApp which syncs URL with state */}
      <Route path="/" element={ProtectedApp} />
      <Route path="/channels/*" element={ProtectedApp} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

export default App;
