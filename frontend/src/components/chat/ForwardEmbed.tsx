import React from 'react';
import { ForwardedMessage } from '../../api/types';
import MarkdownRenderer from '../ui/MarkdownRenderer';

interface ForwardEmbedProps {
  fwd: ForwardedMessage;
  memberMap?: Map<string, string>;
  channelMap?: Map<string, string>;
  onJump?: (channelId: string, messageId: string) => void;
}

export const ForwardEmbed: React.FC<ForwardEmbedProps> = ({ fwd, memberMap, channelMap, onJump }) => {
  const isFromDm = !fwd.server_id;
  const canJump = !isFromDm && !!fwd.channel_id && !!onJump;

  const handleClick = () => {
    if (canJump) onJump!(fwd.channel_id!, fwd.message_id);
  };

  const formattedDate = new Date(fwd.created_at).toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  });

  return (
    <div
      className={`forward-embed${canJump ? ' forward-embed--clickable' : ''}`}
      onClick={canJump ? handleClick : undefined}
      title={canJump ? 'Jump to message' : undefined}
    >
      <div className="forward-embed-bar" />
      <div className="forward-embed-body">
        <div className="forward-embed-meta">
          {fwd.author_avatar_url ? (
            <img className="forward-embed-avatar" src={fwd.author_avatar_url} alt={fwd.author_username} />
          ) : (
            <div className="forward-embed-avatar forward-embed-avatar--placeholder" />
          )}
          <span className="forward-embed-author">
            {fwd.author_display_name || fwd.author_username}
          </span>
          <span className="forward-embed-date">{formattedDate}</span>
        </div>
        {fwd.content ? (
          <div className="forward-embed-content">
            <MarkdownRenderer content={fwd.content} mode="chat" memberMap={memberMap} channelMap={channelMap} />
          </div>
        ) : fwd.attachment_name ? (
          <div className="forward-embed-content forward-embed-content--attachment">
            <span>📎 {fwd.attachment_name}</span>
          </div>
        ) : null}
        <div className="forward-embed-source">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="15 17 20 12 15 7" />
            <path d="M4 18v-2a4 4 0 0 1 4-4h12" />
          </svg>
          {isFromDm ? (
            <span>Direct Message</span>
          ) : (
            <span>#{fwd.channel_name} in {fwd.server_name}</span>
          )}
        </div>
      </div>
    </div>
  );
};
