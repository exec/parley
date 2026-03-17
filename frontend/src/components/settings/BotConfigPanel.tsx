// frontend/src/components/settings/BotConfigPanel.tsx
import React, { useEffect, useState } from 'react';
import {
  BotSummary, AIConfig, AIUsage,
  getAIConfig, setAIConfig, getAIUsage,
  PROVIDER_MODELS, PROVIDER_LABELS, PARLEY_ALLOWANCES,
} from '../../api/bots';

interface Props {
  bot: BotSummary;
  serverId: number;
  isOwner: boolean;
  onRemove: () => void;
}

export const BotConfigPanel: React.FC<Props> = ({ bot, serverId, isOwner, onRemove }) => {
  const isAIChatbot = bot.username === 'ai-chatbot';

  const [config, setConfig] = useState<AIConfig | null>(null);
  const [usage, setUsage] = useState<AIUsage | null>(null);
  const [provider, setProvider] = useState('parley');
  const [model, setModel] = useState('ministral-3:14b');
  const [systemPrompt, setSystemPrompt] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [saving, setSaving] = useState(false);
  const [saveMsg, setSaveMsg] = useState('');

  useEffect(() => {
    if (!isAIChatbot || !isOwner) return;
    getAIConfig(serverId).then(cfg => {
      setConfig(cfg);
      setProvider(cfg.provider);
      setModel(cfg.model);
      setSystemPrompt(cfg.system_prompt);
    }).catch(() => {});
    getAIUsage(serverId).then(setUsage).catch(() => {});
  }, [serverId, isAIChatbot, isOwner]);

  const handleProviderChange = (p: string) => {
    setProvider(p);
    const models = PROVIDER_MODELS[p];
    if (models?.length) setModel(models[0].value);
  };

  const handleSave = async () => {
    setSaving(true);
    setSaveMsg('');
    try {
      await setAIConfig(serverId, { provider, model, system_prompt: systemPrompt, api_key: apiKey || undefined });
      setSaveMsg('Saved!');
      setApiKey('');
      if (provider === 'parley') {
        getAIUsage(serverId).then(setUsage).catch(() => {});
      }
    } catch {
      setSaveMsg('Save failed.');
    } finally {
      setSaving(false);
      setTimeout(() => setSaveMsg(''), 3000);
    }
  };

  const formatTokens = (n: number) => {
    if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
    if (n >= 1_000) return (n / 1_000).toFixed(0) + 'K';
    return String(n);
  };

  const resetDate = usage ? new Date(usage.resets_at).toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : '';

  // Suppress unused variable warning for PARLEY_ALLOWANCES — it's imported for external use
  void PARLEY_ALLOWANCES;

  const sectionTitle: React.CSSProperties = {
    fontSize: 11, fontWeight: 700, textTransform: 'uppercase',
    letterSpacing: '.05em', color: 'var(--parley-text-muted,#888)', marginBottom: 8,
  };
  const label: React.CSSProperties = {
    fontSize: 12, color: 'var(--parley-text-muted,#888)', marginBottom: 4, display: 'block',
  };
  const inputStyle: React.CSSProperties = {
    width: '100%', boxSizing: 'border-box',
    background: 'var(--parley-input,#1a1a1a)',
    border: '1px solid var(--parley-border,#444)',
    borderRadius: 4, color: 'var(--parley-text,#eee)',
    padding: '7px 10px', fontSize: 13, marginBottom: 12,
  };
  const selectStyle: React.CSSProperties = { ...inputStyle, cursor: 'pointer' };

  return (
    <div style={{ flex: 1, padding: '16px 20px', overflowY: 'auto' }}>
      {/* Bot info header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 20 }}>
        <div style={{
          width: 40, height: 40, borderRadius: '50%',
          background: 'var(--parley-accent,#32CD32)',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          fontSize: 18, fontWeight: 700, color: '#fff',
        }}>
          {bot.display_name.charAt(0).toUpperCase()}
        </div>
        <div>
          <div style={{ fontWeight: 700, fontSize: 15, display: 'flex', alignItems: 'center', gap: 6 }}>
            {bot.display_name}
            {bot.is_verified && (
              <span style={{ background: 'var(--parley-accent,#32CD32)', color: '#fff', borderRadius: '50%', width: 16, height: 16, display: 'inline-flex', alignItems: 'center', justifyContent: 'center', fontSize: 10 }}>✓</span>
            )}
          </div>
          <div style={{ fontSize: 12, color: 'var(--parley-text-muted,#888)' }}>@{bot.username} · Bot</div>
        </div>
      </div>

      {isAIChatbot && isOwner && (
        <>
          <div style={sectionTitle}>AI Provider</div>

          <label style={label}>Provider</label>
          <select style={selectStyle} value={provider} onChange={e => handleProviderChange(e.target.value)}>
            {Object.entries(PROVIDER_LABELS).map(([id, lbl]) => (
              <option key={id} value={id}>{lbl}</option>
            ))}
          </select>

          <label style={label}>Model</label>
          <select style={selectStyle} value={model} onChange={e => setModel(e.target.value)}>
            {(PROVIDER_MODELS[provider] ?? []).map(m => (
              <option key={m.value} value={m.value}>{m.label}</option>
            ))}
          </select>

          {provider !== 'parley' && (
            <>
              <label style={label}>
                API Key {config?.api_key_set && <span style={{ color: 'var(--parley-success,#43b581)' }}>● Set</span>}
              </label>
              <input
                type="password"
                style={inputStyle}
                placeholder={config?.api_key_set ? '••••••••••••••••' : 'Enter API key…'}
                value={apiKey}
                onChange={e => setApiKey(e.target.value)}
                autoComplete="new-password"
              />
            </>
          )}

          <label style={label}>System Prompt</label>
          <textarea
            style={{ ...inputStyle, minHeight: 72, resize: 'vertical', fontFamily: 'inherit' }}
            placeholder="You are a helpful assistant."
            value={systemPrompt}
            onChange={e => setSystemPrompt(e.target.value)}
          />

          <button
            onClick={handleSave}
            disabled={saving}
            style={{
              background: 'var(--parley-accent,#32CD32)', border: 'none', color: '#fff',
              borderRadius: 4, padding: '7px 18px', fontWeight: 600, fontSize: 13, cursor: 'pointer',
            }}
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
          {saveMsg && <span style={{ marginLeft: 10, fontSize: 12, color: 'var(--parley-text-muted,#888)' }}>{saveMsg}</span>}

          {provider === 'parley' && usage && (
            <div style={{ marginTop: 20 }}>
              <div style={sectionTitle}>Monthly Usage</div>
              <div style={{ fontSize: 12, color: 'var(--parley-text-muted,#888)', marginBottom: 6 }}>
                {formatTokens(usage.tokens_used)} / {formatTokens(usage.tokens_limit)} tokens · Resets {resetDate}
              </div>
              <div style={{ background: 'var(--parley-bg,#111)', borderRadius: 4, height: 8, overflow: 'hidden' }}>
                <div style={{
                  height: '100%',
                  width: `${Math.min(100, (usage.tokens_used / usage.tokens_limit) * 100)}%`,
                  background: 'var(--parley-accent,#32CD32)',
                  borderRadius: 4,
                  transition: 'width .3s',
                }} />
              </div>
            </div>
          )}
        </>
      )}

      {isOwner && (
        <div style={{ marginTop: 24, paddingTop: 16, borderTop: '1px solid var(--parley-border,#333)' }}>
          <button
            onClick={onRemove}
            style={{ background: 'none', border: '1px solid var(--parley-danger,#f04747)', color: 'var(--parley-danger,#f04747)', borderRadius: 4, padding: '6px 14px', fontSize: 12, cursor: 'pointer' }}
          >
            Remove Bot
          </button>
        </div>
      )}
    </div>
  );
};
