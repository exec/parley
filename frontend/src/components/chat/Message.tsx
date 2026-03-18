import React, { useState, useEffect, useRef } from 'react';
import { useViewportAdjust } from '../../hooks/useViewportAdjust';
import { Reply, SmilePlus, Pencil, Trash2, X, Bot } from 'lucide-react';
import { Message as MessageType } from '../../api/types';
import { Avatar } from '../ui/Avatar';
import { EmojiPicker } from './EmojiPicker';
import MarkdownRenderer from '../ui/MarkdownRenderer';
import { AudioPlayer } from './AudioPlayer';
import { CodeBlock } from '../ui/CodeBlock';
import { isCodeFile, languageFromFilename } from '../../lib/shiki';
import { getParentAuthor } from './NestedReplies';
import { EditHistoryPopover } from './EditHistoryPopover';
import { ThemeLinkEmbed } from '../theme/ThemeLinkEmbed';
import './Chat.css';
import './NestedReplies.css';

const THEME_URL_RE = /https?:\/\/[^/\s]+\/theme\/([0-9a-f-]{36})/gi;

function extractThemeTokens(content: string): string[] {
  const tokens: string[] = [];
  let m: RegExpExecArray | null;
  THEME_URL_RE.lastIndex = 0;
  while ((m = THEME_URL_RE.exec(content)) !== null) tokens.push(m[1]);
  return [...new Set(tokens)];
}

function stripThemeURLs(content: string): string {
  THEME_URL_RE.lastIndex = 0;
  return content.replace(THEME_URL_RE, '').trim();
}

interface MessageProps {
  message: MessageType;
  currentUserId?: string;
  isGrouped?: boolean;
  memberMap?: Map<string, string>;
  channelMap?: Map<string, string>;
  messages?: MessageType[];
  onEdit?: (message: MessageType) => void;
  onDelete?: (messageId: string) => void;
  onReact?: (messageId: string, emoji: string) => void;
  onReply?: (message: MessageType) => void;
  onViewProfile?: (userId: string, username: string) => void;
  onSendMessage?: (userId: string) => void;
  onMiniProfile?: (userId: string, e: React.MouseEvent) => void;
  onScrollToMessage?: (messageId: string) => void;
  canManageMessages?: boolean;
  canAddReactions?: boolean;
}

/** Returns the number of emoji if the text is 1–5 emoji only, else null. */
function getEmojiOnlyCount(text: string): number | null {
  const t = text.trim();
  if (!t) return null;
  // Strip all emoji-related codepoints; if anything non-whitespace remains, not emoji-only
  const stripped = t.replace(/\p{Extended_Pictographic}|\p{Emoji_Modifier}|\p{Regional_Indicator}|\uFE0F|\uFE0E|\u200D/gu, '').trim();
  if (stripped.length > 0) return null;
  // Count distinct emoji sequences (flag = two regional indicators, others = pictographic + modifiers)
  const matches = t.match(/\p{Regional_Indicator}{2}|\p{Extended_Pictographic}[\p{Emoji_Modifier}\uFE0F\uFE0E\u200D\p{Extended_Pictographic}]*/gu);
  if (!matches || matches.length < 1 || matches.length > 5) return null;
  return matches.length;
}

const FileIcon = () => (
  <svg width="13" height="16" viewBox="0 0 13 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" style={{ flexShrink: 0 }}>
    <path d="M7.5 1H2a1 1 0 0 0-1 1v12a1 1 0 0 0 1 1h9a1 1 0 0 0 1-1V5.5L7.5 1Z" />
    <polyline points="7.5,1 7.5,5.5 12,5.5" />
  </svg>
);

const MAX_PREVIEW_BYTES = 100 * 1024; // 100 KB
const COLLAPSE_LINES = 50;

interface CodeAttachmentProps {
  url: string;
  filename: string;
}

const CodeAttachment: React.FC<CodeAttachmentProps> = ({ url, filename }) => {
  const [content, setContent] = useState<string | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();

    fetch(url, { signal: controller.signal })
      .then(async (res) => {
        const lengthHeader = res.headers.get('content-length');
        if (lengthHeader && parseInt(lengthHeader, 10) > MAX_PREVIEW_BYTES) {
          setError(true);
          return;
        }
        const text = await res.text();
        if (cancelled) return;
        if (text.length > MAX_PREVIEW_BYTES) {
          setError(true);
        } else {
          setContent(text);
        }
      })
      .catch(() => {
        if (!cancelled) setError(true);
      });

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [url]);

  if (error) {
    return (
      <a href={url} target="_blank" rel="noopener noreferrer" className="message-attachment-file">
        <FileIcon /> {filename}
      </a>
    );
  }

  if (content === null) {
    return (
      <a href={url} target="_blank" rel="noopener noreferrer" className="message-attachment-file">
        <FileIcon /> {filename}
      </a>
    );
  }

  const lineCount = content.split('\n').length;
  const lang = languageFromFilename(filename);

  return (
    <CodeBlock
      content={content}
      language={lang}
      filename={filename}
      showLineNumbers
      collapsible
      defaultCollapsed={lineCount > COLLAPSE_LINES}
    />
  );
};

export const Message: React.FC<MessageProps> = ({
  message,
  currentUserId,
  isGrouped = false,
  memberMap,
  channelMap,
  messages,
  onEdit,
  onDelete,
  onReact,
  onReply,
  onViewProfile,
  onSendMessage,
  onMiniProfile,
  onScrollToMessage,
  canManageMessages = true,
  canAddReactions = true,
}) => {
  const [showActions, setShowActions] = useState(false);
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number } | null>(null);
  const [userContextMenu, setUserContextMenu] = useState<{ x: number; y: number } | null>(null);
  const [showEmojiPicker, setShowEmojiPicker] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [editValue, setEditValue] = useState(message.content);
  const [lightboxUrl, setLightboxUrl] = useState<string | null>(null);
  const [showEditHistory, setShowEditHistory] = useState(false);
  const editRef = useRef<HTMLTextAreaElement>(null);
  const emojiPickerRef = useRef<HTMLDivElement>(null);
  const contextMenuRef = useRef<HTMLDivElement>(null);
  const userContextMenuRef = useRef<HTMLDivElement>(null);
  const editedSpanRef = useRef<HTMLSpanElement>(null);

  const isOwnMessage = currentUserId && message.author_id === currentUserId;

  const formatTimestamp = (dateString: string): string => {
    const date = new Date(dateString);
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  };

  // Focus edit textarea when editing starts
  useEffect(() => {
    if (isEditing && editRef.current) {
      editRef.current.focus();
      editRef.current.setSelectionRange(editRef.current.value.length, editRef.current.value.length);
    }
  }, [isEditing]);

  const handleEdit = (e: React.MouseEvent) => {
    e.stopPropagation();
    setEditValue(message.content);
    setIsEditing(true);
    closeContextMenu();
  };

  const handleEditSubmit = () => {
    if (editValue.trim() && editValue.trim() !== message.content) {
      onEdit?.({ ...message, content: editValue.trim() });
    }
    setIsEditing(false);
  };

  const handleEditKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleEditSubmit();
    } else if (e.key === 'Escape') {
      setIsEditing(false);
      setEditValue(message.content);
    }
  };

  const handleDelete = (e: React.MouseEvent) => {
    e.stopPropagation();
    onDelete?.(message.id);
    closeContextMenu();
  };

  const handleReply = (e: React.MouseEvent) => {
    e.stopPropagation();
    onReply?.(message);
    closeContextMenu();
  };

  const handleViewProfile = (e: React.MouseEvent) => {
    e.stopPropagation();
    onViewProfile?.(message.author_id, message.author_username);
    closeContextMenu();
    closeUserContextMenu();
  };

  const handleSendMessage = (e: React.MouseEvent) => {
    e.stopPropagation();
    onSendMessage?.(message.author_id);
    closeContextMenu();
    closeUserContextMenu();
  };

  const handleContextMenu = (e: React.MouseEvent) => {
    e.preventDefault();
    setUserContextMenu(null);
    setContextMenu({ x: e.clientX, y: e.clientY });
  };

  // Right-clicking specifically on the username or avatar shows a focused user menu
  const handleUsernameContextMenu = (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation(); // prevent message-level context menu from also firing
    setContextMenu(null);
    setUserContextMenu({ x: e.clientX, y: e.clientY });
  };

  const closeContextMenu = () => setContextMenu(null);
  const closeUserContextMenu = () => setUserContextMenu(null);

  useEffect(() => {
    if (!contextMenu) return;
    const handleClick = () => closeContextMenu();
    document.addEventListener('click', handleClick);
    return () => document.removeEventListener('click', handleClick);
  }, [contextMenu]);

  useEffect(() => {
    if (!userContextMenu) return;
    const handleClick = () => closeUserContextMenu();
    document.addEventListener('click', handleClick);
    return () => document.removeEventListener('click', handleClick);
  }, [userContextMenu]);

  const handleCopy = () => {
    navigator.clipboard.writeText(message.content);
    closeContextMenu();
  };

  const handleReact = (emoji: string) => {
    onReact?.(message.id, emoji);
    setShowEmojiPicker(false);
  };

  // Close emoji picker when clicking outside
  useEffect(() => {
    if (!showEmojiPicker) return;
    const handleClick = (e: MouseEvent) => {
      if (emojiPickerRef.current && !emojiPickerRef.current.contains(e.target as Node)) {
        setShowEmojiPicker(false);
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [showEmojiPicker]);

  useViewportAdjust(contextMenuRef, [contextMenu]);
  useViewportAdjust(userContextMenuRef, [userContextMenu]);
  useViewportAdjust(emojiPickerRef, [showEmojiPicker]);

  const wasEdited = message.updated_at !== message.created_at;

  const parentAuthor = message.parent_id && messages
    ? getParentAuthor(message, messages)
    : null;

  return (
    <>
    {message.parent_id && (
      <div
        className="reply-indicator"
        style={{ cursor: 'pointer' }}
        onClick={() => onScrollToMessage?.(message.parent_id!)}
      >
        <span><Reply size={12} color="currentColor" /> replying to</span>
        <span className="reply-indicator-name">
          @{parentAuthor ?? 'unknown'}
        </span>
      </div>
    )}
    <div
      id={`message-${message.id}`}
      className={`message${message.pending ? ' message-pending' : ''}${isGrouped ? ' message-grouped' : ''}`}
      onMouseEnter={() => setShowActions(true)}
      onMouseLeave={() => setShowActions(false)}
      onContextMenu={handleContextMenu}
    >
      {isGrouped ? (
        <div className="message-avatar-grouped">
          <span className="message-group-time">{formatTimestamp(message.created_at)}</span>
        </div>
      ) : (
        <div className="message-avatar">
          <div
            className="message-avatar-clickable"
            onClick={(e) => onMiniProfile ? onMiniProfile(message.author_id, e) : onViewProfile?.(message.author_id, message.author_username)}
            onContextMenu={handleUsernameContextMenu}
            title="Left-click to view profile · Right-click for options"
          >
            <Avatar
              src={message.author_avatar_url || undefined}
              alt={message.author_username || 'User'}
              fallback={message.author_username || 'User'}
              size="md"
            />
          </div>
        </div>
      )}
      <div className="message-content">
        {!isGrouped && (
        <div className="message-header">
          <span
            className="message-author"
            onClick={(e) => onMiniProfile ? onMiniProfile(message.author_id, e) : onViewProfile?.(message.author_id, message.author_username)}
            onContextMenu={handleUsernameContextMenu}
            title="Left-click to view profile · Right-click for options"
          >
            {message.author_display_name || message.author_username || 'Unknown User'}
          </span>
          {message.author_is_bot && (
            <span className="msg-badge bot" title="Bot">BOT</span>
          )}
          {message.via_api && !message.author_is_bot && (
            <span className="msg-badge selfbot">
              <Bot size={12} color="currentColor" />
              <span className="msg-badge-tooltip">Sent via Selfbot</span>
            </span>
          )}
          <span className="message-timestamp">{formatTimestamp(message.created_at)}</span>
          {wasEdited && (
            <span
              ref={editedSpanRef}
              className="message-edited"
              style={{ cursor: 'pointer', position: 'relative' }}
              onClick={(e) => { e.stopPropagation(); setShowEditHistory(p => !p); }}
              title="View edit history"
            >
              (edited)
              {showEditHistory && (
                <EditHistoryPopover
                  messageId={message.id}
                  onClose={() => setShowEditHistory(false)}
                />
              )}
            </span>
          )}
        </div>
        )}

        {isEditing ? (
          <div className="message-edit-container">
            <textarea
              ref={editRef}
              className="message-edit-input"
              value={editValue}
              onChange={e => setEditValue(e.target.value)}
              onKeyDown={handleEditKeyDown}
              rows={2}
            />
            <div className="message-edit-hint">
              <span>escape to <button className="edit-link-btn" onClick={() => { setIsEditing(false); setEditValue(message.content); }}>cancel</button></span>
              <span>enter to <button className="edit-link-btn" onClick={handleEditSubmit}>save</button></span>
            </div>
          </div>
        ) : (
          <>
            {getEmojiOnlyCount(message.content)
              ? <div className="message-text message-text--jumbo">{message.content}</div>
              : (() => {
                  const tokens = extractThemeTokens(message.content);
                  const cleanContent = tokens.length > 0 ? stripThemeURLs(message.content) : message.content;
                  return (
                    <>
                      {cleanContent && (
                        <div className="message-text">
                          <MarkdownRenderer content={cleanContent} mode="chat" memberMap={memberMap} channelMap={channelMap} onMiniProfile={onMiniProfile} />
                        </div>
                      )}
                      {tokens.map(tok => <ThemeLinkEmbed key={tok} token={tok} />)}
                    </>
                  );
                })()
            }
            {message.attachment_url && (() => {
              const isVoice = message.attachment_name?.startsWith('voice_message_');
              const isAudio = !isVoice && (
                message.attachment_type?.startsWith('audio/') ||
                /\.(mp3|ogg|wav|webm|m4a|aac|flac)(\?|$)/i.test(message.attachment_url)
              );
              return (
                <div className="message-attachment">
                  {message.attachment_type?.startsWith('image/') ? (
                    <img
                      src={message.attachment_url}
                      alt={message.attachment_name || 'attachment'}
                      className="message-attachment-image"
                      style={{ maxWidth: '400px', maxHeight: '300px', borderRadius: '4px', marginTop: '4px', cursor: 'zoom-in' }}
                      onClick={() => setLightboxUrl(message.attachment_url!)}
                    />
                  ) : isVoice || isAudio ? (
                    <AudioPlayer
                      url={message.attachment_url}
                      isVoiceMessage={isVoice}
                      filename={isAudio ? (message.attachment_name || undefined) : undefined}
                    />
                  ) : message.attachment_name && isCodeFile(message.attachment_name) ? (
                    <CodeAttachment
                      url={message.attachment_url}
                      filename={message.attachment_name}
                    />
                  ) : (
                    <a
                      href={message.attachment_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="message-attachment-file"
                    >
                      <FileIcon /> {message.attachment_name || 'attachment'}
                    </a>
                  )}
                </div>
              );
            })()}
          </>
        )}

        {(message.reactions?.length ?? 0) > 0 && (
          <div className="message-reactions">
            {message.reactions!.map(reaction => (
              <button
                key={reaction.emoji}
                className={`reaction-pill${reaction.user_ids.includes(currentUserId ?? '') ? ' reacted' : ''}`}
                onClick={() => handleReact(reaction.emoji)}
                title={reaction.user_ids.length <= 5 ? reaction.user_ids.join(', ') : `${reaction.count} reactions`}
              >
                {reaction.emoji} {reaction.count}
              </button>
            ))}
          </div>
        )}
      </div>

      {showActions && !isEditing && (
        <div className="message-actions">
          <button className="message-action-btn" onClick={handleReply} title="Reply"><Reply size={14} color="currentColor" /></button>
          {canAddReactions && (
            <button className="message-action-btn" onClick={(e) => { e.stopPropagation(); setShowEmojiPicker(p => !p); }} title="Add reaction"><SmilePlus size={14} color="currentColor" /></button>
          )}
          {isOwnMessage && (
            <>
              <button className="message-action-btn" onClick={handleEdit} title="Edit"><Pencil size={14} color="currentColor" /></button>
              <button className="message-action-btn delete" onClick={handleDelete} title="Delete"><Trash2 size={14} color="currentColor" /></button>
            </>
          )}
          {!isOwnMessage && canManageMessages && (
            <button className="message-action-btn delete" onClick={handleDelete} title="Delete (Manage Messages)"><Trash2 size={14} color="currentColor" /></button>
          )}
        </div>
      )}

      {showEmojiPicker && (
        <EmojiPicker ref={emojiPickerRef} onSelect={handleReact} onClose={() => setShowEmojiPicker(false)} />
      )}

      {contextMenu && (
        <div
          ref={contextMenuRef}
          className="message-context-menu"
          style={{ top: contextMenu.y, left: contextMenu.x }}
          onClick={e => e.stopPropagation()}
        >
          <button className="context-menu-item" onClick={handleReply}>Reply</button>
          <button className="context-menu-item" onClick={handleCopy}>Copy Text</button>
          {message.author_id !== currentUserId && (
            <>
              <div className="context-menu-divider" />
              <button className="context-menu-item" onClick={handleSendMessage}>Send Message</button>
              <button className="context-menu-item" onClick={handleViewProfile}>View Profile</button>
            </>
          )}
          {isOwnMessage && (
            <>
              <button className="context-menu-item" onClick={handleEdit}>Edit Message</button>
              <div className="context-menu-divider" />
              <button className="context-menu-item danger" onClick={handleDelete}>Delete Message</button>
            </>
          )}
          {!isOwnMessage && canManageMessages && (
            <>
              <div className="context-menu-divider" />
              <button className="context-menu-item danger" onClick={handleDelete}>Delete Message</button>
            </>
          )}
        </div>
      )}

      {userContextMenu && (
        <div
          ref={userContextMenuRef}
          className="message-context-menu user-context-menu-popup"
          style={{ top: userContextMenu.y, left: userContextMenu.x }}
          onClick={e => e.stopPropagation()}
        >
          <div className="context-menu-username">{message.author_display_name || message.author_username}</div>
          <div className="context-menu-divider" />
          <button className="context-menu-item" onClick={handleViewProfile}>View Profile</button>
          {message.author_id !== currentUserId && (
            <button className="context-menu-item" onClick={handleSendMessage}>Send Message</button>
          )}
        </div>
      )}
    </div>
    {lightboxUrl && (
      <div
        style={{
          position: 'fixed', inset: 0, zIndex: 9999,
          background: 'rgba(0,0,0,0.85)',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
        }}
        onClick={() => setLightboxUrl(null)}
      >
        <button
          onClick={e => { e.stopPropagation(); setLightboxUrl(null); }}
          style={{
            position: 'absolute', top: 16, right: 20,
            background: 'none', border: 'none', color: '#fff',
            fontSize: 32, cursor: 'pointer', lineHeight: 1, padding: '4px 8px',
          }}
          title="Close"
        >
          <X size={18} color="currentColor" />
        </button>
        <img
          src={lightboxUrl}
          alt="Full size"
          style={{ maxWidth: '90vw', maxHeight: '90vh', borderRadius: 8, objectFit: 'contain' }}
          onClick={e => e.stopPropagation()}
        />
      </div>
    )}
    </>
  );
};
