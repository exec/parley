import React, { useEffect, useRef } from 'react';
import './VoiceContextMenu.css';

interface VoiceContextMenuProps {
  position: { x: number; y: number };
  participantId: string;
  canMute: boolean;
  canKick: boolean;
  onMute: () => void;
  onKick: () => void;
  onClose: () => void;
}

export const VoiceContextMenu: React.FC<VoiceContextMenuProps> = ({
  position, canMute, canKick, onMute, onKick, onClose,
}) => {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [onClose]);

  const style: React.CSSProperties = {
    position: 'fixed',
    top: Math.min(position.y, window.innerHeight - 100),
    left: Math.min(position.x, window.innerWidth - 190),
    zIndex: 9999,
  };

  return (
    <div ref={ref} className="vc-context-menu" style={style}>
      {canMute && (
        <button className="vc-context-menu-item" onClick={onMute}>Mute</button>
      )}
      {canKick && (
        <button className="vc-context-menu-item vc-context-menu-item--danger" onClick={onKick}>
          Kick from Voice
        </button>
      )}
    </div>
  );
};
