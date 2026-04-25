import React from 'react';
import './MosaicAvatar.css';

export interface MosaicTile {
  avatarUrl?: string;
  displayName: string;
}

interface Props {
  tiles: MosaicTile[];
  size?: number; // px, default 40
  className?: string;
}

export const MosaicAvatar: React.FC<Props> = ({ tiles, size = 40, className = '' }) => {
  // Cap at 4 tiles for visual; ignore overflow.
  const visible = tiles.slice(0, 4);
  const layout = visible.length || 1;

  return (
    <div
      className={`mosaic-avatar mosaic-${layout} ${className}`.trim()}
      style={{ width: size, height: size }}
      aria-hidden="true"
    >
      {visible.length === 0 ? (
        <div className="mosaic-tile">
          <span className="mosaic-initials">?</span>
        </div>
      ) : (
        visible.map((t, i) => (
          <div
            key={i}
            className="mosaic-tile"
            style={{ backgroundImage: t.avatarUrl ? `url(${t.avatarUrl})` : undefined }}
          >
            {!t.avatarUrl && <span className="mosaic-initials">{initials(t.displayName)}</span>}
          </div>
        ))
      )}
    </div>
  );
};

function initials(name: string): string {
  const parts = name.split(/\s+/).filter(Boolean);
  return ((parts[0]?.[0] ?? '?') + (parts[1]?.[0] ?? '')).toUpperCase();
}
