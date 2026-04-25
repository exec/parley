import React from 'react';
import { createPortal } from 'react-dom';
import { useLocalVolumes } from '../../hooks/useLocalVolumes';
import { VolumeSlider } from './VolumeSlider';
import './VoiceContextMenu.css';

interface Props {
  position: { x: number; y: number };
  targetUserID: string;
  canForceMute: boolean;
  canKick: boolean;
  onForceMute: () => void;
  onKick: () => void;
  onClose: () => void;
}

export const VoiceContextMenu: React.FC<Props> = ({
  position, targetUserID, canForceMute, canKick, onForceMute, onKick, onClose,
}) => {
  const { getVolume, setVolume, toggleMute } = useLocalVolumes();
  const v = getVolume(targetUserID);

  return createPortal(
    <>
      <div className="vc-context-menu-backdrop" onClick={onClose} />
      <div
        className="vc-context-menu"
        style={{
          position: 'fixed',
          left: Math.min(position.x, window.innerWidth - 200),
          top: Math.min(position.y, window.innerHeight - 200),
        }}
        onClick={e => e.stopPropagation()}
      >
        <VolumeSlider value={v} onChange={n => setVolume(targetUserID, n)} />
        <button
          className="vc-context-menu-item"
          onClick={() => { toggleMute(targetUserID); onClose(); }}
        >
          {v === 0 ? 'Unmute for me' : 'Mute for me'}
        </button>
        {(canForceMute || canKick) && <div className="vc-context-menu-divider" />}
        {canForceMute && (
          <button className="vc-context-menu-item vc-context-menu-item--mod" onClick={() => { onForceMute(); onClose(); }}>
            Force mute
          </button>
        )}
        {canKick && (
          <button className="vc-context-menu-item vc-context-menu-item--mod vc-context-menu-item--danger" onClick={() => { onKick(); onClose(); }}>
            Disconnect from call
          </button>
        )}
      </div>
    </>,
    document.body,
  );
};
