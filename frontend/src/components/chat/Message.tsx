import React, { useState, useEffect, useRef } from 'react';
import { Message as MessageType } from '../../api/types';
import { Avatar } from '../ui/Avatar';
import './Chat.css';

interface MessageProps {
  message: MessageType;
  currentUserId?: string;
  onEdit?: (message: MessageType) => void;
  onDelete?: (messageId: string) => void;
  onReply?: (message: MessageType) => void;
}

export const Message: React.FC<MessageProps> = ({
  message,
  currentUserId,
  onEdit,
  onDelete,
  onReply,
}) => {
  const [showActions, setShowActions] = useState(false);
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number } | null>(null);
  const [isEditing, setIsEditing] = useState(false);
  const [editValue, setEditValue] = useState(message.content);
  const editRef = useRef<HTMLTextAreaElement>(null);

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

  const handleContextMenu = (e: React.MouseEvent) => {
    e.preventDefault();
    setContextMenu({ x: e.clientX, y: e.clientY });
  };

  const closeContextMenu = () => setContextMenu(null);

  useEffect(() => {
    if (!contextMenu) return;
    const handleClick = () => closeContextMenu();
    document.addEventListener('click', handleClick);
    return () => document.removeEventListener('click', handleClick);
  }, [contextMenu]);

  const handleCopy = () => {
    navigator.clipboard.writeText(message.content);
    closeContextMenu();
  };

  const wasEdited = message.updated_at !== message.created_at;

  return (
    <div
      className="message"
      onMouseEnter={() => setShowActions(true)}
      onMouseLeave={() => setShowActions(false)}
      onContextMenu={handleContextMenu}
    >
      <div className="message-avatar">
        <Avatar
          alt={message.author_username || 'User'}
          fallback={message.author_username || 'User'}
          size="md"
        />
      </div>
      <div className="message-content">
        <div className="message-header">
          <span className="message-author">{message.author_username || 'Unknown User'}</span>
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
          <div className="message-text">{message.content}</div>
        )}
      </div>

      {showActions && !isEditing && (
        <div className="message-actions">
          <button className="message-action-btn" onClick={handleReply} title="Reply">↩</button>
          {isOwnMessage && (
            <>
              <button className="message-action-btn" onClick={handleEdit} title="Edit">✎</button>
              <button className="message-action-btn delete" onClick={handleDelete} title="Delete">🗑</button>
            </>
          )}
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
          {isOwnMessage && (
            <>
              <button className="context-menu-item" onClick={handleEdit}>Edit Message</button>
              <div className="context-menu-divider" />
              <button className="context-menu-item danger" onClick={handleDelete}>Delete Message</button>
            </>
          )}
        </div>
      )}
    </div>
  );
};
