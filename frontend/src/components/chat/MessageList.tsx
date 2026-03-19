import React, { useEffect, useRef, useCallback, useMemo } from 'react';
import { Message as MessageType } from '../../api/types';
import { Message } from './Message';
import './Chat.css';

interface MessageListProps {
  messages: MessageType[];
  channelId?: string;
  currentUserId?: string;
  memberMap?: Map<string, string>;
  channelMap?: Map<string, string>;
  onLoadMore?: () => void;
  onEdit?: (message: MessageType) => void;
  onDelete?: (messageId: string) => void;
  onReact?: (messageId: string, emoji: string) => void;
  onReply?: (message: MessageType) => void;
  onViewProfile?: (userId: string, username: string) => void;
  onSendMessage?: (userId: string) => void;
  onMiniProfile?: (userId: string, e: React.MouseEvent) => void;
  hasMore?: boolean;
  isLoading?: boolean;
  allMessages?: MessageType[];
  canManageMessages?: boolean;
  canAddReactions?: boolean;
  canKickMembers?: boolean;
  canBanMembers?: boolean;
  onKickMember?: (userId: string) => void;
  onBanMember?: (userId: string) => void;
  onScrollToMessage?: (messageId: string) => void;
}

export const MessageList: React.FC<MessageListProps> = ({
  messages,
  channelId,
  currentUserId,
  memberMap,
  channelMap,
  onLoadMore,
  onEdit,
  onDelete,
  onReact,
  onReply,
  onViewProfile,
  onSendMessage,
  onMiniProfile,
  hasMore = false,
  isLoading = false,
  allMessages,
  canManageMessages = true,
  canAddReactions = true,
  canKickMembers,
  canBanMembers,
  onKickMember,
  onBanMember,
  onScrollToMessage: onScrollToMessageProp,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const shouldAutoScrollRef = useRef(true);

  const handleScrollToMessage = useCallback((messageId: string) => {
    const el = document.getElementById(`message-${messageId}`);
    if (!el) return;
    el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    el.classList.add('message-highlight');
    setTimeout(() => el.classList.remove('message-highlight'), 1500);
  }, []);

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

  // Force scroll to bottom when switching channels
  useEffect(() => {
    shouldAutoScrollRef.current = true;
    if (containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [channelId]);

  // Auto-scroll to bottom on new messages (deferred so DOM is painted)
  useEffect(() => {
    if (!shouldAutoScrollRef.current || !containerRef.current) return;
    const container = containerRef.current;
    requestAnimationFrame(() => {
      container.scrollTop = container.scrollHeight;
    });
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

  // Sort by integer ID so WS events arriving out of order never mis-sequence.
  // Pending (optimistic) messages have a non-numeric id; sort them last.
  const sortedMessages = useMemo(() => {
    return [...messages].sort((a, b) => {
      const aId = parseInt(a.id, 10);
      const bId = parseInt(b.id, 10);
      if (isNaN(aId) && isNaN(bId)) return 0;
      if (isNaN(aId)) return 1;
      if (isNaN(bId)) return -1;
      return aId - bId;
    });
  }, [messages]);

  const groupedMessages = groupMessagesByDate(sortedMessages);

  const GROUP_WINDOW_MS = 10 * 60 * 1000;
  const MAX_GROUP_SIZE = 10;

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

      {Array.from(groupedMessages.entries()).map(([date, dateMessages]) => {
        // Compute grouped flags within each date group (iterative to avoid TDZ)
        const groupedFlags: boolean[] = [];
        for (let idx = 0; idx < dateMessages.length; idx++) {
          const msg = dateMessages[idx];
          if (idx === 0) { groupedFlags.push(false); continue; }
          const prev = dateMessages[idx - 1];
          if (prev.author_id !== msg.author_id) { groupedFlags.push(false); continue; }
          if (new Date(msg.created_at).getTime() - new Date(prev.created_at).getTime() > GROUP_WINDOW_MS) { groupedFlags.push(false); continue; }
          // Count how many consecutive grouped messages are in this streak
          let streakLen = 0;
          for (let i = idx - 1; i >= 0; i--) {
            if (dateMessages[i].author_id !== msg.author_id) break;
            streakLen++;
            if (!groupedFlags[i]) break;
          }
          groupedFlags.push(streakLen < MAX_GROUP_SIZE);
        }

        return (
        <div key={date} className="date-group">
          <div className="date-divider">
            <div className="date-divider-line" />
            <span className="date-divider-text">{formatDateHeader(date)}</span>
            <div className="date-divider-line" />
          </div>
          {dateMessages.map((message, idx) => (
            <Message
              key={message.id}
              message={message}
              currentUserId={currentUserId}
              isGrouped={groupedFlags[idx]}
              memberMap={memberMap}
              channelMap={channelMap}
              messages={allMessages}
              onEdit={onEdit}
              onDelete={onDelete}
              onReact={onReact}
              onReply={onReply}
              onViewProfile={onViewProfile}
              onSendMessage={onSendMessage}
              onMiniProfile={onMiniProfile}
              onScrollToMessage={onScrollToMessageProp ?? handleScrollToMessage}
              canManageMessages={canManageMessages}
              canAddReactions={canAddReactions}
              canKickMembers={canKickMembers}
              canBanMembers={canBanMembers}
              onKickMember={onKickMember}
              onBanMember={onBanMember}
            />
          ))}
        </div>
        );
      })}

      {messages.length === 0 && !isLoading && (
        <div className="message-empty">
          <h3>No messages yet</h3>
          <p>Be the first to send a message in this channel!</p>
        </div>
      )}
    </div>
  );
};