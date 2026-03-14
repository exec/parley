import React, { useState, useEffect } from 'react';
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

  const isOwnMessage = currentUserId && message.author_id === currentUserId;

  const formatTimestamp = (dateString: string): string => {
    const date = new Date(dateString);
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  };

  const handleEdit = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (onEdit) {
      onEdit(message);
    }
  };

  const handleDelete = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (onDelete) {
      onDelete(message.id);
    }
  };

  const handleReply = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (onReply) {
      onReply(message);
    }
  };

  const handleContextMenu = (e: React.MouseEvent) => {
    e.preventDefault();
    setContextMenu({ x: e.clientX, y: e.clientY });
  };

  const closeContextMenu = () => {
    setContextMenu(null);
  };

  // Close context menu on click outside
  useEffect(() => {
    const handleClick = () => closeContextMenu();
    if (contextMenu) {
      document.addEventListener('click', handleClick);
      return () => document.removeEventListener('click', handleClick);
    }
  }, [contextMenu]);

  const handleContextMenuReply = () => {
    if (onReply) {
      onReply(message);
    }
    closeContextMenu();
  };

  const handleContextMenuCopy = () => {
    navigator.clipboard.writeText(message.content);
    closeContextMenu();
  };

  const handleContextMenuDelete = () => {
    if (onDelete) {
      onDelete(message.id);
    }
    closeContextMenu();
  };

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
          <span className="message-author">
            {message.author_username || 'Unknown User'}
          </span>
          <span className="message-timestamp">
            {formatTimestamp(message.created_at)}
          </span>
        </div>
        <div className="message-text">{message.content}</div>
      </div>
      {showActions && (
        <div className="message-actions">
          <button
            className="message-action-btn"
            onClick={handleReply}
            title="Reply"
          >
            Reply
          </button>
          {isOwnMessage && (
            <>
              <button
                className="message-action-btn"
                onClick={handleEdit}
                title="Edit"
              >
                Edit
              </button>
              <button
                className="message-action-btn delete"
                onClick={handleDelete}
                title="Delete"
              >
                Delete
              </button>
            </>
          )}
        </div>
      )}

      {contextMenu && (
        <div
          className="message-context-menu"
          style={{ top: contextMenu.y, left: contextMenu.x }}
          onClick={(e) => e.stopPropagation()}
        >
          <button className="context-menu-item" onClick={handleContextMenuReply}>
            Reply
          </button>
          <button className="context-menu-item" onClick={handleContextMenuCopy}>
            Copy Message
          </button>
          {isOwnMessage && (
            <>
              <div className="context-menu-divider" />
              <button className="context-menu-item danger" onClick={handleContextMenuDelete}>
                Delete
              </button>
            </>
          )}
        </div>
      )}

      {contextMenu && (
        <div
          className="message-context-menu"
          style={{ top: contextMenu.y, left: contextMenu.x }}
          onClick={(e) => e.stopPropagation()}
        >
          <button className="context-menu-item" onClick={handleContextMenuReply}>
            Reply
          </button>
          <button className="context-menu-item" onClick={handleContextMenuCopy}>
            Copy Message
          </button>
          {isOwnMessage && (
            <>
              <div className="context-menu-divider" />
              <button className="context-menu-item danger" onClick={handleContextMenuDelete}>
                Delete
              </button>
            </>
          )}
        </div>
      )}
    </div>
  );
};