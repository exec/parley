import React, { useRef, useEffect, useMemo, useState } from 'react';
import { Participant, Track, ParticipantEvent } from 'livekit-client';
import { MicOff, Monitor } from 'lucide-react';
import './ParticipantTile.css';

const GREEN = '74, 222, 128'; // speaking ring colour (rgb)

interface ParticipantTileProps {
  participant: Participant;
  isLocal?: boolean;
  isSpeaking?: boolean;
  isScreenShare?: boolean;
  displayName?: string;
  avatarUrl?: string;
  onContextMenu?: (e: React.MouseEvent) => void;
  onClick?: (e: React.MouseEvent) => void;
}

export const ParticipantTile: React.FC<ParticipantTileProps> = ({
  participant,
  isLocal,
  isSpeaking,
  isScreenShare,
  displayName,
  avatarUrl,
  onContextMenu,
  onClick,
}) => {
  const videoRef = useRef<HTMLVideoElement>(null);
  const avatarRef = useRef<HTMLDivElement>(null);

  // trackVersion bumps on any track state change to force memo recomputation
  const [trackVersion, setTrackVersion] = useState(0);
  useEffect(() => {
    const bump = () => setTrackVersion(v => v + 1);
    participant.on(ParticipantEvent.TrackMuted, bump);
    participant.on(ParticipantEvent.TrackUnmuted, bump);
    participant.on(ParticipantEvent.TrackSubscribed, bump);
    participant.on(ParticipantEvent.TrackUnsubscribed, bump);
    participant.on(ParticipantEvent.TrackPublished, bump);
    participant.on(ParticipantEvent.TrackUnpublished, bump);
    participant.on(ParticipantEvent.LocalTrackPublished, bump);
    participant.on(ParticipantEvent.LocalTrackUnpublished, bump);
    return () => {
      participant.off(ParticipantEvent.TrackMuted, bump);
      participant.off(ParticipantEvent.TrackUnmuted, bump);
      participant.off(ParticipantEvent.TrackSubscribed, bump);
      participant.off(ParticipantEvent.TrackUnsubscribed, bump);
      participant.off(ParticipantEvent.TrackPublished, bump);
      participant.off(ParticipantEvent.TrackUnpublished, bump);
      participant.off(ParticipantEvent.LocalTrackPublished, bump);
      participant.off(ParticipantEvent.LocalTrackUnpublished, bump);
    };
  }, [participant]);

  // Audio-level ring — poll participant.audioLevel via rAF so we can update at 60fps
  // without React re-renders. AudioLevelChanged event doesn't exist in LiveKit 2.x.
  useEffect(() => {
    if (isScreenShare) return; // no ring on screen-share tiles
    const el = avatarRef.current;
    let rafId: number;

    const update = () => {
      if (el) {
        const l = Math.max(0, Math.min(1, (participant as any).audioLevel ?? 0));
        if (!isSpeaking || l < 0.04) {
          el.style.borderColor = '';
          el.style.boxShadow = '';
        } else {
          const borderAlpha = (0.45 + l * 0.55).toFixed(2);
          const ringSize    = (2 + l * 4).toFixed(1);
          const ringAlpha   = (0.2  + l * 0.45).toFixed(2);
          const glowPx      = Math.round(8 + l * 22);
          const glowAlpha   = (0.18 + l * 0.62).toFixed(2);
          el.style.borderColor = `rgba(${GREEN}, ${borderAlpha})`;
          el.style.boxShadow   = `0 0 0 ${ringSize}px rgba(${GREEN}, ${ringAlpha}), 0 0 ${glowPx}px rgba(${GREEN}, ${glowAlpha})`;
        }
      }
      rafId = requestAnimationFrame(update);
    };
    rafId = requestAnimationFrame(update);

    return () => {
      cancelAnimationFrame(rafId);
      if (el) { el.style.borderColor = ''; el.style.boxShadow = ''; }
    };
  }, [participant, isSpeaking, isScreenShare]);

  const videoPublication = useMemo(() => {
    const source = isScreenShare ? Track.Source.ScreenShare : Track.Source.Camera;
    return Array.from(participant.trackPublications.values()).find(p => p.source === source);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [participant, isScreenShare, trackVersion]);

  const hasVideo = !!(videoPublication?.track && !videoPublication.isMuted);

  // Always mount <video> — toggle visibility so ref stays stable and effect can always attach
  useEffect(() => {
    const el = videoRef.current;
    if (!el || !videoPublication?.track) return;
    videoPublication.track.attach(el);
    return () => {
      videoPublication.track?.detach(el);
    };
  }, [videoPublication?.track]);

  const micPublication = useMemo(() => {
    return Array.from(participant.trackPublications.values()).find(
      p => p.source === Track.Source.Microphone
    );
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [participant, trackVersion]);

  const isMuted = micPublication ? micPublication.isMuted : false;
  const name = displayName || participant.name || participant.identity || '?';
  const initial = name.charAt(0).toUpperCase() || '?';

  return (
    <div className={`participant-tile ${isSpeaking ? 'participant-tile--speaking' : ''} ${isScreenShare ? 'participant-tile--screen' : ''}`} onContextMenu={onContextMenu} onClick={onClick}>
      <div className="participant-tile-media">
        {/* Video always mounted; hidden when no video so ref is stable for attach/detach */}
        <video
          ref={videoRef}
          autoPlay
          playsInline
          muted
          className="participant-tile-video"
          style={{ display: hasVideo ? 'block' : 'none' }}
        />
        {!hasVideo && (
          <div
            ref={avatarRef}
            className={`participant-tile-avatar${isSpeaking && !isScreenShare ? ' participant-tile-avatar--speaking' : ''}`}
          >
            {isScreenShare ? (
              <Monitor size={40} color="var(--parley-accent)" />
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
            <MicOff size={12} color="var(--parley-danger)" />
          </span>
        )}
      </div>
    </div>
  );
};
