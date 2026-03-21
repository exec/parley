import React, { useState, useEffect, useCallback } from 'react';
import { Server } from '../../api/types';
import { getAuditLog, AuditLogEntry } from '../../api/audit';

interface Props {
  server: Server;
  currentUserId: string;
}

const PAGE_SIZE = 50;

function getIcon(action: string): string {
  if (action.startsWith('member.')) return '👤';
  if (action.startsWith('role.')) return '🔑';
  if (action.startsWith('channel.')) return '#️⃣';
  if (action.startsWith('invite.')) return '🔗';
  if (action.startsWith('server.')) return '⚙️';
  return '📋';
}

function describe(entry: AuditLogEntry): string {
  const actor = entry.actor_username || entry.actor_id || 'Unknown';
  const target = entry.target_name || entry.target_id || 'unknown';
  switch (entry.action) {
    case 'member.kick':        return `${actor} kicked ${target}`;
    case 'member.ban':         return `${actor} banned ${target}`;
    case 'member.unban':       return `${actor} unbanned ${target}`;
    case 'member.role_add':    return `${actor} added role ${target} to a member`;
    case 'member.role_remove': return `${actor} removed role ${target} from a member`;
    case 'role.create':        return `${actor} created role ${target}`;
    case 'role.update':        return `${actor} updated role ${target}`;
    case 'role.delete':        return `${actor} deleted role ${target}`;
    case 'channel.create':     return `${actor} created channel ${target}`;
    case 'channel.update':     return `${actor} updated channel ${target}`;
    case 'channel.delete':     return `${actor} deleted channel ${target}`;
    case 'invite.create':      return `${actor} created an invite`;
    case 'invite.revoke':      return `${actor} revoked an invite`;
    case 'server.update':      return `${actor} updated server settings`;
    case 'server.vanity_update': return `${actor} updated the vanity URL`;
    default:                   return `${actor} performed ${entry.action}`;
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

function ChangeDiff({ changes }: { changes: AuditLogEntry['changes'] }) {
  if (!changes) return null;
  const { before, after } = changes;
  const keys = Object.keys(after);
  if (keys.length === 0) return null;

  return (
    <div style={{ marginTop: 4, fontSize: 11, color: 'var(--parley-text-muted)', fontFamily: 'monospace' }}>
      {keys.map(key => {
        const oldVal = before[key];
        const newVal = after[key];
        if (oldVal === newVal) return null;
        return (
          <div key={key} style={{ marginTop: 2 }}>
            <span style={{ fontWeight: 600 }}>{key}:</span>{' '}
            <span style={{ color: 'var(--parley-danger)', textDecoration: 'line-through' }}>
              {JSON.stringify(oldVal)}
            </span>
            {' → '}
            <span style={{ color: '#3ba55d' }}>
              {JSON.stringify(newVal)}
            </span>
          </div>
        );
      })}
    </div>
  );
}

export const AuditLogTab: React.FC<Props> = ({ server, currentUserId }) => {
  // _isOwner available if needed; tab is already gated at ServerSettings level
  void (server.owner_id === currentUserId);

  const [logs, setLogs] = useState<AuditLogEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [action, setAction] = useState('');
  const [actorFilter, setActorFilter] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const fetchLogs = useCallback(async (newOffset: number, actionFilter: string) => {
    setLoading(true);
    setError('');
    try {
      const result = await getAuditLog(server.id, {
        limit: PAGE_SIZE,
        offset: newOffset,
        action: actionFilter || undefined,
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

  // Fetch on mount and when action filter changes
  useEffect(() => {
    setActorFilter('');
    fetchLogs(0, action);
  }, [action]); // eslint-disable-line react-hooks/exhaustive-deps

  // Also fetch on mount (when server changes)
  useEffect(() => {
    fetchLogs(0, action);
  }, [server.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleActionChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    setAction(e.target.value);
    // actorFilter reset happens in the action useEffect
  };

  const handleLoadMore = () => {
    fetchLogs(logs.length, action);
  };

  const visibleLogs = actorFilter
    ? logs.filter(e => e.actor_username.toLowerCase().includes(actorFilter.toLowerCase()))
    : logs;

  return (
    <div>
      <h2 className="settings-page-title">Audit Log</h2>

      {/* Filters */}
      <div style={{ display: 'flex', gap: 12, marginBottom: 16, flexWrap: 'wrap' }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          <label style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.6px', color: 'var(--parley-text-muted)' }}>
            Action
          </label>
          <select
            value={action}
            onChange={handleActionChange}
            className="settings-form-input"
            style={{ minWidth: 200 }}
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
            </optgroup>
            <optgroup label="Invites">
              <option value="invite.create">invite.create</option>
              <option value="invite.revoke">invite.revoke</option>
            </optgroup>
            <optgroup label="Server">
              <option value="server.update">server.update</option>
              <option value="server.vanity_update">server.vanity_update</option>
            </optgroup>
          </select>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          <label style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.6px', color: 'var(--parley-text-muted)' }}>
            Filter by actor
          </label>
          <input
            type="text"
            className="settings-form-input"
            placeholder="Username..."
            value={actorFilter}
            onChange={e => setActorFilter(e.target.value)}
            style={{ minWidth: 180 }}
          />
        </div>
      </div>

      {error && <div className="settings-error" style={{ marginBottom: 12 }}>{error}</div>}

      {loading && logs.length === 0 && (
        <div style={{ fontSize: 13, color: 'var(--parley-text-muted)', padding: '16px 0' }}>Loading...</div>
      )}

      {!loading && visibleLogs.length === 0 && (
        <div style={{ fontSize: 13, color: 'var(--parley-text-muted)', padding: '16px 0' }}>
          No audit log entries found.
        </div>
      )}

      <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
        {visibleLogs.map(entry => (
          <div
            key={entry.id}
            style={{
              display: 'flex',
              gap: 10,
              padding: '8px 10px',
              borderRadius: 4,
              background: 'var(--parley-bg-secondary)',
              border: '1px solid var(--parley-border)',
            }}
          >
            <span style={{ fontSize: 18, flexShrink: 0, lineHeight: 1.4 }}>{getIcon(entry.action)}</span>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontSize: 13, color: 'var(--parley-text-normal)' }}>
                {describe(entry)}
              </div>
              {entry.reason && (
                <div style={{ fontSize: 11, color: 'var(--parley-text-muted)', marginTop: 2 }}>
                  Reason: {entry.reason}
                </div>
              )}
              {entry.changes && <ChangeDiff changes={entry.changes} />}
            </div>
            <span
              style={{ fontSize: 11, color: 'var(--parley-text-muted)', flexShrink: 0, whiteSpace: 'nowrap', alignSelf: 'flex-start', marginTop: 2 }}
              title={entry.created_at}
            >
              {relativeTime(entry.created_at)}
            </span>
          </div>
        ))}
      </div>

      {logs.length < total && (
        <div style={{ marginTop: 12, textAlign: 'center' }}>
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
