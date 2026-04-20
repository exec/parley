export const SITE_URL = (import.meta.env.VITE_SITE_URL as string) || ''
export const APP_VERSION = __APP_VERSION__
export const APP_COMMIT = __APP_COMMIT__

// Returns the canonical site origin for shareable links. Falls back to
// window.location.origin when no build-time SITE_URL was provided — in the
// Tauri desktop build this is always set, so we avoid surfacing
// tauri://localhost in the UI.
export function siteOrigin(): string {
  if (SITE_URL) return SITE_URL;
  if (typeof window !== 'undefined') return window.location.origin;
  return '';
}
