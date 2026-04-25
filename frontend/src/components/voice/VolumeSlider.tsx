import React from 'react';
import './VolumeSlider.css';

interface Props {
  value: number;
  onChange: (v: number) => void;
}

export const VolumeSlider: React.FC<Props> = ({ value, onChange }) => {
  return (
    <div className="vol-slider">
      <input
        type="range"
        min={0}
        max={200}
        step={5}
        value={value}
        onChange={e => onChange(Number(e.target.value))}
        aria-label="Listener volume"
      />
      <span className="vol-slider-value">{value}%</span>
    </div>
  );
};
