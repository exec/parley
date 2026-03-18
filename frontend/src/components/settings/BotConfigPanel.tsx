// frontend/src/components/settings/BotConfigPanel.tsx
import React, { useEffect, useState } from 'react';
import {
  BotSummary, AIConfig, AIUsage,
  getAIConfig, setAIConfig, getAIUsage,
  PROVIDER_MODELS, PROVIDER_LABELS,
} from '../../api/bots';

interface Props {
  bot: BotSummary;
  serverId: number;
  isAdmin: boolean;
  onRemove: () => void;
}

const VERBOSITY_OPTIONS = [
  { value: 'concise',  label: 'Concise' },
  { value: 'verbose',  label: 'Verbose' },
];

const PERSONALITY_OPTIONS = [
  { value: 'friendly',     label: 'Friendly' },
  { value: 'gamer',        label: 'Gamer' },
  { value: 'professional', label: 'Professional' },
  { value: 'unhinged',     label: 'Unhinged' },
  { value: 'hacker',       label: 'Hacker' },
];

const ROLE_OPTIONS = [
  { value: 'assistant', label: 'Assistant' },
  { value: 'member',    label: 'Member' },
  { value: 'tutor',     label: 'Tutor' },
];

export const BotConfigPanel: React.FC<Props> = ({ bot, serverId, isAdmin, onRemove }) => {
  const isAIChatbot = bot.username === 'polly';

  const [config, setConfig] = useState<AIConfig | null>(null);
  const [usage, setUsage] = useState<AIUsage | null>(null);
  const [provider, setProvider] = useState('parley');
  const [model, setModel] = useState('ministral-3:14b');
  const [verbosity, setVerbosity] = useState('concise');
  const [personality, setPersonality] = useState('friendly');
  const [role, setRole] = useState('assistant');
  const [apiKey, setApiKey] = useState('');
  const [saving, setSaving] = useState(false);
  const [saveMsg, setSaveMsg] = useState('');

  useEffect(() => {
    if (!isAIChatbot || !isAdmin) return;
    getAIConfig(serverId).then(cfg => {
      setConfig(cfg);
      setProvider(cfg.provider);
      setModel(cfg.model);
      setVerbosity(cfg.preset_verbosity || 'concise');
      setPersonality(cfg.preset_personality || 'friendly');
      setRole(cfg.preset_role || 'assistant');
    }).catch(() => {});
    getAIUsage(serverId).then(setUsage).catch(() => {});
  }, [serverId, isAIChatbot, isAdmin]);

  const handleProviderChange = (p: string) => {
    setProvider(p);
    const models = PROVIDER_MODELS[p];
    if (models?.length) setModel(models[0].value);
  };

  const needsApiKey = provider !== 'parley' && !config?.api_key_set && !apiKey.trim();

  const handleSave = async () => {
    if (needsApiKey) return;
    setSaving(true);
    setSaveMsg('');
    try {
      await setAIConfig(serverId, {
        provider, model,
        preset_verbosity: verbosity,
        preset_personality: personality,
        preset_role: role,
        api_key: apiKey || undefined,
      });
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

  const usagePct = usage ? Math.min(100, (usage.tokens_used / usage.tokens_limit) * 100) : 0;
  const resetDate = usage ? new Date(usage.resets_at).toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : '';

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

  const pillGroup = (
    options: { value: string; label: string }[],
    current: string,
    onChange: (v: string) => void,
  ) => (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginBottom: 12 }}>
      {options.map(o => (
        <button
          key={o.value}
          onClick={() => onChange(o.value)}
          style={{
            padding: '5px 12px', borderRadius: 20, fontSize: 12, cursor: 'pointer',
            border: current === o.value
              ? '1.5px solid var(--parley-accent,#32CD32)'
              : '1.5px solid var(--parley-border,#444)',
            background: current === o.value
              ? 'var(--parley-accent,#32CD32)22'
              : 'transparent',
            color: current === o.value
              ? 'var(--parley-accent,#32CD32)'
              : 'var(--parley-text-muted,#888)',
            fontWeight: current === o.value ? 600 : 400,
          }}
        >
          {o.label}
        </button>
      ))}
    </div>
  );

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

      {isAIChatbot && isAdmin && (
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
                API Key{' '}
                {config?.api_key_set
                  ? <span style={{ color: 'var(--parley-success,#43b581)' }}>● Set</span>
                  : <span style={{ color: 'var(--parley-danger,#f04747)' }}>(Required)</span>}
              </label>
              <input
                type="password"
                style={{ ...inputStyle, borderColor: needsApiKey ? 'var(--parley-danger,#f04747)' : undefined }}
                placeholder={config?.api_key_set ? '••••••••••••••••' : 'Enter API key…'}
                value={apiKey}
                onChange={e => setApiKey(e.target.value)}
                autoComplete="new-password"
              />
            </>
          )}

          <div style={{ ...sectionTitle, marginTop: 16 }}>Personality</div>

          <label style={label}>Verbosity</label>
          {pillGroup(VERBOSITY_OPTIONS, verbosity, setVerbosity)}

          <label style={label}>Personality</label>
          {pillGroup(PERSONALITY_OPTIONS, personality, setPersonality)}

          <label style={label}>Role</label>
          {pillGroup(ROLE_OPTIONS, role, setRole)}

          <button
            onClick={handleSave}
            disabled={saving || needsApiKey}
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
                {usagePct.toFixed(1)}% used · Resets {resetDate}
              </div>
              <div style={{ background: 'var(--parley-bg,#111)', borderRadius: 4, height: 8, overflow: 'hidden' }}>
                <div style={{
                  height: '100%',
                  width: `${usagePct}%`,
                  background: usagePct >= 90
                    ? 'var(--parley-danger,#f04747)'
                    : usagePct >= 70
                      ? '#f0a847'
                      : 'var(--parley-accent,#32CD32)',
                  borderRadius: 4,
                  transition: 'width .3s',
                }} />
              </div>
            </div>
          )}
        </>
      )}

      {isAdmin && (
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
