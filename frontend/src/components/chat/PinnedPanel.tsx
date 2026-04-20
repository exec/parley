import React, { useEffect, useRef, useState } from 'react';
import { Message } from '../../api/types';
import { getPinnedMessages, unpinMessage } from '../../api/messages';
import MarkdownRenderer from '../ui/MarkdownRenderer';

interface PinnedPanelProps {
  channelId: string;
  canPin: boolean;
  onClose: () => void;
  onScrollToMessage?: (messageId: string) => void;
  memberMap?: Map<string, string>;
  channelMap?: Map<string, string>;
  onUnpin?: (messageId: string) => void;
}

// Pin SVG icon — custom drawn
export const PinIcon: React.FC<{ size?: number; className?: string }> = ({ size = 16, className }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    aria-hidden="true"
  >
    {/* Pin shaft */}
    <line x1="12" y1="17" x2="12" y2="22" />
    {/* Pin body */}
    <path d="M5 17h14" />
    <path d="M15 5h1a2 2 0 0 1 2 2v1a2 2 0 0 1-2 2h-1" />
    <path d="M9 5H8a2 2 0 0 0-2 2v1a2 2 0 0 0 2 2h1" />
    <rect x="9" y="3" width="6" height="14" rx="1" />
  </svg>
);

export const PinnedPanel: React.FC<PinnedPanelProps> = ({
  channelId,
  canPin,
  onClose,
  onScrollToMessage,
  memberMap,
  channelMap,
  onUnpin,
}) => {
  const [pins, setPins] = useState<Message[]>([]);
  const [loading, setLoading] = useState(true);
  const panelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let cancelled = false;
    getPinnedMessages(channelId).then(msgs => {
      if (!cancelled) { setPins(msgs); setLoading(false); }
    }).catch(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [channelId]);

  // Close on outside click
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [onClose]);

  const handleUnpin = async (messageId: string) => {
    try {
      await unpinMessage(channelId, messageId);
      setPins(prev => prev.filter(p => p.id !== messageId));
      onUnpin?.(messageId);
    } catch { /* ignore */ }
  };

  const handleJump = (messageId: string) => {
    onScrollToMessage?.(messageId);
    onClose();
  };

  return (
    <div className="pinned-panel" ref={panelRef}>
      <div className="pinned-panel-header">
        <PinIcon size={14} />
        <span>Pinned Messages</span>
        <button className="pinned-panel-close" onClick={onClose} aria-label="Close">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
            <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
          </svg>
        </button>
      </div>
      <div className="pinned-panel-body">
        {loading ? (
          <div className="pinned-panel-empty">Loading…</div>
        ) : pins.length === 0 ? (
          <div className="pinned-panel-empty">No pinned messages yet.</div>
        ) : (
          pins.map(msg => (
            <div key={msg.id} className="pinned-item">
              <div className="pinned-item-author">
                {msg.author_display_name || msg.author_username}
                <span className="pinned-item-date">
                  {new Date(msg.pinned_at || msg.created_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}
                </span>
              </div>
              <div className="pinned-item-content">
                {msg.content ? (
                  <MarkdownRenderer content={msg.content} mode="chat" memberMap={memberMap} channelMap={channelMap} />
                ) : msg.attachment_name ? (
                  <span className="pinned-item-attachment">📎 {msg.attachment_name}</span>
                ) : null}
              </div>
              <div className="pinned-item-actions">
                <button className="pinned-item-btn" onClick={() => handleJump(msg.id)} title="Jump to message">
                  Jump
                </button>
                {canPin && (
                  <button className="pinned-item-btn pinned-item-btn--unpin" onClick={() => handleUnpin(msg.id)} title="Unpin message">
                    Unpin
                  </button>
                )}
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
};
