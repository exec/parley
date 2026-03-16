import React, { useEffect, useRef, useState, useCallback, useMemo } from 'react';
import './AudioPlayer.css';

// ─── helpers ─────────────────────────────────────────────────────────────────

function fmtTime(s: number) {
  if (!isFinite(s) || s < 0) return '0:00';
  return `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, '0')}`;
}

function fmtSize(b: number) {
  if (b < 1024) return `${b} B`;
  if (b < 1048576) return `${(b / 1024).toFixed(1)} KB`;
  return `${(b / 1048576).toFixed(1)} MB`;
}

/** Generate a deterministic bar-height array from a URL string as a seed. */
function seededBars(url: string, count: number): number[] {
  let h = 0;
  for (const c of url) h = (Math.imul(31, h) + c.charCodeAt(0)) | 0;
  const out: number[] = [];
  for (let i = 0; i < count; i++) {
    h = (Math.imul(1664525, h) + 1013904223) | 0;
    const r = (h >>> 0) / 0xffffffff;
    out.push(0.15 + Math.pow(r, 0.6) * 0.8);
  }
  return out;
}

// ─── icons ───────────────────────────────────────────────────────────────────

const PlayIcon = () => (
  <svg width="13" height="13" viewBox="0 0 13 13" fill="currentColor">
    <polygon points="2,1 11,6.5 2,12" />
  </svg>
);

const PauseIcon = () => (
  <svg width="13" height="13" viewBox="0 0 13 13" fill="currentColor">
    <rect x="1.5" y="1" width="3.5" height="11" rx="1" />
    <rect x="8" y="1" width="3.5" height="11" rx="1" />
  </svg>
);

// ─── component ───────────────────────────────────────────────────────────────

interface AudioPlayerProps {
  url: string;
  isVoiceMessage?: boolean;
  filename?: string;
}

const BAR_W = 3;
const BAR_GAP = 2;
const WAVE_H = 36;

export const AudioPlayer: React.FC<AudioPlayerProps> = ({ url, isVoiceMessage, filename }) => {
  const audioRef = useRef<HTMLAudioElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);

  const [playing, setPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [ready, setReady] = useState(false);
  const [loadError, setLoadError] = useState(false);
  const [fileSize, setFileSize] = useState<number | null>(null);

  const bars = useMemo(() => seededBars(url, 120), [url]);

  // ── waveform drawing ───────────────────────────────────────────────────────

  const draw = useCallback((progress: number) => {
    const canvas = canvasRef.current;
    const wrap = wrapRef.current;
    if (!canvas || !wrap) return;
    const dpr = window.devicePixelRatio || 1;
    const w = wrap.clientWidth;
    if (w === 0) return;
    canvas.width = w * dpr;
    canvas.height = WAVE_H * dpr;
    canvas.style.width = `${w}px`;
    canvas.style.height = `${WAVE_H}px`;
    const ctx = canvas.getContext('2d')!;
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, w, WAVE_H);

    const step = BAR_W + BAR_GAP;
    const count = Math.floor(w / step);
    const progressX = w * progress;

    for (let i = 0; i < count; i++) {
      const x = i * step;
      const barH = Math.max(3, bars[i % bars.length] * WAVE_H * 0.88);
      const y = (WAVE_H - barH) / 2;
      ctx.fillStyle = x < progressX ? '#32CD32' : 'rgba(50,205,50,0.22)';
      ctx.beginPath();
      ctx.roundRect(x, y, BAR_W, barH, 1.5);
      ctx.fill();
    }
  }, [bars]);

  useEffect(() => {
    draw(duration > 0 ? currentTime / duration : 0);
  }, [currentTime, duration, draw]);

  // ── audio element events ───────────────────────────────────────────────────

  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;
    const onMeta = () => { setDuration(audio.duration); setReady(true); };
    const onTime = () => setCurrentTime(audio.currentTime);
    const onPlay = () => setPlaying(true);
    const onPause = () => setPlaying(false);
    const onEnded = () => {
      setPlaying(false);
      setCurrentTime(0);
      audio.currentTime = 0;
    };
    audio.addEventListener('loadedmetadata', onMeta);
    audio.addEventListener('timeupdate', onTime);
    audio.addEventListener('play', onPlay);
    audio.addEventListener('pause', onPause);
    audio.addEventListener('ended', onEnded);
    if (audio.readyState >= 1) onMeta();
    return () => {
      audio.removeEventListener('loadedmetadata', onMeta);
      audio.removeEventListener('timeupdate', onTime);
      audio.removeEventListener('play', onPlay);
      audio.removeEventListener('pause', onPause);
      audio.removeEventListener('ended', onEnded);
    };
  }, []);

  // ── resize observer ────────────────────────────────────────────────────────

  useEffect(() => {
    const wrap = wrapRef.current;
    if (!wrap) return;
    const ro = new ResizeObserver(() => {
      const audio = audioRef.current;
      const dur = audio?.duration ?? duration;
      const cur = audio?.currentTime ?? currentTime;
      draw(dur > 0 ? cur / dur : 0);
    });
    ro.observe(wrap);
    return () => ro.disconnect();
  }, [draw, duration, currentTime]);

  // ── file size ──────────────────────────────────────────────────────────────

  useEffect(() => {
    if (isVoiceMessage) return;
    fetch(url, { method: 'HEAD' })
      .then(r => { const l = r.headers.get('content-length'); if (l) setFileSize(+l); })
      .catch(() => {});
  }, [url, isVoiceMessage]);

  // ── canvas pointer events ──────────────────────────────────────────────────

  const isDragging = useRef(false);

  const seekTo = useCallback((clientX: number) => {
    const canvas = canvasRef.current;
    const audio = audioRef.current;
    if (!canvas || !audio || !ready) return;
    const rect = canvas.getBoundingClientRect();
    const frac = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
    audio.currentTime = frac * audio.duration;
  }, [ready]);

  const onPointerDown = useCallback((e: React.PointerEvent) => {
    isDragging.current = true;
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    seekTo(e.clientX);
  }, [seekTo]);

  const onPointerMove = useCallback((e: React.PointerEvent) => {
    if (!isDragging.current) return;
    seekTo(e.clientX);
  }, [seekTo]);

  const onPointerUp = useCallback(() => { isDragging.current = false; }, []);

  // ── play / pause ───────────────────────────────────────────────────────────

  const toggle = useCallback(() => {
    const audio = audioRef.current;
    if (!audio) return;
    if (audio.paused) audio.play().catch(() => setLoadError(true));
    else audio.pause();
  }, []);

  // ── render ─────────────────────────────────────────────────────────────────

  const displayFilename = useMemo(() => {
    if (!filename) return null;
    return filename.replace(/^voice_message_\d+_?/, '');
  }, [filename]);

  const displayTime = playing ? currentTime : duration;

  if (loadError) {
    return (
      <a
        href={url}
        download={filename || (isVoiceMessage ? 'voice_message' : undefined)}
        className="message-attachment-file"
        style={{ marginTop: 4 }}
      >
        <PlayIcon /> {filename || (isVoiceMessage ? 'Voice message' : 'Audio file')} (download)
      </a>
    );
  }

  return (
    <div className={`audio-player${isVoiceMessage ? ' audio-player--voice' : ' audio-player--file'}`}>
      <audio ref={audioRef} src={url} preload="none" style={{ display: 'none' }} />

      <button
        className="audio-play-btn"
        onClick={toggle}
        title={playing ? 'Pause' : 'Play'}
      >
        {playing ? <PauseIcon /> : <PlayIcon />}
      </button>

      <div className="audio-player-right">
        {!isVoiceMessage && displayFilename && (
          <div className="audio-player-meta">
            <span className="audio-player-filename">{displayFilename}</span>
            {fileSize != null && <span className="audio-player-size">{fmtSize(fileSize)}</span>}
          </div>
        )}
        <div className="audio-waveform-wrap">
          <div ref={wrapRef} className="audio-waveform">
            <canvas
              ref={canvasRef}
              onPointerDown={onPointerDown}
              onPointerMove={onPointerMove}
              onPointerUp={onPointerUp}
              onPointerLeave={onPointerUp}
              style={{ display: 'block', cursor: 'pointer', height: `${WAVE_H}px` }}
            />
          </div>
          <span className="audio-player-time">{fmtTime(displayTime)}</span>
        </div>
      </div>
    </div>
  );
};
