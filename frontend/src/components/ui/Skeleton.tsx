import React from 'react';
import './styles.css';

type SkeletonVariant = 'line' | 'block' | 'avatar' | 'circle';

interface SkeletonProps {
  variant?: SkeletonVariant;
  width?: number | string;
  height?: number | string;
  className?: string;
  style?: React.CSSProperties;
}

/**
 * Lightweight loading placeholder with a subtle shimmer.
 * Use to reserve layout space while data is loading so the page
 * doesn't shift when real content arrives.
 *
 * Variants:
 *   - line   (default): a one-line bar of text
 *   - block  : a rectangular area (cards, rows, paragraphs)
 *   - avatar : a square block sized like an avatar (32px)
 *   - circle : a circular swatch (uses width/height)
 */
export const Skeleton: React.FC<SkeletonProps> = ({
  variant = 'line',
  width,
  height,
  className = '',
  style,
}) => {
  const classes = ['skeleton', `skeleton-${variant}`, className]
    .filter(Boolean)
    .join(' ');

  const inlineStyle: React.CSSProperties = { ...style };
  if (width !== undefined) inlineStyle.width = typeof width === 'number' ? `${width}px` : width;
  if (height !== undefined) inlineStyle.height = typeof height === 'number' ? `${height}px` : height;

  return <div className={classes} style={inlineStyle} aria-hidden="true" />;
};
