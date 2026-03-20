import React, { useEffect, useRef, useState } from 'react';
import { LocalParticipant, Track } from 'livekit-client';
import { Headphones, Volume2, Square, X } from 'lucide-react';
import { listAllSounds, playSound, SoundWithServer } from '../../api/soundboard';
import './SoundboardPanel.css';

interface SoundboardPanelProps {
  channelId: string;
  localParticipant: LocalParticipant | null;
  muted: boolean;
  onClose: () => void;
}

export const SoundboardPanel: React.FC<SoundboardPanelProps> = ({
  channelId,
  localParticipant,
  muted,
  onClose,
}) => {
  const panelRef = useRef<HTMLDivElement>(null);
  const [sounds, setSounds] = useState<SoundWithServer[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [playing, setPlaying] = useState<string | null>(null);
  const [previewing, setPreviewing] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError('');
    listAllSounds()
      .then(data => { if (!cancelled) { setSounds(data); setLoading(false); } })
      .catch(err => { if (!cancelled) { setError(err?.message || 'Failed to load sounds'); setLoading(false); } });
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        onClose();
      }
    };
    const handleKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('mousedown', handleClick);
    document.addEventListener('keydown', handleKey);
    return () => {
      document.removeEventListener('mousedown', handleClick);
      document.removeEventListener('keydown', handleKey);
    };
  }, [onClose]);

  const handlePreview = async (sound: SoundWithServer) => {
    if (previewing || playing) return;
    setPreviewing(sound.id);
    try {
      const audioCtx = new AudioContext();
      const resp = await fetch(sound.file_url);
      const buffer = await resp.arrayBuffer();
      const audioBuffer = await audioCtx.decodeAudioData(buffer);
      const source = audioCtx.createBufferSource();
      source.buffer = audioBuffer;
      source.connect(audioCtx.destination);
      source.start();
      source.onended = () => {
        audioCtx.close();
        setPreviewing(null);
      };
    } catch {
      setPreviewing(null);
    }
  };

  const handlePlay = async (sound: SoundWithServer) => {
    if (playing || previewing) return;
    setPlaying(sound.id);
    try {
      const micPub = localParticipant
        ? Array.from(localParticipant.trackPublications.values()).find(
            p => p.source === Track.Source.Microphone
          )
        : undefined;
      const localAudioTrack = micPub?.audioTrack;
      const originalMST = localAudioTrack?.mediaStreamTrack;

      const audioCtx = new AudioContext();
      const resp = await fetch(sound.file_url);
      const buffer = await resp.arrayBuffer();
      const audioBuffer = await audioCtx.decodeAudioData(buffer);
      const durationMs = Math.round(audioBuffer.duration * 1000);

      const soundSource = audioCtx.createBufferSource();
      soundSource.buffer = audioBuffer;

      if (localAudioTrack && originalMST) {
        // Mix mic + sound into one destination, then swap the published track
        const destination = audioCtx.createMediaStreamDestination();

        const micSource = audioCtx.createMediaStreamSource(new MediaStream([originalMST]));
        const micGain = audioCtx.createGain();
        micGain.gain.value = muted ? 0 : 1;
        micSource.connect(micGain);
        micGain.connect(destination);

        soundSource.connect(destination);
        soundSource.start();

        await localAudioTrack.replaceTrack(destination.stream.getAudioTracks()[0]);

        soundSource.onended = async () => {
          await localAudioTrack.replaceTrack(originalMST).catch(() => {});
          await audioCtx.close();
          setPlaying(null);
        };
      } else {
        // No mic track — play locally only
        soundSource.connect(audioCtx.destination);
        soundSource.start();
        soundSource.onended = () => {
          audioCtx.close();
          setPlaying(null);
        };
      }

      await playSound(channelId, sound.id, durationMs);
    } catch {
      setPlaying(null);
    }
  };

  // Group by server_name
  const grouped = sounds.reduce<Record<string, SoundWithServer[]>>((acc, s) => {
    const key = s.server_name || 'Unknown Server';
    if (!acc[key]) acc[key] = [];
    acc[key].push(s);
    return acc;
  }, {});

  return (
    <div className="soundboard-panel" ref={panelRef}>
      <div className="soundboard-panel-header">
        <span className="soundboard-panel-title">Soundboard</span>
        <button className="soundboard-panel-close" onClick={onClose} title="Close">
          <X size={16} />
        </button>
      </div>

      {loading && <div className="soundboard-panel-status">Loading sounds…</div>}
      {error && <div className="soundboard-panel-status soundboard-panel-error">{error}</div>}
      {!loading && !error && sounds.length === 0 && (
        <div className="soundboard-panel-status">No sounds found. Add sounds in server settings.</div>
      )}

      {!loading && !error && Object.entries(grouped).map(([serverName, serverSounds]) => (
        <div key={serverName}>
          <div className="soundboard-panel-server">{serverName}</div>
          <div className="soundboard-panel-grid">
            {serverSounds.map(sound => {
              const isPlaying = playing === sound.id;
              const isPreviewing = previewing === sound.id;
              const busy = !!(playing || previewing);
              return (
                <div key={sound.id} className="soundboard-panel-card">
                  <span className="soundboard-card-emoji">{sound.emoji || '🔊'}</span>
                  <span className="soundboard-card-name" title={sound.name}>{sound.name}</span>
                  <button
                    className={`soundboard-card-btn${isPreviewing ? ' soundboard-card-btn--active' : ''}`}
                    onClick={() => isPreviewing ? undefined : handlePreview(sound)}
                    disabled={!isPreviewing && busy}
                    title={isPreviewing ? 'Previewing…' : 'Preview'}
                  >
                    {isPreviewing ? <Square size={14} /> : <Headphones size={14} />}
                  </button>
                  <button
                    className={`soundboard-card-btn${isPlaying ? ' soundboard-card-btn--active' : ''}`}
                    onClick={() => isPlaying ? undefined : handlePlay(sound)}
                    disabled={!isPlaying && busy}
                    title={isPlaying ? 'Playing…' : 'Play'}
                  >
                    {isPlaying ? <Square size={14} /> : <Volume2 size={14} />}
                  </button>
                </div>
              );
            })}
          </div>
        </div>
      ))}
    </div>
  );
};
