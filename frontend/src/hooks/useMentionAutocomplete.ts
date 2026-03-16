import { useMemo } from 'react';
import { ServerMember } from '../api/types';

export interface MentionMatch {
  query: string; // text after @ up to cursor
  start: number; // index of '@' in the full text
  end: number;   // cursor position
}

export type MentionSuggestion =
  | { kind: 'member'; member: ServerMember }
  | { kind: 'special'; tag: string }; // '@everyone' | '@here'

const SPECIAL_TAGS = ['@everyone', '@here'];

/** Detect an active @mention at the cursor position. Returns null if not in one. */
export function detectMention(text: string, cursorPos: number): MentionMatch | null {
  const before = text.slice(0, cursorPos);
  const atIdx = before.lastIndexOf('@');
  if (atIdx === -1) return null;
  const query = before.slice(atIdx + 1);
  // Bail if there's any whitespace between @ and cursor — user moved past the mention
  if (/\s/.test(query)) return null;
  return { query, start: atIdx, end: cursorPos };
}

/** Filter members + special tags whose name starts with the query (case-insensitive, max 8). */
export function useMentionSuggestions(
  match: MentionMatch | null,
  members: ServerMember[],
): MentionSuggestion[] {
  return useMemo(() => {
    if (!match) return [];
    const q = match.query.toLowerCase();
    const specials: MentionSuggestion[] = SPECIAL_TAGS
      .filter(t => t.slice(1).startsWith(q)) // strip leading '@' for comparison
      .map(t => ({ kind: 'special', tag: t }));
    const memberSuggestions: MentionSuggestion[] = members
      .filter(m => m.username.toLowerCase().startsWith(q) || (m.display_name ?? '').toLowerCase().startsWith(q))
      .slice(0, 8 - specials.length)
      .map(m => ({ kind: 'member', member: m }));
    return [...specials, ...memberSuggestions];
  }, [match?.query, members]); // eslint-disable-line react-hooks/exhaustive-deps
}

/** Insert the suggestion text at the match position. Returns new text + cursor pos. */
export function insertMentionText(
  text: string,
  match: MentionMatch,
  suggestion: MentionSuggestion,
): { text: string; cursor: number } {
  const inserted = suggestion.kind === 'special'
    ? `${suggestion.tag} `
    : `@${suggestion.member.username} `;
  return {
    text: text.slice(0, match.start) + inserted + text.slice(match.end),
    cursor: match.start + inserted.length,
  };
}

const RESERVED = new Set(['everyone', 'here']);

/**
 * Replace @username patterns in `text` with <@userid> using the member list.
 * Called on send so the backend and renderer receive parseable mention tokens.
 * @everyone and @here are left as-is.
 */
export function resolveMentions(text: string, members: ServerMember[]): string {
  if (!text.includes('@')) return text;
  const map = new Map(members.map(m => [m.username.toLowerCase(), m.user_id]));
  return text.replace(/@(\S+)/g, (full, name) => {
    if (RESERVED.has(name.toLowerCase())) return full;
    const uid = map.get(name.toLowerCase());
    return uid ? `<@${uid}>` : full;
  });
}
