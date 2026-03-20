// frontend/src/components/BotInviteEmbed.tsx
import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { BotInviteInfo, resolveBotInvite, acceptBotInvite } from '../api/bots';
import { EmbedCard } from './EmbedCard';
import { PERMISSION_CATEGORIES, permFromNumber } from '../lib/permissions';

interface ServerOption {
  id: number;
  name: string;
}

interface Props {
  token: string;
}

export const BotInviteEmbed: React.FC<Props> = ({ token }) => {
  const navigate = useNavigate();
  const [bot, setBot] = useState<BotInviteInfo | null>(null);
  const [invalid, setInvalid] = useState(false);
  const [servers, setServers] = useState<ServerOption[]>([]);
  const [selectedServer, setSelectedServer] = useState<number>(0);
  const [adding, setAdding] = useState(false);
  const [added, setAdded] = useState(false);
  const [error, setError] = useState('');
  const [grantedPerms, setGrantedPerms] = useState<bigint>(0n);

  const isLoggedIn = !!localStorage.getItem('token');

  useEffect(() => {
    resolveBotInvite(token)
      .then(b => {
        setBot(b);
        setGrantedPerms(permFromNumber(b.permissions));
      })
      .catch(() => setInvalid(true));
  }, [token]);

  useEffect(() => {
    if (!isLoggedIn) return;
    fetch('/api/servers', {
      headers: { Authorization: `Bearer ${localStorage.getItem('token')}` },
    })
      .then(r => r.json())
      .then((data: { id: string; name: string }[]) => {
        const mapped = data.map(s => ({ id: Number(s.id), name: s.name }));
        setServers(mapped);
        if (mapped.length) setSelectedServer(mapped[0].id);
      })
      .catch(() => {});
  }, [isLoggedIn]);

  if (invalid) {
    return (
      <EmbedCard title="Bot Not Found" actions={null}>
        <p style={{ fontSize: 13, color: 'var(--parley-text-muted,#888)' }}>This invite link is invalid or has expired.</p>
      </EmbedCard>
    );
  }

  if (!bot) {
    return <div style={{ textAlign: 'center', padding: 40, color: 'var(--parley-text-muted,#888)' }}>Loading…</div>;
  }

  const togglePerm = (bit: bigint) => {
    setGrantedPerms(prev => (prev & bit) !== 0n ? prev & ~bit : prev | bit);
  };

  const initial = bot.display_name.charAt(0).toUpperCase();

  const icon = bot.avatar_url
    ? <img src={bot.avatar_url} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
    : initial;

  const handleAdd = async () => {
    if (!isLoggedIn) { navigate('/login'); return; }
    if (!selectedServer) return;
    setAdding(true);
    setError('');
    try {
      await acceptBotInvite(token, selectedServer, grantedPerms);
      setAdded(true);
    } catch (e: unknown) {
      const code = (e as { code?: string })?.code;
      setError(code === '409' ? 'Bot is already in that server.' : 'Failed to add bot.');
    } finally {
      setAdding(false);
    }
  };

  const serverSelector = !added && isLoggedIn && servers.length > 0 ? (
    <select
      value={selectedServer}
      onChange={e => setSelectedServer(Number(e.target.value))}
      style={{
        width: '100%', boxSizing: 'border-box',
        background: 'var(--parley-input,#1a1a1a)',
        border: '1px solid var(--parley-border,#444)',
        borderRadius: 4, color: 'var(--parley-text,#eee)',
        padding: '7px 10px', fontSize: 13,
      }}
    >
      {servers.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}
    </select>
  ) : null;

  const actions = added ? (
    <div style={{ color: 'var(--parley-success,#43b581)', fontWeight: 600, fontSize: 14, textAlign: 'center' }}>
      ✓ Added to server!
    </div>
  ) : (
    <>
      {error && <div style={{ fontSize: 12, color: 'var(--parley-danger,#f04747)', marginBottom: 4 }}>{error}</div>}
      <button
        onClick={handleAdd}
        disabled={adding || (!isLoggedIn ? false : !selectedServer)}
        style={{
          background: 'var(--parley-accent,#32CD32)', border: 'none', color: '#fff',
          borderRadius: 4, padding: '9px', fontSize: 14, fontWeight: 600,
          cursor: 'pointer', width: '100%',
        }}
      >
        {adding ? 'Adding…' : isLoggedIn ? 'Add to Server' : 'Log in to Add'}
      </button>
    </>
  );

  const permissionsSection = bot.permissions === 0 ? (
    <p style={{ fontSize: 12, color: 'var(--parley-text-muted,#888)', margin: '0 0 8px' }}>
      No permissions requested
    </p>
  ) : (
    <div style={{ marginBottom: 10 }}>
      <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--parley-text-muted,#888)', marginBottom: 6, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
        Requested Permissions
      </div>
      <div style={{ columns: '140px 2', columnGap: 12 }}>
        {PERMISSION_CATEGORIES.map(cat => {
          const relevant = cat.permissions.filter(p => (permFromNumber(bot.permissions) & p.bit) !== 0n);
          if (relevant.length === 0) return null;
          return (
            <div key={cat.label} style={{ marginBottom: 8, breakInside: 'avoid' }}>
              <div style={{ fontSize: 11, color: 'var(--parley-text-muted,#888)', marginBottom: 4 }}>{cat.label}</div>
              {relevant.map(p => (
                <label key={p.name} title={p.description} style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, cursor: 'pointer', marginBottom: 2 }}>
                  <input
                    type="checkbox"
                    checked={(grantedPerms & p.bit) !== 0n}
                    onChange={() => togglePerm(p.bit)}
                    style={{ cursor: 'pointer' }}
                  />
                  {p.name}
                </label>
              ))}
            </div>
          );
        })}
      </div>
    </div>
  );

  return (
    <EmbedCard
      icon={icon}
      title={bot.display_name}
      subtitle={`@${bot.username} · AI Chatbot`}
      badge={bot.is_verified}
      actions={actions}
    >
      {bot.owner_username && (
        <p style={{ fontSize: 12, color: 'var(--parley-text-muted,#888)', margin: '0 0 10px' }}>
          by <span style={{ color: 'var(--parley-text,#eee)' }}>@{bot.owner_username}</span>
        </p>
      )}
      {permissionsSection}
      {serverSelector}
    </EmbedCard>
  );
};
