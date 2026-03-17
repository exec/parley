// frontend/src/components/settings/BotsTab.tsx
import React, { useEffect, useState } from 'react';
import { BotSummary, UserBot, listBots, addBot, removeBot, getMyBots, OFFICIAL_BOTS } from '../../api/bots';
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
  const [myBots, setMyBots] = useState<UserBot[]>([]);
  const [adding, setAdding] = useState<string | null>(null);

  useEffect(() => {
    listBots(serverId).then(setBots).catch(() => {});
  }, [serverId]);

  useEffect(() => {
    if (showAdd) getMyBots().then(setMyBots).catch(() => {});
  }, [showAdd]);

  const handleAddByToken = async (token: string, key: string) => {
    setAdding(key);
    try {
      await addBot(serverId, token);
      const updated = await listBots(serverId);
      setBots(updated);
    } catch {
      // 409 = already in server, silently ignore
    } finally {
      setAdding(null);
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

            {myBots.length > 0 && (
              <>
                <div className="bots-official-divider" style={{ marginTop: 0 }}>Your Bots</div>
                {myBots.map(ob => {
                  const alreadyAdded = bots.some(b => b.id === ob.id);
                  const key = `mine-${ob.username}`;
                  return (
                    <div key={ob.username} className="bots-official-row">
                      <div className="bots-official-avatar">{ob.display_name.charAt(0)}</div>
                      <div className="bots-official-info">
                        <div className="bots-official-name">{ob.display_name}</div>
                        <div className="bots-official-desc">@{ob.username}</div>
                      </div>
                      <button
                        className="bots-official-add"
                        disabled={alreadyAdded || adding === key}
                        onClick={() => !alreadyAdded && handleAddByToken(ob.invite_token, key)}
                      >
                        {alreadyAdded ? 'Added' : adding === key ? 'Adding…' : 'Add'}
                      </button>
                    </div>
                  );
                })}
              </>
            )}

            <div className="bots-official-divider" style={myBots.length === 0 ? { marginTop: 0 } : undefined}>Official Bots</div>
            {OFFICIAL_BOTS.map(ob => {
              const alreadyAdded = bots.some(b => b.username === ob.username);
              const key = `official-${ob.username}`;
              return (
                <div key={ob.username} className="bots-official-row">
                  <div className="bots-official-avatar">{ob.displayName.charAt(0)}</div>
                  <div className="bots-official-info">
                    <div className="bots-official-name">{ob.displayName}</div>
                    <div className="bots-official-desc">{ob.description}</div>
                  </div>
                  <button
                    className="bots-official-add"
                    disabled={alreadyAdded || adding === key}
                    onClick={() => !alreadyAdded && handleAddByToken(ob.token, key)}
                  >
                    {alreadyAdded ? 'Added' : adding === key ? 'Adding…' : 'Add'}
                  </button>
                </div>
              );
            })}

            <div className="bots-add-modal-actions" style={{ marginTop: 12 }}>
              <button className="bots-modal-cancel" onClick={() => setShowAdd(false)}>Close</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
