import React, { useEffect, useState } from 'react';
import { ConnectionQuality, ParticipantEvent } from 'livekit-client';
import type { Participant } from 'livekit-client';
import './ConnectionQualityDot.css';

export const ConnectionQualityDot: React.FC<{ participant: Participant }> = ({ participant }) => {
  const [q, setQ] = useState<ConnectionQuality>(participant.connectionQuality);
  useEffect(() => {
    const onChange = () => setQ(participant.connectionQuality);
    participant.on(ParticipantEvent.ConnectionQualityChanged, onChange);
    return () => { participant.off(ParticipantEvent.ConnectionQualityChanged, onChange); };
  }, [participant]);

  let cls = 'cq-dot cq-dot--unknown';
  let label = 'unknown';
  switch (q) {
    case ConnectionQuality.Excellent: cls = 'cq-dot cq-dot--excellent'; label = 'excellent'; break;
    case ConnectionQuality.Good:      cls = 'cq-dot cq-dot--good';      label = 'good';      break;
    case ConnectionQuality.Poor:      cls = 'cq-dot cq-dot--poor';      label = 'poor';      break;
    case ConnectionQuality.Lost:      cls = 'cq-dot cq-dot--lost';      label = 'lost';      break;
  }
  return <span className={cls} title={`Connection: ${label}`} aria-label={`Connection: ${label}`} />;
};
