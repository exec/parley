import React, { useCallback, useState, useRef, useEffect } from 'react';
import { Channel, Message as MessageType, ServerMember } from '../../api/types';
import { MessageList } from './MessageList';
import { MessageInput } from './MessageInput';
import { TypingIndicator } from './TypingIndicator';
import { usePermissions } from '../../hooks/usePermissions';
import { PERM_MANAGE_MESSAGES, PERM_ADD_REACTIONS } from '../../lib/permissions';
import MiniProfile from '../layout/MiniProfile';
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
  memberMap?: Map<string, string>;
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
  memberMap,
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
    const member = members?.find(m => m.user_id === userId);
    if (!member) return;
    const x = e.clientX;
    const y = e.clientY;
    const left = Math.min(x + 10, window.innerWidth - 290);
    const top = Math.min(y, window.innerHeight - 330);
    setMiniProfile({ member, position: { top, left } });
  }, [members]);

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
    <div className="chat-window">
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
  );
};