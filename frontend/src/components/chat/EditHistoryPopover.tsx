import React, { useEffect, useRef, useState } from 'react';
import { getMessageVersions, MessageVersion } from '../../api/messages';
import { useViewportAdjust } from '../../hooks/useViewportAdjust';
import { Skeleton } from '../ui/Skeleton';
import './EditHistoryPopover.css';

interface EditHistoryPopoverProps {
  messageId: string;
  onClose: () => void;
}

export const EditHistoryPopover: React.FC<EditHistoryPopoverProps> = ({ messageId, onClose }) => {
  const [versions, setVersions] = useState<MessageVersion[]>([]);
  const [loading, setLoading] = useState(true);
  const popoverRef = useRef<HTMLDivElement>(null);

  useViewportAdjust(popoverRef, [versions, loading]);

  useEffect(() => {
    let cancelled = false;
    getMessageVersions(messageId)
      .then((data) => {
        if (!cancelled) {
          setVersions(data);
          setLoading(false);
        }
      })
      .catch(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [messageId]);

  // Close on outside click
  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        onClose();
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [onClose]);

  if (!loading && versions.length === 0) return null;

  const formatTimestamp = (dateString: string): string => {
    const date = new Date(dateString);
    return date.toLocaleString([], {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  return (
    <div ref={popoverRef} className="edit-history-popover">
      <div className="edit-history-title">Edit History</div>
      {loading ? (
        <div aria-busy="true" aria-label="Loading edit history">
          {Array.from({ length: 2 }).map((_, i) => (
            <div key={i} className="edit-history-version">
              <Skeleton variant="line" width={140} height={10} style={{ marginBottom: 6 }} />
              <Skeleton variant="line" width="92%" height={12} style={{ marginBottom: 4 }} />
              <Skeleton variant="line" width="68%" height={12} />
            </div>
          ))}
        </div>
      ) : (
        versions.map((version) => (
          <div key={version.id} className="edit-history-version">
            <div className="edit-history-timestamp">{formatTimestamp(version.edited_at)}</div>
            <div className="edit-history-content">{version.content}</div>
          </div>
        ))
      )}
    </div>
  );
};
