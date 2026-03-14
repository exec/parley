import { useState, useEffect } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import {
  apiGetUser,
  apiBanUser,
  apiUnbanUser,
  apiForceLogout,
  apiImpersonateUser,
  apiDeleteUser,
  userStatus,
  User,
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

  // Ban modal
  const [showBanModal, setShowBanModal] = useState(false)
  const [banReason, setBanReason] = useState('')
  const [actionLoading, setActionLoading] = useState(false)

  useEffect(() => {
    if (!id) return
    setLoading(true)
    apiGetUser(Number(id))
      .then(setUser)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [id])

  const reload = () => {
    if (!id) return
    apiGetUser(Number(id)).then(setUser).catch(() => {})
  }

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
    } finally {
      setActionLoading(false)
    }
  }

  const handleUnban = async () => {
    if (!user) return
    if (!confirm(`Unban "${user.username}"?`)) return
    setActionLoading(true)
    try {
      await apiUnbanUser(user.id)
      setSuccess('User unbanned.')
      reload()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Unban failed')
    } finally {
      setActionLoading(false)
    }
  }

  const handleForceLogout = async () => {
    if (!user) return
    if (!confirm(`Force logout "${user.username}"?`)) return
    try {
      await apiForceLogout(user.id)
      setSuccess('Force logout sent.')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Force logout failed')
    }
  }

  const handleImpersonate = async () => {
    if (!user) return
    try {
      const { token } = await apiImpersonateUser(user.id)
      window.open(`https://parley.x86-64.com/impersonate?token=${token}`, '_blank')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Impersonate failed')
    }
  }

  const handleDelete = async () => {
    if (!user) return
    if (!confirm(`DELETE user "${user.username}"?\n\nThis is IRREVERSIBLE.`)) return
    try {
      await apiDeleteUser(user.id)
      navigate('/users')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Delete failed')
    }
  }

  const fmt = (d?: string | null) => d ? new Date(d).toLocaleString() : '—'

  if (loading) {
    return (
      <div style={{ padding: '40px', textAlign: 'center' }}>
        <span className="loading">LOADING USER DATA</span>
      </div>
    )
  }

  if (!user) {
    return (
      <div>
        <div className="alert alert-error">[ERROR] {error || 'User not found'}</div>
        <button className="btn" onClick={() => navigate('/users')}>
          &lt; BACK
        </button>
      </div>
    )
  }

  const status = userStatus(user)

  return (
    <div>
      {/* Header */}
      <div className="page-header" style={{ display: 'flex', alignItems: 'center', gap: '12px', justifyContent: 'space-between' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <button className="btn" onClick={() => navigate('/users')}>
            &lt; BACK
          </button>
          <h1 style={{ margin: 0 }}>{user.username}</h1>
          <StatusBadge status={status} />
        </div>
      </div>

      {error && (
        <div className="alert alert-error" onClick={() => setError('')} style={{ cursor: 'pointer', marginBottom: '12px' }}>
          [ERROR] {error}
        </div>
      )}
      {success && (
        <div className="alert alert-success" onClick={() => setSuccess('')} style={{ cursor: 'pointer', marginBottom: '12px' }}>
          [OK] {success}
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
        {/* User info panel */}
        <div className="panel">
          <div className="panel-title">// USER RECORD</div>
          <div className="detail-grid">
            <span className="detail-label">ID</span>
            <span className="detail-value" style={{ fontSize: '11px' }}>{user.id}</span>

            <span className="detail-label">Username</span>
            <span className="detail-value" style={{ color: 'var(--green)' }}>{user.username}</span>

            <span className="detail-label">Email</span>
            <span className="detail-value">{user.email || '—'}</span>

            <span className="detail-label">Email Verified</span>
            <span className="detail-value">{user.email_verified ? '[YES]' : '[NO]'}</span>

            <span className="detail-label">Phone</span>
            <span className="detail-value">{user.phone_number ?? '—'}</span>

            <span className="detail-label">Phone Verified</span>
            <span className="detail-value">{user.phone_verified ? '[YES]' : '[NO]'}</span>

            <span className="detail-label">Status</span>
            <span className="detail-value"><StatusBadge status={status} /></span>

            <span className="detail-label">System User</span>
            <span className="detail-value">{user.is_system ? '[YES]' : 'no'}</span>

            <span className="detail-label">Joined</span>
            <span className="detail-value">{fmt(user.created_at)}</span>
          </div>

          {status === 'banned' && (
            <div
              style={{
                marginTop: '16px',
                padding: '10px',
                border: '1px solid #5a1a1a',
                background: 'rgba(255,68,68,0.05)',
              }}
            >
              <div style={{ color: 'var(--red)', fontSize: '11px', textTransform: 'uppercase', letterSpacing: '0.1em', marginBottom: '6px' }}>
                // BAN RECORD
              </div>
              <div className="detail-grid">
                <span className="detail-label">Banned At</span>
                <span className="detail-value" style={{ color: 'var(--red)' }}>{fmt(user.banned_at)}</span>

                <span className="detail-label">Reason</span>
                <span className="detail-value" style={{ color: 'var(--red)' }}>{user.ban_reason || '—'}</span>
              </div>
            </div>
          )}
        </div>

        {/* Actions panel */}
        <div className="panel">
          <div className="panel-title">// ACTIONS</div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
            {status === 'active' ? (
              <button
                className="btn btn-warning"
                onClick={() => { setShowBanModal(true); setBanReason('') }}
              >
                [BAN USER]
              </button>
            ) : (
              <button className="btn" onClick={handleUnban} disabled={actionLoading}>
                [UNBAN USER]
              </button>
            )}
            <button className="btn" onClick={handleForceLogout} disabled={actionLoading}>
              [FORCE LOGOUT]
            </button>
            <button className="btn" onClick={handleImpersonate}>
              [IMPERSONATE]
            </button>
            <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '4px 0' }} />
            <button className="btn btn-danger" onClick={handleDelete}>
              [DELETE USER]
            </button>
          </div>

          {/* Link to messages */}
          <div style={{ marginTop: '20px' }}>
            <div className="panel-title">// QUICK LINKS</div>
            <Link
              to={`/messages?user_id=${user.id}`}
              style={{
                display: 'block',
                padding: '6px 0',
                fontSize: '12px',
                color: 'var(--green-dim)',
              }}
            >
              &gt; View messages by this user
            </Link>
          </div>
        </div>
      </div>

      {/* Ban modal */}
      {showBanModal && (
        <Modal title={`BAN: ${user.username}`} onClose={() => setShowBanModal(false)} width={440}>
          <div style={{ marginBottom: '12px', fontSize: '12px', color: 'var(--text-dim)' }}>
            Banning: <span style={{ color: 'var(--green)' }}>{user.username}</span>
          </div>
          <div className="form-group">
            <label>BAN REASON</label>
            <textarea
              rows={4}
              value={banReason}
              onChange={(e) => setBanReason(e.target.value)}
              placeholder="Enter reason..."
              style={{ resize: 'vertical' }}
            />
          </div>
          <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
            <button className="btn" onClick={() => setShowBanModal(false)}>CANCEL</button>
            <button
              className="btn btn-danger"
              onClick={handleBan}
              disabled={actionLoading || !banReason.trim()}
            >
              {actionLoading ? <span className="loading">BANNING</span> : '[CONFIRM BAN]'}
            </button>
          </div>
        </Modal>
      )}
    </div>
  )
}
