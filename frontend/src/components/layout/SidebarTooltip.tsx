import React, { useEffect, useRef, useState, useCallback } from 'react';
import { createPortal } from 'react-dom';

/**
 * Hover tooltip rendered via React portal to document.body. Place inside the
 * trigger element — uses its parent's bounding rect to position itself to the
 * right of the trigger. Escapes any ancestor's overflow/clip/scroll context,
 * so the trigger's container can be width-constrained and overflow-hidden
 * without losing tooltip visibility.
 */
export const SidebarTooltip: React.FC<{ text: string }> = ({ text }) => {
  const anchorRef = useRef<HTMLSpanElement | null>(null);
  const [hovered, setHovered] = useState(false);
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);

  // Watch for hover state on our parent (the trigger) — bind listeners once on
  // mount and remove on unmount.
  useEffect(() => {
    const parent = anchorRef.current?.parentElement;
    if (!parent) return;
    const enter = () => setHovered(true);
    const leave = () => setHovered(false);
    parent.addEventListener('mouseenter', enter);
    parent.addEventListener('mouseleave', leave);
    return () => {
      parent.removeEventListener('mouseenter', enter);
      parent.removeEventListener('mouseleave', leave);
    };
  }, []);

  // Recompute position on hover + on viewport changes while shown.
  const reposition = useCallback(() => {
    const parent = anchorRef.current?.parentElement;
    if (!parent) return;
    const rect = parent.getBoundingClientRect();
    setPos({
      top: rect.top + rect.height / 2,
      left: rect.right + 12,
    });
  }, []);

  useEffect(() => {
    if (!hovered) return;
    reposition();
    const onChange = () => reposition();
    window.addEventListener('resize', onChange);
    window.addEventListener('scroll', onChange, true);
    return () => {
      window.removeEventListener('resize', onChange);
      window.removeEventListener('scroll', onChange, true);
    };
  }, [hovered, reposition]);

  return (
    <>
      <span ref={anchorRef} style={{ display: 'none' }} aria-hidden="true" />
      {hovered && pos && createPortal(
        <div
          className="sidebar-tooltip-portal"
          style={{
            position: 'fixed',
            top: pos.top,
            left: pos.left,
            transform: 'translateY(-50%)',
          }}
          role="tooltip"
        >
          {text}
        </div>,
        document.body,
      )}
    </>
  );
};
