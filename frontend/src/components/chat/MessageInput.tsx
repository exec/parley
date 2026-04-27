import React, { useState, useRef, useCallback, KeyboardEvent, ChangeEvent, useEffect, useMemo } from 'react';
import { uploadFile } from '../../api/upload';
import { AudioPlayer } from './AudioPlayer';
import { GifPicker } from './GifPicker';
import { MentionDropdown } from './MentionDropdown';
import { ChannelTagDropdown } from './ChannelTagDropdown';
import { detectMention, useMentionSuggestions, insertMentionText, resolveMentions, MentionSuggestion } from '../../hooks/useMentionAutocomplete';
import { detectChannelTag, useChannelSuggestions, insertChannelTag, resolveChannelTags } from '../../hooks/useChannelAutocomplete';
import { detectEmojiTrigger, useEmojiSuggestions, insertEmoji, resolveEmojis, getEmojiData, EmojiSuggestion } from '../../hooks/useEmojiAutocomplete';
import { EmojiDropdown } from './EmojiDropdown';
import { SlashCommandDropdown } from './SlashCommandDropdown';
import { detectSlashCommand, useSlashCommands } from '../../hooks/useSlashCommands';
import { invokeCommand } from '../../api/slashCommands';
import { BotCommand, Channel, Message as MessageType, ServerMember, SlashCommandOption } from '../../api/types';
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
  onCancelReply?: () => void;
  serverId?: string;
  channelId?: string;
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
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
  onCancelReply,
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
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [dragActive, setDragActive] = useState(false);
  const dragCounterRef = useRef(0);

  // When the option-picker is active we suppress all textarea-based autocompletes
  // (we check pickerCommand inside the detection memos below).
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

  // Slash-command autocomplete / option-picker state.
  const [slashSelIdx, setSlashSelIdx] = useState(0);
  const [pickerCommand, setPickerCommand] = useState<BotCommand | null>(null);
  const [pickerValues, setPickerValues] = useState<Record<string, unknown>>({});
  const [invoking, setInvoking] = useState(false);
  const [invokeError, setInvokeError] = useState<string | null>(null);
  const pickerInputRefs = useRef<Array<HTMLInputElement | HTMLSelectElement | null>>([]);
  const { commands: allCommands, loading: commandsLoading } = useSlashCommands(serverId);

  // Only run slash detection when no other autocomplete is active AND we're not
  // already in option-picker mode.
  const slashMatch = useMemo(() => {
    if (pickerCommand) return null;
    if (mentionSuggestions.length > 0) return null;
    if (channelSuggestions.length > 0) return null;
    if (emojiSuggestions.length > 0) return null;
    return detectSlashCommand(message, cursorPos);
  }, [message, cursorPos, pickerCommand, mentionSuggestions.length, channelSuggestions.length, emojiSuggestions.length]);

  const slashFiltered = useMemo<BotCommand[]>(() => {
    if (!slashMatch) return [];
    const q = slashMatch.query.toLowerCase();
    return allCommands
      .filter(c => c.name.toLowerCase().startsWith(q))
      .slice(0, 8);
  }, [slashMatch, allCommands]);

  // Show the dropdown whenever the user's typing a '/' query. Distinguish three
  // states so the dropdown can render a hint for each: loading, zero-registered,
  // filtered matches.
  const showSlashDropdown = slashMatch !== null;
  const serverHasNoCommands = !commandsLoading && allCommands.length === 0;

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

  const exitPicker = useCallback((restoreText?: string) => {
    setPickerCommand(null);
    setPickerValues({});
    setInvokeError(null);
    setInvoking(false);
    pickerInputRefs.current = [];
    if (restoreText !== undefined) {
      setMessage(restoreText);
      requestAnimationFrame(() => {
        if (textareaRef.current) {
          textareaRef.current.focus();
          const pos = restoreText.length;
          textareaRef.current.selectionStart = pos;
          textareaRef.current.selectionEnd = pos;
          setCursorPos(pos);
        }
      });
    }
  }, []);

  const handleSlashCommandSelect = useCallback((cmd: BotCommand) => {
    setPickerCommand(cmd);
    // Seed defaults: BOOLEANs default to false so the toggle always has a value;
    // STRING/INTEGER start empty and must be filled (if required).
    const seed: Record<string, unknown> = {};
    for (const opt of cmd.options ?? []) {
      if (opt.type === 'BOOLEAN') seed[opt.name] = false;
    }
    setPickerValues(seed);
    setInvokeError(null);
    setSlashSelIdx(0);
    // Clear the raw "/cmd" text now that we've captured the command.
    setMessage('');
    // Focus the first option input on the next frame (once refs are wired).
    requestAnimationFrame(() => {
      const first = pickerInputRefs.current[0];
      if (first) first.focus();
    });
  }, []);

  const pickerValid = useMemo(() => {
    if (!pickerCommand) return false;
    for (const opt of pickerCommand.options ?? []) {
      if (!opt.required) continue;
      const v = pickerValues[opt.name];
      if (opt.type === 'BOOLEAN') {
        // BOOLEAN is always considered filled (false is a legitimate value).
        continue;
      }
      if (v === undefined || v === null || v === '') return false;
    }
    return true;
  }, [pickerCommand, pickerValues]);

  const runInvoke = useCallback(async () => {
    if (!pickerCommand || !channelId || !pickerValid || invoking) return;
    setInvoking(true);
    setInvokeError(null);
    try {
      // Coerce values to the backend-expected shape.
      const payload: Record<string, unknown> = {};
      for (const opt of pickerCommand.options ?? []) {
        const raw = pickerValues[opt.name];
        if (opt.type === 'BOOLEAN') {
          payload[opt.name] = Boolean(raw);
          continue;
        }
        if (raw === undefined || raw === '') continue; // optional + empty
        if (opt.type === 'INTEGER') {
          const n = typeof raw === 'number' ? raw : parseInt(String(raw), 10);
          if (Number.isNaN(n)) {
            setInvokeError(`"${opt.name}" must be a whole number.`);
            setInvoking(false);
            return;
          }
          payload[opt.name] = n;
          continue;
        }
        // STRING (or STRING with choices)
        payload[opt.name] = String(raw);
      }
      await invokeCommand(channelId, pickerCommand.id, payload);
      // Success: the response message arrives via the normal MESSAGE_CREATE WS.
      exitPicker('');
    } catch (err: unknown) {
      const msg =
        (err && typeof err === 'object' && 'message' in err && typeof (err as { message?: unknown }).message === 'string')
          ? (err as { message: string }).message
          : 'Command failed';
      setInvokeError(msg);
      setInvoking(false);
    }
  }, [pickerCommand, channelId, pickerValid, pickerValues, invoking, exitPicker]);

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
    // If the message has any :shortcode: candidates, ensure the emoji data is
    // loaded so resolveEmojis can replace them. Skip the await on the fast path.
    if (message.includes(':')) await getEmojiData();
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
      setUploadError(null);
      if (fileInputRef.current) fileInputRef.current.value = '';
      if (textareaRef.current) textareaRef.current.style.height = 'auto';
    } catch (err) {
      console.error('Failed to send message:', err);
      setUploadError("Couldn't upload file — try again?");
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
      // Slash command dropdown navigation (checked first; it takes priority over
      // the reply-cancel Escape since the user is clearly mid-selection).
      if (showSlashDropdown) {
        if (slashFiltered.length > 0) {
          if (e.key === 'ArrowDown') { e.preventDefault(); setSlashSelIdx(i => (i + 1) % slashFiltered.length); return; }
          if (e.key === 'ArrowUp') { e.preventDefault(); setSlashSelIdx(i => (i - 1 + slashFiltered.length) % slashFiltered.length); return; }
          if (e.key === 'Tab' || e.key === 'Enter') {
            e.preventDefault();
            handleSlashCommandSelect(slashFiltered[slashSelIdx]);
            return;
          }
        }
        if (e.key === 'Escape') {
          e.preventDefault();
          // Dropping the leading '/' dismisses the dropdown while preserving the user's other text.
          // In start-only mode there is no other text, so just clear.
          setMessage('');
          setCursorPos(0);
          return;
        }
      }
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
      if (e.key === 'Escape' && replyTo && onCancelReply) {
        e.preventDefault();
        onCancelReply();
        return;
      }
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSend(); }
    },
    [handleSend, mentionSuggestions, mentionSelIdx, handleMentionSelect, channelSuggestions, channelSelIdx, handleChannelSelect, emojiSuggestions, emojiSelIdx, handleEmojiSelect, message.length, replyTo, onCancelReply, showSlashDropdown, slashFiltered, slashSelIdx, handleSlashCommandSelect],
  );

  const handleChange = useCallback(
    (e: ChangeEvent<HTMLTextAreaElement>) => {
      setMessage(e.target.value);
      setCursorPos(e.target.selectionStart ?? 0);
      setMentionSelIdx(0);
      setChannelSelIdx(0);
      setEmojiSelIdx(0);
      setSlashSelIdx(0);
      const ta = e.target;
      ta.style.height = 'auto';
      ta.style.height = `${Math.min(ta.scrollHeight, 200)}px`;
      if (e.target.value.length > 0) onTyping?.();
    },
    [onTyping],
  );

  const handleFileChange = useCallback((e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) { setPendingFile(file); setVoiceBlob(null); setUploadError(null); }
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
    setUploadError(null);
  }, []);

  const handleAttachClick = useCallback(() => { fileInputRef.current?.click(); }, []);

  const handleRemoveFile = useCallback(() => {
    setPendingFile(null);
    setUploadError(null);
    if (fileInputRef.current) fileInputRef.current.value = '';
  }, []);

  // Drag and drop handlers
  const hasFileDrag = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    const types = e.dataTransfer?.types;
    if (!types) return false;
    for (let i = 0; i < types.length; i++) {
      if (types[i] === 'Files') return true;
    }
    return false;
  }, []);

  const handleDragEnter = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    if (!hasFileDrag(e)) return;
    e.preventDefault();
    e.stopPropagation();
    dragCounterRef.current += 1;
    setDragActive(true);
  }, [hasFileDrag]);

  const handleDragOver = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    if (!hasFileDrag(e)) return;
    e.preventDefault();
    e.stopPropagation();
  }, [hasFileDrag]);

  const handleDragLeave = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    if (!hasFileDrag(e)) return;
    e.preventDefault();
    e.stopPropagation();
    dragCounterRef.current -= 1;
    if (dragCounterRef.current <= 0) {
      dragCounterRef.current = 0;
      setDragActive(false);
    }
  }, [hasFileDrag]);

  const handleDrop = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    if (!hasFileDrag(e)) return;
    e.preventDefault();
    e.stopPropagation();
    dragCounterRef.current = 0;
    setDragActive(false);
    const file = e.dataTransfer.files?.[0];
    if (file) {
      setPendingFile(file);
      setVoiceBlob(null);
      setUploadError(null);
    }
  }, [hasFileDrag]);

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
    <div
      className="message-input-container"
      onDragOver={(e) => { if (hasFileDrag(e)) e.preventDefault(); }}
      onDrop={(e) => { if (hasFileDrag(e)) e.preventDefault(); }}
    >
      {voiceBlob && voiceBlobUrl && (
        <div className="attachment-preview attachment-preview--voice">
          <AudioPlayer url={voiceBlobUrl} isVoiceMessage />
          <button type="button" className="attachment-preview-remove" onClick={cancelVoice} title="Discard">✕</button>
        </div>
      )}

      {uploadError && (
        <div className="upload-error" role="alert">
          <span className="upload-error-text">{uploadError}</span>
          <button
            type="button"
            className="upload-error-retry"
            onClick={handleSend}
            disabled={isUploading}
          >
            Retry
          </button>
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
          <span className="attachment-preview-size">• {formatFileSize(pendingFile.size)}</span>
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
        {showSlashDropdown && (
          <SlashCommandDropdown
            suggestions={slashFiltered}
            selectedIdx={slashSelIdx}
            onSelect={handleSlashCommandSelect}
            empty={serverHasNoCommands}
            loading={commandsLoading && allCommands.length === 0}
          />
        )}

        <input
          ref={fileInputRef}
          type="file"
          style={{ display: 'none' }}
          onChange={handleFileChange}
          disabled={isBusy}
        />

        <div
          className={`message-input-wrapper${dragActive ? ' drag-active' : ''}`}
          style={{ position: 'relative' }}
          onDragEnter={handleDragEnter}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={handleDrop}
        >
          {dragActive && (
            <div className="drag-overlay">Drop to attach</div>
          )}
          {!pickerCommand && canAttach && (
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
          {!pickerCommand && (
          <button
            type="button"
            className={`input-icon-btn gif-btn${showGifPicker ? ' gif-btn--active' : ''}`}
            onClick={() => setShowGifPicker(p => !p)}
            disabled={isBusy || isRecording}
            title="Send a GIF"
          >
            <GifIcon />
          </button>
          )}
          {pickerCommand ? (
            <SlashOptionPicker
              command={pickerCommand}
              values={pickerValues}
              onChangeValue={(name, v) => setPickerValues(prev => ({ ...prev, [name]: v }))}
              inputRefs={pickerInputRefs}
              invoking={invoking}
              error={invokeError}
              onCancel={() => exitPicker(`/${pickerCommand.name}`)}
              onSubmit={runInvoke}
              canSubmit={pickerValid && !invoking}
            />
          ) : (
            <>
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
            </>
          )}
          {(() => {
            if (pickerCommand) {
              const disabled = !pickerValid || invoking;
              return (
                <button
                  className="send-button"
                  onClick={runInvoke}
                  disabled={disabled}
                  aria-disabled={disabled}
                >
                  {invoking ? '...' : 'Run'}
                </button>
              );
            }
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

        {!pickerCommand && (
          <button
            type="button"
            className={`input-icon-btn mic-btn${isRecording ? ' mic-btn--recording' : ''}`}
            onClick={handleMicClick}
            disabled={isBusy && !isRecording}
            title={isRecording ? 'Stop recording' : 'Voice message'}
          >
            <MicIcon active={isRecording} />
          </button>
        )}
      </div>
    </div>
  );
};

// -------------------- Slash option-picker sub-component --------------------

interface SlashOptionPickerProps {
  command: BotCommand;
  values: Record<string, unknown>;
  onChangeValue: (name: string, value: unknown) => void;
  inputRefs: React.MutableRefObject<Array<HTMLInputElement | HTMLSelectElement | null>>;
  invoking: boolean;
  error: string | null;
  onCancel: () => void;
  onSubmit: () => void;
  canSubmit: boolean;
}

const SlashOptionPicker: React.FC<SlashOptionPickerProps> = ({
  command,
  values,
  onChangeValue,
  inputRefs,
  invoking,
  error,
  onCancel,
  onSubmit,
  canSubmit,
}) => {
  const options = command.options ?? [];

  // Make sure the refs array is sized to the option count.
  if (inputRefs.current.length !== options.length) {
    inputRefs.current = new Array(options.length).fill(null);
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement | HTMLSelectElement>, idx: number) => {
    if (e.key === 'Escape') {
      e.preventDefault();
      onCancel();
      return;
    }
    if (e.key === 'Enter') {
      e.preventDefault();
      if (canSubmit) onSubmit();
      return;
    }
    if (e.key === 'Tab') {
      // Cycle focus through picker inputs. If we fall off the end we let the
      // browser take over (so the Cancel/Run buttons remain reachable).
      const next = e.shiftKey ? idx - 1 : idx + 1;
      if (next >= 0 && next < options.length) {
        e.preventDefault();
        inputRefs.current[next]?.focus();
      }
    }
  };

  const renderOption = (opt: SlashCommandOption, idx: number) => {
    const raw = values[opt.name];
    const label = (
      <span className="slash-picker-option-label">
        {opt.name}
        {opt.required && <span className="slash-picker-option-required" aria-label="required">*</span>}
      </span>
    );

    // STRING with choices → <select>
    if (opt.type === 'STRING' && opt.choices && opt.choices.length > 0) {
      return (
        <span key={opt.name} className="slash-picker-option">
          {label}
          <select
            className="slash-picker-select"
            ref={el => { inputRefs.current[idx] = el; }}
            value={raw === undefined ? '' : String(raw)}
            onChange={e => onChangeValue(opt.name, e.target.value)}
            onKeyDown={e => handleKeyDown(e, idx)}
            disabled={invoking}
            title={opt.description}
          >
            <option value="" disabled={opt.required}>
              {opt.required ? 'Choose…' : '—'}
            </option>
            {opt.choices.map(ch => (
              <option key={String(ch.value)} value={String(ch.value)}>{ch.name}</option>
            ))}
          </select>
        </span>
      );
    }

    // BOOLEAN → two-button pill
    if (opt.type === 'BOOLEAN') {
      const current = Boolean(raw);
      return (
        <span key={opt.name} className="slash-picker-option">
          {label}
          <span className="slash-picker-toggle" title={opt.description}>
            <button
              type="button"
              className={current ? 'active' : ''}
              onClick={() => onChangeValue(opt.name, true)}
              disabled={invoking}
            >Yes</button>
            <button
              type="button"
              className={!current ? 'active' : ''}
              onClick={() => onChangeValue(opt.name, false)}
              disabled={invoking}
            >No</button>
          </span>
          {/* Hidden focus-proxy so Tab cycling can hop over BOOLEAN inputs
              without breaking the indexed ref array. */}
          <input
            ref={el => { inputRefs.current[idx] = el; }}
            type="text"
            readOnly
            style={{ position: 'absolute', width: 0, height: 0, opacity: 0, pointerEvents: 'none' }}
            tabIndex={-1}
            aria-hidden="true"
            defaultValue={current ? 'yes' : 'no'}
            onKeyDown={e => handleKeyDown(e, idx)}
          />
        </span>
      );
    }

    // STRING / INTEGER (plain input)
    const isInt = opt.type === 'INTEGER';
    return (
      <span key={opt.name} className="slash-picker-option">
        {label}
        <input
          className="slash-picker-input"
          ref={el => { inputRefs.current[idx] = el; }}
          type={isInt ? 'number' : 'text'}
          step={isInt ? 1 : undefined}
          min={opt.min_value}
          max={opt.max_value}
          minLength={opt.min_length}
          maxLength={opt.max_length}
          placeholder={opt.description || opt.name}
          value={raw === undefined || raw === null ? '' : String(raw)}
          onChange={e => onChangeValue(opt.name, e.target.value)}
          onKeyDown={e => handleKeyDown(e, idx)}
          disabled={invoking}
          title={opt.description}
        />
      </span>
    );
  };

  return (
    <div className="slash-picker" role="group" aria-label={`Options for /${command.name}`}>
      <span className="slash-picker-chip">/{command.name}</span>
      {options.map((opt, i) => renderOption(opt, i))}
      {invoking && <span className="slash-picker-running">Running…</span>}
      <button
        type="button"
        className="slash-picker-cancel"
        onClick={onCancel}
        disabled={invoking}
      >
        Cancel
      </button>
      {error && (
        <div className="slash-picker-error" role="alert">
          <span>Command failed: {error}</span>
          <button
            type="button"
            className="slash-picker-error-retry"
            onClick={onSubmit}
            disabled={!canSubmit}
          >
            Retry
          </button>
        </div>
      )}
    </div>
  );
};
