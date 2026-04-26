import React, { useEffect, useRef, useState, useCallback } from 'react';
import './SplitVoiceChat.css';

interface Props {
  voice: React.ReactNode;
  chat: React.ReactNode;
  /**
   * localStorage key suffix to remember the last open size for this conversation
   * (e.g., the DM channel id). Different DMs/GCs keep independent splits.
   */
  storageKey: string;
}

const STORAGE_PREFIX = 'parley:splitVC:chatPx:';
const DEFAULT_RATIO = 0.5;
const SNAP_CLOSED_BELOW_PX = 80;
const MIN_OPEN_PX = 120;
const MIN_VOICE_PX = 220;

export const SplitVoiceChat: React.FC<Props> = ({ voice, chat, storageKey }) => {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [containerH, setContainerH] = useState(0);
  const [chatPx, setChatPx] = useState<number>(() => {
    const raw = localStorage.getItem(STORAGE_PREFIX + storageKey);
    const n = raw === null ? NaN : Number(raw);
    return Number.isFinite(n) ? n : -1; // -1 sentinel: not yet sized (use ratio)
  });
  const [dragging, setDragging] = useState(false);
  const lastOpenPxRef = useRef<number>(0);
  const movedDuringDragRef = useRef(false);

  // Track container height so we can apply ratio defaults + clamp drag values.
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const ro = new ResizeObserver(entries => {
      for (const e of entries) setContainerH(e.contentRect.height);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // First-paint default: 50% of container.
  useEffect(() => {
    if (chatPx === -1 && containerH > 0) {
      setChatPx(Math.round(containerH * DEFAULT_RATIO));
    }
  }, [chatPx, containerH]);

  // Persist open sizes only (skip closed state — closing doesn't overwrite the
  // remembered open height so re-opening returns to the last drag position).
  useEffect(() => {
    if (chatPx > 0) {
      localStorage.setItem(STORAGE_PREFIX + storageKey, String(chatPx));
      lastOpenPxRef.current = chatPx;
    }
  }, [chatPx, storageKey]);

  const onPointerDown = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    (e.target as HTMLElement).setPointerCapture?.(e.pointerId);
    setDragging(true);
    movedDuringDragRef.current = false;
  }, []);

  const onPointerMove = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!dragging) return;
    const el = containerRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    // Pointer Y relative to container; chat extends from pointer to bottom.
    const proposedChat = Math.round(rect.bottom - e.clientY);
    const maxChat = Math.max(0, rect.height - MIN_VOICE_PX);
    const clamped = Math.min(maxChat, Math.max(0, proposedChat));
    movedDuringDragRef.current = true;
    setChatPx(clamped < SNAP_CLOSED_BELOW_PX ? 0 : Math.max(MIN_OPEN_PX, clamped));
  }, [dragging]);

  const onPointerUp = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    (e.target as HTMLElement).releasePointerCapture?.(e.pointerId);
    setDragging(false);
  }, []);

  // Click-to-reopen when closed: tapping the resizer bar restores last height.
  // Suppress when the click is the tail end of a drag (otherwise dragging the
  // chat closed snaps it right back open via the click that follows pointerup).
  const onResizerClick = useCallback(() => {
    if (movedDuringDragRef.current) {
      movedDuringDragRef.current = false;
      return;
    }
    if (chatPx !== 0) return;
    const restore = lastOpenPxRef.current > 0
      ? lastOpenPxRef.current
      : Math.round(containerH * DEFAULT_RATIO);
    setChatPx(restore);
  }, [chatPx, containerH]);

  const closed = chatPx === 0;
  const effectiveChatPx = chatPx === -1 ? Math.round(containerH * DEFAULT_RATIO) : chatPx;
  // Voice panel height = container minus chat (no gap; the resizer overlays the seam).
  const voicePx = containerH > 0 ? containerH - (closed ? 0 : effectiveChatPx) : 0;

  return (
    <div ref={containerRef} className="split-vc">
      <div className="split-vc__voice" style={{ flexBasis: containerH > 0 ? `${voicePx}px` : `${100 * (1 - DEFAULT_RATIO)}%` }}>
        {voice}
      </div>
      <div
        className={`split-vc__resizer${dragging ? ' split-vc__resizer--dragging' : ''}${closed ? ' split-vc__resizer--closed' : ''}`}
        style={{ top: containerH > 0 ? `${voicePx}px` : '50%' }}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onClick={closed ? onResizerClick : undefined}
        role="separator"
        aria-orientation="horizontal"
        aria-label={closed ? 'Open text chat' : 'Drag to resize text chat'}
        title={closed ? 'Click to open text chat' : 'Drag to resize'}
      >
        <span className="split-vc__grip" />
      </div>
      <div
        className={`split-vc__chat${closed ? ' split-vc__chat--closed' : ''}`}
        style={{ flexBasis: closed ? 0 : `${effectiveChatPx}px` }}
      >
        {chat}
      </div>
    </div>
  );
};
