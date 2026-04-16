import React from 'react';
import { Mic, MicOff, Headphones, HeadphoneOff, Video, VideoOff, Monitor, MonitorOff, PhoneOff } from 'lucide-react';
import './VoiceControls.css';

interface VoiceControlsProps {
  channelName: string;
  muted: boolean;
  deafened: boolean;
  videoEnabled: boolean;
  screenSharing: boolean;
  vadMode: 'vad' | 'ptt' | 'always';
  pttKey: string;
  onNavigate: () => void;
  onToggleMute: () => void;
  onToggleDeafen: () => void;
  onToggleVideo: () => void;
  onToggleScreenShare: () => void;
  onDisconnect: () => void;
}

export const VoiceControls: React.FC<VoiceControlsProps> = ({
  channelName,
  muted,
  deafened,
  videoEnabled,
  screenSharing,
  vadMode,
  pttKey,
  onNavigate,
  onToggleMute,
  onToggleDeafen,
  onToggleVideo,
  onToggleScreenShare,
  onDisconnect,
}) => {
  const pttLabel = pttKey.replace('Key', '').replace('Digit', '').replace('Space', 'SPACE');

  return (
    <div className="voice-widget">
      <div className="voice-widget-status" onClick={onNavigate}>
        <span className="voice-widget-dot" />
        <div className="voice-widget-info">
          <span className="voice-widget-label">Voice Connected</span>
          <span className="voice-widget-channel">#{channelName}</span>
        </div>
      </div>
      <div className="voice-widget-controls">
        <div className="vw-btn-wrap">
          <button
            className={`vw-btn ${muted ? 'vw-btn--off' : ''}`}
            onClick={onToggleMute}
            title={muted ? 'Unmute' : 'Mute'}
            aria-label={muted ? 'Unmute' : 'Mute'}
            aria-pressed={muted}
          >
            {muted ? <MicOff size={16} color="var(--parley-danger)" /> : <Mic size={16} color="var(--parley-accent)" />}
          </button>
          {muted && <span className="vw-btn-status">Muted</span>}
        </div>
        <div className="vw-btn-wrap">
          <button
            className={`vw-btn ${deafened ? 'vw-btn--off' : ''}`}
            onClick={onToggleDeafen}
            title={deafened ? 'Undeafen' : 'Deafen'}
            aria-label={deafened ? 'Undeafen' : 'Deafen'}
            aria-pressed={deafened}
          >
            {deafened ? <HeadphoneOff size={16} color="var(--parley-danger)" /> : <Headphones size={16} color="var(--parley-accent)" />}
          </button>
          {deafened && <span className="vw-btn-status">Deafened</span>}
        </div>
        <button
          className={`vw-btn ${!videoEnabled ? 'vw-btn--off' : ''}`}
          onClick={onToggleVideo}
          title={videoEnabled ? 'Camera off' : 'Camera on'}
          aria-label={videoEnabled ? 'Turn camera off' : 'Turn camera on'}
          aria-pressed={videoEnabled}
        >
          {videoEnabled ? <Video size={16} color="var(--parley-accent)" /> : <VideoOff size={16} color="var(--parley-text-muted)" />}
        </button>
        <button
          className={`vw-btn ${!screenSharing ? 'vw-btn--off' : ''}`}
          onClick={onToggleScreenShare}
          title={screenSharing ? 'Stop sharing' : 'Share screen'}
          aria-label={screenSharing ? 'Stop sharing screen' : 'Share screen'}
          aria-pressed={screenSharing}
        >
          {screenSharing ? <Monitor size={16} color="var(--parley-accent)" /> : <MonitorOff size={16} color="var(--parley-text-muted)" />}
        </button>
        <button className="vw-btn vw-btn--leave" onClick={onDisconnect} title="Disconnect" aria-label="Disconnect from voice">
          <PhoneOff size={16} color="var(--parley-danger)" />
        </button>
      </div>
      {vadMode === 'ptt' && (
        <div className="voice-widget-ptt">
          Hold <kbd>{pttLabel}</kbd> to talk
        </div>
      )}
      {vadMode === 'vad' && (
        <div className="voice-widget-mode">Voice Activity</div>
      )}
      {vadMode === 'always' && (
        <div className="voice-widget-mode">Always transmitting</div>
      )}
    </div>
  );
};
