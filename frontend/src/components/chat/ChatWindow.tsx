import React, { useCallback } from 'react';
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
  onSendMessage: (content: string, attachmentUrl?: string) => void;
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
}) => {
  const replyValue = replyTo ? `@${replyTo.author_username} ` : '';

  const handleSendMessage = useCallback(
    (content: string, attachmentUrl?: string) => {
      onSendMessage(content, attachmentUrl);
      if (onClearReply) {
        onClearReply();
      }
    },
    [onSendMessage, onClearReply]
  );

  return (
    <div className="chat-window">
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