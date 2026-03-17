# Theming

Parley supports full visual customisation through CSS custom properties applied to the `<body data-theme="id">` attribute.

## Built-in Themes

| ID | Name | Description |
|----|------|-------------|
| `rory` | Rory | Terminal green on black — the classic |
| `citron-dark` | Citron Dark | Discord-style dark blue |
| `citron-light` | Citron Light | Clean light mode |
| `neon-nights` | Neon Nights | Deep purple with hot-pink accents |
| `abyss` | Abyss | Deep ocean blues with cyan accent |
| `sakura` | Sakura | Soft pink and cherry blossom tones |

## Switching Themes

- **Settings → Appearance** — full grid with swatch previews
- **Palette icon** in the sidebar user area — quick-switch popover

## Custom Themes

1. Go to Settings → Appearance → **Create Theme**
2. Enter a name (max 64 characters)
3. Optionally upload a background image (auto-inserts a CSS background rule)
4. Write CSS in the textarea — the preview updates live (300 ms debounce)
5. Click **Save**

Your theme is available immediately and persists across sessions.

### Example: Override accent color

```css
[data-theme="custom"] {
  --accent: hotpink;
  --discord-accent: hotpink;
  --discord-accent-hover: #e0006e;
}
```

### Using Google Fonts

```css
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;700&display=swap');

[data-theme="custom"] {
  font-family: 'Inter', sans-serif;
}
```

## Background Images

When you upload a background image, Parley auto-inserts:

```css
body {
  background-image: url("https://cdn.parley.x86-64.com/...");
  background-size: cover;
  background-repeat: no-repeat;
  background-attachment: fixed;
}
```

You can override this with any standard CSS background properties.

## URL Policy

External URLs in custom CSS are **rejected** (not stripped) for security. Only these origins are allowed:

| Origin | Allowed usage |
|--------|---------------|
| `fonts.googleapis.com` | `@import` for Google Fonts |
| `fonts.gstatic.com` | Font binary files loaded by Google Fonts |
| Parley CDN | Background images uploaded through Parley |

Any other URL causes a save error listing the offending URLs.

## Variable Reference

Override any of these in `[data-theme="custom"] { ... }`:

| Variable | Role |
|----------|------|
| `--bg-primary` | Main chat background |
| `--bg-secondary` | Sidebar / secondary panels |
| `--bg-tertiary` | Inputs, dropdowns |
| `--text-primary` | Main body text |
| `--text-secondary` | Muted/secondary text |
| `--accent` | Highlight color (buttons, borders) |
| `--accent-hover` | Accent on hover |
| `--border` | Border color |
| `--sidebar-width` | Sidebar width (default `232px`) |
| `--discord-bg` | Alias for bg-primary |
| `--discord-bg-secondary` | Alias for bg-secondary |
| `--discord-bg-tertiary` | Alias for bg-tertiary |
| `--discord-bg-hover` | Hover background |
| `--discord-hover` | Hover state alias |
| `--discord-dark` | Darkest background shade |
| `--discord-gray` | Gray tone |
| `--discord-text` | Main text alias |
| `--discord-text-normal` | Normal text |
| `--discord-text-muted` | Muted text |
| `--discord-text-dim` | Dimmed text |
| `--discord-text-link` | Hyperlink color |
| `--discord-accent` | Accent alias |
| `--discord-accent-hover` | Accent hover alias |
| `--discord-blurple` | Brand primary color |
| `--discord-green` | Success / online green |
| `--discord-danger` | Error / delete red |
| `--discord-danger-hover` | Danger on hover |
| `--discord-success` | Success green |
| `--discord-yellow` | Warning yellow |
| `--discord-red` | Alert red |
| `--discord-sidebar` | Sidebar background |
| `--discord-channel-bg` | Channel message area |
| `--discord-input` | Input field background |
| `--discord-border` | Default border |
| `--discord-border-light` | Lighter border |

## Sharing Themes

1. In Settings → Appearance, click **Share** on any custom theme
2. A URL is copied to your clipboard: `https://parley.x86-64.com/theme/<token>`
3. Share the URL — recipients see a preview and can click **Install Theme**
4. Install creates a copy in their own User Themes (with `share_token` reset to null)

## Limits

- Maximum **20 custom themes** per user
- Maximum **50 KB** of CSS per theme
