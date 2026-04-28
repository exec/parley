import { useEffect, useState } from 'react';
import { createHighlighter, type Highlighter, type ThemedToken } from 'shiki';

let highlighterPromise: Promise<Highlighter> | null = null;
const loadedLanguages = new Set<string>();

/**
 * Both Shiki themes ship in the bundle. Picked at highlight-time per the
 * active parley theme so light parley themes (sakura, citron-light, custom
 * light user themes) don't render unreadable near-white tokens.
 */
const THEMES = ['github-dark', 'github-light'] as const;
export type ShikiTheme = typeof THEMES[number];
const DEFAULT_LANG = 'plaintext';

export async function getHighlighter(): Promise<Highlighter> {
  if (!highlighterPromise) {
    highlighterPromise = createHighlighter({
      themes: [...THEMES],
      langs: [DEFAULT_LANG],
    });
  }
  return highlighterPromise;
}

export type { ThemedToken };

async function ensureLanguage(hl: Highlighter, language: string): Promise<boolean> {
  if (loadedLanguages.has(language) || language === DEFAULT_LANG) return true;
  try {
    await hl.loadLanguage(language as Parameters<Highlighter['loadLanguage']>[0]);
    loadedLanguages.add(language);
    return true;
  } catch {
    return false;
  }
}

export async function highlightLines(code: string, lang: string, theme?: ShikiTheme): Promise<ThemedToken[][]> {
  const hl = await getHighlighter();
  const language = lang || DEFAULT_LANG;
  const t = theme ?? parleyShikiTheme();
  if (!(await ensureLanguage(hl, language))) {
    return code.split('\n').map((line) => [{ content: line, offset: 0, htmlAttrs: {} }]);
  }
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const result = hl.codeToTokens(code, { lang: language as any, theme: t });
  return result.tokens;
}

export async function highlight(code: string, lang: string, theme?: ShikiTheme): Promise<string> {
  const hl = await getHighlighter();
  const language = lang || DEFAULT_LANG;
  const t = theme ?? parleyShikiTheme();
  if (!(await ensureLanguage(hl, language))) {
    return hl.codeToHtml(code, { lang: DEFAULT_LANG, theme: t });
  }
  return hl.codeToHtml(code, { lang: language, theme: t });
}

const EXT_MAP: Record<string, string> = {
  '.py': 'python', '.go': 'go', '.rs': 'rust',
  '.js': 'javascript', '.jsx': 'jsx', '.ts': 'typescript', '.tsx': 'tsx',
  '.sh': 'bash', '.bash': 'bash', '.zsh': 'bash',
  '.ps1': 'powershell', '.lua': 'lua',
  '.c': 'c', '.h': 'c', '.cpp': 'cpp', '.hpp': 'cpp',
  '.yaml': 'yaml', '.yml': 'yaml', '.json': 'json', '.toml': 'toml',
  '.rb': 'ruby', '.java': 'java',
  '.asm': 'asm', '.s': 'asm',
  '.html': 'html', '.css': 'css', '.scss': 'scss',
  '.sql': 'sql', '.md': 'markdown',
  '.dockerfile': 'dockerfile', '.tf': 'hcl',
  '.xml': 'xml',
};

export function languageFromFilename(filename: string): string {
  const ext = '.' + filename.split('.').pop()?.toLowerCase();
  return EXT_MAP[ext] || '';
}

export function isCodeFile(filename: string): boolean {
  const ext = '.' + filename.split('.').pop()?.toLowerCase();
  return ext in EXT_MAP;
}

// ─── theme detection ─────────────────────────────────────────────────────────

/**
 * Pick the Shiki theme that pairs well with the active parley theme.
 *
 * Method: read the computed `--parley-channel-bg` (the surface code blocks
 * actually render on) and pick light vs dark by relative luminance. Works for
 * any custom user theme, not just the built-in ones, because it inspects the
 * resolved CSS variable rather than guessing from a known list.
 */
export function parleyShikiTheme(): ShikiTheme {
  if (typeof window === 'undefined' || typeof document === 'undefined') return 'github-dark';
  const cs = getComputedStyle(document.documentElement);
  // Code blocks render on chat-bg (or the panel-bg fallback). Either is fine
  // as a luminance probe — both reflect the active theme's "background feel."
  const probe = cs.getPropertyValue('--parley-channel-bg').trim()
    || cs.getPropertyValue('--parley-bg-secondary').trim()
    || cs.getPropertyValue('--parley-app-bg').trim()
    || '';
  return colorIsLight(probe) ? 'github-light' : 'github-dark';
}

function colorIsLight(c: string): boolean {
  const hex = c.match(/^#([0-9a-f]{3,8})$/i);
  let r = 0, g = 0, b = 0;
  if (hex) {
    const h = hex[1];
    if (h.length === 3) {
      r = parseInt(h[0] + h[0], 16); g = parseInt(h[1] + h[1], 16); b = parseInt(h[2] + h[2], 16);
    } else if (h.length === 6 || h.length === 8) {
      r = parseInt(h.slice(0, 2), 16); g = parseInt(h.slice(2, 4), 16); b = parseInt(h.slice(4, 6), 16);
    }
  } else {
    const rgb = c.match(/rgba?\(\s*([\d.]+)\s*,\s*([\d.]+)\s*,\s*([\d.]+)/i);
    if (rgb) { r = +rgb[1]; g = +rgb[2]; b = +rgb[3]; }
    else return false; // unknown format → treat as dark
  }
  // ITU-R BT.601 luminance, threshold tuned slightly above 0.5 so very-pale
  // dark themes (deep purple/navy) still pick github-dark.
  const lum = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
  return lum > 0.6;
}

/**
 * React hook that returns the current Shiki theme and re-renders subscribers
 * when the parley theme changes (built-in switch via `body[data-theme]` or
 * custom-theme CSS injection via `<style id="custom-theme">`).
 *
 * Subscribe + add to a useEffect dep array so highlights re-render on switch.
 */
export function useShikiTheme(): ShikiTheme {
  const [theme, setTheme] = useState<ShikiTheme>(() => parleyShikiTheme());
  useEffect(() => {
    const update = () => {
      const next = parleyShikiTheme();
      setTheme(prev => (prev === next ? prev : next));
    };
    // body data-theme attribute swap (built-in theme picker)
    const bodyObs = new MutationObserver(update);
    bodyObs.observe(document.body, { attributes: true, attributeFilter: ['data-theme'] });
    // Custom theme CSS injected/replaced as <style id="custom-theme">.
    // We watch <head> for adds/removes of that node, plus the node itself
    // for textContent changes (the editor edits-in-place).
    let styleObs: MutationObserver | null = null;
    const watchStyle = () => {
      styleObs?.disconnect();
      styleObs = new MutationObserver(update);
      const s = document.getElementById('custom-theme');
      if (s) styleObs.observe(s, { childList: true, characterData: true, subtree: true });
    };
    const headObs = new MutationObserver(() => { watchStyle(); update(); });
    headObs.observe(document.head, { childList: true });
    watchStyle();
    return () => { bodyObs.disconnect(); headObs.disconnect(); styleObs?.disconnect(); };
  }, []);
  return theme;
}
