import React, { useState, useEffect, useRef } from 'react';
import { Message as MessageType } from '../../api/types';
import { Avatar } from '../ui/Avatar';
import './Chat.css';

const COMMON_EMOJIS = ['👍', '❤️', '😂', '😮', '😢', '😡', '🎉', '🔥', '👀', '✅', '💯', '🚀'];

interface MessageProps {
  message: MessageType;
  currentUserId?: string;
  onEdit?: (message: MessageType) => void;
  onDelete?: (messageId: string) => void;
  onReact?: (messageId: string, emoji: string) => void;
  onReply?: (message: MessageType) => void;
  onViewProfile?: (userId: string, username: string) => void;
  onSendMessage?: (userId: string) => void;
}

export const Message: React.FC<MessageProps> = ({
  message,
  currentUserId,
  onEdit,
  onDelete,
  onReact,
  onReply,
  onViewProfile,
  onSendMessage,
}) => {
  const [showActions, setShowActions] = useState(false);
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number } | null>(null);
  const [userContextMenu, setUserContextMenu] = useState<{ x: number; y: number } | null>(null);
  const [showEmojiPicker, setShowEmojiPicker] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [editValue, setEditValue] = useState(message.content);
  const editRef = useRef<HTMLTextAreaElement>(null);
  const emojiPickerRef = useRef<HTMLDivElement>(null);

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

  const wasEdited = message.updated_at !== message.created_at;

  return (
    <div
      className={`message${message.pending ? ' message-pending' : ''}`}
      onMouseEnter={() => setShowActions(true)}
      onMouseLeave={() => setShowActions(false)}
      onContextMenu={handleContextMenu}
    >
      <div className="message-avatar">
        <div
          className="message-avatar-clickable"
          onClick={() => onViewProfile?.(message.author_id, message.author_username)}
          onContextMenu={handleUsernameContextMenu}
          title="Left-click to view profile · Right-click for options"
        >
          <Avatar
            alt={message.author_username || 'User'}
            fallback={message.author_username || 'User'}
            size="md"
          />
        </div>
      </div>
      <div className="message-content">
        <div className="message-header">
          <span
            className="message-author"
            onClick={() => onViewProfile?.(message.author_id, message.author_username)}
            onContextMenu={handleUsernameContextMenu}
            title="Left-click to view profile · Right-click for options"
          >
            {message.author_username || 'Unknown User'}
          </span>
          <span className="message-timestamp">{formatTimestamp(message.created_at)}</span>
          {wasEdited && <span className="message-edited">(edited)</span>}
        </div>

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
            <div className="message-text">{message.content}</div>
            {message.attachment_url && (
              <div className="message-attachment">
                {message.attachment_type?.startsWith('image/') ? (
                  <img
                    src={message.attachment_url}
                    alt={message.attachment_name || 'attachment'}
                    className="message-attachment-image"
                    style={{ maxWidth: '400px', maxHeight: '300px', borderRadius: '4px', marginTop: '4px' }}
                  />
                ) : (
                  <a
                    href={message.attachment_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="message-attachment-file"
                  >
                    📎 {message.attachment_name || 'attachment'}
                  </a>
                )}
              </div>
            )}
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
          <button className="message-action-btn" onClick={handleReply} title="Reply">↩</button>
          <button className="message-action-btn" onClick={(e) => { e.stopPropagation(); setShowEmojiPicker(p => !p); }} title="Add reaction">😊</button>
          {isOwnMessage && (
            <>
              <button className="message-action-btn" onClick={handleEdit} title="Edit">✎</button>
              <button className="message-action-btn delete" onClick={handleDelete} title="Delete">🗑</button>
            </>
          )}
        </div>
      )}

      {showEmojiPicker && (
        <div ref={emojiPickerRef} className="emoji-picker">
          {COMMON_EMOJIS.map(emoji => (
            <button key={emoji} className="emoji-picker-btn" onClick={() => handleReact(emoji)}>
              {emoji}
            </button>
          ))}
        </div>
      )}

      {contextMenu && (
        <div
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
        </div>
      )}

      {userContextMenu && (
        <div
          className="message-context-menu user-context-menu-popup"
          style={{ top: userContextMenu.y, left: userContextMenu.x }}
          onClick={e => e.stopPropagation()}
        >
          <div className="context-menu-username">{message.author_username}</div>
          <div className="context-menu-divider" />
          <button className="context-menu-item" onClick={handleViewProfile}>View Profile</button>
          {message.author_id !== currentUserId && (
            <button className="context-menu-item" onClick={handleSendMessage}>Send Message</button>
          )}
        </div>
      )}
    </div>
  );
};
