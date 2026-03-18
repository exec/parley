import { createHighlighter, type Highlighter, type ThemedToken } from 'shiki';

let highlighterPromise: Promise<Highlighter> | null = null;
const loadedLanguages = new Set<string>();

const THEME = 'github-dark';
const DEFAULT_LANG = 'plaintext';

export async function getHighlighter(): Promise<Highlighter> {
  if (!highlighterPromise) {
    highlighterPromise = createHighlighter({
      themes: [THEME],
      langs: [DEFAULT_LANG],
    });
  }
  return highlighterPromise;
}

export type { ThemedToken };

export async function highlightLines(code: string, lang: string): Promise<ThemedToken[][]> {
  const hl = await getHighlighter();
  const language = lang || DEFAULT_LANG;
  if (!loadedLanguages.has(language) && language !== DEFAULT_LANG) {
    try {
      await hl.loadLanguage(language as Parameters<Highlighter['loadLanguage']>[0]);
      loadedLanguages.add(language);
    } catch {
      // Language not supported — return plain tokens (one per line)
      return code.split('\n').map((line) => [{ content: line, offset: 0, htmlAttrs: {} }]);
    }
  }
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const result = hl.codeToTokens(code, { lang: language as any, theme: THEME });
  return result.tokens;
}

export async function highlight(code: string, lang: string): Promise<string> {
  const hl = await getHighlighter();
  const language = lang || DEFAULT_LANG;
  if (!loadedLanguages.has(language) && language !== DEFAULT_LANG) {
    try {
      await hl.loadLanguage(language as Parameters<Highlighter['loadLanguage']>[0]);
      loadedLanguages.add(language);
    } catch {
      return hl.codeToHtml(code, { lang: DEFAULT_LANG, theme: THEME });
    }
  }
  return hl.codeToHtml(code, { lang: language, theme: THEME });
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
