import { RefObject, useLayoutEffect } from 'react';

const MARGIN = 8;

/**
 * After the referenced element renders, nudges it (via CSS transform translate)
 * to ensure it stays fully within the viewport.
 * Works for both position:fixed and position:absolute elements.
 */
export function useViewportAdjust(
  ref: RefObject<HTMLElement | null>,
  deps: unknown[] = []
): void {
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;

    // Reset any previous adjustment before re-measuring
    el.style.transform = '';

    const rect = el.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;

    let dx = 0;
    let dy = 0;

    // Right edge overflow → shift left
    if (rect.right > vw - MARGIN) dx = vw - MARGIN - rect.right;
    // Left edge overflow → shift right (after applying dx)
    if (rect.left + dx < MARGIN) dx = MARGIN - rect.left;
    // Bottom edge overflow → shift up
    if (rect.bottom > vh - MARGIN) dy = vh - MARGIN - rect.bottom;
    // Top edge overflow → shift down (after applying dy)
    if (rect.top + dy < MARGIN) dy = MARGIN - rect.top;

    if (dx !== 0 || dy !== 0) {
      el.style.transform = `translate(${dx}px, ${dy}px)`;
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
}
