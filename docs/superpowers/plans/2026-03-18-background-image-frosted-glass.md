# Background Image & Frosted Glass Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make custom theme background images visible through a frosted glass effect by adding six CSS variables that control transparency and blur on every layout panel.

**Architecture:** Six new `--parley-*` CSS variables are added as overridable fallbacks to existing `background-color` rules across 7 CSS files — zero visual change unless the variables are set. The theme editor gains preset buttons that compute `rgba()` values from the current base theme's hex palette and inject a sentinel-delimited variable block into the CSS textarea. The AI system prompt is updated to document the new variables.

**Tech Stack:** CSS custom properties, `backdrop-filter`, React (CustomThemeEditor.tsx), TypeScript utility functions, Go (AI system prompt string constant).

**Spec:** `docs/superpowers/specs/2026-03-18-background-image-frosted-glass-design.md`

---

## File Map

| File | Change |
|---|---|
| `frontend/src/components/layout/MainLayout.css` | Add variable fallbacks to `.main-layout`, `.main-content`, `.vc-chat-sidebar` |
| `frontend/src/components/layout/Sidebar.css` | Add variable fallback + blur to `.sidebar` |
| `frontend/src/components/layout/ChannelList.css` | Add variable fallbacks + blur to `.channel-list`, `.server-header`, `.user-area` |
| `frontend/src/components/layout/UserSidebar.css` | Add variable fallback + blur to `.user-sidebar` |
| `frontend/src/components/layout/DmPanel.css` | Add variable fallbacks + blur to `.dm-panel`, `.dm-panel-user-area` |
| `frontend/src/components/chat/Chat.css` | Add variable fallback to `.chat-container`, `.chat-window`, `.chat-header`, `.message-input-container` |
| `frontend/src/lib/themeGlass.ts` | **New file** — `hexToRgb`, `buildGlassPreset`, `injectGlassVars` utilities |
| `frontend/src/components/settings/CustomThemeEditor.tsx` | Import utilities, add preset buttons to Background Image section, auto-apply on upload |
| `internal/ai/worker.go` | Append frosted glass variable docs to `systemPrompt` constant |

---

## Task 1: CSS variable hooks — layout wrappers

**Files:**
- Modify: `frontend/src/components/layout/MainLayout.css`

- [ ] **Step 1: Add variable fallbacks to `.main-layout` and `.main-content`**

In `MainLayout.css`, find and update:

```css
/* Before */
.main-layout {
  ...
  background-color: var(--bg-primary);
  ...
}

.main-content {
  ...
  background-color: var(--bg-primary);
  ...
}
```

```css
/* After */
.main-layout {
  ...
  background-color: var(--parley-app-bg, var(--bg-primary));
  ...
}

.main-content {
  ...
  background-color: var(--parley-app-bg, var(--bg-primary));
  ...
}
```

- [ ] **Step 2: Add variable fallback to `.vc-chat-sidebar`**

`.vc-chat-sidebar` uses `background:` shorthand (not `background-color:`). Update it and add blur:

```css
/* Before */
.vc-chat-sidebar {
  ...
  background: var(--bg-primary);
  ...
}
```

```css
/* After */
.vc-chat-sidebar {
  ...
  background: var(--parley-panel-bg, var(--bg-primary));
  backdrop-filter: blur(var(--parley-panel-blur, 0px));
  -webkit-backdrop-filter: blur(var(--parley-panel-blur, 0px));
  ...
}
```

- [ ] **Step 3: Verify no visual change**

Open the app in a browser. The layout should look exactly the same as before.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/layout/MainLayout.css
git commit -m "feat: add parley-app-bg and parley-panel-bg hooks to layout wrappers"
```

---

## Task 2: CSS variable hooks — sidebar panels

**Files:**
- Modify: `frontend/src/components/layout/Sidebar.css`
- Modify: `frontend/src/components/layout/UserSidebar.css`

- [ ] **Step 1: Update `.sidebar` in Sidebar.css**

```css
/* Before */
.sidebar {
  ...
  background-color: var(--bg-primary);
  ...
}
```

```css
/* After */
.sidebar {
  ...
  background-color: var(--parley-panel-bg, var(--bg-primary));
  backdrop-filter: blur(var(--parley-panel-blur, 0px));
  -webkit-backdrop-filter: blur(var(--parley-panel-blur, 0px));
  ...
}
```

Note: `.sidebar::-webkit-scrollbar-track` also has `background: var(--bg-primary)` — leave that one alone (scrollbar tracks don't need to be transparent).

- [ ] **Step 2: Update `.user-sidebar` in UserSidebar.css**

```css
/* Before */
.user-sidebar {
  ...
  background-color: var(--bg-secondary);
  ...
}
```

```css
/* After */
.user-sidebar {
  ...
  background-color: var(--parley-panel-bg, var(--bg-secondary));
  backdrop-filter: blur(var(--parley-panel-blur, 0px));
  -webkit-backdrop-filter: blur(var(--parley-panel-blur, 0px));
  ...
}
```

Note: `.user-sidebar::-webkit-scrollbar` track at line 56 (`background: var(--bg-secondary)`) — leave alone.

- [ ] **Step 3: Verify no visual change**

Open the app. Sidebar and member list should look identical to before.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/layout/Sidebar.css frontend/src/components/layout/UserSidebar.css
git commit -m "feat: add panel-bg and panel-blur hooks to sidebar and member list"
```

---

## Task 3: CSS variable hooks — channel list and DM panel

**Files:**
- Modify: `frontend/src/components/layout/ChannelList.css`
- Modify: `frontend/src/components/layout/DmPanel.css`

- [ ] **Step 1: Update three selectors in ChannelList.css**

`.channel-list` (line 5) — panel bg + blur:
```css
.channel-list {
  ...
  background-color: var(--parley-panel-bg, var(--bg-secondary));
  backdrop-filter: blur(var(--parley-panel-blur, 0px));
  -webkit-backdrop-filter: blur(var(--parley-panel-blur, 0px));
  ...
}
```

`.server-header` (line 18) — panel header bg (no blur — it's inside an already-blurred panel):
```css
.server-header {
  ...
  background-color: var(--parley-panel-header-bg, var(--bg-tertiary));
  ...
}
```

`.user-area` (line 264) — panel footer bg (no blur):
```css
.user-area {
  ...
  background-color: var(--parley-panel-footer-bg, var(--bg-primary));
  ...
}
```

- [ ] **Step 2: Check DmPanel.css for relevant selectors**

Read `frontend/src/components/layout/DmPanel.css` and find `.dm-panel` and `.dm-panel-user-area`. Apply the same pattern:

`.dm-panel` → `var(--parley-panel-bg, var(--bg-secondary))` + blur
`.dm-panel-user-area` → `var(--parley-panel-footer-bg, var(--bg-primary))` (no blur)

- [ ] **Step 3: Verify no visual change in server view and DM view**

Switch between a server channel and a DM. Both should look identical to before.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/layout/ChannelList.css frontend/src/components/layout/DmPanel.css
git commit -m "feat: add panel bg hooks to channel list, server header, user area, and DM panel"
```

---

## Task 4: CSS variable hooks — chat area

**Files:**
- Modify: `frontend/src/components/chat/Chat.css`

- [ ] **Step 1: Update four selectors in Chat.css**

`.chat-container` (line 6):
```css
.chat-container {
  ...
  background-color: var(--parley-chat-bg, var(--parley-channel-bg));
  ...
}
```

`.chat-window` (line 366):
```css
.chat-window {
  ...
  background-color: var(--parley-chat-bg, var(--parley-channel-bg));
  ...
}
```

`.chat-header` (line 373):
```css
.chat-header {
  ...
  background-color: var(--parley-chat-bg, var(--parley-channel-bg));
  ...
}
```

`.message-input-container` (line 254):
```css
.message-input-container {
  ...
  background-color: var(--parley-chat-bg, var(--parley-channel-bg));
  ...
}
```

- [ ] **Step 2: Verify no visual change**

Open a channel. Message list, header, and input area should look identical.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/chat/Chat.css
git commit -m "feat: add parley-chat-bg hook to chat area elements"
```

---

## Task 5: Glass utility library

**Files:**
- Create: `frontend/src/lib/themeGlass.ts`

This file provides three pure functions used by the theme editor preset buttons.

- [ ] **Step 1: Create the file with `hexToRgb`**

```typescript
/**
 * Converts a CSS hex color string to an RGB tuple string suitable for rgba().
 * Supports both #rrggbb and #rgb shorthand.
 * Returns "0, 0, 0" on invalid input.
 */
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
```

- [ ] **Step 2: Add `parseBaseVarHex` helper**

This parses a single variable value out of the `--key:#hexval` compact string used by `BASE_VARS` in `themePreview.ts`.

```typescript
/**
 * Extracts a hex color value for a given variable name from a BASE_VARS string.
 * BASE_VARS entries look like: "--parley-sidebar:#050505;--parley-bg-secondary:#0a0a0a;..."
 * Returns "#000000" if not found.
 */
function parseBaseVarHex(varsString: string, varName: string): string {
  const re = new RegExp(varName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '\\s*:\\s*(#[0-9a-fA-F]{3,8})');
  const m = varsString.match(re);
  return m ? m[1] : '#000000';
}
```

- [ ] **Step 3: Add `buildGlassPreset`**

Computes the six CSS variable declarations for a given base theme and preset type.

```typescript
import { BASE_VARS } from './themePreview';

export type GlassPreset = 'frosted' | 'clear';

/**
 * Builds the CSS variable block for the frosted glass effect.
 * Uses hex colors from BASE_VARS for the given base theme.
 */
export function buildGlassPreset(baseTheme: string, preset: GlassPreset): string {
  const vars = BASE_VARS[baseTheme] ?? BASE_VARS['abyss'];

  // Map BASE_VARS keys to glass roles:
  //   panel bg  → --parley-bg-secondary  (channel list / sidebar bg)
  //   header bg → --parley-bg-hover      (closest to bg-tertiary in BASE_VARS)
  //   footer bg → --parley-channel-bg    (closest to bg-primary in BASE_VARS)
  //   chat bg   → --parley-channel-bg
  const panelHex   = parseBaseVarHex(vars, '--parley-bg-secondary');
  const headerHex  = parseBaseVarHex(vars, '--parley-bg-hover');
  const footerHex  = parseBaseVarHex(vars, '--parley-channel-bg');
  const chatHex    = parseBaseVarHex(vars, '--parley-channel-bg');

  const opacity = preset === 'frosted'
    ? { panel: 0.60, header: 0.70, footer: 0.70, chat: 0.78 }
    : { panel: 0.28, header: 0.32, footer: 0.32, chat: 0.40 };
  const blur = preset === 'frosted' ? '14px' : '0px';

  return [
    `  --parley-app-bg: transparent;`,
    `  --parley-panel-bg: rgba(${hexToRgb(panelHex)}, ${opacity.panel});`,
    `  --parley-panel-blur: ${blur};`,
    `  --parley-panel-header-bg: rgba(${hexToRgb(headerHex)}, ${opacity.header});`,
    `  --parley-panel-footer-bg: rgba(${hexToRgb(footerHex)}, ${opacity.footer});`,
    `  --parley-chat-bg: rgba(${hexToRgb(chatHex)}, ${opacity.chat});`,
  ].join('\n');
}
```

- [ ] **Step 4: Add `injectGlassVars`**

Injects, replaces, or removes the glass variable block in a CSS string. The block is delimited by `/* bg-glass-start */` and `/* bg-glass-end */` so it can be located and replaced on subsequent preset clicks.

```typescript
const GLASS_START = '/* bg-glass-start */';
const GLASS_END   = '/* bg-glass-end */';
const GLASS_PAT   = /\/\* bg-glass-start \*\/[\s\S]*?\/\* bg-glass-end \*\//;

/**
 * Injects or replaces the glass variable block in a CSS string.
 * Pass null for glassVars to remove an existing block (Solid preset).
 *
 * Insertion point: immediately after the opening brace of the first [data-theme] block.
 * If no [data-theme] block exists, one is created wrapping the glass variables.
 */
export function injectGlassVars(css: string, glassVars: string | null): string {
  // Remove any existing glass block first.
  let cleaned = css.replace(GLASS_PAT, '').replace(/\n{3,}/g, '\n\n').trim();

  if (glassVars === null) return cleaned;

  const block = `${GLASS_START}\n${glassVars}\n${GLASS_END}`;

  // Find [data-theme] { and insert the block right after the {
  const dataThemePat = /(\[data-theme\]\s*\{)/;
  if (dataThemePat.test(cleaned)) {
    return cleaned.replace(dataThemePat, `$1\n${block}`);
  }

  // No [data-theme] block — prepend one.
  const wrapper = `[data-theme] {\n${block}\n}`;
  return cleaned ? `${wrapper}\n\n${cleaned}` : wrapper;
}
```

- [ ] **Step 5: Verify the file compiles**

```bash
cd /home/dylan/Developer/parley/frontend && npx tsc --noEmit
```

Expected: no errors relating to `themeGlass.ts`.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/themeGlass.ts
git commit -m "feat: add themeGlass utilities (hexToRgb, buildGlassPreset, injectGlassVars)"
```

---

## Task 6: Theme editor — preset buttons

**Files:**
- Modify: `frontend/src/components/settings/CustomThemeEditor.tsx`

- [ ] **Step 1: Import utilities**

At the top of `CustomThemeEditor.tsx`, add:

```typescript
import { buildGlassPreset, injectGlassVars, type GlassPreset } from '../../lib/themeGlass';
```

- [ ] **Step 2: Add `glassPreset` state**

Inside the component, alongside the other state declarations:

```typescript
const [glassPreset, setGlassPreset] = useState<GlassPreset | 'solid'>(
  () => {
    // Detect existing glass block in CSS on load.
    if ((existing?.css ?? '').includes('/* bg-glass-start */')) {
      // Check blur value to determine which preset was active.
      return (existing?.css ?? '').includes('--parley-panel-blur: 0px') ? 'clear' : 'frosted';
    }
    return 'solid';
  }
);
```

- [ ] **Step 3: Add `applyGlassPreset` handler**

```typescript
const applyGlassPreset = (preset: GlassPreset | 'solid') => {
  setGlassPreset(preset);
  if (preset === 'solid') {
    setCSS(prev => injectGlassVars(prev, null));
  } else {
    const vars = buildGlassPreset(baseTheme, preset);
    setCSS(prev => injectGlassVars(prev, vars));
  }
};
```

- [ ] **Step 4: Auto-apply Frosted on image upload**

Find the `handleUpload` function. After `setCSS(prev => rule + ...)`, add:

```typescript
// Auto-apply frosted glass preset on first upload.
const vars = buildGlassPreset(baseTheme, 'frosted');
setCSS(prev => injectGlassVars(prev, vars));
setGlassPreset('frosted');
```

Note: Read the existing `handleUpload` function in full before replacing it — it may have evolved. The rewrite below must preserve all error handling and loading state. The key change is chaining both `setCSS` calls into one functional updater that also applies the glass preset:

```typescript
const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
  const file = e.target.files?.[0]; if (!file) return;
  try {
    const url = await uploadFile(file);
    setBgUrl(url);
    const bgRule = `body { background-image: url("${url}"); background-size: cover; background-repeat: no-repeat; background-attachment: fixed; }\n`;
    const vars = buildGlassPreset(baseTheme, 'frosted');
    setCSS(prev => {
      const withBg = bgRule + prev.replace(/body\s*\{[^}]*background-image[^}]*\}\n?/g, '');
      return injectGlassVars(withBg, vars);
    });
    setGlassPreset('frosted');
  } catch { setError('Upload failed'); }
};
```

- [ ] **Step 5: Also update `glassPreset` when base theme changes**

When `baseTheme` changes and a glass preset is active, regenerate the glass variables with the new theme colors. Add a `useEffect`:

```typescript
useEffect(() => {
  if (glassPreset === 'solid') return;
  const vars = buildGlassPreset(baseTheme, glassPreset);
  setCSS(prev => injectGlassVars(prev, vars));
}, [baseTheme]); // eslint-disable-line react-hooks/exhaustive-deps
```

- [ ] **Step 6: Add preset buttons to the UI**

Find the Background Image section in the JSX (where `bgUrl` is conditionally shown). Add the preset buttons row immediately after the existing `{bgUrl && <button ... >Remove</button>}` line:

```tsx
{bgUrl && (
  <div className="theme-editor-glass-row">
    <span className="theme-editor-label theme-editor-glass-label">Style</span>
    {(['solid', 'frosted', 'clear'] as const).map(p => (
      <button
        key={p}
        className={`theme-editor-glass-btn${glassPreset === p ? ' theme-editor-glass-btn--active' : ''}`}
        onClick={() => applyGlassPreset(p)}
        type="button"
      >
        {p === 'solid' ? 'Solid' : p === 'frosted' ? 'Frosted' : 'Clear'}
      </button>
    ))}
  </div>
)}
```

- [ ] **Step 7: Add CSS for preset buttons**

In `CustomThemeEditor.css`, append:

```css
.theme-editor-glass-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
}

.theme-editor-glass-label {
  font-size: 11px;
  color: var(--parley-text-muted);
  margin-right: 2px;
  white-space: nowrap;
}

.theme-editor-glass-btn {
  padding: 3px 10px;
  border-radius: 4px;
  border: 1px solid var(--parley-border);
  background: var(--parley-bg-tertiary);
  color: var(--parley-text-muted);
  font-size: 12px;
  cursor: pointer;
  font-family: inherit;
  transition: background 0.12s, color 0.12s, border-color 0.12s;
}

.theme-editor-glass-btn:hover {
  background: var(--parley-bg-hover);
  color: var(--parley-text-normal);
}

.theme-editor-glass-btn--active {
  background: var(--parley-accent);
  color: #fff;
  border-color: var(--parley-accent);
}
```

- [ ] **Step 8: Also re-run glass preset when bgUrl is removed**

Find the "Remove" button's `onClick` in the JSX. Update it to also clear the glass preset:

```tsx
{bgUrl && <button className="theme-editor-remove-bg" onClick={() => {
  setBgUrl(null);
  setCSS(prev => {
    const withoutBg = prev.replace(/body\s*\{[^}]*background-image[^}]*\}\n?/g, '');
    return injectGlassVars(withoutBg, null);
  });
  setGlassPreset('solid');
}}>Remove</button>}
```

- [ ] **Step 9: Verify in the browser**

1. Open theme editor, upload a background image.
2. The Frosted preset should activate automatically. Verify variables appear in the CSS textarea.
3. Click Clear — verify the opacity values decrease and blur becomes 0px.
4. Click Solid — verify glass variables are removed from the CSS textarea.
5. Change base theme while Frosted is active — verify the rgba() colors update in the textarea.
6. Save the theme and open the app — verify the background image is visible through frosted panels.

- [ ] **Step 10: Commit**

```bash
git add frontend/src/components/settings/CustomThemeEditor.tsx frontend/src/components/settings/CustomThemeEditor.css
git commit -m "feat: add frosted glass preset buttons to theme editor background section"
```

---

## Task 7: LLM system prompt update

**Files:**
- Modify: `internal/ai/worker.go`

- [ ] **Step 1: Append glass variable docs to `systemPrompt`**

Find the end of the `systemPrompt` constant in `internal/ai/worker.go`. The last line currently reads:
```
Output ONLY raw CSS. No markdown code fences. No explanation. No comments.
Start immediately with [data-theme] { (or a Google Fonts @import followed by [data-theme] {) and end with }.`
```

Insert the following section before that final output instruction line:

```
Background image & frosted glass (optional — only set these when the user wants a background image effect):
  --parley-app-bg              background of .main-layout and .main-content — set to transparent to show a body background image
  --parley-panel-bg            background of .sidebar, .channel-list, .user-sidebar, .dm-panel, .vc-chat-sidebar — use rgba() for transparency
  --parley-panel-blur          backdrop-filter blur on panels — e.g. 14px for frosted glass, 0px for clear
  --parley-panel-header-bg     background of .server-header (top bar of channel list) — use rgba() slightly more opaque than --parley-panel-bg
  --parley-panel-footer-bg     background of .user-area and .dm-panel-user-area (current user strip) — use rgba() matching panel tint
  --parley-chat-bg             background of the chat area (.chat-container, .chat-window, .chat-header, .message-input-container)

To create a frosted glass effect over a body background image, add inside [data-theme]:
  --parley-app-bg: transparent;
  --parley-panel-bg: rgba(R, G, B, 0.6);
  --parley-panel-blur: 14px;
  --parley-panel-header-bg: rgba(R2, G2, B2, 0.70);
  --parley-panel-footer-bg: rgba(R3, G3, B3, 0.70);
  --parley-chat-bg: rgba(R4, G4, B4, 0.78);
And add before [data-theme]:
  body { background-image: url("IMAGE_URL"); background-size: cover; background-attachment: fixed; background-repeat: no-repeat; }

```

- [ ] **Step 2: Build and verify**

```bash
cd /home/dylan/Developer/parley && go build ./cmd/api/...
```

Expected: compiles with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/ai/worker.go
git commit -m "feat: document frosted glass CSS variables in AI theme generation prompt"
```

---

## Task 8: Push and deploy

- [ ] **Step 1: Push to GitHub**

```bash
git push
```

This triggers the GitHub Actions deploy workflow, which will `npm ci && npm run build` on all API servers and copy the new frontend bundle. The Go backend restart picks up the updated system prompt.

- [ ] **Step 2: Verify on live app**

1. Log in, go to Settings → Appearance → create or edit a custom theme.
2. Upload a background image — Frosted preset should auto-activate.
3. Save and view the app — background image should be visible through all panels.
4. Test switching between Solid/Frosted/Clear presets.
5. Test in DM view (DmPanel) and verify background shows there too.
6. Test AI theme generation with a prompt like "frosted glass theme with a dark forest background" — verify the generated CSS includes the new variables.
