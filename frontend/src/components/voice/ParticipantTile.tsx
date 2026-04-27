import React, { useRef, useEffect, useMemo, useState } from 'react';
import { Track, ParticipantEvent } from 'livekit-client';
import type { Participant, RemoteAudioTrack } from 'livekit-client';
import { MicOff, Monitor, VolumeX } from 'lucide-react';
import { useLocalVolumes } from '../../hooks/useLocalVolumes';
import { ConnectionQualityDot } from './ConnectionQualityDot';
import './ParticipantTile.css';

const GREEN = '74, 222, 128'; // speaking ring colour (rgb)

interface ParticipantTileProps {
  participant: Participant;
  isLocal?: boolean;
  isSpeaking?: boolean;
  isScreenShare?: boolean;
  displayName?: string;
  avatarUrl?: string;
  activeSoundEmoji?: string;
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
  activeSoundEmoji,
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

  // Audio-level ring — event-driven via IsSpeakingChanged (server-sent, ~100-200ms interval).
  // rAF only runs while speaking to smoothly track audioLevel between server updates.
  // Direct DOM manipulation avoids React re-renders.
  useEffect(() => {
    if (isScreenShare) return;
    const el = avatarRef.current;
    let rafId: number | null = null;

    const applyRing = () => {
      if (!el) return;
      const l = Math.max(0, Math.min(1, participant.audioLevel));
      const borderAlpha = (0.45 + l * 0.55).toFixed(2);
      const ringSize    = (2 + l * 4).toFixed(1);
      const ringAlpha   = (0.2  + l * 0.45).toFixed(2);
      const glowPx      = Math.round(8 + l * 22);
      const glowAlpha   = (0.18 + l * 0.62).toFixed(2);
      el.style.borderColor = `rgba(${GREEN}, ${borderAlpha})`;
      el.style.boxShadow   = `0 0 0 ${ringSize}px rgba(${GREEN}, ${ringAlpha}), 0 0 ${glowPx}px rgba(${GREEN}, ${glowAlpha})`;
      rafId = requestAnimationFrame(applyRing);
    };

    const clearRing = () => {
      if (el) { el.style.borderColor = ''; el.style.boxShadow = ''; }
    };

    const onSpeakingChanged = (speaking: boolean) => {
      if (speaking) {
        applyRing();
      } else {
        if (rafId !== null) { cancelAnimationFrame(rafId); rafId = null; }
        clearRing();
      }
    };

    participant.on(ParticipantEvent.IsSpeakingChanged, onSpeakingChanged);
    if (participant.isSpeaking) applyRing();

    return () => {
      participant.off(ParticipantEvent.IsSpeakingChanged, onSpeakingChanged);
      if (rafId !== null) cancelAnimationFrame(rafId);
      clearRing();
    };
  }, [participant, isScreenShare]);

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

  const { getVolume } = useLocalVolumes();
  const localVol = getVolume(participant.identity);
  const isLocallyMuted = localVol === 0;

  useEffect(() => {
    participant.audioTrackPublications.forEach(pub => {
      // setVolume only exists on RemoteAudioTrack — local mic tracks lack it
      // and ignoring `participant.isLocal` shape differences is simpler than
      // narrowing the union, so feature-detect.
      const t = pub.track as RemoteAudioTrack | undefined;
      if (t && t.kind === Track.Kind.Audio && typeof t.setVolume === 'function') {
        t.setVolume(localVol / 100);
      }
    });
  }, [localVol, participant]);

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
        {isLocallyMuted && !isLocal && (
          <span className="participant-tile-locally-muted" title="Muted for me">
            <VolumeX size={14} />
          </span>
        )}
        <ConnectionQualityDot participant={participant} />
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
        {activeSoundEmoji && (
          <span className="participant-tile-sound-emoji" title="Playing sound">
            {activeSoundEmoji}
          </span>
        )}
      </div>
    </div>
  );
};
