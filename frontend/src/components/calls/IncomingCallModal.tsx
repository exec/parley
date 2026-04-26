import React, { useEffect, useRef } from 'react';
import { Phone, PhoneOff } from 'lucide-react';
import type { IncomingRing } from '../../context/CallContext';
import './IncomingCallModal.css';

interface Props {
  ring: IncomingRing;
  showEndCurrentAccept: boolean;
  onAccept: () => void;
  onEndCurrentAndAccept: () => void;
  onDecline: () => void;
}

export const IncomingCallModal: React.FC<Props> = ({ ring, showEndCurrentAccept, onAccept, onEndCurrentAndAccept, onDecline }) => {
  const audioRef = useRef<HTMLAudioElement>(null);

  useEffect(() => {
    audioRef.current?.play().catch(() => {});
    return () => { audioRef.current?.pause(); };
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onDecline();
      if (e.key === 'Enter')  onAccept();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onAccept, onDecline]);

  return (
    <div className="incoming-modal-backdrop" role="dialog" aria-modal="true" aria-live="assertive">
      <div className="incoming-modal">
        <audio ref={audioRef} src="/ringtone.mp3" loop autoPlay preload="auto" />
        {ring.caller.avatar_url
          ? <img src={ring.caller.avatar_url} alt="" className="incoming-avatar" />
          : <div className="incoming-avatar incoming-avatar--placeholder" />}
        <p className="incoming-text">
          Incoming call from <strong>{ring.caller.display_name || ring.caller.username}</strong>...
        </p>
        <div className="incoming-buttons">
          <button className="incoming-btn incoming-btn--decline" aria-label="Decline call" onClick={onDecline}>
            <PhoneOff size={20} /> Decline
          </button>
          {showEndCurrentAccept && (
            <button className="incoming-btn incoming-btn--end-current" aria-label="End current call and accept" onClick={onEndCurrentAndAccept}>
              End current & Accept
            </button>
          )}
          <button className="incoming-btn incoming-btn--accept" aria-label="Accept call" onClick={onAccept}>
            <Phone size={20} /> Accept
          </button>
        </div>
      </div>
    </div>
  );
};
