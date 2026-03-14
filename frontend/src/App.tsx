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

function MainApp() {
  const {
    currentUser,
    servers,
    activeServer,
    channels,
    activeChannel,
    messages,
    members,
    isLoadingMessages,
    selectServer,
    selectChannel,
    createServer,
    createChannel,
    deleteChannel,
    sendMessage,
    receiveMessage,
    logout,
  } = useApp();

  const [showCreateServer, setShowCreateServer] = useState(false);
  const [showCreateChannel, setShowCreateChannel] = useState(false);
  const [showManageRoles, setShowManageRoles] = useState(false);

  useWebSocket({
    onMessage: receiveMessage,
    activeChannelId: activeChannel?.id ?? null,
  });

  return (
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
      ) : null}

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
    </MainLayout>
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
