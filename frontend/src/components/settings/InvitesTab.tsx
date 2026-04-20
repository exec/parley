import React, { useState, useEffect, useRef, useCallback } from 'react';
import { Server } from '../../api/types';
import { siteOrigin } from '../../config';
import {
  Invite,
  InviteMember,
  listServerInvites,
  createInvite,
  revokeInvite,
  getInviteMembers,
} from '../../api/servers';
import './InvitesTab.css';

interface Props {
  server: Server | null;
}

function relativeTime(dateStr: string): string {
  const diff = (Date.now() - new Date(dateStr).getTime()) / 1000;
  if (diff < 60) return 'just now';
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

function timeRemaining(dateStr: string): string {
  const diff = (new Date(dateStr).getTime() - Date.now()) / 1000;
  if (diff <= 0) return 'Expired';
  if (diff < 3600) return `Expires in ${Math.ceil(diff / 60)}m`;
  if (diff < 86400) return `Expires in ${Math.ceil(diff / 3600)}h`;
  return `Expires in ${Math.ceil(diff / 86400)}d`;
}

const MAX_USES_OPTIONS = [
  { label: 'No limit', value: undefined as number | undefined },
  { label: '1', value: 1 },
  { label: '5', value: 5 },
  { label: '10', value: 10 },
  { label: '25', value: 25 },
  { label: '50', value: 50 },
  { label: '100', value: 100 },
];

const EXPIRY_OPTIONS = [
  { label: 'Never', value: undefined as string | undefined },
  { label: '30 minutes', value: '30m' },
  { label: '1 hour', value: '1h' },
  { label: '6 hours', value: '6h' },
  { label: '12 hours', value: '12h' },
  { label: '1 day', value: '1d' },
  { label: '7 days', value: '7d' },
  { label: '30 days', value: '30d' },
];

interface MembersPopoverProps {
  serverId: string;
  code: string;
  onClose: () => void;
}

const MembersPopover: React.FC<MembersPopoverProps> = ({ serverId, code, onClose }) => {
  const [members, setMembers] = useState<InviteMember[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    getInviteMembers(serverId, code)
      .then(m => { if (!cancelled) setMembers(m ?? []); })
      .catch(() => { if (!cancelled) setMembers([]); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [serverId, code]);

  return (
    <div className="invites-members-popover">
      <div className="invites-members-popover-header">
        <span>Members who used this invite</span>
        <button className="invites-members-popover-close" onClick={onClose}>×</button>
      </div>
      {loading && <div className="invites-members-popover-empty">Loading...</div>}
      {!loading && members.length === 0 && (
        <div className="invites-members-popover-empty">No members have used this invite yet.</div>
      )}
      {!loading && members.slice(0, 10).map(m => (
        <div key={m.user_id} className="invites-member-row">
          <div className="invites-member-avatar">
            {m.avatar_url
              ? <img src={m.avatar_url} alt={m.username} />
              : (m.username || 'U').charAt(0).toUpperCase()
            }
          </div>
          <div className="invites-member-info">
            <span className="invites-member-name">{m.display_name || m.username}</span>
            <span className="invites-member-joined">Joined {relativeTime(m.joined_at)}</span>
          </div>
        </div>
      ))}
    </div>
  );
};

export const InvitesTab: React.FC<Props> = ({ server }) => {
  const [invites, setInvites] = useState<Invite[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState('');
  const [newMaxUses, setNewMaxUses] = useState<number | undefined>(undefined);
  const [newExpiresIn, setNewExpiresIn] = useState<string | undefined>(undefined);
  const [createdInvite, setCreatedInvite] = useState<Invite | null>(null);
  const [revoking, setRevoking] = useState<string | null>(null);
  const [revokeConfirm, setRevokeConfirm] = useState<string | null>(null);
  const [openMembersFor, setOpenMembersFor] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const copiedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const loadInvites = useCallback(async () => {
    if (!server) return;
    setLoading(true);
    setError('');
    try {
      const list = await listServerInvites(server.id);
      setInvites(list ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load invites');
    } finally {
      setLoading(false);
    }
  }, [server]);

  useEffect(() => {
    loadInvites();
  }, [loadInvites]);

  const handleCreate = async () => {
    if (!server) return;
    setCreating(true);
    setCreateError('');
    setCreatedInvite(null);
    try {
      const invite = await createInvite(server.id, {
        max_uses: newMaxUses,
        expires_in: newExpiresIn,
      });
      setCreatedInvite(invite);
      await loadInvites();
    } catch (e) {
      setCreateError(e instanceof Error ? e.message : 'Failed to create invite');
    } finally {
      setCreating(false);
    }
  };

  const handleRevoke = async (code: string) => {
    if (!server) return;
    setRevoking(code);
    try {
      await revokeInvite(server.id, code);
      setRevokeConfirm(null);
      if (openMembersFor === code) setOpenMembersFor(null);
      await loadInvites();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to revoke invite');
    } finally {
      setRevoking(null);
    }
  };

  const handleCopy = (code: string) => {
    const url = `${siteOrigin()}/invite/${code}`;
    navigator.clipboard.writeText(url);
    setCopied(code);
    if (copiedTimerRef.current) clearTimeout(copiedTimerRef.current);
    copiedTimerRef.current = setTimeout(() => setCopied(null), 2000);
  };

  if (!server) return null;

  return (
    <div className="invites-tab">
      <h2 className="settings-page-title">Invites</h2>

      {/* Create form */}
      <div className="invites-create-card">
        <div className="invites-create-title">Create Invite</div>
        <div className="invites-create-row">
          <div className="invites-create-field">
            <label className="settings-form-label">Max Uses</label>
            <select
              className="invites-select"
              value={newMaxUses ?? ''}
              onChange={e => setNewMaxUses(e.target.value === '' ? undefined : Number(e.target.value))}
            >
              {MAX_USES_OPTIONS.map(opt => (
                <option key={opt.label} value={opt.value ?? ''}>
                  {opt.label}
                </option>
              ))}
            </select>
          </div>
          <div className="invites-create-field">
            <label className="settings-form-label">Expires After</label>
            <select
              className="invites-select"
              value={newExpiresIn ?? ''}
              onChange={e => setNewExpiresIn(e.target.value === '' ? undefined : e.target.value)}
            >
              {EXPIRY_OPTIONS.map(opt => (
                <option key={opt.label} value={opt.value ?? ''}>
                  {opt.label}
                </option>
              ))}
            </select>
          </div>
          <button
            className="settings-btn settings-btn-primary invites-create-btn"
            onClick={handleCreate}
            disabled={creating}
          >
            {creating ? 'Creating...' : 'Create Invite'}
          </button>
        </div>

        {createError && <div className="settings-error" style={{ marginTop: 10 }}>{createError}</div>}

        {createdInvite && (
          <div className="invites-new-link-box">
            <span className="invites-new-link-url">
              {siteOrigin()}/invite/{createdInvite.code}
            </span>
            <button
              className="settings-btn settings-btn-ghost"
              onClick={() => handleCopy(createdInvite.code)}
            >
              {copied === createdInvite.code ? 'Copied!' : 'Copy'}
            </button>
            <button
              className="settings-btn settings-btn-ghost"
              onClick={() => setCreatedInvite(null)}
            >
              Dismiss
            </button>
          </div>
        )}
      </div>

      {/* Invites list */}
      <div className="invites-section-title">Active Invites</div>

      {error && <div className="settings-error">{error}</div>}
      {loading && invites.length === 0 && (
        <div className="invites-empty">Loading...</div>
      )}
      {!loading && invites.length === 0 && (
        <div className="invites-empty">No active invites. Create one above!</div>
      )}

      <div className="invites-list">
        {invites.map(invite => {
          const isExpiredTime = invite.expires_at ? new Date(invite.expires_at).getTime() < Date.now() : false;
          const isActive = invite.is_active && !isExpiredTime;
          const usesLabel = invite.max_uses != null
            ? `${invite.use_count} / ${invite.max_uses} uses`
            : `${invite.use_count} use${invite.use_count !== 1 ? 's' : ''}`;
          const expiryLabel = invite.expires_at
            ? timeRemaining(invite.expires_at)
            : 'Never expires';
          const isRevokingThis = revoking === invite.code;
          const membersOpen = openMembersFor === invite.code;

          return (
            <div key={invite.id} className="invites-row">
              <div className="invites-row-main">
                <div className="invites-row-left">
                  <span className="invites-code">{invite.code}</span>
                  <span className="invites-creator">by {invite.creator_username}</span>
                </div>
                <div className="invites-row-meta">
                  <span className={`invites-uses${isActive ? ' active' : ''}`}>{usesLabel}</span>
                  <span className={`invites-expiry${isExpiredTime ? ' expired' : ''}`}>{expiryLabel}</span>
                  <span className="invites-created">{relativeTime(invite.created_at)}</span>
                </div>
                <div className="invites-row-actions">
                  <button
                    className="settings-btn settings-btn-ghost invites-action-btn"
                    onClick={() => handleCopy(invite.code)}
                  >
                    {copied === invite.code ? 'Copied!' : 'Copy'}
                  </button>
                  <button
                    className={`settings-btn settings-btn-ghost invites-action-btn${membersOpen ? ' active-btn' : ''}`}
                    onClick={() => setOpenMembersFor(membersOpen ? null : invite.code)}
                  >
                    Members
                  </button>
                  {revokeConfirm === invite.code ? (
                    <>
                      <button
                        className="settings-btn settings-btn-danger invites-action-btn"
                        onClick={() => handleRevoke(invite.code)}
                        disabled={isRevokingThis}
                      >
                        {isRevokingThis ? 'Revoking...' : 'Confirm'}
                      </button>
                      <button
                        className="settings-btn settings-btn-secondary invites-action-btn"
                        onClick={() => setRevokeConfirm(null)}
                      >
                        Cancel
                      </button>
                    </>
                  ) : (
                    <button
                      className="settings-btn settings-btn-danger invites-action-btn"
                      onClick={() => setRevokeConfirm(invite.code)}
                    >
                      Revoke
                    </button>
                  )}
                </div>
              </div>

              {membersOpen && (
                <MembersPopover
                  serverId={server.id}
                  code={invite.code}
                  onClose={() => setOpenMembersFor(null)}
                />
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
};
