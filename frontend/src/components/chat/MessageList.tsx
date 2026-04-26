import React, { useEffect, useLayoutEffect, useRef, useCallback, useMemo, useState } from 'react';
import { ChevronDown } from 'lucide-react';
import { Message as MessageType } from '../../api/types';
import { Message } from './Message';
import { SystemMessage } from './SystemMessage';
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
  canPin?: boolean;
  onKickMember?: (userId: string) => void;
  onBanMember?: (userId: string) => void;
  onScrollToMessage?: (messageId: string) => void;
  onPin?: (messageId: string) => void;
  onUnpin?: (messageId: string) => void;
  onForward?: (message: MessageType) => void;
  onJumpToMessage?: (channelId: string, messageId: string) => void;
  jumpToMessageId?: string | null;
  onJumpClear?: () => void;
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
  canPin,
  onKickMember,
  onBanMember,
  onScrollToMessage: onScrollToMessageProp,
  onPin,
  onUnpin,
  onForward,
  onJumpToMessage,
  jumpToMessageId,
  onJumpClear,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);

  // Auto-scroll: true when user is at bottom (new messages should pull them along)
  const shouldAutoScrollRef = useRef(true);
  // Suppress scroll handler briefly after channel switch to avoid race
  const channelSwitchTimeRef = useRef(0);
  // Track channel for switch detection
  const prevChannelIdRef = useRef<string | undefined>(undefined);

  // Scroll anchor: snapshot taken right before triggering a load-more.
  // Used to restore visual position after messages are prepended.
  const scrollAnchorRef = useRef<{ scrollTop: number; scrollHeight: number } | null>(null);
  // Track first message ID to detect prepend vs append
  const prevFirstMsgIdRef = useRef<string | undefined>(undefined);

  // Ref-based load lock so we never fire multiple loads before React state propagates.
  // This is the key fix for the multi-page load bug.
  const loadingLockRef = useRef(false);

  // Scroll-to-bottom button state
  const [isAtBottom, setIsAtBottom] = useState(true);
  const [unreadCount, setUnreadCount] = useState(0);
  // Track the last message ID to detect new appended messages vs prepended history
  const prevLastMsgIdRef = useRef<string | undefined>(undefined);

  const resolveUser = useCallback((userId: string): { displayName: string } => {
    const fromMap = memberMap?.get(userId);
    if (fromMap) return { displayName: fromMap };
    const found = messages.find(m => m.author_id === userId);
    if (found) return { displayName: found.author_display_name || found.author_username };
    return { displayName: 'Someone' };
  }, [memberMap, messages]);

  const handleScrollToMessage = useCallback((messageId: string) => {
    const el = document.getElementById(`message-${messageId}`);
    if (!el) return;
    el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    el.classList.add('message-highlight');
    setTimeout(() => el.classList.remove('message-highlight'), 1500);
  }, []);

  // Jump to a specific message when jumpToMessageId changes
  useEffect(() => {
    if (!jumpToMessageId) return;
    // Wait a tick for the DOM to settle after messages load
    const timer = setTimeout(() => {
      handleScrollToMessage(jumpToMessageId);
      onJumpClear?.();
    }, 100);
    return () => clearTimeout(timer);
  }, [jumpToMessageId, handleScrollToMessage, onJumpClear]);

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
        if (!groups.has(date)) groups.set(date, []);
        groups.get(date)!.push(msg);
      });
      return groups;
    },
    []
  );

  const formatDateHeader = (dateString: string): string => {
    const date = new Date(dateString);
    const today = new Date();
    const yesterday = new Date(today);
    yesterday.setDate(yesterday.getDate() - 1);
    if (date.toDateString() === today.toDateString()) return 'Today';
    if (date.toDateString() === yesterday.toDateString()) return 'Yesterday';
    return date.toLocaleDateString('en-US', {
      weekday: 'long',
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    });
  };

  // Sort by integer ID so WS events arriving out of order never mis-sequence.
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

  const firstMsgId = sortedMessages[0]?.id;
  const lastMsgId = sortedMessages[sortedMessages.length - 1]?.id;

  // Keep an up-to-date ref for isAtBottom so the unread effect can read it
  // without needing it as a dependency (avoids stale closure issues).
  const isAtBottomRef = useRef(true);
  useEffect(() => { isAtBottomRef.current = isAtBottom; }, [isAtBottom]);

  // Track new messages arriving at the END while scrolled away (unread count).
  // Distinguishes appends (new WS message) from prepends (load-more history)
  // by verifying the last message ID numerically advanced.
  useEffect(() => {
    const prevLast = prevLastMsgIdRef.current;
    prevLastMsgIdRef.current = lastMsgId;
    if (prevLast === undefined || lastMsgId === prevLast) return;
    const prevN = parseInt(prevLast, 10);
    const curN = parseInt(lastMsgId ?? '', 10);
    if (!isNaN(prevN) && !isNaN(curN) && curN > prevN && !isAtBottomRef.current) {
      setUnreadCount(c => c + 1);
    }
  }, [lastMsgId]);

  // Main scroll-position management. useLayoutEffect runs synchronously after DOM
  // update, before paint — critical so the user never sees a flash of wrong position.
  useLayoutEffect(() => {
    if (!containerRef.current) return;
    const container = containerRef.current;
    const changedChannel = prevChannelIdRef.current !== channelId;

    if (changedChannel) {
      // Full reset on channel switch
      prevChannelIdRef.current = channelId;
      prevFirstMsgIdRef.current = firstMsgId;
      shouldAutoScrollRef.current = true;
      loadingLockRef.current = false;
      scrollAnchorRef.current = null;
      channelSwitchTimeRef.current = Date.now();
      setIsAtBottom(true);
      setUnreadCount(0);
      container.scrollTop = container.scrollHeight;
      return;
    }

    // Prepend detected: first message changed and we have a saved anchor.
    // Restore the viewport so the user sees the same message they were looking at.
    if (firstMsgId !== prevFirstMsgIdRef.current && scrollAnchorRef.current !== null) {
      const { scrollTop: savedTop, scrollHeight: savedH } = scrollAnchorRef.current;
      const delta = container.scrollHeight - savedH;
      container.scrollTop = savedTop + delta;
      scrollAnchorRef.current = null;
      prevFirstMsgIdRef.current = firstMsgId;
      return;
    }

    prevFirstMsgIdRef.current = firstMsgId;

    // Auto-scroll to bottom when new messages arrive and user was already at bottom
    if (shouldAutoScrollRef.current) {
      container.scrollTop = container.scrollHeight;
    }
  }, [sortedMessages, channelId, firstMsgId]);

  // Clear the load lock when the isLoading prop goes false (load completed)
  useEffect(() => {
    if (!isLoading) {
      loadingLockRef.current = false;
    }
  }, [isLoading]);

  // Trigger a load-more with scroll anchor saved. The ref lock prevents double-fires
  // that would happen between scroll events and React state propagation.
  const triggerLoadMore = useCallback(() => {
    if (!onLoadMore || !hasMore || loadingLockRef.current || !containerRef.current) return;
    scrollAnchorRef.current = {
      scrollTop: containerRef.current.scrollTop,
      scrollHeight: containerRef.current.scrollHeight,
    };
    loadingLockRef.current = true;
    onLoadMore();
  }, [onLoadMore, hasMore]);

  const scrollToBottom = useCallback(() => {
    if (!containerRef.current) return;
    containerRef.current.scrollTop = containerRef.current.scrollHeight;
    shouldAutoScrollRef.current = true;
    setIsAtBottom(true);
    setUnreadCount(0);
  }, []);

  const handleScroll = useCallback(() => {
    if (!containerRef.current) return;
    // Ignore scroll events fired immediately after a channel switch
    if (Date.now() - channelSwitchTimeRef.current < 400) return;

    const { scrollTop, scrollHeight, clientHeight } = containerRef.current;
    const atBottom = scrollHeight - scrollTop - clientHeight < 100;

    setIsAtBottom(atBottom);
    if (atBottom) {
      shouldAutoScrollRef.current = true;
      setUnreadCount(0);
    } else {
      shouldAutoScrollRef.current = false;
    }

    // Load more history when scrolled near the top
    if (scrollTop < 150) {
      triggerLoadMore();
    }
  }, [triggerLoadMore]);

  const groupedMessages = groupMessagesByDate(sortedMessages);
  const GROUP_WINDOW_MS = 10 * 60 * 1000;
  const MAX_GROUP_SIZE = 10;

  return (
    <div className="message-list-wrapper">
      <div
        ref={containerRef}
        className="message-list-container"
        onScroll={handleScroll}
      >
        {hasMore && <div className="load-more-trigger" />}

        {isLoading && hasMore && (
          <div className="message-loading">Loading older messages...</div>
        )}

        {Array.from(groupedMessages.entries()).map(([date, dateMessages]) => {
          const groupedFlags: boolean[] = [];
          for (let idx = 0; idx < dateMessages.length; idx++) {
            const msg = dateMessages[idx];
            if (idx === 0) { groupedFlags.push(false); continue; }
            const prev = dateMessages[idx - 1];
            // System events break message grouping — a user message right after
            // a system event must render its full header even if the same user
            // sent the previous chat message.
            if (msg.system_event || prev.system_event) { groupedFlags.push(false); continue; }
            if (prev.author_id !== msg.author_id) { groupedFlags.push(false); continue; }
            if (new Date(msg.created_at).getTime() - new Date(prev.created_at).getTime() > GROUP_WINDOW_MS) {
              groupedFlags.push(false); continue;
            }
            let streakLen = 0;
            for (let i = idx - 1; i >= 0; i--) {
              if (dateMessages[i].system_event) break;
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
              {dateMessages.map((message, idx) => {
                if (message.system_event) {
                  return (
                    <SystemMessage
                      key={message.id}
                      event={message.system_event}
                      resolveUser={resolveUser}
                      createdAt={message.created_at}
                    />
                  );
                }
                return (
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
                    canPin={canPin}
                    onKickMember={onKickMember}
                    onBanMember={onBanMember}
                    onPin={onPin}
                    onUnpin={onUnpin}
                    onForward={onForward}
                    onJumpToMessage={onJumpToMessage}
                  />
                );
              })}
            </div>
          );
        })}

        {messages.length === 0 && !isLoading && (
          <div className="message-empty" role="status" aria-live="polite">
            <p className="message-empty-title">No messages yet — say hi!</p>
          </div>
        )}
      </div>

      {/* Scroll-to-bottom button — shown when user has scrolled up from the latest messages */}
      {!isAtBottom && (
        <button
          className="scroll-to-bottom-btn"
          onClick={scrollToBottom}
          title="Jump to latest messages"
        >
          {unreadCount > 0 && (
            <span className="scroll-to-bottom-badge">
              {unreadCount > 99 ? '99+' : unreadCount}
            </span>
          )}
          <ChevronDown size={20} />
        </button>
      )}
    </div>
  );
};
