import React, { useEffect, useState, useCallback } from 'react';
import { listAPIKeys, createAPIKey, revokeAPIKey, renameBotUser, APIKeyInfo, CreateKeyResponse } from '../../api/developer';

export const DeveloperTab: React.FC = () => {
  const [keys, setKeys] = useState<APIKeyInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  // Create form
  const [showCreate, setShowCreate] = useState(false);
  const [createType, setCreateType] = useState<'bot' | 'user'>('bot');
  const [createName, setCreateName] = useState('');
  const [botUsername, setBotUsername] = useState('');
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState('');
  const [createdKey, setCreatedKey] = useState<CreateKeyResponse | null>(null);
  const [copied, setCopied] = useState(false);

  // Rename
  const [renamingBotId, setRenamingBotId] = useState<number | null>(null);
  const [renameValue, setRenameValue] = useState('');
  const [renamingLoading, setRenamingLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const data = await listAPIKeys();
      setKeys(data);
    } catch (e: unknown) {
      setError((e as { message?: string })?.message || 'Failed to load keys');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleCreate = async () => {
    if (!createName.trim() && createType === 'user') {
      setCreateError('Key name is required');
      return;
    }
    if (createType === 'bot' && !botUsername.trim()) {
      setCreateError('Bot username is required');
      return;
    }
    setCreating(true);
    setCreateError('');
    try {
      const result = await createAPIKey(createType, createName.trim(), createType === 'bot' ? botUsername.trim() : undefined);
      setCreatedKey(result);
      setShowCreate(false);
      setCreateName('');
      setBotUsername('');
      await load();
    } catch (e: unknown) {
      setCreateError((e as { message?: string })?.message || 'Failed to create key');
    } finally {
      setCreating(false);
    }
  };

  const handleRevoke = async (id: number) => {
    if (!window.confirm('Revoke this API key? This cannot be undone.')) return;
    try {
      await revokeAPIKey(id);
      setKeys(prev => prev.filter(k => k.id !== id));
    } catch (e: unknown) {
      setError((e as { message?: string })?.message || 'Failed to revoke key');
    }
  };

  const handleRename = async (botId: number) => {
    if (!renameValue.trim()) return;
    setRenamingLoading(true);
    try {
      await renameBotUser(botId, renameValue.trim());
      setKeys(prev => prev.map(k => k.user_id === botId ? { ...k, bot_username: renameValue.trim() } : k));
      setRenamingBotId(null);
      setRenameValue('');
    } catch (e: unknown) {
      setError((e as { message?: string })?.message || 'Failed to rename bot');
    } finally {
      setRenamingLoading(false);
    }
  };

  const handleCopy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // fallback: select text
    }
  };

  const formatDate = (s: string) => new Date(s).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });

  return (
    <>
      <h2 className="settings-page-title">Developer</h2>

      <div className="settings-section">
        <div className="settings-section-title">API Documentation</div>
        <p className="settings-form-hint" style={{ marginBottom: 10 }}>
          Build bots and integrations on Parley.{' '}
          <a href="https://parley.x86-64.com/docs/developer" target="_blank" rel="noopener noreferrer"
            style={{ color: 'var(--accent)' }}>
            View the API docs &rarr;
          </a>
        </p>
      </div>

      {/* New key reveal */}
      {createdKey && (
        <div className="dev-key-reveal">
          <div className="dev-key-reveal-title">
            &#10003; API Key Created &mdash; copy it now, it won&apos;t be shown again
          </div>
          <div className="dev-key-reveal-row">
            <code className="dev-key-code">{createdKey.key}</code>
            <button className="settings-btn settings-btn-ghost dev-copy-btn" onClick={() => handleCopy(createdKey.key)}>
              {copied ? 'Copied!' : 'Copy'}
            </button>
          </div>
          <button className="settings-btn settings-btn-ghost" style={{ marginTop: 10 }} onClick={() => setCreatedKey(null)}>
            Done
          </button>
        </div>
      )}

      <div className="settings-section">
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
          <div className="settings-section-title" style={{ margin: 0 }}>Your API Keys</div>
          {!showCreate && (
            <button className="settings-btn settings-btn-primary" onClick={() => setShowCreate(true)}>
              + Create Key
            </button>
          )}
        </div>

        {/* Create form */}
        {showCreate && (
          <div className="dev-create-form">
            <div className="dev-type-toggle">
              <button
                className={`dev-type-btn${createType === 'bot' ? ' active' : ''}`}
                onClick={() => setCreateType('bot')}
              >
                Bot
              </button>
              <button
                className={`dev-type-btn${createType === 'user' ? ' active' : ''}`}
                onClick={() => setCreateType('user')}
              >
                User (Selfbot)
              </button>
            </div>
            {createType === 'bot' && (
              <div className="settings-form-group">
                <label className="settings-form-label">Bot Username</label>
                <input
                  className="settings-form-input"
                  value={botUsername}
                  onChange={e => setBotUsername(e.target.value)}
                  placeholder="MyBot"
                  maxLength={32}
                />
              </div>
            )}
            <div className="settings-form-group">
              <label className="settings-form-label">Key Name {createType === 'user' && <span style={{ color: '#555' }}>(optional)</span>}</label>
              <input
                className="settings-form-input"
                value={createName}
                onChange={e => setCreateName(e.target.value)}
                placeholder={createType === 'bot' ? 'e.g. My Bot Key' : 'e.g. Personal automation'}
                maxLength={100}
              />
            </div>
            {createType === 'user' && (
              <p className="settings-form-hint">
                Selfbot keys authenticate as you. Keep them secret &mdash; they have full access to your account.
              </p>
            )}
            {createError && <div className="settings-error" style={{ marginBottom: 8 }}>{createError}</div>}
            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <button className="settings-btn settings-btn-ghost" onClick={() => { setShowCreate(false); setCreateError(''); }}>
                Cancel
              </button>
              <button className="settings-btn settings-btn-primary" onClick={handleCreate} disabled={creating}>
                {creating ? 'Creating...' : 'Create Key'}
              </button>
            </div>
          </div>
        )}

        {error && <div className="settings-error">{error}</div>}

        {loading ? (
          <div className="settings-form-hint">Loading keys...</div>
        ) : keys.length === 0 ? (
          <div className="dev-empty">No API keys yet. Create one to get started.</div>
        ) : (
          <div className="dev-key-table">
            <div className="dev-key-header-row">
              <span>Key</span>
              <span>Type</span>
              <span>Name / Bot Username</span>
              <span>Created</span>
              <span>Last Used</span>
              <span></span>
            </div>
            {keys.map(k => (
              <div key={k.id} className="dev-key-row">
                <span className="dev-key-prefix">{k.key_prefix}&hellip;</span>
                <span className={`dev-key-type ${k.is_bot ? 'bot' : 'user'}`}>
                  {k.is_bot ? 'Bot' : 'User'}
                </span>
                <span className="dev-key-name">
                  {k.is_bot && renamingBotId === k.user_id ? (
                    <span style={{ display: 'flex', gap: 4, alignItems: 'center' }}>
                      <input
                        className="settings-form-input"
                        style={{ padding: '2px 6px', fontSize: 12, height: 26 }}
                        value={renameValue}
                        onChange={e => setRenameValue(e.target.value)}
                        onKeyDown={e => { if (e.key === 'Enter') handleRename(k.user_id); if (e.key === 'Escape') setRenamingBotId(null); }}
                        autoFocus
                      />
                      <button className="settings-btn settings-btn-primary" style={{ padding: '2px 8px', fontSize: 12 }} disabled={renamingLoading} onClick={() => handleRename(k.user_id)}>Save</button>
                      <button className="settings-btn settings-btn-ghost" style={{ padding: '2px 8px', fontSize: 12 }} onClick={() => setRenamingBotId(null)}>&#x2715;</button>
                    </span>
                  ) : (
                    <span>
                      {k.is_bot ? (k.bot_username || '—') : k.name}
                      {k.is_bot && (
                        <button
                          className="dev-rename-btn"
                          title="Rename bot"
                          onClick={() => { setRenamingBotId(k.user_id); setRenameValue(k.bot_username || ''); }}
                        >
                          &#x270F;
                        </button>
                      )}
                    </span>
                  )}
                </span>
                <span className="dev-key-date">{formatDate(k.created_at)}</span>
                <span className="dev-key-date">{k.last_used_at ? formatDate(k.last_used_at) : '—'}</span>
                <span>
                  <button className="dev-revoke-btn" onClick={() => handleRevoke(k.id)}>Revoke</button>
                </span>
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  );
};
