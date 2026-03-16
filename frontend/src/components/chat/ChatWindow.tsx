import React, { useCallback, useState, useRef, useEffect } from 'react';
import { Channel, Message as MessageType, ServerMember } from '../../api/types';
import { MessageList } from './MessageList';
import { MessageInput } from './MessageInput';
import { TypingIndicator } from './TypingIndicator';
import { usePermissions } from '../../hooks/usePermissions';
import { PERM_MANAGE_MESSAGES, PERM_ADD_REACTIONS } from '../../lib/permissions';
import MiniProfile from '../layout/MiniProfile';
import { SearchPanel } from '../search/SearchPanel';
import './Chat.css';

interface TypingUser {
  userId: string;
  username: string;
}

interface ChatWindowProps {
  channel: Channel;
  messages: MessageType[];
  currentUserId?: string;
  members?: ServerMember[];
  channels?: Channel[];
  memberMap?: Map<string, string>;
  channelMap?: Map<string, string>;
  onSendMessage: (content: string, attachmentUrl?: string, attachmentName?: string, attachmentType?: string, parentId?: string) => void;
  onLoadMore?: () => void;
  onEdit?: (message: MessageType) => void;
  onDelete?: (messageId: string) => void;
  onReact?: (messageId: string, emoji: string) => void;
  onReply?: (message: MessageType) => void;
  onViewProfile?: (userId: string, username: string) => void;
  onSendMessageToUser?: (userId: string) => void;
  hasMore?: boolean;
  isLoading?: boolean;
  replyTo?: MessageType | null;
  onClearReply?: () => void;
  typingUsers?: TypingUser[];
  onTyping?: () => void;
  canManageChannels?: boolean;
  onUpdateTopic?: (channelId: string, topic: string) => void;
  onNavigateToChannel?: (channelId: string) => void;
  showMembers?: boolean;
  onToggleMembers?: () => void;
  onToggleChannelList?: () => void;
  headerPrefix?: string;
  headerAvatar?: string;
  isOnline?: boolean;
  onlineUserIds?: Set<string>;
}

export const ChatWindow: React.FC<ChatWindowProps> = ({
  channel,
  messages,
  currentUserId,
  members,
  channels = [],
  memberMap,
  channelMap,
  onSendMessage,
  onLoadMore,
  onEdit,
  onDelete,
  onReact,
  onReply,
  onViewProfile,
  onSendMessageToUser,
  hasMore = false,
  isLoading = false,
  replyTo,
  onClearReply,
  typingUsers = [],
  onTyping,
  canManageChannels = false,
  onUpdateTopic,
  onNavigateToChannel,
  showMembers = true,
  onToggleMembers,
  onToggleChannelList,
  headerPrefix = '#',
  headerAvatar,
  isOnline,
  onlineUserIds,
}) => {
  const { hasPerm: checkPerm } = usePermissions(channel.server_id || undefined, channel.id || undefined);
  const canManageMessages = !channel.server_id || checkPerm(PERM_MANAGE_MESSAGES);
  const canAddReactions = !channel.server_id || checkPerm(PERM_ADD_REACTIONS);

  const [miniProfile, setMiniProfile] = useState<{
    member: ServerMember;
    position: { top: number; left: number };
  } | null>(null);

  const handleMiniProfile = useCallback((userId: string, e: React.MouseEvent) => {
    let member = members?.find(m => m.user_id === userId);
    if (!member) {
      // DM / no-members context: build a minimal member from the message itself
      const msg = messages.find(m => m.author_id === userId);
      if (!msg) return;
      member = {
        id: userId,
        server_id: '',
        user_id: userId,
        username: msg.author_username,
        display_name: msg.author_display_name,
        avatar_url: msg.author_avatar_url,
        joined_at: '',
      } as ServerMember;
    }
    const x = e.clientX;
    const y = e.clientY;
    const left = Math.min(x + 10, window.innerWidth - 290);
    const top = Math.min(y, window.innerHeight - 330);
    setMiniProfile({ member, position: { top, left } });
  }, [members, messages]);

  const [showSearch, setShowSearch] = useState(false);

  const replyValue = replyTo ? `@${replyTo.author_username} ` : '';
  const [editingTopic, setEditingTopic] = useState(false);
  const [topicValue, setTopicValue] = useState(channel.topic ?? '');
  const topicInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setTopicValue(channel.topic ?? '');
  }, [channel.topic, channel.id]);

  useEffect(() => {
    if (editingTopic) topicInputRef.current?.focus();
  }, [editingTopic]);

  const startEditTopic = () => {
    if (!canManageChannels) return;
    setTopicValue(channel.topic ?? '');
    setEditingTopic(true);
  };

  const commitTopic = () => {
    setEditingTopic(false);
    onUpdateTopic?.(channel.id, topicValue.trim());
  };

  const handleTopicKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') { e.preventDefault(); commitTopic(); }
    if (e.key === 'Escape') { setEditingTopic(false); setTopicValue(channel.topic ?? ''); }
  };

  const handleSendMessage = useCallback(
    (content: string, attachmentUrl?: string, attachmentName?: string, attachmentType?: string, parentId?: string) => {
      onSendMessage(content, attachmentUrl, attachmentName, attachmentType, parentId);
      if (onClearReply) {
        onClearReply();
      }
    },
    [onSendMessage, onClearReply]
  );

  return (
    <div className="chat-window-outer" style={{ display: 'flex', flex: 1, overflow: 'hidden', minWidth: 0 }}>
    <div className="chat-window" style={{ flex: 1, minWidth: 0 }}>
      <div
        className={`chat-header${canManageChannels ? ' chat-header-editable' : ''}`}
        onClick={startEditTopic}
        title={canManageChannels ? 'Click to edit topic' : undefined}
      >
        {headerAvatar ? (
          <div className="chat-header-avatar">
            <img src={headerAvatar} alt={channel.name} />
          </div>
        ) : null}
        <div className="chat-header-text">
        <div className="chat-header-name-row">
          <span className="chat-header-name">{headerPrefix} {channel.name}</span>
          {isOnline !== undefined && (
            <span className={`chat-header-status-dot ${isOnline ? 'online' : 'offline'}`} title={isOnline ? 'Online' : 'Offline'} />
          )}
          {channel.server_id && (
            <div className="chat-header-actions" onClick={e => e.stopPropagation()}>
              {/* Hamburger — mobile only, toggles channel list */}
              {onToggleChannelList && (
                <button className="chat-header-btn chat-header-btn--hamburger" onClick={onToggleChannelList} title="Toggle channel list">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <line x1="3" y1="6" x2="21" y2="6" /><line x1="3" y1="12" x2="21" y2="12" /><line x1="3" y1="18" x2="21" y2="18" />
                  </svg>
                </button>
              )}
              {/* Members toggle */}
              {onToggleMembers && (
                <button className="chat-header-btn" onClick={onToggleMembers} title={showMembers ? 'Hide members' : 'Show members'} style={{ color: showMembers ? '#32CD32' : '#555' }}>
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
                    <circle cx="9" cy="7" r="4" />
                    <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
                    <path d="M16 3.13a4 4 0 0 1 0 7.75" />
                  </svg>
                </button>
              )}
              {/* Search */}
              <button className="chat-header-btn" onClick={() => setShowSearch(s => !s)} title="Search messages" style={{ color: showSearch ? '#32CD32' : '#555' }}>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <circle cx="11" cy="11" r="8" />
                  <line x1="21" y1="21" x2="16.65" y2="16.65" />
                </svg>
              </button>
            </div>
          )}
        </div>
        {editingTopic ? (
          <input
            ref={topicInputRef}
            className="channel-topic-input"
            value={topicValue}
            onChange={e => setTopicValue(e.target.value)}
            onBlur={commitTopic}
            onKeyDown={handleTopicKeyDown}
            placeholder="Set a topic..."
            onClick={e => e.stopPropagation()}
          />
        ) : (
          <div className="channel-topic">
            {channel.topic || (canManageChannels ? <span className="channel-topic-placeholder">Add a topic...</span> : null)}
          </div>
        )}
        </div>
      </div>
      <MessageList
        messages={messages}
        currentUserId={currentUserId}
        memberMap={memberMap}
        channelMap={channelMap}
        onLoadMore={onLoadMore}
        onEdit={onEdit}
        onDelete={onDelete}
        onReact={onReact}
        onReply={onReply}
        onViewProfile={onViewProfile}
        onSendMessage={onSendMessageToUser}
        onMiniProfile={handleMiniProfile}
        hasMore={hasMore}
        isLoading={isLoading}
        allMessages={messages}
        canManageMessages={canManageMessages}
        canAddReactions={canAddReactions}
      />
      <TypingIndicator typingUsers={typingUsers} />
      <MessageInput
        channelName={channel.name}
        onSendMessage={handleSendMessage}
        onTyping={onTyping}
        initialValue={replyValue}
        members={members}
        channels={channels}
        replyTo={replyTo}
        serverId={channel.server_id || undefined}
        channelId={channel.id || undefined}
      />
      {miniProfile && (
        <MiniProfile
          member={miniProfile.member}
          isCurrentUser={miniProfile.member.user_id === currentUserId}
          isOnline={onlineUserIds?.has(miniProfile.member.user_id) ?? false}
          position={miniProfile.position}
          onClose={() => setMiniProfile(null)}
          onSendMessage={(uid) => { onSendMessageToUser?.(uid); setMiniProfile(null); }}
          onViewProfile={(uid) => {
            onViewProfile?.(uid, miniProfile.member.username);
            setMiniProfile(null);
          }}
          canManageRoles={false}
        />
      )}
    </div>
    {showSearch && channel.server_id && (
      <SearchPanel
        serverId={channel.server_id}
        members={members ?? []}
        channels={channels}
        memberMap={memberMap ?? new Map()}
        channelMap={channelMap ?? new Map()}
        onClose={() => setShowSearch(false)}
        onNavigateToChannel={(channelId) => {
          onNavigateToChannel?.(channelId);
          setShowSearch(false);
        }}
      />
    )}
    </div>
  );
};