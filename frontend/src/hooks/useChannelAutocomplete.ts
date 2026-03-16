import { useMemo } from 'react';
import { Channel } from '../api/types';

export interface ChannelTagMatch {
  query: string; // text after # up to cursor
  start: number; // index of '#' in the full text
  end: number;   // cursor position
}

/** Detect an active #channel tag at the cursor position. Returns null if not in one. */
export function detectChannelTag(text: string, cursorPos: number): ChannelTagMatch | null {
  const before = text.slice(0, cursorPos);
  const hashIdx = before.lastIndexOf('#');
  if (hashIdx === -1) return null;
  const query = before.slice(hashIdx + 1);
  // Bail if there's whitespace between # and cursor — user moved past the tag
  if (/\s/.test(query)) return null;
  return { query, start: hashIdx, end: cursorPos };
}

/** Filter text channels whose name starts with the query (case-insensitive, max 8). */
export function useChannelSuggestions(
  match: ChannelTagMatch | null,
  channels: Channel[],
): Channel[] {
  return useMemo(() => {
    if (!match) return [];
    const q = match.query.toLowerCase();
    return channels
      .filter(c => c.type === 0 && c.name.toLowerCase().startsWith(q))
      .slice(0, 8);
  }, [match?.query, channels]); // eslint-disable-line react-hooks/exhaustive-deps
}

/**
 * Insert a channel tag at the match position.
 * Stores <#channelID> in the text (invisible to user until render).
 */
export function insertChannelTag(
  text: string,
  match: ChannelTagMatch,
  channel: Channel,
): { text: string; cursor: number } {
  const inserted = `<#${channel.id}> `;
  return {
    text: text.slice(0, match.start) + inserted + text.slice(match.end),
    cursor: match.start + inserted.length,
  };
}

/**
 * Replace any remaining #channel-name patterns in `text` with <#channelID>.
 * Called on send so unresolved tags are still converted.
 */
export function resolveChannelTags(text: string, channels: Channel[]): string {
  if (!text.includes('#')) return text;
  const map = new Map(channels.map(c => [c.name.toLowerCase(), c.id]));
  return text.replace(/#(\S+)/g, (full, name) => {
    // Don't replace already-encoded tags like <#123>
    if (full.startsWith('<#')) return full;
    const id = map.get(name.toLowerCase());
    return id ? `<#${id}>` : full;
  });
}
