import React, { useRef, useEffect, useMemo } from 'react';
import { Participant, Track } from 'livekit-client';
import { MicOff, Monitor } from 'lucide-react';
import './ParticipantTile.css';

interface ParticipantTileProps {
  participant: Participant;
  isLocal?: boolean;
  isSpeaking?: boolean;
  isScreenShare?: boolean;
  displayName?: string;
  avatarUrl?: string;
}

export const ParticipantTile: React.FC<ParticipantTileProps> = ({
  participant,
  isLocal,
  isSpeaking,
  isScreenShare,
  displayName,
  avatarUrl,
}) => {
  const videoRef = useRef<HTMLVideoElement>(null);

  const videoPublication = useMemo(() => {
    const source = isScreenShare ? Track.Source.ScreenShare : Track.Source.Camera;
    return Array.from(participant.trackPublications.values()).find(
      p => p.source === source
    );
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [participant, participant.trackPublications.size, isScreenShare]);

  const hasVideo = !!(videoPublication?.track && !videoPublication.isMuted);

  useEffect(() => {
    if (!videoRef.current || !videoPublication?.track) return;
    videoPublication.track.attach(videoRef.current);
    return () => {
      videoPublication.track?.detach(videoRef.current!);
    };
  }, [videoPublication?.track]);

  const micPublication = useMemo(() => {
    return Array.from(participant.trackPublications.values()).find(
      p => p.source === Track.Source.Microphone
    );
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [participant, participant.trackPublications.size]);

  const isMuted = micPublication ? micPublication.isMuted : true;
  const name = displayName ?? participant.name ?? participant.identity;
  const initial = name.charAt(0).toUpperCase();

  return (
    <div className={`participant-tile ${isSpeaking ? 'participant-tile--speaking' : ''} ${isScreenShare ? 'participant-tile--screen' : ''}`}>
      <div className="participant-tile-media">
        {hasVideo ? (
          <video
            ref={videoRef}
            autoPlay
            playsInline
            muted={isLocal}
            className="participant-tile-video"
          />
        ) : (
          <div className="participant-tile-avatar">
            {isScreenShare ? (
              <Monitor size={40} color="#32CD32" />
            ) : avatarUrl ? (
              <img src={avatarUrl} alt={name} />
            ) : (
              <span className="participant-tile-initial">{initial}</span>
            )}
          </div>
        )}
      </div>

      <div className="participant-tile-footer">
        <span className="participant-tile-name">
          {name}
          {isLocal && <span className="participant-tile-you">You</span>}
        </span>
        {isMuted && !isScreenShare && (
          <span className="participant-tile-muted">
            <MicOff size={12} color="#cc4444" />
          </span>
        )}
      </div>
    </div>
  );
};
