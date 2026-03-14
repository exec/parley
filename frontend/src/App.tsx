import { Routes, Route, Navigate } from 'react-router-dom';
import { useState } from 'react';
import { Login } from './pages/Login';
import { Register } from './pages/Register';
import { AppProvider, useApp } from './context/AppContext';
import { useWebSocket } from './hooks/useWebSocket';
import MainLayout from './components/layout/MainLayout';
import { ChatWindow } from './components/chat/ChatWindow';
import { CreateServerModal } from './components/modals/CreateServerModal';
import { CreateChannelModal } from './components/modals/CreateChannelModal';
import { ManageRolesModal } from './components/modals/ManageRolesModal';
import { DmChat } from './components/chat/DmChat';
import { UserProfileModal } from './components/modals/UserProfileModal';
import { Homepage } from './pages/Homepage';

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
    createChannel,
    deleteChannel,
    sendMessage,
    receiveMessage,
    receiveDmMessage,
    logout,
    // DM
    dmChannels,
    activeDmChannel,
    dmMessages,
    isLoadingDms,
    selectDmChannel,
    sendDmMessage,
    openDmChannel,
  } = useApp();

  const [showCreateServer, setShowCreateServer] = useState(false);
  const [showCreateChannel, setShowCreateChannel] = useState(false);
  const [showManageRoles, setShowManageRoles] = useState(false);
  const [showVoiceModal, setShowVoiceModal] = useState(false);
  const [showProfile, setShowProfile] = useState(false);
  const [profileUserId, setProfileUserId] = useState<string | null>(null);

  const [showHomepage, setShowHomepage] = useState(false);

  useWebSocket({
    onMessage: receiveMessage,
    onDmMessage: receiveDmMessage,
    activeChannelId: activeChannel?.id ?? null,
  });

  const handleViewProfile = (userId: string) => {
    setProfileUserId(userId);
    setShowProfile(true);
  };

  if (showHomepage || (!isLoadingServers && servers.length === 0 && dmChannels.length === 0)) {
    return (
      <>
        <Homepage
          onCreateServer={() => setShowCreateServer(true)}
          dmChannels={dmChannels}
          onSelectDm={selectDmChannel}
          onOpenDm={openDmChannel}
          currentUserId={currentUser?.id}
        />
        <CreateServerModal
          isOpen={showCreateServer}
          onClose={() => setShowCreateServer(false)}
          onCreate={createServer}
        />
      </>
    );
  }

  // Show DM chat if a DM channel is active
  if (activeDmChannel) {
    return (
      <>
        <MainLayout
          servers={servers}
          activeServerId={null}
          onServerSelect={selectServer}
          onCreateServer={() => setShowCreateServer(true)}
          channels={channels}
          activeChannelId={null}
          onChannelSelect={selectChannel}
          onCreateChannel={() => setShowCreateChannel(true)}
          onDeleteChannel={deleteChannel}
          onManageRoles={() => setShowManageRoles(true)}
          serverName={`@${activeDmChannel.other_username}`}
          members={[]}
          currentUser={currentUser ?? undefined}
          ownerId={currentUser?.id}
          onLogout={logout}
          onVoiceChannelClick={() => setShowVoiceModal(true)}
          onHomepage={() => setShowHomepage(true)}
          onViewProfile={handleViewProfile}
        >
          <DmChat
            channel={activeDmChannel}
            messages={dmMessages}
            currentUserId={currentUser?.id}
            onSendMessage={sendDmMessage}
            isLoading={isLoadingDms}
          />
        </MainLayout>

        {showVoiceModal && (
          <div className="modal-overlay" onClick={() => setShowVoiceModal(false)}>
            <div className="modal-content" onClick={e => e.stopPropagation()}>
              <div className="modal-header">
                <h2 className="modal-title">Voice Channels</h2>
                <button className="modal-close" onClick={() => setShowVoiceModal(false)}>&times;</button>
              </div>
              <div className="modal-body">
                <p style={{color: 'var(--discord-text-muted)', textAlign: 'center', padding: '20px 0'}}>
                  Voice channels are coming soon!<br/>
                  <span style={{fontSize: '13px', marginTop: '8px', display: 'block'}}>
                    We're working on it. Stay tuned.
                  </span>
                </p>
              </div>
            </div>
          </div>
        )}

        <UserProfileModal
          isOpen={showProfile}
          onClose={() => setShowProfile(false)}
          userId={profileUserId}
          onStartDm={openDmChannel}
        />
      </>
    );
  }

  return (
    <>
      <MainLayout
        servers={servers}
        activeServerId={activeServer?.id ?? null}
        onServerSelect={selectServer}
        onCreateServer={() => setShowCreateServer(true)}
        channels={channels}
        activeChannelId={activeChannel?.id ?? null}
        onChannelSelect={selectChannel}
        onCreateChannel={() => setShowCreateChannel(true)}
        onDeleteChannel={deleteChannel}
        onManageRoles={() => setShowManageRoles(true)}
        serverName={activeServer?.name ?? ''}
        members={members}
        currentUser={currentUser ?? undefined}
        ownerId={activeServer?.owner_id}
        onLogout={logout}
        onVoiceChannelClick={() => setShowVoiceModal(true)}
        onHomepage={() => setShowHomepage(true)}
        onViewProfile={handleViewProfile}
      >
        {activeChannel ? (
          <ChatWindow
            channel={activeChannel}
            messages={messages}
            currentUserId={currentUser?.id}
            onSendMessage={sendMessage}
            isLoading={isLoadingMessages}
          />
        ) : activeServer ? (
          <div className="no-channel-selected">
            <p>Select a channel to start chatting</p>
          </div>
        ) : (
          <div className="no-channel-selected">
            <p>Select a channel or start a DM</p>
          </div>
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
      <ManageRolesModal
        isOpen={showManageRoles}
        onClose={() => setShowManageRoles(false)}
        members={members}
      />
      {showVoiceModal && (
        <div className="modal-overlay" onClick={() => setShowVoiceModal(false)}>
          <div className="modal-content" onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h2 className="modal-title">Voice Channels</h2>
              <button className="modal-close" onClick={() => setShowVoiceModal(false)}>&times;</button>
            </div>
            <div className="modal-body">
              <p style={{color: 'var(--discord-text-muted)', textAlign: 'center', padding: '20px 0'}}>
                Voice channels are coming soon!<br/>
                <span style={{fontSize: '13px', marginTop: '8px', display: 'block'}}>
                  We're working on it. Stay tuned.
                </span>
              </p>
            </div>
          </div>
        </div>
      )}

      <UserProfileModal
        isOpen={showProfile}
        onClose={() => setShowProfile(false)}
        userId={profileUserId}
        onStartDm={openDmChannel}
      />
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

function App() {
  return (
    <Routes>
      <Route path="/login" element={<AuthRoute><Login /></AuthRoute>} />
      <Route path="/register" element={<AuthRoute><Register /></AuthRoute>} />
      <Route
        path="/"
        element={
          <ProtectedRoute>
            <AppProvider>
              <MainApp />
            </AppProvider>
          </ProtectedRoute>
        }
      />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

export default App;