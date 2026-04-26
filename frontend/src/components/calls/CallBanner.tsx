import React from 'react';
import { Phone } from 'lucide-react';
import './CallBanner.css';

interface Props {
  participantCount: number;
  onJoin: () => void;
}

export const CallBanner: React.FC<Props> = ({ participantCount, onJoin }) => {
  if (participantCount <= 0) return null;
  return (
    <div className="call-banner" role="status">
      <Phone size={14} />
      <span>{participantCount} in call</span>
      <button className="call-banner-join" onClick={onJoin}>Join</button>
    </div>
  );
};
