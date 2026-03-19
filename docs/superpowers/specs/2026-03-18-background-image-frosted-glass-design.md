# Background Image & Frosted Glass Theme Support

**Date:** 2026-03-18
**Status:** Approved

## Problem

Background images uploaded via the theme editor are not visible. All layout panels have solid `background-color` properties that cover the `body` background-image entirely. The image exists in the DOM but is completely buried under opaque containers.

## Solution

Full-page background with frosted glass panels. Six new CSS variables let users (and the AI generator) control transparency and blur on every panel independently. Preset buttons in the editor make it one-click.

## New CSS Variables

| Variable | Fallback | Applied to |
|---|---|---|
| `--parley-app-bg` | `var(--bg-primary)` | `.main-layout`, `.main-content` |
| `--parley-panel-bg` | `var(--bg-secondary)` | `.sidebar`, `.channel-list`, `.user-sidebar`, `.dm-panel`, `.vc-chat-sidebar` |
| `--parley-panel-blur` | `0px` | `backdrop-filter` on same elements |
| `--parley-panel-header-bg` | `var(--bg-tertiary)` | `.server-header` |
| `--parley-panel-footer-bg` | `var(--bg-primary)` | `.user-area`, `.dm-panel-user-area` |
| `--parley-chat-bg` | `var(--parley-channel-bg)` | `.chat-container`, `.chat-window`, `.chat-header`, `.message-input-container` |

All variables default to the existing opaque values — zero visual change for themes that don't set them.

## CSS Changes

Seven files need updates. Each targeted selector gets `background-color` changed from a hard-coded variable to `var(--parley-NEW, var(--existing))`. Panel elements additionally get `backdrop-filter: blur(var(--parley-panel-blur, 0px))` and `-webkit-backdrop-filter` for Safari.

### Files and selectors

**`MainLayout.css`**
- `.main-layout` → `background-color: var(--parley-app-bg, var(--bg-primary))`
- `.main-content` → `background-color: var(--parley-app-bg, var(--bg-primary))`
- `.vc-chat-sidebar` → `background-color: var(--parley-panel-bg, var(--bg-primary))` + blur

**`Sidebar.css`**
- `.sidebar` → `background-color: var(--parley-panel-bg, var(--bg-primary))` + blur

**`ChannelList.css`**
- `.channel-list` → `background-color: var(--parley-panel-bg, var(--bg-secondary))` + blur
- `.server-header` → `background-color: var(--parley-panel-header-bg, var(--bg-tertiary))`
- `.user-area` → `background-color: var(--parley-panel-footer-bg, var(--bg-primary))`

**`UserSidebar.css`**
- `.user-sidebar` → `background-color: var(--parley-panel-bg, var(--bg-secondary))` + blur

**`DmPanel.css`**
- `.dm-panel` → `background-color: var(--parley-panel-bg, var(--bg-secondary))` + blur
- `.dm-panel-user-area` → `background-color: var(--parley-panel-footer-bg, var(--bg-primary))`

**`Chat.css`**
- `.chat-container` → `background-color: var(--parley-chat-bg, var(--parley-channel-bg))`
- `.chat-window` → same
- `.chat-header` → same
- `.message-input-container` → same

## Theme Editor UI

In `CustomThemeEditor.tsx`, the Background Image section gains three preset buttons shown only when `bgUrl` is set:

**Solid** — removes the `/* bg-glass-start */ ... /* bg-glass-end */` block from CSS entirely.

**Frosted** *(applied automatically on first image upload)* — injects:
```css
/* bg-glass-start */
--parley-app-bg: transparent;
--parley-panel-bg: rgba(R, G, B, 0.60);
--parley-panel-blur: 14px;
--parley-panel-header-bg: rgba(R2, G2, B2, 0.70);
--parley-panel-footer-bg: rgba(R3, G3, B3, 0.70);
--parley-chat-bg: rgba(R4, G4, B4, 0.78);
/* bg-glass-end */
```

**Clear** — same structure, lower opacity (0.28 panels, 0.32 header/footer, 0.40 chat), no blur.

### Hex-to-RGB conversion

`BASE_VARS` stores colors as hex strings (e.g. `#0a1628`). Preset logic must parse these into `R, G, B` tuples before building the `rgba()` strings. Implement a small `hexToRgb(hex: string): [number, number, number]` helper that strips `#`, parses pairs as base-16 integers. Apply per color:
- `--bg-secondary` hex → panels
- `--bg-tertiary` hex → panel headers
- `--bg-primary` hex → panel footers
- `--parley-channel-bg` hex → chat

### Injection algorithm

The block is delimited by `/* bg-glass-start */` and `/* bg-glass-end */` as exact strings.

**Insert:** Find the first `[data-theme]` block open brace. Insert the glass block immediately after the `{`. If no `[data-theme]` block exists, wrap the glass variables in one: `[data-theme] {\n/* bg-glass-start */\n...\n/* bg-glass-end */\n}`.

**Replace:** Regex `\/\* bg-glass-start \*\/[\s\S]*?\/\* bg-glass-end \*\//` to find and replace the existing block.

**Remove (Solid preset):** Same regex, replace with empty string. Clean up the resulting double blank line.

### Background-attachment prerequisite

The `handleUpload` function already injects `background-attachment: fixed` into the body rule when an image is uploaded. The presets do not re-assert this — it is guaranteed by the upload path. Manual CSS authors are responsible for including it themselves; this is documented in the hint text below the background image upload.

## LLM System Prompt Update

Add a new section to `internal/ai/worker.go` `systemPrompt` after the existing variables:

```
Background image & frosted glass (optional — only set these when the user wants a background image effect):
  --parley-app-bg              background of .main-layout and .main-content — set to transparent to reveal body background image
  --parley-panel-bg            background of .sidebar, .channel-list, .user-sidebar, .dm-panel, .vc-chat-sidebar — use rgba() for transparency
  --parley-panel-blur          backdrop-filter blur on panels — e.g. 14px for frosted glass, 0px for clear
  --parley-panel-header-bg     background of .server-header (top of channel list) — use rgba() matching panel tint
  --parley-panel-footer-bg     background of .user-area and .dm-panel-user-area (user strip at bottom) — use rgba() matching panel tint
  --parley-chat-bg             background of chat area (.chat-container, .chat-window, .chat-header, .message-input-container)

To create a frosted glass effect with a background image:
  1. Set body { background-image: url("..."); background-size: cover; background-attachment: fixed; background-repeat: no-repeat; }
  2. Set --parley-app-bg: transparent
  3. Set --parley-panel-bg: rgba(R, G, B, 0.6) using the theme's secondary bg color
  4. Set --parley-panel-blur: 14px
  5. Set --parley-panel-header-bg and --parley-panel-footer-bg to rgba() variants slightly more opaque than the panel
  6. Set --parley-chat-bg: rgba(R, G, B, 0.78) using the theme's channel-bg color
```

## Preview Iframe

The `PREVIEW_HTML` in `CustomThemeEditor.tsx` does not support background image preview (no CDN auth in a sandboxed iframe). The glass variables will still reflect in the live app immediately on save. No changes needed to preview HTML.

## Mobile

On mobile, `.left-drawer` is `position: fixed` and slides in as a drawer. Its children (`.sidebar`, `.channel-list`) receive the same CSS variable changes and `backdrop-filter` — the frosted glass effect works correctly in this context because `backdrop-filter` on children of a fixed element composites against the body background as expected in all modern browsers.

## Scope

- 7 CSS files (targeted, minimal changes per file)
- `CustomThemeEditor.tsx` (preset buttons + `hexToRgb` helper + CSS injection logic)
- `internal/ai/worker.go` (system prompt addition)
