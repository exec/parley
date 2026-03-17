// frontend/src/components/settings/BotsTab.tsx
import React, { useEffect, useState } from 'react';
import { BotSummary, listBots, addBot, removeBot, OFFICIAL_BOTS } from '../../api/bots';
import { BotConfigPanel } from './BotConfigPanel';
import './BotsTab.css';

interface Props {
  serverId: number;
  isAdmin: boolean;
}

export const BotsTab: React.FC<Props> = ({ serverId, isAdmin }) => {
  const [bots, setBots] = useState<BotSummary[]>([]);
  const [selected, setSelected] = useState<BotSummary | null>(null);
  const [showAdd, setShowAdd] = useState(false);
  const [inviteInput, setInviteInput] = useState('');
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState('');
  const [addingOfficial, setAddingOfficial] = useState<string | null>(null);

  useEffect(() => {
    listBots(serverId).then(setBots).catch(() => {});
  }, [serverId]);

  const handleAdd = async () => {
    // Extract token from URL or raw token
    const token = inviteInput.includes('/bots/invite/')
      ? inviteInput.split('/bots/invite/')[1].split('?')[0]
      : inviteInput.trim();
    if (!token) return;
    setAdding(true);
    setAddError('');
    try {
      await addBot(serverId, token);
      const updated = await listBots(serverId);
      setBots(updated);
      setShowAdd(false);
      setInviteInput('');
    } catch {
      setAddError('Failed to add bot. Check the invite link.');
    } finally {
      setAdding(false);
    }
  };

  const handleAddOfficial = async (token: string, username: string) => {
    setAddingOfficial(username);
    try {
      await addBot(serverId, token);
      const updated = await listBots(serverId);
      setBots(updated);
    } catch {
      // 409 = already in server, silently ignore
    } finally {
      setAddingOfficial(null);
    }
  };

  const handleRemove = async (bot: BotSummary) => {
    if (!window.confirm(`Remove ${bot.display_name} from this server?`)) return;
    try {
      await removeBot(serverId, bot.id);
      setBots(prev => prev.filter(b => b.id !== bot.id));
      if (selected?.id === bot.id) setSelected(null);
    } catch {
      alert('Failed to remove bot. Please try again.');
    }
  };

  return (
    <div className="bots-tab">
      <div className="bots-list-panel">
        <div className="bots-list-title">Bots</div>
        {bots.map(bot => (
          <button
            key={bot.id}
            className={`bots-list-item${selected?.id === bot.id ? ' active' : ''}`}
            onClick={() => setSelected(bot)}
          >
            <div className="bots-list-avatar">
              {bot.avatar_url
                ? <img src={bot.avatar_url} alt="" style={{ width: '100%', height: '100%', borderRadius: '50%', objectFit: 'cover' }} />
                : bot.display_name.charAt(0).toUpperCase()}
            </div>
            <span className="bots-list-name">{bot.display_name}</span>
            {bot.is_verified && <span className="bots-verified" title="Verified">✓</span>}
          </button>
        ))}
        {isAdmin && (
          <button className="bots-add-btn" onClick={() => setShowAdd(true)}>+ Add Bot</button>
        )}
      </div>

      {selected ? (
        <BotConfigPanel
          bot={selected}
          serverId={serverId}
          isAdmin={isAdmin}
          onRemove={() => handleRemove(selected)}
        />
      ) : (
        <div className="bots-empty">
          {bots.length === 0 ? 'No bots yet. Add a bot to get started.' : 'Select a bot to configure it.'}
        </div>
      )}

      {showAdd && (
        <div className="bots-add-modal-overlay" onClick={() => setShowAdd(false)}>
          <div className="bots-add-modal" onClick={e => e.stopPropagation()}>
            <h3>Add Bot</h3>
            <input
              placeholder="Paste a bot invite link or token"
              value={inviteInput}
              onChange={e => setInviteInput(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handleAdd()}
              autoFocus
            />
            {addError && <div style={{ fontSize: 12, color: 'var(--parley-danger,#f04747)', marginBottom: 4 }}>{addError}</div>}
            <div className="bots-add-modal-actions">
              <button className="bots-modal-cancel" onClick={() => setShowAdd(false)}>Cancel</button>
              <button className="bots-modal-submit" onClick={handleAdd} disabled={adding}>
                {adding ? 'Adding…' : 'Add'}
              </button>
            </div>

            <div className="bots-official-divider">Official Bots</div>
            {OFFICIAL_BOTS.map(ob => {
              const alreadyAdded = bots.some(b => b.username === ob.username);
              return (
                <div key={ob.username} className="bots-official-row">
                  <div className="bots-official-avatar">{ob.displayName.charAt(0)}</div>
                  <div className="bots-official-info">
                    <div className="bots-official-name">{ob.displayName}</div>
                    <div className="bots-official-desc">{ob.description}</div>
                  </div>
                  <button
                    className="bots-official-add"
                    disabled={alreadyAdded || addingOfficial === ob.username}
                    onClick={() => !alreadyAdded && handleAddOfficial(ob.token, ob.username)}
                  >
                    {alreadyAdded ? 'Added' : addingOfficial === ob.username ? 'Adding…' : 'Add'}
                  </button>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
};
