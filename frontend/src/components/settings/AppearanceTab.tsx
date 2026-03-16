import React, { useState } from 'react';
import { useTheme } from '../../context/ThemeContext';
import { shareTheme } from '../../api/themes';
import { CustomThemeEditor } from './CustomThemeEditor';
import './AppearanceTab.css';

const PALETTES: Record<string,[string,string,string,string]> = {
  'rory':         ['#000','#0a0a0a','#32CD32','#1a1a1a'],
  'citron-dark':  ['#36393f','#2f3136','#5865f2','#dcddde'],
  'citron-light': ['#fff','#f2f3f5','#5865f2','#2e3338'],
  'neon-nights':  ['#0d0221','#130333','#ff2d78','#f0f0f0'],
  'abyss':        ['#0a1628','#0d1f3c','#00b4d8','#cad5e2'],
  'sakura':       ['#fff9fb','#fde8f0','#d4609c','#3d1a2e'],
};
const NAMES: Record<string,string> = {
  'rory':'Rory','citron-dark':'Citron Dark','citron-light':'Citron Light',
  'neon-nights':'Neon Nights','abyss':'Abyss','sakura':'Sakura',
};

export const AppearanceTab: React.FC = () => {
  const theme = useTheme();
  const [editingId, setEditingId] = useState<number | 'new' | null>(null);
  const [copiedId, setCopiedId] = useState<number | null>(null);

  const handleShare = async (id: number) => {
    try {
      const { share_url } = await shareTheme(id);
      await navigator.clipboard.writeText(share_url);
      setCopiedId(id); setTimeout(() => setCopiedId(null), 2000);
    } catch {}
  };

  if (editingId !== null) {
    const existing = editingId !== 'new' ? theme.customThemes.find(t => t.id === editingId) : undefined;
    return (
      <CustomThemeEditor
        existing={existing}
        onSave={async data => {
          if (editingId === 'new') await theme.createCustomTheme(data);
          else await theme.updateCustomTheme(editingId as number, data);
          setEditingId(null);
        }}
        onCancel={() => setEditingId(null)}
      />
    );
  }

  return (
    <div className="appearance-tab">
      <div className="appearance-section-title">Built-in Themes</div>
      <div className="appearance-theme-grid">
        {theme.builtinIds.map(id => {
          const [c1,c2,c3,c4] = PALETTES[id];
          return (
            <button key={id} className={`appearance-theme-swatch${theme.activeTheme===id?' active':''}`}
              onClick={() => theme.setBuiltin(id)} title={NAMES[id]}>
              <div className="appearance-swatch-colors">
                <span style={{background:c1}}/><span style={{background:c2}}/>
                <span style={{background:c3}}/><span style={{background:c4}}/>
              </div>
              <div className="appearance-swatch-name">{NAMES[id]}</div>
            </button>
          );
        })}
      </div>

      <div className="appearance-section-title">My Themes</div>
      <div className="appearance-theme-counter">{theme.customThemes.length} / 20 themes</div>
      <div className="appearance-custom-list">
        {theme.customThemes.map(t => {
          const isActive = theme.activeTheme==='custom' && theme.activeCustomThemeId===t.id;
          return (
            <div key={t.id} className={`appearance-custom-item${isActive?' active':''}`}>
              <span className="appearance-custom-name">{t.name}</span>
              <div className="appearance-custom-actions">
                {!isActive && <button onClick={() => theme.setCustom(t.id)}>Apply</button>}
                <button onClick={() => setEditingId(t.id)}>Edit</button>
                <button onClick={() => handleShare(t.id)}>{copiedId===t.id?'Copied!':'Share'}</button>
                <button className="danger" onClick={() => theme.deleteCustomTheme(t.id)}>Delete</button>
              </div>
            </div>
          );
        })}
      </div>
      {theme.customThemes.length < 20 && (
        <button className="appearance-create-btn" onClick={() => setEditingId('new')}>+ Create Theme</button>
      )}
    </div>
  );
};
