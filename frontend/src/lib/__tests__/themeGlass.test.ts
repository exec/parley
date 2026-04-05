import { describe, it, expect } from 'vitest';
import { hexToRgb, buildGlassPreset, injectGlassVars, GLASS_PAT } from '../themeGlass';

describe('hexToRgb', () => {
  it('converts 6-digit hex', () => {
    expect(hexToRgb('#ff8800')).toBe('255, 136, 0');
  });

  it('converts 3-digit hex', () => {
    expect(hexToRgb('#f00')).toBe('255, 0, 0');
  });

  it('handles hex without #', () => {
    expect(hexToRgb('00ff00')).toBe('0, 255, 0');
  });

  it('returns 0, 0, 0 for black', () => {
    expect(hexToRgb('#000000')).toBe('0, 0, 0');
  });

  it('returns 0, 0, 0 for invalid hex', () => {
    expect(hexToRgb('#xyz')).toBe('0, 0, 0');
  });

  it('handles uppercase hex', () => {
    expect(hexToRgb('#AABBCC')).toBe('170, 187, 204');
  });
});

describe('buildGlassPreset', () => {
  it('returns CSS variables for frosted preset', () => {
    const result = buildGlassPreset('abyss', 'frosted');
    expect(result).toContain('--parley-app-bg: transparent');
    expect(result).toContain('--parley-panel-blur: 14px');
    expect(result).toContain('--parley-panel-bg: rgba(');
    expect(result).toContain('0.6)'); // panel opacity
  });

  it('returns CSS variables for clear preset', () => {
    const result = buildGlassPreset('abyss', 'clear');
    expect(result).toContain('--parley-panel-blur: 0px');
    expect(result).toContain('0.28)'); // panel opacity
  });

  it('falls back to abyss for unknown base theme', () => {
    const abyssResult = buildGlassPreset('abyss', 'frosted');
    const unknownResult = buildGlassPreset('nonexistent-theme', 'frosted');
    expect(unknownResult).toBe(abyssResult);
  });

  it('contains all expected CSS variables', () => {
    const result = buildGlassPreset('abyss', 'frosted');
    expect(result).toContain('--parley-app-bg');
    expect(result).toContain('--parley-panel-bg');
    expect(result).toContain('--parley-panel-blur');
    expect(result).toContain('--parley-panel-header-bg');
    expect(result).toContain('--parley-panel-footer-bg');
    expect(result).toContain('--parley-chat-bg');
    expect(result).toContain('--parley-input-bg');
  });
});

describe('GLASS_PAT', () => {
  it('matches a glass block', () => {
    const css = '/* bg-glass-start */\n  --parley-app-bg: transparent;\n/* bg-glass-end */';
    expect(GLASS_PAT.test(css)).toBe(true);
  });

  it('does not match when markers are absent', () => {
    const css = '--parley-app-bg: transparent;';
    expect(GLASS_PAT.test(css)).toBe(false);
  });
});

describe('injectGlassVars', () => {
  const glassVars = '  --parley-app-bg: transparent;\n  --parley-panel-blur: 14px;';

  it('removes existing glass block when glassVars is null', () => {
    const css = `[data-theme] {\n  --color: red;\n/* bg-glass-start */\n  old-vars\n/* bg-glass-end */\n}`;
    const result = injectGlassVars(css, null);
    expect(result).not.toContain('bg-glass-start');
    expect(result).not.toContain('old-vars');
    expect(result).toContain('--color: red');
  });

  it('injects glass block into existing [data-theme] block', () => {
    const css = `[data-theme] {\n  --color: red;\n}`;
    const result = injectGlassVars(css, glassVars);
    expect(result).toContain('/* bg-glass-start */');
    expect(result).toContain('/* bg-glass-end */');
    expect(result).toContain('--parley-app-bg: transparent');
    expect(result).toContain('--color: red');
  });

  it('replaces existing glass block with new one', () => {
    const css = `[data-theme] {\n  --color: red;\n/* bg-glass-start */\n  old\n/* bg-glass-end */\n}`;
    const result = injectGlassVars(css, glassVars);
    expect(result).not.toContain('old');
    expect(result).toContain('--parley-panel-blur: 14px');
  });

  it('wraps in [data-theme] when none exists', () => {
    const css = 'body { color: red; }';
    const result = injectGlassVars(css, glassVars);
    expect(result).toContain('[data-theme] {');
    expect(result).toContain('/* bg-glass-start */');
    expect(result).toContain('body { color: red; }');
  });

  it('returns just the wrapper when css is empty', () => {
    const result = injectGlassVars('', glassVars);
    expect(result).toContain('[data-theme] {');
    expect(result).toContain('/* bg-glass-start */');
    expect(result).toContain('/* bg-glass-end */');
  });
});
