import { describe, it, expect } from 'vitest';
import { validateCSS } from '../cssValidator';

describe('validateCSS', () => {
  it('returns null for CSS with no urls', () => {
    expect(validateCSS('body { color: red; }')).toBeNull();
  });

  it('returns null for allowed Google Fonts urls', () => {
    const css = `@font-face { src: url('https://fonts.googleapis.com/css?family=Roboto'); }`;
    expect(validateCSS(css)).toBeNull();
  });

  it('returns null for allowed fonts.gstatic.com urls', () => {
    const css = `@font-face { src: url("https://fonts.gstatic.com/s/roboto/v30/font.woff2"); }`;
    expect(validateCSS(css)).toBeNull();
  });

  it('returns null for allowed CDN urls', () => {
    const css = `body { background: url(https://parley-prod.nyc3.cdn.digitaloceanspaces.com/img.png); }`;
    expect(validateCSS(css)).toBeNull();
  });

  it('rejects @import statements', () => {
    const result = validateCSS('@import url("https://evil.com/steal.css");');
    expect(result).toEqual({ offendingUrls: ['@import is not allowed'] });
  });

  it('rejects @import case-insensitively', () => {
    const result = validateCSS('@IMPORT "https://evil.com/steal.css";');
    expect(result).toEqual({ offendingUrls: ['@import is not allowed'] });
  });

  it('rejects urls from disallowed hosts', () => {
    const css = `body { background: url('https://evil.com/tracker.png'); }`;
    const result = validateCSS(css);
    expect(result).not.toBeNull();
    expect(result!.offendingUrls).toContain('https://evil.com/tracker.png');
  });

  it('skips data: urls without :// (no protocol separator)', () => {
    // data: URIs don't contain "://" so they are skipped by the protocol check
    const css = `body { background: url(data:image/png;base64,abc123); }`;
    expect(validateCSS(css)).toBeNull();
  });

  it('allows fragment-only references (#)', () => {
    const css = `body { background: url(#gradient); }`;
    expect(validateCSS(css)).toBeNull();
  });

  it('allows relative urls without protocol', () => {
    const css = `body { background: url(images/bg.png); }`;
    expect(validateCSS(css)).toBeNull();
  });

  it('collects multiple offending urls', () => {
    const css = `
      body { background: url('https://evil.com/a.png'); }
      div { background: url('https://bad.org/b.png'); }
    `;
    const result = validateCSS(css);
    expect(result).not.toBeNull();
    expect(result!.offendingUrls).toHaveLength(2);
    expect(result!.offendingUrls).toContain('https://evil.com/a.png');
    expect(result!.offendingUrls).toContain('https://bad.org/b.png');
  });

  it('handles mixed allowed and disallowed urls', () => {
    const css = `
      body { background: url('https://fonts.googleapis.com/css?family=Roboto'); }
      div { background: url('https://evil.com/tracker.png'); }
    `;
    const result = validateCSS(css);
    expect(result).not.toBeNull();
    expect(result!.offendingUrls).toHaveLength(1);
    expect(result!.offendingUrls).toContain('https://evil.com/tracker.png');
  });

  it('treats host comparison as case-insensitive', () => {
    const css = `body { background: url('https://FONTS.GOOGLEAPIS.COM/css?family=Roboto'); }`;
    expect(validateCSS(css)).toBeNull();
  });

  it('skips unparseable urls gracefully', () => {
    const css = `body { background: url('https://'); }`;
    // Invalid URL — caught by try/catch, skipped
    expect(validateCSS(css)).toBeNull();
  });
});
