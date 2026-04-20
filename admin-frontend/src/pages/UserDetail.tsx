import { useState, useEffect } from 'react'
import { SITE_URL } from '../config'
import { useParams, useNavigate, Link } from 'react-router-dom'
import {
  apiGetUser, apiBanUser, apiUnbanUser, apiForceLogout,
  apiImpersonateUser, apiDeleteUser, apiSetBadges, apiAddUserInvites,
  userStatus, User,
} from '../api'
import StatusBadge from '../components/StatusBadge'
import Modal from '../components/Modal'

export default function UserDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [showBanModal, setShowBanModal] = useState(false)
  const [banReason, setBanReason] = useState('')
  const [actionLoading, setActionLoading] = useState(false)
  const [badgesLoading, setBadgesLoading] = useState(false)
  const [inviteCount, setInviteCount] = useState(1)
  const [inviteLoading, setInviteLoading] = useState(false)

  useEffect(() => {
    if (!id) return
    setLoading(true)
    apiGetUser(Number(id))
      .then(setUser)
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }, [id])

  const reload = () => { if (id) apiGetUser(Number(id)).then(setUser).catch(() => {}) }

  const handleBan = async () => {
    if (!user) return
    setActionLoading(true)
    try {
      await apiBanUser(user.id, banReason)
      setSuccess('User banned.')
      setShowBanModal(false)
      reload()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Ban failed')
    } finally { setActionLoading(false) }
  }

  const handleUnban = async () => {
    if (!user || !confirm(`Unban "${user.username}"?`)) return
    setActionLoading(true)
    try { await apiUnbanUser(user.id); setSuccess('User unbanned.'); reload() }
    catch (e) { setError(e instanceof Error ? e.message : 'Unban failed') }
    finally { setActionLoading(false) }
  }

  const handleForceLogout = async () => {
    if (!user || !confirm(`Force logout "${user.username}"?`)) return
    try { await apiForceLogout(user.id); setSuccess('Force logout sent.') }
    catch (e) { setError(e instanceof Error ? e.message : 'Force logout failed') }
  }

  const handleImpersonate = async () => {
    if (!user) return
    try {
      const { token } = await apiImpersonateUser(user.id)
      window.open(`${SITE_URL}/impersonate?token=${token}`, '_blank')
    } catch (e) { setError(e instanceof Error ? e.message : 'Impersonate failed') }
  }

  const handleToggleBadge = async (bit: number) => {
    if (!user) return
    const newBadges = (user.badges ?? 0) ^ bit
    setBadgesLoading(true)
    try {
      const res = await apiSetBadges(user.id, newBadges)
      setUser({ ...user, badges: res.badges })
      setSuccess('Badges updated.')
    } catch (e) { setError(e instanceof Error ? e.message : 'Badge update failed') }
    finally { setBadgesLoading(false) }
  }

  const handleAddInvites = async () => {
    if (!user) return
    if (inviteCount < 1 || inviteCount > 10) {
      setError('Invite count must be between 1 and 10.')
      return
    }
    setInviteLoading(true)
    try {
      await apiAddUserInvites(user.id, inviteCount)
      setSuccess(`Added ${inviteCount} invite${inviteCount === 1 ? '' : 's'}.`)
      reload()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to add invites')
    } finally {
      setInviteLoading(false)
    }
  }

  const handleDelete = async () => {
    if (!user || !confirm(`Delete user "${user.username}"? This is irreversible.`)) return
    try { await apiDeleteUser(user.id); navigate('/users') }
    catch (e) { setError(e instanceof Error ? e.message : 'Delete failed') }
  }

  const fmt = (d?: string | null) => d ? new Date(d).toLocaleString('en-US', { dateStyle: 'medium', timeStyle: 'short' }) : '—'

  if (loading) {
    return (
      <div style={{ padding: '60px', textAlign: 'center', color: 'var(--text-secondary)' }}>
        <div className="loading-spinner" style={{ margin: '0 auto 12px' }} />
        Loading user…
      </div>
    )
  }

  if (!user) {
    return (
      <div>
        <div className="alert alert-error">{error || 'User not found'}</div>
        <button className="btn" onClick={() => navigate('/users')}>← Back to users</button>
      </div>
    )
  }

  const status = userStatus(user)

  return (
    <div>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '24px', flexWrap: 'wrap' }}>
        <button className="btn" onClick={() => navigate('/users')}>← Users</button>
        <h1 style={{ fontSize: '22px', fontWeight: '700', letterSpacing: '-0.02em', margin: 0 }}>{user.username}</h1>
        <StatusBadge status={status} />
      </div>

      {error && <div className="alert alert-error" onClick={() => setError('')} style={{ cursor: 'pointer', marginBottom: '14px' }}>{error}</div>}
      {success && <div className="alert alert-success" onClick={() => setSuccess('')} style={{ cursor: 'pointer', marginBottom: '14px' }}>{success}</div>}

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0,2fr) minmax(0,1fr)', gap: '16px', alignItems: 'start' }}>
        {/* User record */}
        <div className="card">
          <div className="card-title">Account Details</div>
          <div className="detail-grid">
            <span className="detail-label">ID</span>
            <span className="detail-value mono" style={{ color: 'var(--text-muted)' }}>{user.id}</span>

            <span className="detail-label">Username</span>
            <span className="detail-value" style={{ fontWeight: '600', color: 'var(--text)' }}>{user.username}</span>

            <span className="detail-label">Email</span>
            <span className="detail-value">{user.email || '—'}</span>

            <span className="detail-label">Email verified</span>
            <span className="detail-value">
              {user.email_verified
                ? <span style={{ color: 'var(--green)', fontWeight: '600' }}>Yes</span>
                : <span style={{ color: 'var(--text-muted)' }}>No</span>}
            </span>

            <span className="detail-label">Phone</span>
            <span className="detail-value">{user.phone_number ?? '—'}</span>

            <span className="detail-label">Phone verified</span>
            <span className="detail-value">
              {user.phone_verified
                ? <span style={{ color: 'var(--green)', fontWeight: '600' }}>Yes</span>
                : <span style={{ color: 'var(--text-muted)' }}>No</span>}
            </span>

            <span className="detail-label">Status</span>
            <span className="detail-value"><StatusBadge status={status} /></span>

            <span className="detail-label">Invites</span>
            <span className="detail-value" style={{ fontWeight: '600', color: user.invite_count > 0 ? 'var(--accent)' : 'var(--text-muted)' }}>
              {user.invite_count ?? 0}
            </span>

            <span className="detail-label">Joined</span>
            <span className="detail-value mono" style={{ color: 'var(--text-secondary)' }}>{fmt(user.created_at)}</span>

            <span className="detail-label">Reg. IP</span>
            <span className="detail-value mono" style={{ color: 'var(--text-secondary)', fontSize: '12px' }}>
              {user.registration_ip || '—'}
            </span>

            <span className="detail-label">Last IP</span>
            <span className="detail-value mono" style={{ color: 'var(--text-secondary)', fontSize: '12px' }}>
              {user.last_seen_ip || '—'}
            </span>
          </div>

          {status === 'banned' && (
            <div style={{
              marginTop: '18px',
              padding: '14px',
              borderRadius: '8px',
              border: '1px solid rgba(248,113,113,0.25)',
              background: 'var(--red-soft)',
            }}>
              <div style={{ fontSize: '11px', fontWeight: '700', color: 'var(--red)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>
                Ban Record
              </div>
              <div className="detail-grid">
                <span className="detail-label">Banned at</span>
                <span className="detail-value" style={{ color: 'var(--red)' }}>{fmt(user.banned_at)}</span>
                <span className="detail-label">Reason</span>
                <span className="detail-value" style={{ color: 'var(--text)' }}>{user.ban_reason || '—'}</span>
              </div>
            </div>
          )}
        </div>

        {/* Actions + badges */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
          <div className="card">
            <div className="card-title">Actions</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
              {status === 'active' ? (
                <button className="btn btn-warning" style={{ justifyContent: 'center' }}
                  onClick={() => { setShowBanModal(true); setBanReason('') }}>
                  Ban user
                </button>
              ) : (
                <button className="btn" style={{ justifyContent: 'center' }}
                  onClick={handleUnban} disabled={actionLoading}>
                  Unban user
                </button>
              )}
              <button className="btn" style={{ justifyContent: 'center' }} onClick={handleForceLogout} disabled={actionLoading}>
                Force logout
              </button>
              <button className="btn" style={{ justifyContent: 'center' }} onClick={handleImpersonate}>
                Impersonate
              </button>
              <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '4px 0' }} />
              <button className="btn btn-danger" style={{ justifyContent: 'center' }} onClick={handleDelete}>
                Delete account
              </button>
            </div>

            <div style={{ marginTop: '16px', paddingTop: '14px', borderTop: '1px solid var(--border)' }}>
              <div style={{ fontSize: '11px', fontWeight: '600', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>
                Quick links
              </div>
              <Link to={`/messages?user_id=${user.id}`}
                style={{ fontSize: '13px', color: 'var(--accent)', display: 'block', padding: '4px 0' }}>
                View messages →
              </Link>
            </div>
          </div>

          <div className="card">
            <div className="card-title">Invites</div>
            <div style={{ fontSize: '13px', color: 'var(--text-secondary)', marginBottom: '10px' }}>
              Current balance: <strong style={{ color: 'var(--text)' }}>{user.invite_count ?? 0}</strong>
            </div>
            <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
              <input
                type="number"
                min={1}
                max={10}
                value={inviteCount}
                onChange={e => setInviteCount(Math.max(1, Math.min(10, Number(e.target.value) || 1)))}
                style={{ width: '72px' }}
                disabled={inviteLoading}
              />
              <button
                className="btn btn-primary"
                onClick={handleAddInvites}
                disabled={inviteLoading}
                style={{ flex: 1, justifyContent: 'center' }}
              >
                {inviteLoading ? 'Adding…' : `Add ${inviteCount}`}
              </button>
            </div>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '6px' }}>
              Max 10 per click.
            </div>
          </div>

          <div className="card">
            <div className="card-title">Badges</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
              {[
                { bit: 2, label: 'Parley Admin', color: 'var(--accent)' },
                { bit: 1, label: 'Donor', color: 'var(--yellow)' },
              ].map(({ bit, label, color }) => {
                const active = ((user.badges ?? 0) & bit) !== 0
                return (
                  <label key={bit} style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: '10px',
                    cursor: 'pointer',
                    padding: '8px 10px',
                    borderRadius: '7px',
                    background: active ? 'rgba(255,255,255,0.04)' : 'transparent',
                    border: `1px solid ${active ? 'rgba(255,255,255,0.1)' : 'transparent'}`,
                    transition: 'all 0.12s',
                  }}>
                    <input
                      type="checkbox"
                      checked={active}
                      disabled={badgesLoading}
                      onChange={() => handleToggleBadge(bit)}
                      style={{ accentColor: color, width: '15px', height: '15px' }}
                    />
                    <span style={{ fontSize: '13px', fontWeight: '500', color: active ? color : 'var(--text-secondary)' }}>
                      {label}
                    </span>
                  </label>
                )
              })}
              {badgesLoading && <span className="loading" style={{ fontSize: '12px' }}>Saving</span>}
            </div>
          </div>
        </div>
      </div>

      {/* Ban modal */}
      {showBanModal && (
        <Modal title={`Ban: ${user.username}`} onClose={() => setShowBanModal(false)} width={440}>
          <div style={{ marginBottom: '14px', fontSize: '13px', color: 'var(--text-secondary)' }}>
            This will revoke access for <strong style={{ color: 'var(--text)' }}>{user.username}</strong>.
          </div>
          <div className="form-group">
            <label>Ban reason</label>
            <textarea rows={4} value={banReason} onChange={e => setBanReason(e.target.value)} placeholder="Describe the reason…" />
          </div>
          <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
            <button className="btn" onClick={() => setShowBanModal(false)}>Cancel</button>
            <button className="btn btn-danger" onClick={handleBan} disabled={actionLoading || !banReason.trim()}>
              {actionLoading ? 'Banning…' : 'Confirm ban'}
            </button>
          </div>
        </Modal>
      )}
    </div>
  )
}
