import React, { useEffect, useRef, useCallback } from 'react';
import { Message as MessageType } from '../../api/types';
import { Message } from './Message';
import './Chat.css';

interface MessageListProps {
  messages: MessageType[];
  currentUserId?: string;
  onLoadMore?: () => void;
  onEdit?: (message: MessageType) => void;
  onDelete?: (messageId: string) => void;
  onReact?: (messageId: string, emoji: string) => void;
  onReply?: (message: MessageType) => void;
  onViewProfile?: (userId: string, username: string) => void;
  onSendMessage?: (userId: string) => void;
  hasMore?: boolean;
  isLoading?: boolean;
}

export const MessageList: React.FC<MessageListProps> = ({
  messages,
  currentUserId,
  onLoadMore,
  onEdit,
  onDelete,
  onReact,
  onReply,
  onViewProfile,
  onSendMessage,
  hasMore = false,
  isLoading = false,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const shouldAutoScrollRef = useRef(true);

  // Group messages by date
  const groupMessagesByDate = useCallback(
    (msgs: MessageType[]): Map<string, MessageType[]> => {
      const groups = new Map<string, MessageType[]>();

      msgs.forEach((msg) => {
        const date = new Date(msg.created_at).toLocaleDateString('en-US', {
          year: 'numeric',
          month: 'long',
          day: 'numeric',
        });

        if (!groups.has(date)) {
          groups.set(date, []);
        }
        groups.get(date)!.push(msg);
      });

      return groups;
    },
    []
  );

  // Format date for display
  const formatDateHeader = (dateString: string): string => {
    const date = new Date(dateString);
    const today = new Date();
    const yesterday = new Date(today);
    yesterday.setDate(yesterday.getDate() - 1);

    if (date.toDateString() === today.toDateString()) {
      return 'Today';
    } else if (date.toDateString() === yesterday.toDateString()) {
      return 'Yesterday';
    }
    return date.toLocaleDateString('en-US', {
      weekday: 'long',
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    });
  };

  // Auto-scroll to bottom on new messages
  useEffect(() => {
    if (shouldAutoScrollRef.current && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [messages]);

  // Handle scroll - both for infinite loading and auto-scroll reset
  const handleScroll = useCallback(() => {
    if (!containerRef.current) return;

    const { scrollTop, scrollHeight, clientHeight } = containerRef.current;

    // Reset auto-scroll when user scrolls to bottom
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 100;
    if (isAtBottom) {
      shouldAutoScrollRef.current = true;
    }

    // Load more when near the top
    if (onLoadMore && !isLoading && hasMore && scrollTop < 100) {
      shouldAutoScrollRef.current = false;
      onLoadMore();
    }
  }, [onLoadMore, hasMore, isLoading]);

  const groupedMessages = groupMessagesByDate(messages);

  return (
    <div
      ref={containerRef}
      className="message-list-container"
      onScroll={handleScroll}
    >
      {/* Invisible trigger for infinite scroll */}
      {hasMore && <div className="load-more-trigger" />}

      {isLoading && hasMore && (
        <div className="message-loading">Loading older messages...</div>
      )}

      {Array.from(groupedMessages.entries()).map(([date, dateMessages]) => (
        <div key={date} className="date-group">
          <div className="date-divider">
            <div className="date-divider-line" />
            <span className="date-divider-text">{formatDateHeader(date)}</span>
            <div className="date-divider-line" />
          </div>
          {dateMessages.map((message) => (
            <Message
              key={message.id}
              message={message}
              currentUserId={currentUserId}
              onEdit={onEdit}
              onDelete={onDelete}
              onReact={onReact}
              onReply={onReply}
              onViewProfile={onViewProfile}
              onSendMessage={onSendMessage}
            />
          ))}
        </div>
      ))}

      {messages.length === 0 && !isLoading && (
        <div className="message-empty">
          <h3>No messages yet</h3>
          <p>Be the first to send a message in this channel!</p>
        </div>
      )}
    </div>
  );
};