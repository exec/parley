import React, { useState, useRef, useCallback, KeyboardEvent, ChangeEvent, useEffect, useMemo } from 'react';
import { uploadFile } from '../../api/upload';
import { AudioPlayer } from './AudioPlayer';
import { GifPicker } from './GifPicker';
import { MentionDropdown } from './MentionDropdown';
import { ChannelTagDropdown } from './ChannelTagDropdown';
import { detectMention, useMentionSuggestions, insertMentionText, resolveMentions, MentionSuggestion } from '../../hooks/useMentionAutocomplete';
import { detectChannelTag, useChannelSuggestions, insertChannelTag, resolveChannelTags } from '../../hooks/useChannelAutocomplete';
import { detectEmojiTrigger, useEmojiSuggestions, insertEmoji, resolveEmojis, EmojiSuggestion } from '../../hooks/useEmojiAutocomplete';
import { EmojiDropdown } from './EmojiDropdown';
import { Channel, Message as MessageType, ServerMember } from '../../api/types';
import { usePermissions } from '../../hooks/usePermissions';
import { PERM_SEND_MESSAGES, PERM_ATTACH_FILES } from '../../lib/permissions';
import './Chat.css';

interface MessageInputProps {
  channelName: string;
  onSendMessage: (content: string, attachmentUrl?: string, attachmentName?: string, attachmentType?: string, parentId?: string) => void;
  onTyping?: () => void;
  disabled?: boolean;
  placeholder?: string;
  initialValue?: string;
  members?: ServerMember[];
  channels?: Channel[];
  replyTo?: MessageType | null;
  serverId?: string;
  channelId?: string;
}

const PaperclipIcon = () => (
  <svg width="14" height="18" viewBox="0 0 14 18" fill="none" stroke="currentColor"
    strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
    <path d="M11.5 5.5V12C11.5 14.76 9.26 17 6.5 17C3.74 17 1.5 14.76 1.5 12V4.5C1.5 2.57 3.07 1 5 1C6.93 1 8.5 2.57 8.5 4.5V12C8.5 12.83 7.83 13.5 7 13.5C6.17 13.5 5.5 12.83 5.5 12V5.5" />
  </svg>
);

const GifIcon = () => (
  <svg width="20" height="14" viewBox="0 0 20 14" fill="none" stroke="currentColor"
    strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
    <rect x="1" y="1" width="18" height="12" rx="2.5" />
    <path d="M5.5 7.5H7.5V9C7.5 9 7 9.5 5.75 9.5C4.5 9.5 4 8.5 4 7C4 5.5 4.8 4.5 5.9 4.5C6.7 4.5 7.2 5 7.4 5.3" />
    <line x1="9.5" y1="4.5" x2="9.5" y2="9.5" />
    <path d="M12 4.5H15M12 7H14M12 4.5V9.5" />
  </svg>
);

const MicIcon = ({ active }: { active?: boolean }) => (
  <svg width="14" height="18" viewBox="0 0 14 18" fill="none" stroke="currentColor"
    strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
    <rect x="4" y="1" width="6" height="9" rx="3" fill={active ? 'currentColor' : 'none'} fillOpacity={active ? 0.2 : 0} />
    <path d="M1.5 8.5C1.5 11.81 4.19 14.5 7 14.5C9.81 14.5 12.5 11.81 12.5 8.5" />
    <line x1="7" y1="14.5" x2="7" y2="17" />
    <line x1="4.5" y1="17" x2="9.5" y2="17" />
  </svg>
);

function fmtDur(s: number) {
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

export const MessageInput: React.FC<MessageInputProps> = ({
  channelName,
  onSendMessage,
  onTyping,
  disabled = false,
  initialValue = '',
  members = [],
  channels = [],
  replyTo,
  serverId,
  channelId,
}) => {
  const { hasPerm: checkPerm, loading: permsLoading } = usePermissions(serverId, channelId);
  const canSend = !serverId || permsLoading || checkPerm(PERM_SEND_MESSAGES);
  const canAttach = !serverId || permsLoading || checkPerm(PERM_ATTACH_FILES);
  const MAX_LENGTH = 4000;

  const [message, setMessage] = useState(initialValue);
  const [pendingFile, setPendingFile] = useState<File | null>(null);
  const [isUploading, setIsUploading] = useState(false);
  const [showGifPicker, setShowGifPicker] = useState(false);

  // Mention autocomplete state
  const [cursorPos, setCursorPos] = useState(0);
  const [mentionSelIdx, setMentionSelIdx] = useState(0);
  const mentionMatch = useMemo(() => detectMention(message, cursorPos), [message, cursorPos]);
  const mentionSuggestions = useMentionSuggestions(mentionMatch, members);

  // Channel tag autocomplete (mutually exclusive with mention dropdown)
  const [channelSelIdx, setChannelSelIdx] = useState(0);
  const channelMatch = useMemo(
    () => mentionSuggestions.length === 0 ? detectChannelTag(message, cursorPos) : null,
    [message, cursorPos, mentionSuggestions.length],
  );
  const channelSuggestions = useChannelSuggestions(channelMatch, channels);

  // Emoji autocomplete state (mutually exclusive with mention and channel dropdowns)
  const [emojiSelIdx, setEmojiSelIdx] = useState(0);
  const emojiMatch = useMemo(
    () => mentionSuggestions.length === 0 && channelSuggestions.length === 0 ? detectEmojiTrigger(message, cursorPos) : null,
    [message, cursorPos, mentionSuggestions.length, channelSuggestions.length],
  );
  const emojiSuggestions = useEmojiSuggestions(emojiMatch);

  const [isRecording, setIsRecording] = useState(false);
  const [voiceBlob, setVoiceBlob] = useState<Blob | null>(null);
  const [recordingDuration, setRecordingDuration] = useState(0);
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const recordTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const streamRef = useRef<MediaStream | null>(null);

  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (initialValue) {
      setMessage(initialValue);
      setTimeout(() => {
        if (textareaRef.current) {
          textareaRef.current.focus();
          textareaRef.current.selectionStart = textareaRef.current.value.length;
          textareaRef.current.selectionEnd = textareaRef.current.value.length;
        }
      }, 0);
    }
  }, [initialValue]);

  const voiceBlobUrl = useMemo(() => {
    if (!voiceBlob) return null;
    return URL.createObjectURL(voiceBlob);
  }, [voiceBlob]);
  useEffect(() => {
    return () => { if (voiceBlobUrl) URL.revokeObjectURL(voiceBlobUrl); };
  }, [voiceBlobUrl]);

  const imagePrevUrl = useMemo(() => {
    if (!pendingFile || !pendingFile.type.startsWith('image/')) return null;
    return URL.createObjectURL(pendingFile);
  }, [pendingFile]);
  useEffect(() => {
    return () => { if (imagePrevUrl) URL.revokeObjectURL(imagePrevUrl); };
  }, [imagePrevUrl]);

  const handleEmojiSelect = useCallback((suggestion: EmojiSuggestion) => {
    if (!emojiMatch) return;
    const { text: newText, cursor } = insertEmoji(message, emojiMatch, suggestion.native);
    setMessage(newText);
    setCursorPos(cursor);
    setEmojiSelIdx(0);
    requestAnimationFrame(() => {
      if (textareaRef.current) {
        textareaRef.current.focus();
        textareaRef.current.selectionStart = cursor;
        textareaRef.current.selectionEnd = cursor;
        textareaRef.current.style.height = 'auto';
        textareaRef.current.style.height = `${Math.min(textareaRef.current.scrollHeight, 200)}px`;
      }
    });
  }, [emojiMatch, message]);

  const handleSend = useCallback(async () => {
    const trimmedMessage = resolveEmojis(resolveMentions(resolveChannelTags(message.trim(), channels), members));
    if (!trimmedMessage && !pendingFile && !voiceBlob) return;

    setIsUploading(true);
    try {
      let attachmentUrl: string | undefined;
      let attachmentName: string | undefined;
      let attachmentType: string | undefined;

      if (voiceBlob) {
        const ext = voiceBlob.type.includes('ogg') ? '.ogg' : '.webm';
        const file = new File([voiceBlob], `voice_message_${Date.now()}${ext}`, { type: voiceBlob.type });
        attachmentUrl = await uploadFile(file);
        attachmentName = file.name;
        attachmentType = file.type;
        setVoiceBlob(null);
      } else if (pendingFile) {
        attachmentUrl = await uploadFile(pendingFile);
        attachmentName = pendingFile.name;
        attachmentType = pendingFile.type;
      }

      onSendMessage(trimmedMessage, attachmentUrl, attachmentName, attachmentType, replyTo?.id);
      setMessage('');
      setPendingFile(null);
      if (fileInputRef.current) fileInputRef.current.value = '';
      if (textareaRef.current) textareaRef.current.style.height = 'auto';
    } catch (err) {
      console.error('Failed to send message:', err);
    } finally {
      setIsUploading(false);
    }
  }, [message, pendingFile, voiceBlob, onSendMessage, replyTo]);

  const handleChannelSelect = useCallback((channel: import('../../api/types').Channel) => {
    if (!channelMatch) return;
    const { text: newText, cursor } = insertChannelTag(message, channelMatch, channel);
    setMessage(newText);
    setCursorPos(cursor);
    setChannelSelIdx(0);
    requestAnimationFrame(() => {
      if (textareaRef.current) {
        textareaRef.current.focus();
        textareaRef.current.selectionStart = cursor;
        textareaRef.current.selectionEnd = cursor;
        textareaRef.current.style.height = 'auto';
        textareaRef.current.style.height = `${Math.min(textareaRef.current.scrollHeight, 200)}px`;
      }
    });
  }, [channelMatch, message]);

  const handleMentionSelect = useCallback((suggestion: MentionSuggestion) => {
    if (!mentionMatch) return;
    const { text: newText, cursor } = insertMentionText(message, mentionMatch, suggestion);
    setMessage(newText);
    setCursorPos(cursor);
    setMentionSelIdx(0);
    // Restore focus and move cursor after the inserted mention
    requestAnimationFrame(() => {
      if (textareaRef.current) {
        textareaRef.current.focus();
        textareaRef.current.selectionStart = cursor;
        textareaRef.current.selectionEnd = cursor;
        textareaRef.current.style.height = 'auto';
        textareaRef.current.style.height = `${Math.min(textareaRef.current.scrollHeight, 200)}px`;
      }
    });
  }, [mentionMatch, message]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      // Mention dropdown navigation
      if (mentionSuggestions.length > 0) {
        if (e.key === 'ArrowDown') { e.preventDefault(); setMentionSelIdx(i => (i + 1) % mentionSuggestions.length); return; }
        if (e.key === 'ArrowUp') { e.preventDefault(); setMentionSelIdx(i => (i - 1 + mentionSuggestions.length) % mentionSuggestions.length); return; }
        if (e.key === 'Tab' || e.key === 'Enter') { e.preventDefault(); handleMentionSelect(mentionSuggestions[mentionSelIdx]); return; }
        if (e.key === 'Escape') { setCursorPos(message.length); return; }
      }
      // Channel tag dropdown navigation
      if (channelSuggestions.length > 0) {
        if (e.key === 'ArrowDown') { e.preventDefault(); setChannelSelIdx(i => (i + 1) % channelSuggestions.length); return; }
        if (e.key === 'ArrowUp') { e.preventDefault(); setChannelSelIdx(i => (i - 1 + channelSuggestions.length) % channelSuggestions.length); return; }
        if (e.key === 'Tab' || e.key === 'Enter') { e.preventDefault(); handleChannelSelect(channelSuggestions[channelSelIdx]); return; }
        if (e.key === 'Escape') { setCursorPos(message.length); return; }
      }
      // Emoji dropdown navigation
      if (emojiSuggestions.length > 0) {
        if (e.key === 'ArrowDown') { e.preventDefault(); setEmojiSelIdx(i => (i + 1) % emojiSuggestions.length); return; }
        if (e.key === 'ArrowUp') { e.preventDefault(); setEmojiSelIdx(i => (i - 1 + emojiSuggestions.length) % emojiSuggestions.length); return; }
        if (e.key === 'Tab' || e.key === 'Enter') { e.preventDefault(); handleEmojiSelect(emojiSuggestions[emojiSelIdx]); return; }
        if (e.key === 'Escape') { setCursorPos(message.length); return; }
      }
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSend(); }
    },
    [handleSend, mentionSuggestions, mentionSelIdx, handleMentionSelect, channelSuggestions, channelSelIdx, handleChannelSelect, emojiSuggestions, emojiSelIdx, handleEmojiSelect, message.length],
  );

  const handleChange = useCallback(
    (e: ChangeEvent<HTMLTextAreaElement>) => {
      setMessage(e.target.value);
      setCursorPos(e.target.selectionStart ?? 0);
      setMentionSelIdx(0);
      setChannelSelIdx(0);
      setEmojiSelIdx(0);
      const ta = e.target;
      ta.style.height = 'auto';
      ta.style.height = `${Math.min(ta.scrollHeight, 200)}px`;
      if (e.target.value.length > 0) onTyping?.();
    },
    [onTyping],
  );

  const handleFileChange = useCallback((e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) { setPendingFile(file); setVoiceBlob(null); }
  }, []);

  const handlePaste = useCallback((e: React.ClipboardEvent<HTMLTextAreaElement>) => {
    const items = Array.from(e.clipboardData.items);
    const fileItem = items.find(i => i.kind === 'file');
    if (!fileItem) return;
    const file = fileItem.getAsFile();
    if (!file) return;
    e.preventDefault();
    setPendingFile(file);
    setVoiceBlob(null);
  }, []);

  const handleAttachClick = useCallback(() => { fileInputRef.current?.click(); }, []);

  const handleRemoveFile = useCallback(() => {
    setPendingFile(null);
    if (fileInputRef.current) fileInputRef.current.value = '';
  }, []);

  const cancelVoice = useCallback(() => {
    setVoiceBlob(null);
    setRecordingDuration(0);
  }, []);

  const handleGifSelect = useCallback((url: string) => {
    setShowGifPicker(false);
    // Send the GIF as an image attachment with no text
    onSendMessage('', url, 'gif', 'image/gif');
  }, [onSendMessage]);

  const startRecording = useCallback(async () => {
    if (isRecording) return;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      streamRef.current = stream;
      const mimeType = MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
        ? 'audio/webm;codecs=opus'
        : MediaRecorder.isTypeSupported('audio/ogg;codecs=opus')
          ? 'audio/ogg;codecs=opus'
          : 'audio/webm';
      chunksRef.current = [];
      const mr = new MediaRecorder(stream, { mimeType });
      mr.ondataavailable = (e) => { if (e.data.size > 0) chunksRef.current.push(e.data); };
      mr.onstop = () => {
        const blob = new Blob(chunksRef.current, { type: mimeType });
        setVoiceBlob(blob);
        setPendingFile(null);
        stream.getTracks().forEach(t => t.stop());
        if (recordTimerRef.current) clearInterval(recordTimerRef.current);
      };
      mr.start();
      mediaRecorderRef.current = mr;
      setIsRecording(true);
      setRecordingDuration(0);
      recordTimerRef.current = setInterval(() => setRecordingDuration(d => d + 1), 1000);
    } catch {
      // Mic permission denied or unavailable
    }
  }, [isRecording]);

  const stopRecording = useCallback(() => {
    mediaRecorderRef.current?.stop();
    setIsRecording(false);
    if (recordTimerRef.current) clearInterval(recordTimerRef.current);
  }, []);

  const handleMicClick = useCallback(() => {
    if (isRecording) stopRecording();
    else startRecording();
  }, [isRecording, startRecording, stopRecording]);

  const isBusy = disabled || isUploading;
  const defaultPlaceholder = !canSend
    ? 'You do not have permission to send messages in this channel.'
    : `Message #${channelName}`;

  if (!canSend) {
    return (
      <div className="message-input-container">
        <div className="message-input-row">
          <div className="message-input-wrapper" style={{ opacity: 0.5, cursor: 'not-allowed' }}>
            <textarea
              className="message-textarea"
              placeholder="You do not have permission to send messages in this channel."
              disabled
              rows={1}
              style={{ cursor: 'not-allowed' }}
            />
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="message-input-container">
      {voiceBlob && voiceBlobUrl && (
        <div className="attachment-preview attachment-preview--voice">
          <AudioPlayer url={voiceBlobUrl} isVoiceMessage />
          <button type="button" className="attachment-preview-remove" onClick={cancelVoice} title="Discard">✕</button>
        </div>
      )}

      {pendingFile && !voiceBlob && (
        <div className="attachment-preview">
          {imagePrevUrl ? (
            <img src={imagePrevUrl} alt={pendingFile.name} className="attachment-preview-image" />
          ) : (
            <span className="attachment-preview-icon"><PaperclipIcon /></span>
          )}
          <span className="attachment-preview-name">{pendingFile.name}</span>
          <button type="button" className="attachment-preview-remove" onClick={handleRemoveFile} title="Remove">✕</button>
        </div>
      )}

      {isRecording && (
        <div className="recording-indicator">
          <span className="recording-dot" />
          <span className="recording-label">Recording {fmtDur(recordingDuration)}</span>
          <span className="recording-hint">click mic to stop</span>
        </div>
      )}

      <div className="message-input-row" style={{ position: 'relative' }}>
        {showGifPicker && (
          <GifPicker onSelect={handleGifSelect} onClose={() => setShowGifPicker(false)} />
        )}
        {mentionSuggestions.length > 0 && (
          <MentionDropdown
            suggestions={mentionSuggestions}
            selectedIdx={mentionSelIdx}
            onSelect={handleMentionSelect}
          />
        )}
        {channelSuggestions.length > 0 && (
          <ChannelTagDropdown
            suggestions={channelSuggestions}
            selectedIdx={channelSelIdx}
            onSelect={handleChannelSelect}
          />
        )}
        {emojiSuggestions.length > 0 && (
          <EmojiDropdown
            suggestions={emojiSuggestions}
            selectedIdx={emojiSelIdx}
            onSelect={handleEmojiSelect}
          />
        )}

        <input
          ref={fileInputRef}
          type="file"
          style={{ display: 'none' }}
          onChange={handleFileChange}
          disabled={isBusy}
        />

        <div className="message-input-wrapper">
          {canAttach && (
          <button
            type="button"
            className="input-icon-btn attach-btn"
            onClick={handleAttachClick}
            disabled={isBusy || isRecording}
            title="Attach file"
          >
            <PaperclipIcon />
          </button>
          )}
          <button
            type="button"
            className={`input-icon-btn gif-btn${showGifPicker ? ' gif-btn--active' : ''}`}
            onClick={() => setShowGifPicker(p => !p)}
            disabled={isBusy || isRecording}
            title="Send a GIF"
          >
            <GifIcon />
          </button>
          <textarea
            ref={textareaRef}
            className="message-textarea"
            value={message}
            onChange={handleChange}
            onKeyDown={handleKeyDown}
            onSelect={e => setCursorPos((e.target as HTMLTextAreaElement).selectionStart ?? 0)}
            onClick={e => setCursorPos((e.target as HTMLTextAreaElement).selectionStart ?? 0)}
            onPaste={handlePaste}
            placeholder={isRecording ? 'Recording…' : defaultPlaceholder}
            disabled={isBusy}
            maxLength={MAX_LENGTH}
            rows={1}
          />
          {message.length > MAX_LENGTH * 0.9 && (
            <span
              className={`char-counter${message.length > MAX_LENGTH ? ' char-counter--over' : ''}`}
              aria-live="polite"
            >
              {message.length} / {MAX_LENGTH}
            </span>
          )}
          {(() => {
            const sendDisabled = isBusy || message.length > MAX_LENGTH || (!message.trim() && !pendingFile && !voiceBlob);
            return (
              <button
                className="send-button"
                onClick={handleSend}
                disabled={sendDisabled}
                aria-disabled={sendDisabled}
              >
                {isUploading ? '...' : 'Send'}
              </button>
            );
          })()}
        </div>

        <button
          type="button"
          className={`input-icon-btn mic-btn${isRecording ? ' mic-btn--recording' : ''}`}
          onClick={handleMicClick}
          disabled={isBusy && !isRecording}
          title={isRecording ? 'Stop recording' : 'Voice message'}
        >
          <MicIcon active={isRecording} />
        </button>
      </div>
    </div>
  );
};
