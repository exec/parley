import React, { useCallback } from 'react';
import { Channel, Message as MessageType } from '../../api/types';
import { MessageList } from './MessageList';
import { MessageInput } from './MessageInput';
import './Chat.css';

interface ChatWindowProps {
  channel: Channel;
  messages: MessageType[];
  currentUserId?: string;
  onSendMessage: (content: string) => void;
  onLoadMore?: () => void;
  onEdit?: (message: MessageType) => void;
  onDelete?: (messageId: string) => void;
  onReply?: (message: MessageType) => void;
  onViewProfile?: (userId: string, username: string) => void;
  onSendMessageToUser?: (userId: string) => void;
  hasMore?: boolean;
  isLoading?: boolean;
  replyTo?: MessageType | null;
  onClearReply?: () => void;
}

export const ChatWindow: React.FC<ChatWindowProps> = ({
  channel,
  messages,
  currentUserId,
  onSendMessage,
  onLoadMore,
  onEdit,
  onDelete,
  onReply,
  onViewProfile,
  onSendMessageToUser,
  hasMore = false,
  isLoading = false,
  replyTo,
  onClearReply,
}) => {
  const replyValue = replyTo ? `@${replyTo.author_username} ` : '';

  const handleSendMessage = useCallback(
    (content: string) => {
      onSendMessage(content);
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
        onReply={onReply}
        onViewProfile={onViewProfile}
        onSendMessage={onSendMessageToUser}
        hasMore={hasMore}
        isLoading={isLoading}
      />
      <MessageInput
        channelName={channel.name}
        onSendMessage={handleSendMessage}
        initialValue={replyValue}
      />
    </div>
  );
};