import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTheme } from '../../context/ThemeContext';
import { shareTheme, publishTheme } from '../../api/themes';
import { CustomThemeEditor } from './CustomThemeEditor';
import { themePreviewStyle } from '../../lib/themePreview';
import '../ui/styles.css';
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
  const navigate = useNavigate();
  const [editingId, setEditingId] = useState<number | 'new' | null>(null);
  const [copiedId, setCopiedId] = useState<number | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<number | null>(null);
  const [publishingId, setPublishingId] = useState<number | null>(null);

  const [shareError, setShareError] = useState('');
  const handleShare = async (id: number) => {
    setShareError('');
    try {
      const { share_url } = await shareTheme(id);
      await navigator.clipboard.writeText(share_url);
      setCopiedId(id); setTimeout(() => setCopiedId(null), 2000);
    } catch (e) {
      setShareError(e instanceof Error ? e.message : 'Failed to share');
    }
  };

  const handleTogglePublish = async (id: number, currentlyPublished: boolean) => {
    setPublishingId(id);
    try {
      await publishTheme(id, !currentlyPublished);
      theme.setThemePublished(id, !currentlyPublished);
    } catch {}
    finally { setPublishingId(null); }
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
      <div className="appearance-section-title">Message Display</div>
      <div className="appearance-display-options">
        <div className="appearance-display-option">
          <div className="appearance-display-option-info">
            <div className="appearance-display-option-label">Compact Mode</div>
            <div className="appearance-display-option-desc">Hide profile pictures and condense messages into a tighter list</div>
          </div>
          <button
            type="button"
            className={`custom-toggle${theme.compactMode ? ' on' : ''}`}
            onClick={() => theme.setCompactMode(!theme.compactMode)}
            aria-pressed={theme.compactMode}
          />
        </div>
      </div>

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

      <div className="appearance-section-title" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <span>User Themes</span>
        <button
          className="appearance-repo-btn"
          onClick={() => navigate('/themes')}
        >
          Browse Theme Repository →
        </button>
      </div>
      <div className="appearance-theme-counter">{theme.customThemes.length} / 20 themes</div>
      {shareError && <div className="settings-error" style={{ marginBottom: 8 }}>{shareError}</div>}
      <div className="appearance-custom-list">
        {theme.customThemes.map(t => {
          const isActive = theme.activeTheme==='custom' && theme.activeCustomThemeId===t.id;
          return (
            <div key={t.id} className={`appearance-custom-item${isActive?' active':''}`} style={themePreviewStyle(t.css)}>
              <span className="appearance-custom-name">{t.name}</span>
              <div className="appearance-custom-actions">
                {!isActive && <button onClick={() => theme.setCustom(t.id)}>Apply</button>}
                <button onClick={() => setEditingId(t.id)}>Edit</button>
                <button onClick={() => handleShare(t.id)}>{copiedId===t.id?'Copied!':'Share'}</button>
                <button
                  className={t.is_published ? 'published' : ''}
                  disabled={publishingId===t.id}
                  onClick={() => handleTogglePublish(t.id, !!t.is_published)}
                >{publishingId===t.id ? '…' : t.is_published ? 'Published' : 'Publish'}</button>
                <button className="danger" title="Remove theme" onClick={() => setConfirmDeleteId(t.id)}>✕</button>
              </div>
            </div>
          );
        })}
      </div>
      {confirmDeleteId !== null && (
        <div className="appearance-confirm-overlay">
          <div className="appearance-confirm-dialog">
            <p>Delete "<strong>{theme.customThemes.find(t => t.id === confirmDeleteId)?.name}</strong>"?</p>
            <p style={{fontSize:'12px',color:'var(--parley-text-muted)'}}>This cannot be undone. If this is your active theme, it will revert to Abyss.</p>
            <div style={{display:'flex',gap:'8px',justifyContent:'flex-end',marginTop:'12px'}}>
              <button className="appearance-cancel-btn" onClick={() => setConfirmDeleteId(null)}>Cancel</button>
              <button className="appearance-danger-btn" onClick={async () => { await theme.deleteCustomTheme(confirmDeleteId); setConfirmDeleteId(null); }}>Delete</button>
            </div>
          </div>
        </div>
      )}
      {theme.customThemes.length < 20 && (
        <button className="appearance-create-btn" onClick={() => setEditingId('new')}>+ Create Theme</button>
      )}
    </div>
  );
};
