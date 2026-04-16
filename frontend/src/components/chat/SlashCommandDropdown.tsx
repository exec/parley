import React, { useRef } from 'react';
import { BotCommand } from '../../api/types';
import { useViewportAdjust } from '../../hooks/useViewportAdjust';
import './SlashCommandDropdown.css';

interface Props {
  suggestions: BotCommand[];
  selectedIdx: number;
  onSelect: (command: BotCommand) => void;
  /** True when the server has zero registered commands (regardless of filter). */
  empty?: boolean;
  loading?: boolean;
}

export const SlashCommandDropdown: React.FC<Props> = ({
  suggestions,
  selectedIdx,
  onSelect,
  empty,
  loading,
}) => {
  const ref = useRef<HTMLDivElement>(null);
  useViewportAdjust(ref, [suggestions.length, empty, loading]);

  // Render nothing when we have neither matches nor an empty-state hint to show.
  if (!empty && !loading && suggestions.length === 0) return null;

  return (
    <div ref={ref} className="slash-command-dropdown">
      <div className="slash-command-dropdown-header">Commands</div>
      {loading && suggestions.length === 0 && (
        <div className="slash-command-empty">Loading commands…</div>
      )}
      {!loading && empty && suggestions.length === 0 && (
        <div className="slash-command-empty">No commands registered in this server.</div>
      )}
      {suggestions.map((cmd, i) => (
        <div
          key={cmd.id}
          className={`slash-command-option${i === selectedIdx ? ' selected' : ''}`}
          onMouseDown={e => { e.preventDefault(); onSelect(cmd); }}
        >
          <span className="slash-command-option-name">/{cmd.name}</span>
          {cmd.description && (
            <span className="slash-command-option-desc">{cmd.description}</span>
          )}
          {cmd.bot_display_name && (
            <span className="slash-command-option-bot">{cmd.bot_display_name}</span>
          )}
        </div>
      ))}
    </div>
  );
};
