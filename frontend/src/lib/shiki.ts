import { createHighlighter, type Highlighter } from 'shiki';

let highlighterPromise: Promise<Highlighter> | null = null;
const loadedLanguages = new Set<string>();

const parleyTheme = {
  name: 'parley',
  type: 'dark' as const,
  colors: {
    'editor.background': '#0a0a0a',
    'editor.foreground': '#e0e0e0',
  },
  tokenColors: [
    { scope: ['keyword', 'storage.type', 'storage.modifier'], settings: { foreground: '#32CD32' } },
    { scope: ['string', 'string.quoted'], settings: { foreground: '#228B22' } },
    { scope: ['comment', 'punctuation.definition.comment'], settings: { foreground: '#555555' } },
    { scope: ['constant.numeric', 'constant.language'], settings: { foreground: '#66ff66' } },
    { scope: ['entity.name.type', 'support.type'], settings: { foreground: '#44aa44' } },
    { scope: ['entity.name.function', 'support.function'], settings: { foreground: '#ffffff' } },
    { scope: ['variable', 'variable.other'], settings: { foreground: '#e0e0e0' } },
    { scope: ['punctuation'], settings: { foreground: '#888888' } },
    { scope: ['entity.name.tag'], settings: { foreground: '#32CD32' } },
    { scope: ['entity.other.attribute-name'], settings: { foreground: '#44aa44' } },
  ],
};

const DEFAULT_LANG = 'plaintext';

export async function getHighlighter(): Promise<Highlighter> {
  if (!highlighterPromise) {
    highlighterPromise = createHighlighter({
      themes: [parleyTheme],
      langs: [DEFAULT_LANG],
    });
  }
  return highlighterPromise;
}

export async function highlight(code: string, lang: string): Promise<string> {
  const hl = await getHighlighter();
  const language = lang || DEFAULT_LANG;
  if (!loadedLanguages.has(language) && language !== DEFAULT_LANG) {
    try {
      await hl.loadLanguage(language as Parameters<Highlighter['loadLanguage']>[0]);
      loadedLanguages.add(language);
    } catch {
      return hl.codeToHtml(code, { lang: DEFAULT_LANG, theme: 'parley' });
    }
  }
  return hl.codeToHtml(code, { lang: language, theme: 'parley' });
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
