# Theming System Design

**Date:** 2026-03-16
**Status:** Approved

## Overview

A complete theming system for Parley: 6 built-in themes, user-created custom themes with CSS and background image support, theme sharing via public links, and a flash-free theme application mechanism. Theme preferences are stored server-side and synced to `localStorage` for instant load-time application.

---

## 1. Architecture

### CSS Variable Approach

Themes are implemented as `[data-theme="id"] { }` CSS blocks defining custom properties. JavaScript sets `document.body.dataset.theme` to activate a theme. No stylesheet swaps, no flash.

The existing `:root { }` variable block in `index.css` is converted to `[data-theme="terminal"] { }`. All other built-in themes are added as additional blocks in the same file.

For custom themes, a `<style id="custom-theme">` tag is injected into `<head>` containing the user's validated CSS, scoped under their active `data-theme`.

### Flash-Free Boot

`index.html` contains an inline `<script>` that executes synchronously before any JS bundle loads:

```html
<script>
  (function(){
    var t = localStorage.getItem('parley-theme') || 'terminal';
    document.body.dataset.theme = t;
  })();
</script>
```

This sets the theme attribute before the first paint, eliminating any flash of the default theme.

### ThemeContext

A React context (`ThemeContext`) wraps the entire app. It holds:
- `activeTheme: string` — active built-in theme ID, or `"custom"`
- `activeCustomThemeId: number | null`
- `customThemes: UserTheme[]`
- `setBuiltinTheme(id: string): void`
- `setCustomTheme(id: number): void`
- `saveCustomTheme(theme: NewTheme): Promise<UserTheme>`

On login, `ThemeContext` calls `GET /api/me/preferences`, applies the stored theme, and syncs `localStorage`. On logout it resets to `"terminal"`.

---

## 2. Built-in Themes

Six built-in themes ship with the app:

| ID | Name | Description |
|----|------|-------------|
| `terminal` | Terminal | Default. Pure black bg, lime green (#32CD32) text. Retro terminal aesthetic. |
| `citron-dark` | Citron Dark | Discord dark clone. #36393F bg, #2C2F33 sidebar, #7289DA blurple accent, white text. |
| `citron-light` | Citron Light | Light theme. Off-white bg, light gray sidebars, #7289DA blurple accents, dark text. |
| `neon-nights` | Neon Nights | Synthwave/cyberpunk. Deep purple bg, hot pink + cyan accents, subtle glow on active elements. |
| `abyss` | Abyss | Ocean deep. Near-black navy bg, teal/cerulean accents, muted blue-gray secondary surfaces. |
| `sakura` | Sakura | Japanese cherry blossom. Warm white bg, dusty rose accents. Tiled low-poly cherry blossom SVG background built into the theme. |

Each theme defines the full canonical CSS variable set (see Section 7 — Variable Reference). Hard-coded hex values in component CSS files that affect overall look (badge colors, status dots, etc.) are converted to CSS variables as part of this implementation.

### Sakura Background

The Sakura theme includes a low-poly SVG of cherry blossoms generated as part of this implementation, embedded directly into the CSS as a `data:image/svg+xml` URI in the `background-image` property of the chat area. No external image dependency.

---

## 3. Database Schema

### `user_preferences`

```sql
CREATE TABLE user_preferences (
  user_id           INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  active_theme      VARCHAR(32) NOT NULL DEFAULT 'terminal',
  active_custom_theme_id INT REFERENCES user_themes(id) ON DELETE SET NULL
);
```

`active_theme` holds a built-in theme ID or `"custom"`. When `"custom"`, `active_custom_theme_id` identifies the active `user_themes` row.

### `user_themes`

```sql
CREATE TABLE user_themes (
  id             SERIAL PRIMARY KEY,
  user_id        INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name           VARCHAR(64) NOT NULL,
  css            TEXT NOT NULL DEFAULT '',
  background_url VARCHAR(512),
  share_token    UUID UNIQUE DEFAULT gen_random_uuid(),
  created_at     TIMESTAMP NOT NULL DEFAULT NOW()
);
```

`share_token` is generated on creation but the theme is only publicly accessible once the user explicitly shares it (the token is returned by the share endpoint). The token itself is the public identifier — no separate "is_public" flag needed; possession of the token implies access.

---

## 4. API Endpoints

All `/api/me/...` endpoints require authentication. `/api/themes/:token` is public.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/me/preferences` | Returns active theme, active custom theme ID, and full list of user's custom themes |
| `PUT` | `/api/me/preferences/theme` | Set active built-in or custom theme. Body: `{"theme": "citron-dark"}` or `{"theme": "custom", "custom_theme_id": 3}` |
| `POST` | `/api/me/themes` | Create a custom theme. Validates CSS. Rejects with 400 if disallowed URLs found. |
| `PUT` | `/api/me/themes/:id` | Update a custom theme. Same validation. |
| `DELETE` | `/api/me/themes/:id` | Delete a custom theme. If it was active, preference resets to `"terminal"`. |
| `POST` | `/api/me/themes/:id/share` | Returns the share token for the theme (idempotent — generates once, returns same token on subsequent calls). |
| `GET` | `/api/themes/:token` | Public. Returns theme name, css, background_url. No auth required. |
| `POST` | `/api/me/themes/install/:token` | Install a shared theme into the authenticated user's theme list. Validates CSS of the incoming theme. |

---

## 5. CSS URL Validation

Custom CSS is validated before any save or apply operation — both on the server (authoritative) and optionally on the client for fast feedback.

**Allowlist:**
- `fonts.googleapis.com` — Google Fonts CSS imports
- `fonts.gstatic.com` — Google Fonts font files
- The app's own CDN domain (DigitalOcean Spaces bucket URL)

**Behavior on violation:** The save or install is rejected with HTTP 400. The error response lists every offending URL found:

```json
{
  "error": "Theme contains disallowed external URLs",
  "offending_urls": ["https://evil.com/tracker.woff2", "https://example.com/bg.jpg"]
}
```

The frontend displays this to the user verbatim so they understand exactly what to fix. No silent stripping — the theme does not apply at all.

This also applies to shared themes being installed: the installing user's account never receives CSS that contains disallowed URLs, even if the original creator somehow bypassed validation.

---

## 6. Frontend UI

### Appearance Tab (User Settings)

A new "Appearance" tab is added to User Settings alongside the existing Account and Voice tabs.

**Built-in Themes section:**
- Grid of 6 theme swatches, each showing the theme name and a small color palette preview (3–4 color blocks)
- Currently active theme is highlighted with a border/checkmark
- Clicking a swatch applies the theme immediately (updates `data-theme`, syncs to DB and localStorage)

**My Themes section:**
- Lists all user-created custom themes
- Each entry has: theme name, Apply / Edit / Share / Delete actions
- "Create Theme" button opens the inline custom theme editor

**Custom Theme Editor (inline panel, not a modal):**
- Name field (max 64 chars)
- Background image upload button (reuses `/api/upload`; images only; max 50MB per existing limit)
- CSS textarea with placeholder example and a note: _"Google Fonts allowed via `@import url(...)`. All other external URLs are blocked."_
- Live preview pane: renders a simplified mock of the channel/message view with the CSS applied in real time (debounced, client-side only — does not hit the server until Save)
- Save button: runs URL validation client-side first for fast feedback, then sends to server
- Error display: shows the 400 response's `offending_urls` list inline below the textarea

**Share flow:**
- Clicking Share on a theme calls `POST /api/me/themes/:id/share` and copies the resulting URL (`/theme/:token`) to clipboard
- A small "Copied!" confirmation appears

### Quick-Switch Popover (Sidebar)

A `Palette` icon (16px, lucide-react) is added to the bottom-left user controls bar, between the existing mic and settings icons.

Clicking it opens a small popover anchored above the icon:
- Built-in theme swatches (compact, name + color strip)
- Saved custom themes as a scrollable list
- "Manage Themes" link at the bottom that opens User Settings → Appearance

Clicking any item applies the theme immediately and closes the popover.

### Shared Theme Route (`/theme/:token`)

A standalone route rendered outside the main authenticated app layout, accessible without login.

On load:
1. Fetches `GET /api/themes/:token`
2. Immediately applies the theme to `document.body.dataset.theme` and injects the CSS
3. Renders the theme name and a palette preview
4. Opens a modal: **"Install this theme?"** with two buttons:
   - **Install** — if logged in: calls `POST /api/me/themes/install/:token`, saves to their theme list. If not logged in: redirects to login with a return URL, installs after auth.
   - **Discard** — reverts `data-theme` to the user's previous theme and removes injected CSS

---

## 7. CSS Variable Reference

Every theme must define this full set of custom properties:

```css
[data-theme="example"] {
  /* Backgrounds */
  --bg-primary: ;
  --bg-secondary: ;
  --bg-tertiary: ;

  /* Text */
  --text-primary: ;
  --text-secondary: ;

  /* Accent */
  --accent: ;
  --accent-hover: ;

  /* Borders */
  --border: ;

  /* Discord aliases (used throughout component CSS) */
  --discord-bg: ;
  --discord-bg-primary: ;
  --discord-bg-secondary: ;
  --discord-bg-tertiary: ;
  --discord-bg-hover: ;
  --discord-text: ;
  --discord-text-normal: ;
  --discord-text-muted: ;
  --discord-text-dim: ;
  --discord-text-link: ;
  --discord-accent: ;
  --discord-accent-hover: ;
  --discord-blurple: ;
  --discord-green: ;
  --discord-danger: ;
  --discord-danger-hover: ;
  --discord-success: ;
  --discord-yellow: ;
  --discord-red: ;
  --discord-sidebar: ;
  --discord-channel-bg: ;
  --discord-input: ;
  --discord-border: ;
  --discord-border-light: ;
}
```

Custom themes do not need to define every variable — undefined variables fall back to the active built-in theme's values (or `:root` defaults if none). This allows partial overrides.

---

## 8. Docs Fix (Nginx)

**Problem:** The React app's client-side router intercepts all routes including `/docs/developer/`, rendering a blank page or redirecting to home.

**Fix:** Add a `location` block in nginx config before the React catch-all:

```nginx
location /docs/developer/ {
  alias /path/to/parley/docs/.vitepress/dist/;
  try_files $uri $uri/ $uri.html =404;
}
```

This causes nginx to serve the VitePress static build directly for all `/docs/developer/` paths. The React app never sees these requests.

---

## 9. Theming Documentation

A new `docs/theming.md` file is created and included in the VitePress nav. Contents:

- **Overview** — how the CSS variable theming system works
- **Variable Reference** — full table of all CSS custom properties with descriptions
- **Built-in Themes** — list of all 6 themes with their color values
- **Creating a Custom Theme** — step-by-step walkthrough of the theme editor
- **Using Google Fonts** — example `@import` snippet, link to fonts.google.com
- **Background Images** — how to upload an image and reference it in CSS
- **URL Policy** — what's allowed, what's blocked, and why
- **Sharing Themes** — how share links work, what recipients see
- **Example Theme** — a complete minimal custom theme CSS snippet

---

## 10. Security Considerations

- **CSS URL validation** is enforced server-side on every write path (create, update, install shared theme). Client-side validation is for UX only.
- **Shared theme install** re-validates the CSS at install time, not just at creation time.
- **SSRF** is not a concern because no server-side URL fetching occurs — all external content either comes from the Google Fonts CDN (trusted, browser-native) or the app's own CDN.
- **CSP header** updated to add `fonts.googleapis.com` to `style-src` and `fonts.gstatic.com` to `font-src`.
- **Custom theme CSS** runs in the user's own browser only. It cannot affect other users.
- Theme CSS does not support `<script>` or event handlers — it is purely CSS injected via `<style>` tag. CSS-based attacks (e.g., attribute selectors leaking data) are a known class of risk but are low-severity in a chat app context.
