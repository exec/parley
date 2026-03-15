import React, { useCallback, useState, useRef, useEffect } from 'react';
import { Channel, Message as MessageType } from '../../api/types';
import { MessageList } from './MessageList';
import { MessageInput } from './MessageInput';
import { TypingIndicator } from './TypingIndicator';
import './Chat.css';

interface TypingUser {
  userId: string;
  username: string;
}

interface ChatWindowProps {
  channel: Channel;
  messages: MessageType[];
  currentUserId?: string;
  onSendMessage: (content: string, attachmentUrl?: string, attachmentName?: string, attachmentType?: string) => void;
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
}

export const ChatWindow: React.FC<ChatWindowProps> = ({
  channel,
  messages,
  currentUserId,
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
}) => {
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
    (content: string, attachmentUrl?: string, attachmentName?: string, attachmentType?: string) => {
      onSendMessage(content, attachmentUrl, attachmentName, attachmentType);
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
        <span className="chat-header-name"># {channel.name}</span>
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
      <MessageList
        messages={messages}
        currentUserId={currentUserId}
        onLoadMore={onLoadMore}
        onEdit={onEdit}
        onDelete={onDelete}
        onReact={onReact}
        onReply={onReply}
        onViewProfile={onViewProfile}
        onSendMessage={onSendMessageToUser}
        hasMore={hasMore}
        isLoading={isLoading}
      />
      <TypingIndicator typingUsers={typingUsers} />
      <MessageInput
        channelName={channel.name}
        onSendMessage={handleSendMessage}
        onTyping={onTyping}
        initialValue={replyValue}
      />
    </div>
  );
};