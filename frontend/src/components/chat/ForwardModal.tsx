import React, { useState, useRef, useEffect, useCallback } from 'react';
import { Channel, DmChannel, Message, ForwardedMessage } from '../../api/types';
import { forwardToChannel, forwardToDm } from '../../api/messages';

interface ForwardModalProps {
  message: Message;
  channels: Channel[];
  dmChannels: DmChannel[];
  currentChannelId?: string;
  onClose: () => void;
}

export const ForwardModal: React.FC<ForwardModalProps> = ({
  message,
  channels,
  dmChannels,
  currentChannelId,
  onClose,
}) => {
  const [search, setSearch] = useState('');
  const [sending, setSending] = useState<string | null>(null);
  const [sent, setSent] = useState<Set<string>>(new Set());
  const [error, setError] = useState<string | null>(null);
  const overlayRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (overlayRef.current && e.target === overlayRef.current) onClose();
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [onClose]);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [onClose]);

  // Build the forwarded message snapshot from the current message
  const buildFwd = useCallback((): ForwardedMessage => ({
    message_id: message.id,
    channel_id: message.channel_id,
    // channel_name and server details will be enriched by the server if needed;
    // for display we keep the snapshot minimal — the embed renders what's stored
    author_username: message.author_username,
    author_display_name: message.author_display_name,
    author_avatar_url: message.author_avatar_url,
    content: message.content,
    attachment_name: message.attachment_name,
    created_at: message.created_at,
  }), [message]);

  // Text channels only (exclude voice, bin, current channel)
  const textChannels = channels.filter(
    c => c.type === 0 && c.id !== currentChannelId
  );

  const q = search.toLowerCase();
  const filteredChannels = q
    ? textChannels.filter(c => c.name.toLowerCase().includes(q))
    : textChannels;
  const filteredDms = q
    ? dmChannels.filter(d =>
        (d.other_display_name || d.other_username).toLowerCase().includes(q)
      )
    : dmChannels;

  const handleForwardToChannel = async (channel: Channel) => {
    if (sending || sent.has(channel.id)) return;
    setSending(channel.id);
    setError(null);
    try {
      await forwardToChannel(channel.id, buildFwd());
      setSent(prev => new Set([...prev, channel.id]));
    } catch {
      setError('Failed to forward message.');
    } finally {
      setSending(null);
    }
  };

  const handleForwardToDm = async (dm: DmChannel) => {
    if (sending || sent.has(dm.id)) return;
    setSending(dm.id);
    setError(null);
    try {
      await forwardToDm(dm.id, buildFwd());
      setSent(prev => new Set([...prev, dm.id]));
    } catch {
      setError('Failed to forward message.');
    } finally {
      setSending(null);
    }
  };

  return (
    <div className="forward-modal-overlay" ref={overlayRef}>
      <div className="forward-modal">
        <div className="forward-modal-header">
          <span>Forward Message</span>
          <button className="forward-modal-close" onClick={onClose} aria-label="Close">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
              <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>

        <div className="forward-modal-search">
          <input
            ref={inputRef}
            type="text"
            placeholder="Search channels or DMs..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            className="forward-modal-search-input"
          />
        </div>

        <div className="forward-modal-body">
          {filteredChannels.length > 0 && (
            <div className="forward-modal-section">
              <div className="forward-modal-section-label">Channels</div>
              {filteredChannels.map(channel => (
                <button
                  key={channel.id}
                  className={`forward-modal-item${sent.has(channel.id) ? ' forward-modal-item--sent' : ''}`}
                  onClick={() => handleForwardToChannel(channel)}
                  disabled={!!sending || sent.has(channel.id)}
                >
                  <span className="forward-modal-item-icon">#</span>
                  <span className="forward-modal-item-name">{channel.name}</span>
                  {sent.has(channel.id) && <span className="forward-modal-item-check">✓</span>}
                  {sending === channel.id && <span className="forward-modal-item-spinner" />}
                </button>
              ))}
            </div>
          )}

          {filteredDms.length > 0 && (
            <div className="forward-modal-section">
              <div className="forward-modal-section-label">Direct Messages</div>
              {filteredDms.map(dm => (
                <button
                  key={dm.id}
                  className={`forward-modal-item${sent.has(dm.id) ? ' forward-modal-item--sent' : ''}`}
                  onClick={() => handleForwardToDm(dm)}
                  disabled={!!sending || sent.has(dm.id)}
                >
                  <span className="forward-modal-item-icon">@</span>
                  <span className="forward-modal-item-name">
                    {dm.other_display_name || dm.other_username}
                  </span>
                  {sent.has(dm.id) && <span className="forward-modal-item-check">✓</span>}
                  {sending === dm.id && <span className="forward-modal-item-spinner" />}
                </button>
              ))}
            </div>
          )}

          {filteredChannels.length === 0 && filteredDms.length === 0 && (
            <div className="forward-modal-empty">No channels or DMs found.</div>
          )}
        </div>

        {error && <div className="forward-modal-error">{error}</div>}
      </div>
    </div>
  );
};
