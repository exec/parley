# Theming System Design

**Date:** 2026-03-16
**Status:** Draft

## Overview

A complete theming system for Parley: 6 built-in themes, user-created custom themes with CSS and background image support, theme sharing via public links, and a flash-free theme application mechanism. Theme preferences are stored server-side and synced to `localStorage` for instant load-time application.

---

## 1. Architecture

### CSS Variable Approach

Themes are implemented as `[data-theme="id"] { }` CSS blocks defining custom properties. JavaScript sets `document.body.dataset.theme` to activate a theme. No stylesheet swaps, no flash.

The existing `:root { }` variable block in `index.css` is converted to `[data-theme="terminal"] { }`. All other built-in themes are added as additional blocks in the same file.

For custom themes, a `<style id="custom-theme">` tag is injected into `<head>` containing the user's validated CSS. This tag is only present when a custom theme is active.

### Flash-Free Boot

`index.html` contains an inline `<script>` that executes synchronously before any JS bundle loads:

```html
<script>
  (function(){
    var t = localStorage.getItem('parley-theme') || 'terminal';
    document.body.dataset.theme = t;
    var css = localStorage.getItem('parley-custom-css');
    if (css && t === 'custom') {
      var s = document.createElement('style');
      s.id = 'custom-theme';
      s.textContent = css;
      document.head.appendChild(s);
    }
  })();
</script>
```

This sets the theme attribute and injects any custom CSS before the first paint, eliminating any flash of the default theme.

### ThemeContext

A React context (`ThemeContext`) wraps the entire app. It holds:

```ts
interface ThemeContextValue {
  activeTheme: string;                     // built-in ID or "custom"
  activeCustomThemeId: number | null;
  customThemes: UserTheme[];
  setBuiltinTheme(id: string): Promise<void>;
  setCustomTheme(id: number): Promise<void>;
  createCustomTheme(theme: NewTheme): Promise<UserTheme>;
  updateCustomTheme(id: number, theme: NewTheme): Promise<UserTheme>;
  deleteCustomTheme(id: number): Promise<void>;
}

interface UserTheme {
  id: number;
  name: string;
  css: string;
  background_url: string | null;
  share_token: string | null;   // null until user explicitly shares the theme
  created_at: string;           // ISO 8601
}

interface NewTheme {
  name: string;
  css: string;
  background_url?: string | null;
}
```

On login, `ThemeContext` calls `GET /api/me/preferences`, applies the stored theme, and syncs `localStorage` (`parley-theme` for the theme ID, `parley-custom-css` for the active custom CSS). On logout it resets to `"terminal"` and clears `parley-custom-css`.

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

The Sakura theme includes a low-poly SVG of cherry blossoms embedded directly into the CSS as a `data:image/svg+xml` URI in the `background-image` property of the chat area. No external image dependency.

---

## 3. Database Schema

### Migration Order

`user_themes` **must be created before** `user_preferences` due to the foreign key dependency.

### `user_themes`

```sql
CREATE TABLE user_themes (
  id             SERIAL PRIMARY KEY,
  user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name           VARCHAR(64) NOT NULL,
  css            TEXT NOT NULL DEFAULT '',
  background_url VARCHAR(512),
  share_token    UUID UNIQUE,              -- NULL until user explicitly shares
  created_at     TIMESTAMP NOT NULL DEFAULT NOW()
);
```

`share_token` is `NULL` by default. It is generated only when `POST /api/me/themes/:id/share` is called. A `NULL` token causes `GET /api/themes/:token` to return 404. This prevents unintended exposure of themes that have never been shared.

### `user_preferences`

```sql
CREATE TABLE user_preferences (
  user_id                BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  active_theme           VARCHAR(32) NOT NULL DEFAULT 'terminal'
                           CHECK (active_theme IN (
                             'terminal', 'citron-dark', 'citron-light',
                             'neon-nights', 'abyss', 'sakura', 'custom'
                           )),
  active_custom_theme_id INT REFERENCES user_themes(id) ON DELETE SET NULL
);
```

`active_theme` is constrained to the 6 built-in IDs or `"custom"`. When `"custom"`, `active_custom_theme_id` identifies the active `user_themes` row.

**Row creation:** A `user_preferences` row is inserted at user registration with the defaults (`active_theme = 'terminal'`, `active_custom_theme_id = NULL`). The `GET /api/me/preferences` endpoint does not need to handle a missing row.

**Theme deletion cascade:** When `DELETE /api/me/themes/:id` is called and the deleted theme is currently active (`active_custom_theme_id = id`), the handler must atomically update both `active_custom_theme_id = NULL` and `active_theme = 'terminal'` in the same transaction. The FK `ON DELETE SET NULL` handles `active_custom_theme_id` but does not reset `active_theme` — the application layer must do this explicitly.

**Per-user theme limit:** A user may own at most 20 custom themes. Both the `POST /api/me/themes` (create) and `POST /api/me/themes/install/:token` (install) handlers must count existing rows for the user and return `400` with `{"error": "Theme limit reached (20 max)"}` if the limit is exceeded.

---

## 4. API Endpoints

All `/api/me/...` endpoints require authentication. `/api/themes/:token` is public.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/me/preferences` | Returns active theme, active custom theme ID, and full list of user's custom themes |
| `PUT` | `/api/me/preferences/theme` | Set active built-in or custom theme |
| `POST` | `/api/me/themes` | Create a custom theme. Validates CSS. |
| `PUT` | `/api/me/themes/:id` | Update a custom theme. Same validation. |
| `DELETE` | `/api/me/themes/:id` | Delete a custom theme. Resets preference if it was active. |
| `POST` | `/api/me/themes/:id/share` | Generate share token if not already set. Returns share URL. Idempotent. Response: `{"share_url": "https://parley.gg/theme/<token>"}` |
| `GET` | `/api/themes/:token` | Public. Returns theme name, css, background_url. |
| `POST` | `/api/me/themes/install/:token` | Install a shared theme as an independent copy in the user's theme list. |

### `GET /api/me/preferences` — Response Shape

```json
{
  "active_theme": "custom",
  "active_custom_theme_id": 7,
  "custom_themes": [
    {
      "id": 7,
      "name": "My Dark Red Theme",
      "css": "...",
      "background_url": "https://cdn.example.com/uploads/abc.jpg",
      "share_token": null,
      "created_at": "2026-03-16T12:00:00Z"
    }
  ]
}
```

### `PUT /api/me/preferences/theme` — Request & Validation

```json
// Set a built-in theme:
{ "theme": "citron-dark" }

// Set a custom theme:
{ "theme": "custom", "custom_theme_id": 7 }
```

Error cases:
- `theme` is not a recognized built-in ID and not `"custom"` → `400 {"error": "Unknown theme ID"}`
- `theme` is `"custom"` but `custom_theme_id` is missing → `400 {"error": "custom_theme_id required when theme is 'custom'"}`
- `custom_theme_id` refers to a theme belonging to a different user → `403 {"error": "Forbidden"}`

### `POST /api/me/themes/install/:token` — Ownership Semantics

Installing a shared theme creates a **full independent copy** in the installing user's `user_themes` table:
- `user_id` is set to the installing user's ID
- `name`, `css`, and `background_url` are copied verbatim from the source theme
- `share_token` is set to `NULL` (the installed copy is private until the new owner shares it)
- `created_at` is the installation timestamp

The CSS is re-validated against the URL allowlist at install time, even if it passed validation for the original creator.

If the token does not exist or corresponds to a `NULL` token → `404 {"error": "Theme not found"}`.

---

## 5. CSS URL Validation

Custom CSS is validated before any save or apply operation — server-side (authoritative) and client-side (fast feedback only).

**CSS size limit:** Maximum 50 KB (51,200 bytes) of CSS per theme. Enforced server-side on create and update. Response: `400 {"error": "CSS exceeds 50 KB limit"}`.

**URL allowlist:**
- `fonts.googleapis.com` — Google Fonts CSS imports
- `fonts.gstatic.com` — Google Fonts font files
- The app's own CDN domain (DigitalOcean Spaces bucket URL)

All other hostnames in `url()` expressions are disallowed.

**Behavior on violation:** The save or install is rejected with `400`. The error response lists every offending URL:

```json
{
  "error": "Theme contains disallowed external URLs",
  "offending_urls": ["https://evil.com/tracker.woff2", "https://example.com/bg.jpg"]
}
```

The frontend displays this verbatim so the user knows exactly what to fix. No silent stripping — the theme does not apply at all if it fails validation, even for the user's own account.

---

## 6. Frontend UI

### Appearance Tab (User Settings)

A new "Appearance" tab is added to User Settings alongside the existing Account and Voice tabs.

**Built-in Themes section:**
- Grid of 6 theme swatches, each showing the theme name and a small color palette preview (3–4 color blocks representing the primary bg, secondary bg, accent, and text colors)
- Currently active theme is highlighted with a border/checkmark
- Clicking a swatch applies the theme immediately (updates `data-theme`, syncs to DB and localStorage)

**My Themes section:**
- Lists all user-created custom themes (up to 20)
- Each entry has: theme name, Apply / Edit / Share / Delete actions
- "Create Theme" button opens the inline custom theme editor
- The "20 themes" limit is shown as a counter (e.g., "3 / 20 themes")

**Custom Theme Editor (inline panel, not a modal):**
- Name field (max 64 chars; server returns `400 {"error": "Name must be 64 characters or fewer"}` if exceeded)
- Background image upload button (reuses `/api/upload`; images only; max 50MB per existing limit). When an image is uploaded, the returned CDN URL is stored in the `background_url` field of the theme and a `body { background-image: url(...); background-size: cover; background-repeat: tile; }` rule is automatically inserted or replaced at the top of the CSS textarea. The user may edit or remove this rule manually. `background_url` is **UI metadata only** — it lets the editor show the current image and a "Remove background" button without parsing the CSS. At theme activation time, only the `css` string is used; `background_url` is not applied separately by the application layer.
- CSS textarea with placeholder example and a note: _"Google Fonts allowed via `@import url(...)`. All other external URLs are blocked."_
- Live preview pane: a sandboxed `<iframe>` with `sandbox=""` (no flags). The iframe's `<style>` is updated in real time as the user types (debounced 300ms). The iframe is isolated from the host page — in-progress CSS does not affect the main app until Save. **Trade-off:** The empty `sandbox` attribute prevents `@import` from loading Google Fonts inside the preview (CORS is blocked for unique-origin sandboxed frames). The preview will show the correct layout and colors but not custom fonts. This is an accepted trade-off; the font will render correctly in the actual applied theme.
- Save button: runs URL validation client-side first for fast feedback, then sends to server
- Error display: shows the 400 response's `offending_urls` list inline below the textarea

**Share flow:**
- Clicking Share on a theme calls `POST /api/me/themes/:id/share` and copies the resulting `/theme/:token` URL to clipboard
- A small "Copied!" confirmation appears next to the button

### Quick-Switch Popover (Sidebar)

A `Palette` icon (16px, lucide-react) is added to the bottom-left user controls bar. This bar is rendered in two places in the codebase — **both must receive the icon**:
- `components/layout/ChannelList.tsx` — shown in the server/channel view (already imports lucide-react)
- `components/layout/DmPanel.tsx` — shown in the DM view (**does not currently import lucide-react**; a new import must be added)

The icon is placed between the existing mic mute and settings (gear) icons.

Clicking it opens a small popover anchored above the icon:
- Built-in theme swatches (compact, name + color strip)
- Saved custom themes as a scrollable list (max-height capped; scrolls if > ~5 items)
- "Manage Themes" link at the bottom that opens User Settings → Appearance

Clicking any item applies the theme immediately and closes the popover.

### Shared Theme Route (`/theme/:token`)

A standalone route rendered outside the main authenticated app layout, accessible without login.

**On load:**
1. Save current `localStorage.getItem('parley-theme')` and `localStorage.getItem('parley-custom-css')` to local variables (for Discard revert)
2. Fetch `GET /api/themes/:token`. If 404 → show "Theme not found" error page.
3. Apply the shared theme to `document.body.dataset.theme = "custom"` and inject the CSS into a `<style id="shared-theme-preview">` tag
4. Render the theme name and a palette preview
5. Open a modal: **"Install this theme?"** with two buttons:
   - **Install** — see install flow below
   - **Discard** — restore `document.body.dataset.theme` to the pre-visit value, restore or remove the `<style>` tag, remove `shared-theme-preview`. For anonymous visitors, "pre-visit value" is the saved localStorage value from step 1.

**Install flow:**
- If logged in: call `POST /api/me/themes/install/:token`, then optionally `PUT /api/me/preferences/theme` to make it active. Show success confirmation.
- If not logged in: redirect to `/login?redirect=/theme/:token`. After successful authentication, the login/register page reads the `?redirect=` query parameter and redirects to it. The theme route page re-applies the preview and re-opens the install modal on arrival.

The login and register pages must be updated to read the `?redirect=` query parameter and redirect to it after successful auth. (The existing invite flow at `InvitePage.tsx` already uses `?redirect=` — align with this convention.)

---

## 7. CSS Variable Reference

Every built-in theme must define this complete set of custom properties. Custom user themes may define any subset — undefined variables fall through to the active built-in theme's values.

```css
[data-theme="example"] {
  /* Layout */
  --sidebar-width: ;

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

  /* Discord aliases */
  --discord-bg: ;
  --discord-bg-primary: ;
  --discord-bg-secondary: ;
  --discord-bg-tertiary: ;
  --discord-bg-hover: ;
  --discord-hover: ;
  --discord-dark: ;
  --discord-gray: ;
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

**Note on missing variables:** During implementation, the full set of variables actively referenced in component CSS files must be audited. `--discord-dark`, `--discord-hover`, and `--discord-gray` are referenced in `styles.css` and `Auth.css` but are not currently defined in `index.css`. They must be added to all 6 built-in theme blocks (not just Terminal) as part of this work.

**Note on `styles.css` `:root` conflict:** `frontend/src/components/ui/styles.css` contains its own `:root { }` block with hardcoded Discord dark color values (e.g., `--discord-bg: #313338`). Because `:root` has equal specificity to `[data-theme="..."]` when declared later in the cascade, these hardcoded values will override theme variables for any variable defined in both places. This `:root` block in `styles.css` must be removed entirely during implementation — its values must be folded into each built-in theme's `[data-theme]` block in `index.css`.

**Note on `[data-theme="custom"]`:** There is no `[data-theme="custom"]` CSS block. When a custom theme is active, `data-theme` is set to `"custom"` on `<body>` — this matches no built-in theme block, so all variables fall back to browser defaults unless the injected `<style id="custom-theme">` tag defines them. Custom themes are expected to either define a full variable set or use `@import` / cascade from a built-in theme. The implementation must not add an empty `[data-theme="custom"] { }` block, as it would shadow the `<style>` injection mechanism.

---

## 8. Docs Fix (Nginx)

**Note:** This is an infrastructure fix bundled with the theming work for delivery convenience. It can be split into a separate task if preferred.

**Problem:** The React app's client-side router intercepts all routes including `/docs/developer/`, rendering a blank page or redirecting to home.

**Fix:** Add a `location` block in the nginx config before the React catch-all:

```nginx
location /docs/developer/ {
  alias /path/to/parley/docs/.vitepress/dist/;
  try_files $uri $uri/ $uri.html =404;
}
```

This causes nginx to serve the VitePress static build directly for all `/docs/developer/` paths. The React app never sees these requests.

---

## 9. Theming Documentation

A new `docs/theming.md` file is created and linked from the VitePress nav. Contents:

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

- **CSS URL validation** is enforced server-side on every write path (create, update, install shared theme). Client-side validation is for UX only and is not trusted.
- **Shared theme install** re-validates the CSS at install time, not just at creation time.
- **No server-side URL fetching** occurs at any point — SSRF is not applicable. External content either loads from Google Fonts (trusted, browser-native) or the app's own CDN.
- **CSP header** updated to add `fonts.googleapis.com` to `style-src` and `fonts.gstatic.com` to `font-src`, everything else locked to the app's own origin and CDN.
- **Custom theme CSS** runs in the user's own browser only. It cannot affect other users' sessions.
- **Live preview iframe** is sandboxed — the `sandbox` attribute is set without `allow-scripts` so no JavaScript executes inside the preview frame.
- **Share token privacy:** `share_token` is `NULL` until the user explicitly calls the share endpoint. Themes are never publicly accessible until the user opts in. A token, once generated, cannot be revoked (deleting and recreating the theme would produce a new token).
- CSS-based data exfiltration attacks (attribute selector oracles) are a known CSS risk class. They are low-severity in a chat app context and are accepted as a trade-off for CSS theme flexibility.
