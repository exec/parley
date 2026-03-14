import React, { useState, useRef, useEffect, useCallback, ChangeEvent } from 'react';
import { DmChannel, DmMessage } from '../../api/types';
import { Spinner } from '../ui/Spinner';
import { uploadFile } from '../../api/upload';
import './Chat.css';

interface DmChatProps {
  channel: DmChannel;
  messages: DmMessage[];
  currentUserId?: string;
  onSendMessage: (content: string, attachmentUrl?: string) => Promise<void>;
  isLoading: boolean;
}

export function DmChat({ channel, messages, currentUserId, onSendMessage, isLoading }: DmChatProps) {
  const [input, setInput] = useState('');
  const [isSending, setIsSending] = useState(false);
  const [pendingFile, setPendingFile] = useState<File | null>(null);
  const [isUploading, setIsUploading] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if ((!input.trim() && !pendingFile) || isSending || isUploading) return;

    setIsUploading(true);
    setIsSending(true);
    try {
      let attachmentUrl: string | undefined;
      if (pendingFile) {
        attachmentUrl = await uploadFile(pendingFile);
      }
      await onSendMessage(input.trim(), attachmentUrl);
      setInput('');
      setPendingFile(null);
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsSending(false);
      setIsUploading(false);
    }
  };

  const handleFileChange = useCallback((e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      setPendingFile(file);
    }
  }, []);

  const handleAttachClick = useCallback(() => {
    fileInputRef.current?.click();
  }, []);

  const handleRemoveFile = useCallback(() => {
    setPendingFile(null);
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }
  }, []);

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
                        {msg.attachment_url && (
                          <div className="message-attachment">
                            {msg.attachment_type?.startsWith('image/') ? (
                              <img
                                src={msg.attachment_url}
                                alt={msg.attachment_name || 'attachment'}
                                className="message-attachment-image"
                                style={{ maxWidth: '400px', maxHeight: '300px', borderRadius: '4px', marginTop: '4px' }}
                              />
                            ) : (
                              <a
                                href={msg.attachment_url}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="message-attachment-file"
                              >
                                📎 {msg.attachment_name || 'attachment'}
                              </a>
                            )}
                          </div>
                        )}
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
        {pendingFile && (
          <div className="attachment-preview">
            <span>📎 {pendingFile.name}</span>
            <button
              type="button"
              className="attachment-preview-remove"
              onClick={handleRemoveFile}
              title="Remove attachment"
            >
              ✕
            </button>
          </div>
        )}
        <div className="dm-input-wrapper">
          <button type="button" className="input-action-button" title="Add friends">+</button>
          <input
            ref={fileInputRef}
            type="file"
            style={{ display: 'none' }}
            onChange={handleFileChange}
            disabled={isSending || isUploading}
          />
          <button
            type="button"
            className="attach-button"
            onClick={handleAttachClick}
            disabled={isSending || isUploading}
            title="Attach file"
          >
            📎
          </button>
          <input
            type="text"
            className="dm-input"
            placeholder={`Message @${channel.other_username}`}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            disabled={isSending || isUploading}
          />
          <button
            type="submit"
            className="send-button"
            disabled={(!input.trim() && !pendingFile) || isSending || isUploading}
          >
            {isUploading ? '...' : 'Send'}
          </button>
        </div>
      </form>
    </div>
  );
}