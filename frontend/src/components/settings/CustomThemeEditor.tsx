import React, { useState, useEffect, useRef, useCallback } from 'react';
import { UserTheme, NewTheme } from '../../api/themes';
import { BUILTIN_IDS } from '../../context/ThemeContext';
import { validateCSS } from '../../lib/cssValidator';
import './CustomThemeEditor.css';

const BUILTIN_LABELS: Record<string, string> = {
  'rory': 'Rory',
  'citron-dark': 'Citron Dark',
  'citron-light': 'Citron Light',
  'neon-nights': 'Neon Nights',
  'abyss': 'Abyss',
  'sakura': 'Sakura',
};

const PREVIEW_HTML = `<!DOCTYPE html><html><head><meta charset="utf-8"><style>
*{margin:0;padding:0;box-sizing:border-box;}
body{font-family:sans-serif;background:var(--bg-primary,#000);color:var(--text-primary,#fff);display:flex;height:100vh;overflow:hidden;}
.sb{width:56px;background:var(--discord-sidebar,#111);}
.ch{width:120px;background:var(--discord-bg-secondary,#0a0a0a);padding:8px;}
.ch h4{font-size:10px;color:var(--discord-text-muted,#666);margin-bottom:6px;}
.c{font-size:11px;color:var(--discord-text-muted,#666);padding:3px 4px;border-radius:3px;}
.c.a{background:var(--discord-bg-hover,#1a1a1a);color:var(--discord-text-normal,#fff);}
.chat{flex:1;background:var(--discord-channel-bg,#000);padding:10px;display:flex;flex-direction:column;justify-content:flex-end;}
.m{margin-bottom:8px;}.m .n{font-size:11px;font-weight:700;color:var(--discord-accent,#32CD32);}
.m .t{font-size:12px;color:var(--discord-text-normal,#eee);}
.inp{background:var(--discord-input,#111);border-radius:6px;padding:8px;font-size:11px;color:var(--discord-text-muted,#666);}
</style><style id="u"></style></head><body>
<div class="sb"></div>
<div class="ch"><h4># CHANNELS</h4>
<div class="c a"># general</div><div class="c"># random</div><div class="c"># memes</div></div>
<div class="chat">
<div class="m"><div class="n">Alice</div><div class="t">Hello! 👋</div></div>
<div class="m"><div class="n">Bob</div><div class="t">Hey there!</div></div>
<div class="inp">Message #general</div></div></body></html>`;

async function uploadFile(file: File): Promise<string> {
  const form = new FormData();
  form.append('file', file);
  const res = await fetch('/api/upload', {
    method: 'POST',
    headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
    body: form,
  });
  if (!res.ok) throw new Error('Upload failed');
  return (await res.json()).url as string;
}

interface Props {
  existing?: UserTheme;
  onSave(data: NewTheme): Promise<void>;
  onCancel(): void;
}

export const CustomThemeEditor: React.FC<Props> = ({ existing, onSave, onCancel }) => {
  const [name, setName] = useState(existing?.name || '');
  const [css, setCSS] = useState(existing?.css || '');
  const [baseTheme, setBaseTheme] = useState(existing?.base_theme || 'rory');
  const [bgUrl, setBgUrl] = useState<string | null>(existing?.background_url || null);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const debRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const updatePreview = useCallback((c: string) => {
    const doc = iframeRef.current?.contentDocument;
    const el = doc?.getElementById('u');
    if (el) el.textContent = c;
  }, []);

  useEffect(() => {
    const f = iframeRef.current;
    if (!f) return;
    f.srcdoc = PREVIEW_HTML;
    f.onload = () => updatePreview(css);
  }, []); // eslint-disable-line

  useEffect(() => {
    if (debRef.current) clearTimeout(debRef.current);
    debRef.current = setTimeout(() => updatePreview(css), 300);
    return () => { if (debRef.current) clearTimeout(debRef.current); };
  }, [css, updatePreview]);

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]; if (!file) return;
    try {
      const url = await uploadFile(file);
      setBgUrl(url);
      const rule = `body { background-image: url("${url}"); background-size: cover; background-repeat: no-repeat; background-attachment: fixed; }\n`;
      setCSS(prev => rule + prev.replace(/body\s*\{[^}]*background-image[^}]*\}\n?/g, ''));
    } catch { setError('Upload failed'); }
  };

  const handleSave = async () => {
    setError(null);
    if (!name.trim()) { setError('Name is required'); return; }
    const v = validateCSS(css);
    if (v) { setError(`Disallowed URLs: ${v.offendingUrls.join(', ')}`); return; }
    setSaving(true);
    try { await onSave({ name: name.trim(), css, base_theme: baseTheme, background_url: bgUrl }); }
    catch (e) { setError(e instanceof Error ? e.message : 'Save failed'); }
    finally { setSaving(false); }
  };

  return (
    <div className="theme-editor">
      <div className="theme-editor-header">
        <button className="theme-editor-back" onClick={onCancel}>←</button>
        <span className="theme-editor-title">{existing ? 'Edit Theme' : 'Create Theme'}</span>
      </div>

      <div className="theme-editor-field">
        <label className="theme-editor-label">Name</label>
        <input className="theme-editor-input" value={name}
          onChange={e => setName(e.target.value.slice(0,64))} maxLength={64} placeholder="My Theme" />
      </div>

      <div className="theme-editor-field">
        <label className="theme-editor-label">Base Theme</label>
        <div className="theme-editor-base-row">
          {BUILTIN_IDS.map(id => (
            <button
              key={id}
              className={`theme-editor-base-btn${baseTheme === id ? ' theme-editor-base-btn--active' : ''}`}
              data-theme={id}
              onClick={() => setBaseTheme(id)}
              title={BUILTIN_LABELS[id]}
            >
              {BUILTIN_LABELS[id]}
            </button>
          ))}
        </div>
        <div className="theme-editor-hint">Your CSS overrides these base variables. Cannot be another custom theme.</div>
      </div>

      <div className="theme-editor-field">
        <label className="theme-editor-label">Background Image</label>
        <div className="theme-editor-bg-row">
          {bgUrl && <div className="theme-editor-bg-preview" style={{backgroundImage:`url(${bgUrl})`}} />}
          <label className="theme-editor-upload-btn">
            {bgUrl ? 'Change' : 'Upload Image'}
            <input type="file" accept="image/*" style={{display:'none'}} onChange={handleUpload} />
          </label>
          {bgUrl && <button className="theme-editor-remove-bg" onClick={() => {
            setBgUrl(null);
            setCSS(prev => prev.replace(/body\s*\{[^}]*background-image[^}]*\}\n?/g, ''));
          }}>Remove</button>}
        </div>
      </div>

      <div className="theme-editor-field">
        <label className="theme-editor-label">Custom CSS</label>
        <textarea className="theme-editor-textarea" value={css} onChange={e => setCSS(e.target.value)}
          placeholder={`/* Override variables for any built-in theme selector */\n:root {\n  --accent: hotpink;\n}\n\n/* Google Fonts allowed */\n@import url('https://fonts.googleapis.com/css2?family=Inter');`} />
        <div className="theme-editor-hint">Google Fonts allowed. All other external URLs are blocked.</div>
      </div>

      {error && <div className="theme-editor-error">{error}</div>}

      <div className="theme-editor-preview-label">Preview</div>
      <div className="theme-editor-preview-note">Google Fonts won't load in preview but will work in the live app.</div>
      <iframe ref={iframeRef} className="theme-editor-iframe" sandbox="" title="Theme preview" />

      <div className="theme-editor-actions">
        <button className="theme-editor-save" onClick={handleSave} disabled={saving}>
          {saving ? 'Saving…' : 'Save'}
        </button>
        <button className="theme-editor-cancel" onClick={onCancel}>Cancel</button>
      </div>
    </div>
  );
};
