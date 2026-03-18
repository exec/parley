import React, { useRef } from 'react';
import { MentionSuggestion } from '../../hooks/useMentionAutocomplete';
import { useViewportAdjust } from '../../hooks/useViewportAdjust';
import './MentionDropdown.css';

interface Props {
  suggestions: MentionSuggestion[];
  selectedIdx: number;
  onSelect: (suggestion: MentionSuggestion) => void;
}

export const MentionDropdown: React.FC<Props> = ({ suggestions, selectedIdx, onSelect }) => {
  const ref = useRef<HTMLDivElement>(null);
  useViewportAdjust(ref, []);

  if (suggestions.length === 0) return null;

  return (
    <div ref={ref} className="mention-dropdown">
      <div className="mention-dropdown-header">Mentions</div>
      {suggestions.map((s, i) => {
        if (s.kind === 'special') {
          return (
            <div
              key={s.tag}
              className={`mention-option${i === selectedIdx ? ' selected' : ''}`}
              onMouseDown={e => { e.preventDefault(); onSelect(s); }}
            >
              <div className="mention-option-avatar mention-option-avatar--everyone">@</div>
              <span className="mention-option-name mention-option-name--everyone">{s.tag}</span>
              <span className="mention-option-everyone-hint">Notify all members</span>
            </div>
          );
        }
        const m = s.member;
        return (
          <div
            key={m.user_id}
            className={`mention-option${i === selectedIdx ? ' selected' : ''}`}
            onMouseDown={e => { e.preventDefault(); onSelect(s); }}
          >
            <div className="mention-option-avatar">
              {m.avatar_url
                ? <img src={m.avatar_url} alt={m.username} />
                : (m.display_name || m.username).charAt(0).toUpperCase()}
            </div>
            <span className="mention-option-name">{m.display_name || m.username}</span>
          </div>
        );
      })}
    </div>
  );
};
