import React, { useState, useRef, useEffect } from 'react';
import { Palette } from 'lucide-react';
import { useTheme } from '../../context/ThemeContext';
import './ThemePopover.css';

const SWATCH: Record<string,string> = {
  'rory':'linear-gradient(135deg,#000 50%,#32CD32 50%)',
  'citron-dark':'linear-gradient(135deg,#36393f 50%,#5865f2 50%)',
  'citron-light':'linear-gradient(135deg,#fff 50%,#5865f2 50%)',
  'neon-nights':'linear-gradient(135deg,#0d0221 50%,#ff2d78 50%)',
  'abyss':'linear-gradient(135deg,#0a1628 50%,#00b4d8 50%)',
  'sakura':'linear-gradient(135deg,#fff9fb 50%,#d4609c 50%)',
};
const NAMES: Record<string,string> = {
  'rory':'Rory','citron-dark':'Citron Dark','citron-light':'Citron Light',
  'neon-nights':'Neon Nights','abyss':'Abyss','sakura':'Sakura',
};

export const ThemePopover: React.FC<{ onOpenSettings(): void }> = ({ onOpenSettings }) => {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const theme = useTheme();

  useEffect(() => {
    const h = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false); };
    document.addEventListener('mousedown', h);
    return () => document.removeEventListener('mousedown', h);
  }, []);

  return (
    <div className="theme-popover-wrap" ref={ref}>
      <button className="theme-popover-btn" title="Switch theme" onClick={() => setOpen(o => !o)}>
        <Palette size={16} />
      </button>
      {open && (
        <div className="theme-popover">
          <div className="theme-popover-section">Built-in</div>
          {theme.builtinIds.map(id => (
            <button key={id} className={`theme-popover-item${theme.activeTheme===id?' active':''}`}
              onClick={() => { theme.setBuiltin(id); setOpen(false); }}>
              <span className="theme-popover-swatch" style={{background: SWATCH[id]}} />
              {NAMES[id]}
            </button>
          ))}
          {theme.customThemes.length > 0 && (
            <>
              <div className="theme-popover-divider" />
              <div className="theme-popover-section">My Themes</div>
              {theme.customThemes.map(t => (
                <button key={t.id}
                  className={`theme-popover-item${theme.activeTheme==='custom'&&theme.activeCustomThemeId===t.id?' active':''}`}
                  onClick={() => { theme.setCustom(t.id); setOpen(false); }}>
                  {t.name}
                </button>
              ))}
            </>
          )}
          <div className="theme-popover-divider" />
          <button className="theme-popover-manage" onClick={() => { onOpenSettings(); setOpen(false); }}>
            Manage themes →
          </button>
        </div>
      )}
    </div>
  );
};
