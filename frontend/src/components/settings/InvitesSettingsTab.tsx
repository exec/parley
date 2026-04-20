import React, { useCallback, useEffect, useState } from 'react';
import { Check, Copy, Mail } from 'lucide-react';
import { copyToClipboard } from '../../lib/tauri';
import { listMyInvites, createMyInvite, RegistrationInvite } from '../../api/invites';

// Settings tab that shows the user their registration invite credit and lets
// them generate + copy single-use invite codes. Parley is invite-only during
// early launch; every account starts with 1 credit.
export const InvitesSettingsTab: React.FC = () => {
  const [count, setCount] = useState<number>(0);
  const [invites, setInvites] = useState<RegistrationInvite[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [generating, setGenerating] = useState(false);
  const [copiedCode, setCopiedCode] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const res = await listMyInvites();
      setCount(res.invite_count);
      setInvites(res.invites ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load invites');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleGenerate = async () => {
    setError('');
    setGenerating(true);
    try {
      await createMyInvite();
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to generate code');
    } finally {
      setGenerating(false);
    }
  };

  const handleCopy = async (code: string) => {
    try {
      await copyToClipboard(code);
      setCopiedCode(code);
      setTimeout(() => setCopiedCode(c => (c === code ? null : c)), 1600);
    } catch {
      // swallowed — the copy helper already falls back silently on the web
    }
  };

  return (
    <>
      <h2 className="settings-page-title">Invites</h2>

      <div className="settings-section">
        <p className="settings-form-hint" style={{ marginBottom: 16 }}>
          Parley is invite-only during early launch. Each code works once — share it
          with someone you'd like to bring in.
        </p>

        {error && (
          <div style={{
            padding: '10px 12px',
            marginBottom: 12,
            border: '1px solid rgba(248,113,113,0.3)',
            background: 'rgba(248,113,113,0.08)',
            color: 'var(--parley-danger, #f87171)',
            borderRadius: 6,
            fontSize: 13,
          }}>
            {error}
          </div>
        )}

        <div style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '14px 16px',
          background: 'var(--parley-panel-bg, var(--bg-tertiary))',
          border: '1px solid var(--parley-border, var(--border))',
          borderRadius: 8,
        }}>
          <div>
            <div style={{ fontSize: 11, color: 'var(--parley-text-muted)', textTransform: 'uppercase', letterSpacing: 0.5, fontWeight: 600 }}>
              Invites remaining
            </div>
            <div style={{
              fontSize: 26,
              fontWeight: 700,
              color: count > 0 ? 'var(--parley-accent, var(--accent))' : 'var(--parley-text-muted)',
              lineHeight: 1.15,
              marginTop: 2,
            }}>
              {count}
            </div>
          </div>
          <button
            type="button"
            className="settings-btn settings-btn-primary"
            onClick={handleGenerate}
            disabled={generating || count <= 0 || loading}
            title={count <= 0 ? 'No invites left' : 'Generate a new invite code'}
          >
            {generating ? 'Generating…' : 'Generate code'}
          </button>
        </div>
      </div>

      <div className="settings-section">
        <div className="settings-section-title">Your codes</div>

        {loading ? (
          <div style={{ padding: 24, color: 'var(--parley-text-muted)', fontSize: 13 }}>
            Loading…
          </div>
        ) : invites.length === 0 ? (
          <div style={{
            padding: 24,
            textAlign: 'center',
            color: 'var(--parley-text-muted)',
            fontSize: 13,
            border: '1px dashed var(--parley-border, var(--border))',
            borderRadius: 8,
          }}>
            <Mail size={18} style={{ opacity: 0.5, marginBottom: 6 }} />
            <div>You haven't generated any codes yet.</div>
          </div>
        ) : (
          <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 8 }}>
            {invites.map(ri => {
              const used = !!ri.used_at;
              return (
                <li
                  key={ri.code}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    padding: '10px 12px',
                    background: used ? 'transparent' : 'var(--parley-panel-bg, var(--bg-tertiary))',
                    border: '1px solid var(--parley-border, var(--border))',
                    borderRadius: 6,
                    opacity: used ? 0.55 : 1,
                  }}
                >
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div style={{
                      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
                      fontSize: 14,
                      fontWeight: 600,
                      color: used ? 'var(--parley-text-muted)' : 'var(--parley-text-normal, var(--text-primary))',
                      textDecoration: used ? 'line-through' : 'none',
                    }}>
                      {ri.code}
                    </div>
                    <div style={{ fontSize: 11, color: 'var(--parley-text-muted)', marginTop: 2 }}>
                      {used
                        ? `Used${ri.invitee_username ? ` by @${ri.invitee_username}` : ''}`
                        : 'Unused'}
                    </div>
                  </div>
                  {!used && (
                    <button
                      type="button"
                      className="settings-btn settings-btn-ghost"
                      onClick={() => handleCopy(ri.code)}
                      style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}
                    >
                      {copiedCode === ri.code ? <Check size={14} /> : <Copy size={14} />}
                      {copiedCode === ri.code ? 'Copied' : 'Copy'}
                    </button>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </>
  );
};
