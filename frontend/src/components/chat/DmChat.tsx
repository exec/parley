import { useState, useRef, useEffect } from 'react';
import { DmChannel, DmMessage } from '../../api/types';
import { Spinner } from '../ui/Spinner';

interface DmChatProps {
  channel: DmChannel;
  messages: DmMessage[];
  currentUserId?: string;
  onSendMessage: (content: string) => Promise<void>;
  isLoading: boolean;
}

export function DmChat({ channel, messages, currentUserId, onSendMessage, isLoading }: DmChatProps) {
  const [input, setInput] = useState('');
  const [isSending, setIsSending] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim() || isSending) return;

    setIsSending(true);
    try {
      await onSendMessage(input.trim());
      setInput('');
    } catch (err) {
      console.error(err);
    } finally {
      setIsSending(false);
    }
  };

  const formatTime = (dateStr: string) => {
    const date = new Date(dateStr);
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  };

  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr);
    return date.toLocaleDateString();
  };

  // Group messages by date
  const groupedMessages: { date: string; messages: DmMessage[] }[] = [];
  let currentDate = '';
  messages.forEach(msg => {
    const msgDate = formatDate(msg.created_at);
    if (msgDate !== currentDate) {
      currentDate = msgDate;
      groupedMessages.push({ date: msgDate, messages: [msg] });
    } else {
      groupedMessages[groupedMessages.length - 1].messages.push(msg);
    }
  });

  return (
    <div className="dm-chat">
      <div className="dm-header">
        <div className="dm-header-info">
          <div className="dm-header-avatar">
            {channel.other_username.charAt(0).toUpperCase()}
          </div>
          <div className="dm-header-name">@{channel.other_username}</div>
        </div>
      </div>

      <div className="dm-messages">
        {isLoading ? (
          <div className="messages-loading">
            <Spinner />
          </div>
        ) : (
          <>
            {groupedMessages.map((group, groupIdx) => (
              <div key={groupIdx}>
                <div className="date-divider">
                  <span>{group.date}</span>
                </div>
                {group.messages.map((msg, msgIdx) => {
                  const isOwnMessage = msg.author_id === currentUserId;
                  const showAvatar = msgIdx === 0 ||
                    group.messages[msgIdx - 1].author_id !== msg.author_id;

                  return (
                    <div
                      key={msg.id}
                      className={`dm-message ${isOwnMessage ? 'own-message' : ''}`}
                    >
                      {showAvatar ? (
                        <div className="message-avatar">
                          {msg.author_username.charAt(0).toUpperCase()}
                        </div>
                      ) : (
                        <div className="message-avatar-spacer" />
                      )}
                      <div className="message-content-wrapper">
                        {showAvatar && (
                          <div className="message-header">
                            <span className="message-author">{msg.author_username}</span>
                            <span className="message-time">{formatTime(msg.created_at)}</span>
                          </div>
                        )}
                        <div className="message-content">{msg.content}</div>
                      </div>
                    </div>
                  );
                })}
              </div>
            ))}
            <div ref={messagesEndRef} />
          </>
        )}
      </div>

      <form className="dm-input-container" onSubmit={handleSubmit}>
        <div className="dm-input-wrapper">
          <button type="button" className="input-action-button" title="Add friends">+</button>
          <input
            type="text"
            className="dm-input"
            placeholder={`Message @${channel.other_username}`}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            disabled={isSending}
          />
          <button
            type="submit"
            className="send-button"
            disabled={!input.trim() || isSending}
          >
            Send
          </button>
        </div>
      </form>
    </div>
  );
}