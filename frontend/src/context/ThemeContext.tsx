import React, { createContext, useContext, useEffect, useState, useCallback } from 'react';
import {
  UserTheme, NewTheme,
  getPreferences, setBuiltinTheme, setCustomTheme,
  createTheme, updateTheme, deleteTheme,
} from '../api/themes';

export const BUILTIN_IDS = ['abyss','citron-dark','citron-light','neon-nights','rory','sakura'];

interface ThemeContextValue {
  activeTheme: string;
  activeCustomThemeId: number | null;
  customThemes: UserTheme[];
  builtinIds: string[];
  setBuiltin(id: string): Promise<void>;
  setCustom(id: number, themeData?: UserTheme): Promise<void>;
  createCustomTheme(t: NewTheme): Promise<UserTheme>;
  updateCustomTheme(id: number, t: NewTheme): Promise<UserTheme>;
  deleteCustomTheme(id: number): Promise<void>;
  setThemePublished(id: number, published: boolean): void;
  applyTheme(id: string, css?: string, baseTheme?: string): void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

function applyToDOM(id: string, css?: string | null, baseTheme?: string | null) {
  localStorage.setItem('parley-theme', id);
  const existing = document.getElementById('custom-theme');
  if (id === 'custom' && css) {
    const base = baseTheme || 'abyss';
    document.body.dataset.theme = base;
    localStorage.setItem('parley-theme-base', base);
    if (existing) { existing.textContent = css; }
    else { const s = document.createElement('style'); s.id='custom-theme'; s.textContent=css; document.head.appendChild(s); }
    localStorage.setItem('parley-custom-css', css);
  } else {
    document.body.dataset.theme = id;
    localStorage.removeItem('parley-theme-base');
    localStorage.removeItem('parley-custom-css');
    existing?.remove();
  }
}

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [activeTheme, setActiveTheme] = useState(() => localStorage.getItem('parley-theme') || 'abyss');
  const [activeCustomThemeId, setActiveCustomThemeId] = useState<number | null>(null);
  const [customThemes, setCustomThemes] = useState<UserTheme[]>([]);

  useEffect(() => {
    // Always apply the cached theme immediately so CSS variables resolve on
    // unauthenticated pages (login, register, etc.) before any auth check.
    const cached = localStorage.getItem('parley-theme') || 'abyss';
    const cachedCSS = localStorage.getItem('parley-custom-css') || null;
    const cachedBase = localStorage.getItem('parley-theme-base') || null;
    applyToDOM(cached, cachedCSS, cachedBase);

    if (!localStorage.getItem('token')) return;
    getPreferences().then(p => {
      setActiveTheme(p.active_theme);
      setActiveCustomThemeId(p.active_custom_theme_id);
      setCustomThemes(p.custom_themes);
      const activeCustom = p.active_theme === 'custom' && p.active_custom_theme_id
        ? p.custom_themes.find(t => t.id === p.active_custom_theme_id)
        : undefined;
      applyToDOM(p.active_theme, activeCustom?.css, activeCustom?.base_theme);
    }).catch(() => {});
  }, []);

  const setBuiltin = useCallback(async (id: string) => {
    await setBuiltinTheme(id);
    setActiveTheme(id); setActiveCustomThemeId(null); applyToDOM(id);
  }, []);

  const setCustom = useCallback(async (id: number, themeData?: UserTheme) => {
    await setCustomTheme(id);
    setActiveTheme('custom'); setActiveCustomThemeId(id);
    const t = themeData || customThemes.find(x => x.id === id);
    if (themeData && !customThemes.find(x => x.id === id)) {
      setCustomThemes(prev => [...prev, themeData]);
    }
    applyToDOM('custom', t?.css, t?.base_theme);
  }, [customThemes]);

  const createCustomTheme = useCallback(async (t: NewTheme) => {
    const created = await createTheme(t);
    setCustomThemes(prev => [...prev, created]);
    return created;
  }, []);

  const updateCustomTheme = useCallback(async (id: number, t: NewTheme) => {
    const updated = await updateTheme(id, t);
    setCustomThemes(prev => prev.map(x => x.id === id ? updated : x));
    if (activeTheme === 'custom' && activeCustomThemeId === id) {
      applyToDOM('custom', updated.css, updated.base_theme);
    }
    return updated;
  }, [activeTheme, activeCustomThemeId]);

  const deleteCustomTheme = useCallback(async (id: number) => {
    await deleteTheme(id);
    setCustomThemes(prev => prev.filter(x => x.id !== id));
    if (activeCustomThemeId === id) { setActiveTheme('abyss'); setActiveCustomThemeId(null); applyToDOM('abyss'); }
  }, [activeCustomThemeId]);

  const applyTheme = useCallback((id: string, css?: string, baseTheme?: string) => {
    applyToDOM(id, css, baseTheme); setActiveTheme(id);
    if (id !== 'custom') setActiveCustomThemeId(null);
  }, []);

  const setThemePublished = useCallback((id: number, published: boolean) => {
    setCustomThemes(prev => prev.map(x => x.id === id ? { ...x, is_published: published } : x));
  }, []);

  return (
    <ThemeContext.Provider value={{
      activeTheme, activeCustomThemeId, customThemes, builtinIds: BUILTIN_IDS,
      setBuiltin, setCustom, createCustomTheme, updateCustomTheme, deleteCustomTheme, setThemePublished, applyTheme,
    }}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme() {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used within ThemeProvider');
  return ctx;
}

/** Call on logout to clear theme state from localStorage. */
export function resetThemeOnLogout() {
  applyToDOM('abyss', null);
}
