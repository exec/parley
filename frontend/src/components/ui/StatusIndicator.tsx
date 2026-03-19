import React from 'react';
import './StatusIndicator.css';

export type StatusType = 'online' | 'dnd' | 'afk' | 'invisible' | 'offline';

interface StatusIndicatorProps {
  status: StatusType;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

export const StatusIndicator: React.FC<StatusIndicatorProps> = ({ status, size = 'md', className = '' }) => {
  return (
    <span
      className={`status-indicator status-${status} status-size-${size} ${className}`}
      aria-label={status}
    />
  );
};
