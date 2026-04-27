import { useState, useEffect } from 'react';
import { SearchIndex, init } from 'emoji-mart';

export interface EmojiMatch {
  query: string; // text between : and cursor
  start: number; // index of the opening ':'
  end: number;   // cursor position
}

export interface EmojiSuggestion {
  id: string;
  name: string;
  native: string; // actual emoji character
}

// Lazy-load emoji-mart data on first need (dynamic import keeps it out of the
// main bundle; the data is ~430KB raw and only used by users who type :emoji:).
let initPromise: Promise<any> | null = null;
let loadedData: any = null;
function ensureInit(): Promise<any> {
  if (!initPromise) {
    initPromise = import('@emoji-mart/data').then(mod => {
      loadedData = mod.default;
      return init({ data: loadedData }).then(() => loadedData);
    });
  }
  return initPromise;
}

/** Detect an active :emoji: trigger at the cursor. Requires ≥2 chars after ':'. */
export function detectEmojiTrigger(text: string, cursorPos: number): EmojiMatch | null {
  const before = text.slice(0, cursorPos);
  const colonIdx = before.lastIndexOf(':');
  if (colonIdx === -1) return null;
  const query = before.slice(colonIdx + 1);
  // Need at least 2 chars, no spaces, no nested colons
  if (query.length < 2 || /[\s:]/.test(query)) return null;
  // Warm the emoji data load on first trigger so resolveEmojis has it ready
  // by the time the user hits send.
  ensureInit();
  return { query, start: colonIdx, end: cursorPos };
}

/** Search emojis matching the query. Returns up to 10 results. */
export function useEmojiSuggestions(match: EmojiMatch | null): EmojiSuggestion[] {
  const [suggestions, setSuggestions] = useState<EmojiSuggestion[]>([]);

  useEffect(() => {
    if (!match) { setSuggestions([]); return; }
    let cancelled = false;
    ensureInit().then(() =>
      (SearchIndex.search(match.query) as Promise<any[]>).then(results => {
        if (cancelled) return;
        setSuggestions(
          (results ?? []).slice(0, 10).map(e => ({
            id: e.id as string,
            name: e.name as string,
            native: e.skins[0].native as string,
          }))
        );
      })
    );
    return () => { cancelled = true; };
  }, [match?.query]); // eslint-disable-line react-hooks/exhaustive-deps

  return suggestions;
}

/** Replace the :trigger at match position with the emoji char + space. */
export function insertEmoji(
  text: string,
  match: EmojiMatch,
  native: string,
): { text: string; cursor: number } {
  const inserted = native + ' ';
  return {
    text: text.slice(0, match.start) + inserted + text.slice(match.end),
    cursor: match.start + inserted.length,
  };
}

/** Convert any remaining :shortcode: patterns to native emoji on send.
 * If the emoji data hasn't loaded yet, returns text unchanged — callers that
 * need resolution should await getEmojiData() first.
 */
export function resolveEmojis(text: string): string {
  if (!text.includes(':')) return text;
  const d = loadedData;
  if (!d) return text;
  return text.replace(/:([a-z0-9_+\-]+):/gi, (full, shortcode) => {
    const key = shortcode.toLowerCase();
    const emoji = d.emojis[key] ?? d.emojis[d.aliases?.[key]];
    return emoji ? emoji.skins[0].native : full;
  });
}

/** Returns a promise that resolves once emoji data is loaded. Use this if you
 * need resolveEmojis to actually replace shortcodes before send.
 */
export function getEmojiData(): Promise<any> {
  return ensureInit();
}
