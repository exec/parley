import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  apiGetUsers, apiDeleteUser, apiBanUser, apiUnbanUser,
  apiForceLogout, apiImpersonateUser, userStatus, User,
} from '../api'
import StatusBadge from '../components/StatusBadge'
import Modal from '../components/Modal'

const PAGE_SIZE = 50

export default function Users() {
  const navigate = useNavigate()
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [q, setQ] = useState('')
  const [offset, setOffset] = useState(0)
  const [hasMore, setHasMore] = useState(false)
  const [banModal, setBanModal] = useState<User | null>(null)
  const [banReason, setBanReason] = useState('')
  const [banLoading, setBanLoading] = useState(false)

  const load = useCallback(async (query: string, off: number) => {
    setLoading(true)
    setError('')
    try {
      const res = await apiGetUsers(query, PAGE_SIZE, off)
      setUsers(res ?? [])
      setHasMore((res ?? []).length === PAGE_SIZE)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load users')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load(q, offset) }, [load, q, offset])

  const handleSearch = (e: React.FormEvent) => { e.preventDefault(); setOffset(0); load(q, 0) }

  const handleDelete = async (u: User) => {
    if (!confirm(`Delete user "${u.username}"? This is irreversible.`)) return
    try {
      await apiDeleteUser(u.id)
      setSuccess(`User ${u.username} deleted.`)
      load(q, offset)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Delete failed')
    }
  }

  const handleForceLogout = async (u: User) => {
    if (!confirm(`Force logout "${u.username}"?`)) return
    try {
      await apiForceLogout(u.id)
      setSuccess(`Force logout sent to ${u.username}.`)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Force logout failed')
    }
  }

  const handleImpersonate = async (u: User) => {
    try {
      const { token } = await apiImpersonateUser(u.id)
      window.open(`https://parley.x86-64.com/impersonate?token=${token}`, '_blank')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Impersonate failed')
    }
  }

  const handleBan = async () => {
    if (!banModal) return
    setBanLoading(true)
    try {
      await apiBanUser(banModal.id, banReason)
      setSuccess(`User ${banModal.username} banned.`)
      setBanModal(null)
      load(q, offset)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Ban failed')
    } finally {
      setBanLoading(false)
    }
  }

  const currentPage = Math.floor(offset / PAGE_SIZE) + 1
  const fmt = (d: string) => new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })

  return (
    <div>
      <div className="page-header">
        <h1>Users</h1>
        <p style={{ color: 'var(--text-secondary)', fontSize: '14px', marginTop: '4px' }}>
          Search, view and manage platform accounts
        </p>
      </div>

      {error && <div className="alert alert-error" onClick={() => setError('')} style={{ cursor: 'pointer' }}>{error}</div>}
      {success && <div className="alert alert-success" onClick={() => setSuccess('')} style={{ cursor: 'pointer' }}>{success}</div>}

      {/* Search */}
      <form onSubmit={handleSearch} className="search-bar">
        <input
          type="search"
          placeholder="Search by username, email or phone…"
          value={q}
          onChange={e => setQ(e.target.value)}
          style={{ maxWidth: '380px' }}
        />
        <button type="submit" className="btn btn-primary">Search</button>
        {q && (
          <button type="button" className="btn" onClick={() => { setQ(''); setOffset(0) }}>
            Clear
          </button>
        )}
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', marginLeft: '4px' }}>
          {users.length} result{users.length !== 1 ? 's' : ''}
        </span>
      </form>

      {/* Table */}
      <div className="table-card">
        {loading ? (
          <div style={{ padding: '48px', textAlign: 'center', color: 'var(--text-secondary)' }}>
            <div className="loading-spinner" style={{ margin: '0 auto 10px' }} />
            <div>Loading users…</div>
          </div>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table className="data-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Username</th>
                  <th>Email</th>
                  <th>Phone</th>
                  <th>Status</th>
                  <th>Joined</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {users.length === 0 ? (
                  <tr>
                    <td colSpan={7} style={{ textAlign: 'center', padding: '48px', color: 'var(--text-muted)' }}>
                      No users found
                    </td>
                  </tr>
                ) : users.map(u => (
                  <tr key={u.id}>
                    <td><span className="mono" style={{ color: 'var(--text-muted)' }}>{u.id}</span></td>
                    <td style={{ fontWeight: '600', color: 'var(--text)' }}>{u.username}</td>
                    <td>{u.email || <span style={{ color: 'var(--text-muted)' }}>—</span>}</td>
                    <td>{u.phone_number ?? <span style={{ color: 'var(--text-muted)' }}>—</span>}</td>
                    <td><StatusBadge status={userStatus(u)} /></td>
                    <td><span className="mono" style={{ color: 'var(--text-muted)' }}>{fmt(u.created_at)}</span></td>
                    <td>
                      <div className="actions">
                        <button className="btn" onClick={() => navigate(`/users/${u.id}`)}>View</button>
                        {userStatus(u) === 'active' ? (
                          <button className="btn btn-warning" onClick={() => { setBanModal(u); setBanReason('') }}>Ban</button>
                        ) : (
                          <button className="btn" onClick={async () => {
                            try { await apiUnbanUser(u.id); setSuccess(`${u.username} unbanned.`); load(q, offset) }
                            catch (e) { setError(e instanceof Error ? e.message : 'Unban failed') }
                          }}>Unban</button>
                        )}
                        <button className="btn" onClick={() => handleForceLogout(u)}>Force logout</button>
                        <button className="btn" onClick={() => handleImpersonate(u)}>Impersonate</button>
                        <button className="btn btn-danger" onClick={() => handleDelete(u)}>Delete</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Pagination */}
      <div className="pagination">
        <button className="btn" disabled={currentPage === 1} onClick={() => setOffset(offset - PAGE_SIZE)}>
          ← Prev
        </button>
        <span>Page {currentPage}</span>
        <button className="btn" disabled={!hasMore} onClick={() => setOffset(offset + PAGE_SIZE)}>
          Next →
        </button>
      </div>

      {/* Ban modal */}
      {banModal && (
        <Modal title={`Ban user: ${banModal.username}`} onClose={() => setBanModal(null)} width={440}>
          <div style={{ marginBottom: '16px', fontSize: '13px', color: 'var(--text-secondary)' }}>
            This will immediately revoke access for <strong style={{ color: 'var(--text)' }}>{banModal.username}</strong>.
          </div>
          <div className="form-group">
            <label>Ban reason</label>
            <textarea
              rows={4}
              value={banReason}
              onChange={e => setBanReason(e.target.value)}
              placeholder="Describe the reason for this ban…"
            />
          </div>
          <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
            <button className="btn" onClick={() => setBanModal(null)}>Cancel</button>
            <button
              className="btn btn-danger"
              onClick={handleBan}
              disabled={banLoading || !banReason.trim()}
            >
              {banLoading ? 'Banning…' : 'Confirm ban'}
            </button>
          </div>
        </Modal>
      )}
    </div>
  )
}
