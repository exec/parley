import React, { useRef } from 'react';
import { Channel } from '../../api/types';
import { useViewportAdjust } from '../../hooks/useViewportAdjust';
import './ChannelTagDropdown.css';

interface Props {
  suggestions: Channel[];
  selectedIdx: number;
  onSelect: (channel: Channel) => void;
}

export const ChannelTagDropdown: React.FC<Props> = ({ suggestions, selectedIdx, onSelect }) => {
  const ref = useRef<HTMLDivElement>(null);
  useViewportAdjust(ref, []);

  if (suggestions.length === 0) return null;

  return (
    <div ref={ref} className="channel-tag-dropdown">
      <div className="channel-tag-dropdown-header">Channels</div>
      {suggestions.map((ch, i) => (
        <div
          key={ch.id}
          className={`channel-tag-option${i === selectedIdx ? ' selected' : ''}`}
          onMouseDown={e => { e.preventDefault(); onSelect(ch); }}
        >
          <span className="channel-tag-option-icon">#</span>
          <span className="channel-tag-option-name">{ch.name}</span>
        </div>
      ))}
    </div>
  );
};
