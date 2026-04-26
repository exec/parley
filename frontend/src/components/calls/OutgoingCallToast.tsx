import React, { useEffect, useRef } from 'react';
import { useCall } from '../../context/CallContext';
import './CallBanner.css';

export const OutgoingCallToast: React.FC = () => {
  const { state, outgoingTarget, cancel } = useCall();
  const audioRef = useRef<HTMLAudioElement>(null);

  useEffect(() => {
    if (state === 'outgoing') {
      audioRef.current?.play().catch(() => {});
    } else {
      audioRef.current?.pause();
    }
  }, [state]);

  if (state !== 'outgoing') return null;

  const label = outgoingTarget?.display_name || outgoingTarget?.username || 'user';

  return (
    <div className="outgoing-toast" role="status">
      <audio ref={audioRef} src="/ringback.mp3" loop preload="auto" />
      <span>Calling {label}…</span>
      <button onClick={cancel} aria-label="Cancel call">Cancel</button>
    </div>
  );
};
