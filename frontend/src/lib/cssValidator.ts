const CDN_HOST = (import.meta.env.VITE_CDN_HOST as string) || '';
const ALLOWED = new Set([
  'fonts.googleapis.com',
  'fonts.gstatic.com',
  'parley-prod.nyc3.cdn.digitaloceanspaces.com',
  ...(CDN_HOST ? [CDN_HOST] : []),
]);
const URL_RE = /url\(\s*['"]?([^'"\s)]+)['"]?\s*\)/gi;

export interface CSSValidationError { offendingUrls: string[]; }

export function validateCSS(css: string): CSSValidationError | null {
  const bad: string[] = [];
  let m: RegExpExecArray | null;
  URL_RE.lastIndex = 0;
  while ((m = URL_RE.exec(css)) !== null) {
    const raw = m[1].trim();
    if (raw.startsWith('data:') || raw.startsWith('#') || !raw.includes('://')) continue;
    try {
      const { hostname } = new URL(raw);
      if (!ALLOWED.has(hostname.toLowerCase())) bad.push(raw);
    } catch { /* skip unparseable */ }
  }
  return bad.length ? { offendingUrls: bad } : null;
}
