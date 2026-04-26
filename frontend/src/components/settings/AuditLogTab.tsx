import React, { useState, useEffect, useCallback } from 'react';
import { Server } from '../../api/types';
import { getAuditLog, AuditLogEntry } from '../../api/audit';
import {
  PERM_ADMINISTRATOR,
  PERM_MANAGE_SERVER,
  PERM_MANAGE_ROLES,
  PERM_MANAGE_CHANNELS,
  PERM_KICK_MEMBERS,
  PERM_BAN_MEMBERS,
  PERM_MANAGE_NICKNAMES,
  PERM_CHANGE_NICKNAME,
  PERM_CREATE_INVITE,
  PERM_VIEW_AUDIT_LOG,
  PERM_MANAGE_WEBHOOKS,
  PERM_MANAGE_EXPRESSIONS,
  PERM_MANAGE_EVENTS,
  PERM_MODERATE_MEMBER,
  PERM_VIEW_CHANNEL,
  PERM_SEND_MESSAGES,
  PERM_EMBED_LINKS,
  PERM_ATTACH_FILES,
  PERM_ADD_REACTIONS,
  PERM_MENTION_EVERYONE,
  PERM_MANAGE_MESSAGES,
  PERM_READ_MESSAGE_HISTORY,
  PERM_USE_EXTERNAL_EMOJI,
  PERM_PIN_MESSAGES,
  PERM_MANAGE_THREADS,
  PERM_CREATE_PUBLIC_THREADS,
  PERM_SEND_MESSAGES_IN_THREADS,
  PERM_CREATE_POSTS,
  PERM_MANAGE_POSTS,
  PERM_MANAGE_TAGS,
  PERM_CONNECT,
  PERM_SPEAK,
  PERM_MUTE_MEMBERS,
  PERM_DEAFEN_MEMBERS,
  PERM_MOVE_MEMBERS,
  PERM_USE_VAD,
  PERM_PRIORITY_SPEAKER,
  PERM_STREAM,
  PERM_USE_SOUNDBOARD,
  PERM_SEND_VOICE_MESSAGES,
} from '../../lib/permissions';
import './AuditLogTab.css';

interface Props {
  server: Server;
  currentUserId: string;
}

const PAGE_SIZE = 50;

// Mirrors the bit constants in lib/permissions.ts (in declaration order).
const PERM_NAMES: Array<[bigint, string]> = [
  [PERM_ADMINISTRATOR,           'Administrator'],
  [PERM_MANAGE_SERVER,           'Manage Server'],
  [PERM_MANAGE_ROLES,            'Manage Roles'],
  [PERM_MANAGE_CHANNELS,         'Manage Channels'],
  [PERM_KICK_MEMBERS,            'Kick Members'],
  [PERM_BAN_MEMBERS,             'Ban Members'],
  [PERM_MANAGE_NICKNAMES,        'Manage Nicknames'],
  [PERM_CHANGE_NICKNAME,         'Change Nickname'],
  [PERM_CREATE_INVITE,           'Create Invite'],
  [PERM_VIEW_AUDIT_LOG,          'View Audit Log'],
  [PERM_MANAGE_WEBHOOKS,         'Manage Webhooks'],
  [PERM_MANAGE_EXPRESSIONS,      'Manage Expressions'],
  [PERM_MANAGE_EVENTS,           'Manage Events'],
  [PERM_MODERATE_MEMBER,         'Moderate Member'],
  [PERM_VIEW_CHANNEL,            'View Channel'],
  [PERM_SEND_MESSAGES,           'Send Messages'],
  [PERM_EMBED_LINKS,             'Embed Links'],
  [PERM_ATTACH_FILES,            'Attach Files'],
  [PERM_ADD_REACTIONS,           'Add Reactions'],
  [PERM_MENTION_EVERYONE,        'Mention Everyone'],
  [PERM_MANAGE_MESSAGES,         'Manage Messages'],
  [PERM_READ_MESSAGE_HISTORY,    'Read Message History'],
  [PERM_USE_EXTERNAL_EMOJI,      'Use External Emoji'],
  [PERM_PIN_MESSAGES,            'Pin Messages'],
  [PERM_MANAGE_THREADS,          'Manage Threads'],
  [PERM_CREATE_PUBLIC_THREADS,   'Create Public Threads'],
  [PERM_SEND_MESSAGES_IN_THREADS,'Send Messages in Threads'],
  [PERM_CREATE_POSTS,            'Create Posts'],
  [PERM_MANAGE_POSTS,            'Manage Posts'],
  [PERM_MANAGE_TAGS,             'Manage Tags'],
  [PERM_CONNECT,                 'Connect'],
  [PERM_SPEAK,                   'Speak'],
  [PERM_MUTE_MEMBERS,            'Mute Members'],
  [PERM_DEAFEN_MEMBERS,          'Deafen Members'],
  [PERM_MOVE_MEMBERS,            'Move Members'],
  [PERM_USE_VAD,                 'Use VAD'],
  [PERM_PRIORITY_SPEAKER,        'Priority Speaker'],
  [PERM_STREAM,                  'Stream'],
  [PERM_USE_SOUNDBOARD,          'Use Soundboard'],
  [PERM_SEND_VOICE_MESSAGES,     'Send Voice Messages'],
];

function getIcon(action: string): string {
  if (action.startsWith('member.')) return '👤';
  if (action.startsWith('role.')) return '🔑';
  if (action.startsWith('channel.')) return '#️⃣';
  if (action.startsWith('invite.')) return '🔗';
  if (action.startsWith('server.')) return '⚙️';
  if (action.startsWith('soundboard.')) return '🔊';
  if (action.startsWith('bot.')) return '🤖';
  if (action.startsWith('voice.')) return '🎙️';
  return '📋';
}

function categoryClass(action: string): string {
  if (action.startsWith('member.'))     return 'audit-row--member';
  if (action.startsWith('role.'))       return 'audit-row--role';
  if (action.startsWith('channel.'))    return 'audit-row--channel';
  if (action.startsWith('server.'))     return 'audit-row--server';
  if (action.startsWith('soundboard.')) return 'audit-row--soundboard';
  if (action.startsWith('bot.'))        return 'audit-row--bot';
  if (action.startsWith('voice.'))      return 'audit-row--voice';
  if (action.startsWith('invite.'))     return 'audit-row--invite';
  return '';
}

function describe(entry: AuditLogEntry): string {
  const actor = entry.actor_username || entry.actor_id || 'Unknown';
  const target = entry.target_name || entry.target_id || 'unknown';
  switch (entry.action) {
    case 'member.kick':                return `${actor} kicked ${target}`;
    case 'member.ban':                 return `${actor} banned ${target}`;
    case 'member.unban':               return `${actor} unbanned ${target}`;
    case 'member.role_add':            return `${actor} added role ${target} to a member`;
    case 'member.role_remove':         return `${actor} removed role ${target} from a member`;
    case 'role.create':                return `${actor} created role ${target}`;
    case 'role.update':                return `${actor} updated role ${target}`;
    case 'role.delete':                return `${actor} deleted role ${target}`;
    case 'channel.create':             return `${actor} created channel ${target}`;
    case 'channel.update':             return `${actor} updated channel ${target}`;
    case 'channel.delete':             return `${actor} deleted channel ${target}`;
    case 'channel.overwrite_set':      return `${actor} updated permissions on ${target}`;
    case 'channel.overwrite_delete':   return `${actor} cleared a permission overwrite on ${target}`;
    case 'invite.create':              return `${actor} created an invite`;
    case 'invite.revoke':              return `${actor} revoked an invite`;
    case 'server.update':              return `${actor} updated server settings`;
    case 'server.vanity_update':       return `${actor} updated the vanity URL`;
    case 'server.categories_update':   return `${actor} updated server categories`;
    case 'soundboard.create':          return `${actor} added sound ${target}`;
    case 'soundboard.update':          return `${actor} updated sound ${target}`;
    case 'soundboard.delete':          return `${actor} deleted sound ${target}`;
    case 'bot.add':                    return `${actor} added bot ${target}`;
    case 'bot.remove':                 return `${actor} removed bot ${target}`;
    case 'bot.ai_config_update':       return `${actor} updated bot AI configuration`;
    case 'voice.force_mute':           return `${actor} server-muted ${target}`;
    case 'voice.force_disconnect':     return `${actor} disconnected ${target} from voice`;
    default:                           return `${actor} performed ${entry.action}`;
  }
}

function relativeTime(isoDate: string): string {
  const diff = Date.now() - new Date(isoDate).getTime();
  const s = Math.floor(diff / 1000);
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

// ── Diff renderer ────────────────────────────────────────────────────────────

function decodePerms(bits: bigint): string[] {
  return PERM_NAMES.filter(([b]) => (bits & b) !== 0n).map(([, name]) => name);
}

function toBigIntSafe(v: unknown): bigint | null {
  if (v === null || v === undefined) return 0n;
  if (typeof v === 'bigint') return v;
  if (typeof v === 'number') {
    if (!Number.isFinite(v)) return null;
    try {
      return BigInt(Math.trunc(v));
    } catch {
      return null;
    }
  }
  if (typeof v === 'string') {
    if (v === '') return 0n;
    try {
      return BigInt(v);
    } catch {
      return null;
    }
  }
  return null;
}

function isPermKey(key: string): boolean {
  return key === 'permissions' || key === 'allow' || key === 'deny' || key === 'granted_permissions';
}

function PermDiff({ before, after }: { before: bigint; after: bigint }) {
  const beforeSet = new Set(decodePerms(before));
  const afterSet = new Set(decodePerms(after));
  const added   = [...afterSet].filter(n => !beforeSet.has(n));
  const removed = [...beforeSet].filter(n => !afterSet.has(n));
  if (added.length === 0 && removed.length === 0) {
    return <span>(no permission changes)</span>;
  }
  return (
    <span>
      {added.map(n => (
        <span key={`+${n}`} className="audit-diff__perm-add">+ {n}</span>
      ))}
      {removed.map(n => (
        <span key={`-${n}`} className="audit-diff__perm-remove">- {n}</span>
      ))}
    </span>
  );
}

function PermList({ bits }: { bits: bigint }) {
  const names = decodePerms(bits);
  if (names.length === 0) return <span>(none)</span>;
  return (
    <span>
      {names.map(n => (
        <span key={n} className="audit-diff__perm-add">+ {n}</span>
      ))}
    </span>
  );
}

function ColorSwatch({ hex }: { hex: string }) {
  return (
    <span className="audit-diff__color-row">
      <span className="audit-diff__color-swatch" style={{ background: hex }} />
      <code>{hex}</code>
    </span>
  );
}

function renderValue(key: string, value: unknown): React.ReactNode {
  if (value === undefined || value === null) return <code>—</code>;
  if (key === 'color' && typeof value === 'string') return <ColorSwatch hex={value} />;
  if (typeof value === 'boolean') return value ? 'Yes' : 'No';
  if (Array.isArray(value)) {
    return `${value.length} item${value.length === 1 ? '' : 's'}`;
  }
  if (typeof value === 'string') return <code>{value || '""'}</code>;
  if (typeof value === 'number') return <code>{String(value)}</code>;
  return <code>{JSON.stringify(value)}</code>;
}

function ChangeDiff({ changes }: { changes: AuditLogEntry['changes'] }) {
  if (!changes || typeof changes !== 'object') return null;

  const changesObj = changes as Record<string, unknown>;
  const before = (changesObj.before && typeof changesObj.before === 'object' && !Array.isArray(changesObj.before))
    ? (changesObj.before as Record<string, unknown>)
    : undefined;
  const after = (changesObj.after && typeof changesObj.after === 'object' && !Array.isArray(changesObj.after))
    ? (changesObj.after as Record<string, unknown>)
    : undefined;

  // Flat keys (anything not "before"/"after")
  const flatKeys = Object.keys(changesObj).filter(k => k !== 'before' && k !== 'after');

  // Union of before/after keys for diff rows.
  const diffKeys = new Set<string>();
  if (after) Object.keys(after).forEach(k => diffKeys.add(k));
  if (before) Object.keys(before).forEach(k => diffKeys.add(k));

  if (flatKeys.length === 0 && diffKeys.size === 0) return null;

  return (
    <div className="audit-diff">
      {flatKeys.map(key => {
        const v = changesObj[key];
        // Permission bitfield case for top-level keys (e.g. granted_permissions on bot.add)
        if (isPermKey(key)) {
          const bits = toBigIntSafe(v);
          if (bits !== null) {
            return (
              <div key={key} className="audit-diff__row">
                <span className="audit-diff__key">{key}:</span>{' '}
                <PermList bits={bits} />
              </div>
            );
          }
        }
        return (
          <div key={key} className="audit-diff__row">
            <span className="audit-diff__key">{key}:</span>{' '}
            {renderValue(key, v)}
          </div>
        );
      })}
      {[...diffKeys].map(key => {
        const oldVal = before?.[key];
        const newVal = after?.[key];
        // Skip if structurally equal (comparing JSON repr handles primitives + simple shapes).
        if (JSON.stringify(oldVal) === JSON.stringify(newVal)) return null;

        // Permission bitfield diff
        if (isPermKey(key)) {
          const b = toBigIntSafe(oldVal);
          const a = toBigIntSafe(newVal);
          if (b !== null && a !== null) {
            return (
              <div key={key} className="audit-diff__row">
                <span className="audit-diff__key">{key}:</span>{' '}
                <PermDiff before={b} after={a} />
              </div>
            );
          }
        }

        return (
          <div key={key} className="audit-diff__row">
            <span className="audit-diff__key">{key}:</span>{' '}
            <span className="audit-diff__before">{renderValue(key, oldVal)}</span>
            {' → '}
            <span className="audit-diff__after">{renderValue(key, newVal)}</span>
          </div>
        );
      })}
    </div>
  );
}

// ── Component ────────────────────────────────────────────────────────────────

export const AuditLogTab: React.FC<Props> = ({ server, currentUserId }) => {
  // _isOwner available if needed; tab is already gated at ServerSettings level
  void (server.owner_id === currentUserId);

  const [logs, setLogs] = useState<AuditLogEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [action, setAction] = useState('');
  const [actorFilter, setActorFilter] = useState('');
  const [targetFilter, setTargetFilter] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const fetchLogs = useCallback(async (newOffset: number, actionFilter: string, targetParam: string) => {
    setLoading(true);
    setError('');
    try {
      const result = await getAuditLog(server.id, {
        limit: PAGE_SIZE,
        offset: newOffset,
        action: actionFilter || undefined,
        target: targetParam || undefined,
      });
      if (newOffset === 0) {
        setLogs(result.logs ?? []);
      } else {
        setLogs(prev => [...prev, ...(result.logs ?? [])]);
      }
      setTotal(result.total ?? 0);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load audit log');
    } finally {
      setLoading(false);
    }
  }, [server.id]);

  // Reset filters and refetch when the action filter changes.
  useEffect(() => {
    setActorFilter('');
    setTargetFilter('');
    fetchLogs(0, action, '');
  }, [action]); // eslint-disable-line react-hooks/exhaustive-deps

  // Also fetch on mount (when server changes).
  useEffect(() => {
    fetchLogs(0, action, targetFilter);
  }, [server.id]); // eslint-disable-line react-hooks/exhaustive-deps

  // Debounced server-side target search (300ms).
  useEffect(() => {
    const t = setTimeout(() => {
      fetchLogs(0, action, targetFilter);
    }, 300);
    return () => clearTimeout(t);
  }, [targetFilter]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleActionChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    setAction(e.target.value);
    // actor + target reset happens in the action useEffect
  };

  const handleLoadMore = () => {
    fetchLogs(logs.length, action, targetFilter);
  };

  // Actor filter remains client-side substring.
  const visibleLogs = actorFilter
    ? logs.filter(e => e.actor_username.toLowerCase().includes(actorFilter.toLowerCase()))
    : logs;

  return (
    <div>
      <h2 className="settings-page-title">Audit Log</h2>

      {/* Filters */}
      <div className="audit-filter">
        <div className="audit-filter__field">
          <label className="audit-filter__label">Action</label>
          <select
            value={action}
            onChange={handleActionChange}
            className="settings-form-input audit-filter__select"
          >
            <option value="">All actions</option>
            <optgroup label="Members">
              <option value="member.kick">member.kick</option>
              <option value="member.ban">member.ban</option>
              <option value="member.unban">member.unban</option>
              <option value="member.role_add">member.role_add</option>
              <option value="member.role_remove">member.role_remove</option>
            </optgroup>
            <optgroup label="Roles">
              <option value="role.create">role.create</option>
              <option value="role.update">role.update</option>
              <option value="role.delete">role.delete</option>
            </optgroup>
            <optgroup label="Channels">
              <option value="channel.create">channel.create</option>
              <option value="channel.update">channel.update</option>
              <option value="channel.delete">channel.delete</option>
              <option value="channel.overwrite_set">channel.overwrite_set</option>
              <option value="channel.overwrite_delete">channel.overwrite_delete</option>
            </optgroup>
            <optgroup label="Soundboard">
              <option value="soundboard.create">soundboard.create</option>
              <option value="soundboard.update">soundboard.update</option>
              <option value="soundboard.delete">soundboard.delete</option>
            </optgroup>
            <optgroup label="Bots">
              <option value="bot.add">bot.add</option>
              <option value="bot.remove">bot.remove</option>
              <option value="bot.ai_config_update">bot.ai_config_update</option>
            </optgroup>
            <optgroup label="Voice">
              <option value="voice.force_mute">voice.force_mute</option>
              <option value="voice.force_disconnect">voice.force_disconnect</option>
            </optgroup>
            <optgroup label="Invites">
              <option value="invite.create">invite.create</option>
              <option value="invite.revoke">invite.revoke</option>
            </optgroup>
            <optgroup label="Server">
              <option value="server.update">server.update</option>
              <option value="server.vanity_update">server.vanity_update</option>
              <option value="server.categories_update">server.categories_update</option>
            </optgroup>
          </select>
        </div>

        <div className="audit-filter__field">
          <label className="audit-filter__label">Filter by actor</label>
          <input
            type="text"
            className="settings-form-input audit-filter__input"
            placeholder="Username..."
            value={actorFilter}
            onChange={e => setActorFilter(e.target.value)}
          />
        </div>

        <div className="audit-filter__field">
          <label className="audit-filter__label">Filter by target</label>
          <input
            type="text"
            className="settings-form-input audit-filter__input"
            placeholder="Username, channel, role..."
            value={targetFilter}
            onChange={e => setTargetFilter(e.target.value)}
          />
        </div>
      </div>

      {error && <div className="settings-error audit-error">{error}</div>}

      {loading && logs.length === 0 && (
        <div className="audit-loading">Loading...</div>
      )}

      {!loading && visibleLogs.length === 0 && (
        <div className="audit-empty">No audit log entries found.</div>
      )}

      <div className="audit-list">
        {visibleLogs.map(entry => (
          <div key={entry.id} className={`audit-row ${categoryClass(entry.action)}`}>
            {entry.actor_avatar_url ? (
              <img
                className="audit-row__icon-img"
                src={entry.actor_avatar_url}
                alt={entry.actor_username || 'actor'}
              />
            ) : (
              <span className="audit-row__icon">{getIcon(entry.action)}</span>
            )}
            <div className="audit-row__body">
              <div className="audit-row__desc">{describe(entry)}</div>
              {entry.reason && (
                <div className="audit-row__reason">Reason: {entry.reason}</div>
              )}
              {entry.changes && <ChangeDiff changes={entry.changes} />}
            </div>
            <span className="audit-row__time" title={entry.created_at}>
              {relativeTime(entry.created_at)}
            </span>
          </div>
        ))}
      </div>

      {logs.length < total && (
        <div className="audit-load-more">
          <button
            className="settings-btn settings-btn-secondary"
            onClick={handleLoadMore}
            disabled={loading}
          >
            {loading ? 'Loading...' : 'Load More'}
          </button>
        </div>
      )}
    </div>
  );
};
