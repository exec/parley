import { BASE_VARS } from './themePreview';

export function hexToRgb(hex: string): string {
  const clean = hex.replace('#', '').trim();
  let r = 0, g = 0, b = 0;
  if (clean.length === 3) {
    r = parseInt(clean[0] + clean[0], 16);
    g = parseInt(clean[1] + clean[1], 16);
    b = parseInt(clean[2] + clean[2], 16);
  } else if (clean.length === 6) {
    r = parseInt(clean.slice(0, 2), 16);
    g = parseInt(clean.slice(2, 4), 16);
    b = parseInt(clean.slice(4, 6), 16);
  }
  if (isNaN(r) || isNaN(g) || isNaN(b)) return '0, 0, 0';
  return `${r}, ${g}, ${b}`;
}

function parseBaseVarHex(varsString: string, varName: string): string {
  const escaped = varName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const re = new RegExp(escaped + '\\s*:\\s*(#[0-9a-fA-F]{3,6})');
  const m = varsString.match(re);
  return m ? m[1] : '#000000';
}

export type GlassPreset = 'frosted' | 'clear';

export function buildGlassPreset(baseTheme: string, preset: GlassPreset): string {
  const vars = BASE_VARS[baseTheme] ?? BASE_VARS['abyss'];

  // Map BASE_VARS keys to glass roles:
  //   panel bg    → --parley-bg-secondary  (channel list / sidebar bg)
  //   header bg   → --parley-bg-hover      (closest to bg-tertiary in BASE_VARS)
  //   footer bg   → --parley-channel-bg    (closest to bg-primary in BASE_VARS)
  //   chat bg     → --parley-channel-bg
  const panelHex  = parseBaseVarHex(vars, '--parley-bg-secondary');
  const footerHex = parseBaseVarHex(vars, '--parley-channel-bg');
  const chatHex   = parseBaseVarHex(vars, '--parley-channel-bg');

  const opacity = preset === 'frosted'
    ? { panel: 0.60, header: 0.70, footer: 0.70, chat: 0.78 }
    : { panel: 0.28, header: 0.32, footer: 0.32, chat: 0.40 };
  const blur = preset === 'frosted' ? '14px' : '0px';

  return [
    `  --parley-app-bg: transparent;`,
    `  --parley-panel-bg: rgba(${hexToRgb(panelHex)}, ${opacity.panel});`,
    `  --parley-panel-blur: ${blur};`,
    `  --parley-panel-header-bg: transparent;`,
    `  --parley-panel-footer-bg: rgba(${hexToRgb(footerHex)}, ${opacity.footer});`,
    `  --parley-chat-bg: rgba(${hexToRgb(chatHex)}, ${opacity.chat});`,
  ].join('\n');
}

const GLASS_START = '/* bg-glass-start */';
const GLASS_END   = '/* bg-glass-end */';
const GLASS_PAT   = /\/\* bg-glass-start \*\/[\s\S]*?\/\* bg-glass-end \*\//;

export function injectGlassVars(css: string, glassVars: string | null): string {
  // Remove any existing glass block first.
  let cleaned = css.replace(GLASS_PAT, '').replace(/\n{3,}/g, '\n\n').trim();

  if (glassVars === null) return cleaned;

  const block = `${GLASS_START}\n${glassVars}\n${GLASS_END}`;

  // Insert block at END of [data-theme] so it overrides any duplicate declarations the LLM may add.
  const dataThemePat = /^(\[data-theme\]\s*\{)([\s\S]*?)(\n\})/m;
  if (dataThemePat.test(cleaned)) {
    return cleaned.replace(dataThemePat, `$1$2\n${block}\n$3`);
  }

  // No [data-theme] block — wrap in one.
  const wrapper = `[data-theme] {\n${block}\n}`;
  return cleaned ? `${wrapper}\n\n${cleaned}` : wrapper;
}
