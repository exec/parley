import React, { useState, useRef, useCallback, KeyboardEvent, ChangeEvent, useEffect } from 'react';
import { uploadFile } from '../../api/upload';
import './Chat.css';

interface MessageInputProps {
  channelName: string;
  onSendMessage: (content: string, attachmentUrl?: string, attachmentName?: string, attachmentType?: string) => void;
  onTyping?: () => void;
  disabled?: boolean;
  placeholder?: string;
  initialValue?: string;
}

export const MessageInput: React.FC<MessageInputProps> = ({
  channelName,
  onSendMessage,
  onTyping,
  disabled = false,
  initialValue = '',
}) => {
  const [message, setMessage] = useState(initialValue);
  const [pendingFile, setPendingFile] = useState<File | null>(null);
  const [isUploading, setIsUploading] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Update message when initialValue changes
  useEffect(() => {
    if (initialValue) {
      setMessage(initialValue);
      // Focus and set cursor position at end
      setTimeout(() => {
        if (textareaRef.current) {
          textareaRef.current.focus();
          textareaRef.current.selectionStart = textareaRef.current.value.length;
          textareaRef.current.selectionEnd = textareaRef.current.value.length;
        }
      }, 0);
    }
  }, [initialValue]);

  const handleSend = useCallback(async () => {
    const trimmedMessage = message.trim();
    if (!trimmedMessage && !pendingFile) return;

    setIsUploading(true);
    try {
      let attachmentUrl: string | undefined;
      let attachmentName: string | undefined;
      let attachmentType: string | undefined;
      if (pendingFile) {
        attachmentUrl = await uploadFile(pendingFile);
        attachmentName = pendingFile.name;
        attachmentType = pendingFile.type;
      }

      onSendMessage(trimmedMessage, attachmentUrl, attachmentName, attachmentType);
      setMessage('');
      setPendingFile(null);
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }

      // Reset textarea height
      if (textareaRef.current) {
        textareaRef.current.style.height = 'auto';
      }
    } catch (err) {
      console.error('Failed to send message:', err);
    } finally {
      setIsUploading(false);
    }
  }, [message, pendingFile, onSendMessage]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSend();
      }
    },
    [handleSend]
  );

  const handleChange = useCallback(
    (e: ChangeEvent<HTMLTextAreaElement>) => {
      setMessage(e.target.value);

      // Auto-resize textarea
      const textarea = e.target;
      textarea.style.height = 'auto';
      textarea.style.height = `${Math.min(textarea.scrollHeight, 200)}px`;

      // Notify parent that user is typing (parent throttles the WS send)
      if (e.target.value.length > 0) {
        onTyping?.();
      }
    },
    [onTyping]
  );

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

  const defaultPlaceholder = `Message #${channelName}`;
  const isBusy = disabled || isUploading;

  return (
    <div className="message-input-container">
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
      <div className="message-input-wrapper">
        <input
          ref={fileInputRef}
          type="file"
          style={{ display: 'none' }}
          onChange={handleFileChange}
          disabled={isBusy}
        />
        <button
          type="button"
          className="attach-button"
          onClick={handleAttachClick}
          disabled={isBusy}
          title="Attach file"
        >
          📎
        </button>
        <textarea
          ref={textareaRef}
          className="message-textarea"
          value={message}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          placeholder={defaultPlaceholder}
          disabled={isBusy}
          rows={1}
        />
        <button
          className="send-button"
          onClick={handleSend}
          disabled={isBusy || (!message.trim() && !pendingFile)}
        >
          {isUploading ? '...' : 'Send'}
        </button>
      </div>
    </div>
  );
};
