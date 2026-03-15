import React from 'react';
import { EmojiSuggestion } from '../../hooks/useEmojiAutocomplete';
import './EmojiDropdown.css';

interface Props {
  suggestions: EmojiSuggestion[];
  selectedIdx: number;
  onSelect: (suggestion: EmojiSuggestion) => void;
}

export const EmojiDropdown: React.FC<Props> = ({ suggestions, selectedIdx, onSelect }) => {
  if (suggestions.length === 0) return null;

  return (
    <div className="emoji-dropdown">
      <div className="emoji-dropdown-header">Emoji</div>
      {suggestions.map((s, i) => (
        <div
          key={s.id}
          className={`emoji-option${i === selectedIdx ? ' selected' : ''}`}
          onMouseDown={e => { e.preventDefault(); onSelect(s); }}
        >
          <span className="emoji-option-native">{s.native}</span>
          <span className="emoji-option-name">:{s.id}:</span>
          <span className="emoji-option-label">{s.name}</span>
        </div>
      ))}
    </div>
  );
};
