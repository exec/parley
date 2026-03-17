// Key CSS variables for each built-in base theme, used to make previews accurate.
const BASE_VARS: Record<string, string> = {
  rory:          `--parley-sidebar:#050505;--parley-bg-secondary:#0a0a0a;--parley-bg-hover:#0d1a0d;--parley-text-muted:#228B22;--parley-text-normal:#32CD32;--parley-accent:#32CD32;--parley-channel-bg:#000000;--parley-input:#0a0a0a`,
  'citron-dark': `--parley-sidebar:#2f3136;--parley-bg-secondary:#2f3136;--parley-bg-hover:#32353b;--parley-text-muted:#72767d;--parley-text-normal:#dcddde;--parley-accent:#5865f2;--parley-channel-bg:#36393f;--parley-input:#40444b`,
  'citron-light':`--parley-sidebar:#e3e5e8;--parley-bg-secondary:#f2f3f5;--parley-bg-hover:#ebedef;--parley-text-muted:#747f8d;--parley-text-normal:#2e3338;--parley-accent:#5865f2;--parley-channel-bg:#f2f3f5;--parley-input:#ebedef`,
  'neon-nights': `--parley-sidebar:#0a011a;--parley-bg-secondary:#130333;--parley-bg-hover:#1e054d;--parley-text-muted:#c4b5e0;--parley-text-normal:#f0f0f0;--parley-accent:#ff2d78;--parley-channel-bg:#0d0221;--parley-input:#1e054d`,
  abyss:         `--parley-sidebar:#07101e;--parley-bg-secondary:#0d1f3c;--parley-bg-hover:#152c52;--parley-text-muted:#8ba3bf;--parley-text-normal:#cad5e2;--parley-accent:#00b4d8;--parley-channel-bg:#0a1628;--parley-input:#152c52`,
  sakura:        `--parley-sidebar:#f8e0ec;--parley-bg-secondary:#fde8f0;--parley-bg-hover:#fce0ec;--parley-text-muted:#8d5a78;--parley-text-normal:#3d1a2e;--parley-accent:#d4609c;--parley-channel-bg:#fff9fb;--parley-input:#fde8f0`,
};

function esc(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

/** Returns a CSS block that sets the base theme variables on [data-theme]. */
export function themeVarsCSS(base: string): string {
  const vars = BASE_VARS[base] ?? BASE_VARS.rory;
  return `[data-theme]{${vars}}`;
}

/**
 * Builds a complete self-contained HTML preview for use in ThemeLinkEmbed.
 * Slightly scaled-down sizing so it fits neatly in the embed card.
 */
export function buildEmbedPreviewHTML(
  base: string,
  customCSS: string,
  displayName: string,
  avatarUrl: string | null | undefined,
): string {
  const initial = esc((displayName.charAt(0) || '?').toUpperCase());
  const safeName = esc(displayName || 'You');
  const safeCss = customCSS.replace(/<\/style>/gi, '');
  const avatarEl = avatarUrl
    ? `<img src="${esc(avatarUrl)}" class="av">`
    : `<div class="av av-letter">${initial}</div>`;

  return `<!DOCTYPE html><html><head><meta charset="utf-8"><style>
*{margin:0;padding:0;box-sizing:border-box}
${themeVarsCSS(base)}
html,body{height:100%}
body{font-family:sans-serif;background:var(--parley-channel-bg,#000);color:var(--parley-text-normal,#fff);display:flex;overflow:hidden}
.sb{width:32px;background:var(--parley-sidebar,#111);flex-shrink:0}
.ch{width:84px;background:var(--parley-bg-secondary,#0a0a0a);padding:5px;flex-shrink:0}
.ch h4{font-size:8px;color:var(--parley-text-muted,#666);margin-bottom:4px;font-weight:700;text-transform:uppercase;letter-spacing:.5px}
.c{font-size:9px;color:var(--parley-text-muted,#666);padding:2px 3px;border-radius:2px}
.c.a{background:var(--parley-bg-hover,#1a1a1a);color:var(--parley-text-normal,#fff)}
.chat{flex:1;background:var(--parley-channel-bg,#000);padding:7px;display:flex;flex-direction:column;justify-content:flex-end;overflow:hidden}
.m{margin-bottom:4px;display:flex;align-items:flex-start;gap:5px}
.av{width:18px;height:18px;border-radius:50%;object-fit:cover;flex-shrink:0}
.av-letter{background:var(--parley-accent,#888);display:flex;align-items:center;justify-content:center;font-size:9px;font-weight:700;color:#fff}
.mc{flex:1;min-width:0}
.n{font-size:9px;font-weight:700;color:var(--parley-accent,#32CD32)}
.t{font-size:9px;color:var(--parley-text-normal,#eee)}
.inp{background:var(--parley-input,#111);border-radius:3px;padding:3px 6px;font-size:9px;color:var(--parley-text-muted,#666)}
</style><style>${safeCss}</style></head>
<body data-theme="${esc(base)}">
<div class="sb"></div>
<div class="ch"><h4>channels</h4><div class="c a"># general</div><div class="c"># random</div><div class="c"># memes</div></div>
<div class="chat">
<div class="m"><div class="av av-letter" style="background:var(--parley-bg-hover,#333);color:var(--parley-text-muted,#888)">B</div><div class="mc"><div class="n">Bob</div><div class="t">Looks great! 👌</div></div></div>
<div class="m">${avatarEl}<div class="mc"><div class="n">${safeName}</div><div class="t">Thanks!</div></div></div>
<div class="inp">Message #general</div>
</div>
</body></html>`;
}
