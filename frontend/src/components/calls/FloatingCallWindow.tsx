import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useCall } from '../../context/CallContext';
import './FloatingCallWindow.css';

const STORAGE_KEY = 'parley.floatingPosition';

interface Position { x: number; y: number; }

function readPos(): Position {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) return JSON.parse(raw);
  } catch {}
  return { x: window.innerWidth - 360, y: 24 };
}

interface Props {
  renderCompact: () => React.ReactNode;
  onExpand: () => void;
}

export const FloatingCallWindow: React.FC<Props> = ({ renderCompact, onExpand }) => {
  const { state, floatingMode, setFloatingMode } = useCall();
  const [pos, setPos] = useState<Position>(readPos);
  const dragRef = useRef<{ startX: number; startY: number; origX: number; origY: number } | null>(null);

  useEffect(() => { localStorage.setItem(STORAGE_KEY, JSON.stringify(pos)); }, [pos]);

  const onMouseDown = useCallback((e: React.MouseEvent) => {
    dragRef.current = { startX: e.clientX, startY: e.clientY, origX: pos.x, origY: pos.y };
    e.preventDefault();
  }, [pos]);

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!dragRef.current) return;
      const dx = e.clientX - dragRef.current.startX;
      const dy = e.clientY - dragRef.current.startY;
      setPos({
        x: Math.max(0, Math.min(window.innerWidth - 320, dragRef.current.origX + dx)),
        y: Math.max(0, Math.min(window.innerHeight - 240, dragRef.current.origY + dy)),
      });
    };
    const onUp = () => { dragRef.current = null; };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
    return () => { window.removeEventListener('mousemove', onMove); window.removeEventListener('mouseup', onUp); };
  }, []);

  if (!floatingMode || state !== 'connected') return null;

  return (
    <div className="floating-call-window" style={{ left: pos.x, top: pos.y }}>
      <div className="floating-call-handle" onMouseDown={onMouseDown}>
        <span>Call</span>
        <span className="floating-call-actions">
          <button onClick={onExpand} title="Expand" aria-label="Expand to full">⛶</button>
          <button onClick={() => setFloatingMode(false)} title="Dock" aria-label="Dock">▭</button>
        </span>
      </div>
      <div className="floating-call-body">
        {renderCompact()}
      </div>
    </div>
  );
};
