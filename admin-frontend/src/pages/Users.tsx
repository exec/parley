import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  apiGetUsers,
  apiDeleteUser,
  apiBanUser,
  apiUnbanUser,
  apiForceLogout,
  apiImpersonateUser,
  userStatus,
  User,
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

  // Modal state
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

  useEffect(() => {
    load(q, offset)
  }, [load, q, offset])

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault()
    setOffset(0)
    load(q, 0)
  }

  const handleDelete = async (u: User) => {
    if (!confirm(`DELETE user "${u.username}" (${u.id})?\n\nThis action is IRREVERSIBLE.`)) return
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

  const openBanModal = (u: User) => {
    setBanModal(u)
    setBanReason('')
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

  return (
    <div>
      <div className="page-header">
        <h1>USER MANAGEMENT</h1>
      </div>

      {error && (
        <div className="alert alert-error" onClick={() => setError('')} style={{ cursor: 'pointer' }}>
          [ERROR] {error}
        </div>
      )}
      {success && (
        <div className="alert alert-success" onClick={() => setSuccess('')} style={{ cursor: 'pointer' }}>
          [OK] {success}
        </div>
      )}

      {/* Search */}
      <form onSubmit={handleSearch} className="search-bar">
        <input
          type="search"
          placeholder="Search username / email / phone..."
          value={q}
          onChange={(e) => setQ(e.target.value)}
          style={{ maxWidth: '360px' }}
        />
        <button type="submit" className="btn btn-primary">
          [SEARCH]
        </button>
        {q && (
          <button
            type="button"
            className="btn"
            onClick={() => { setQ(''); setOffset(0) }}
          >
            [CLEAR]
          </button>
        )}
        <span style={{ fontSize: '11px', color: 'var(--text-dim)', marginLeft: '8px' }}>
          {users.length} result{users.length !== 1 ? 's' : ''}
        </span>
      </form>

      {/* Table */}
      <div style={{ overflowX: 'auto' }}>
        {loading ? (
          <div style={{ padding: '32px', textAlign: 'center' }}>
            <span className="loading">LOADING</span>
          </div>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>ID</th>
                <th>USERNAME</th>
                <th>EMAIL</th>
                <th>PHONE</th>
                <th>STATUS</th>
                <th>JOINED</th>
                <th>ACTIONS</th>
              </tr>
            </thead>
            <tbody>
              {users.length === 0 ? (
                <tr>
                  <td colSpan={7} style={{ textAlign: 'center', padding: '32px', color: 'var(--text-dim)' }}>
                    [NO USERS FOUND]
                  </td>
                </tr>
              ) : users.map((u) => (
                <tr key={u.id}>
                  <td style={{ fontSize: '11px', color: 'var(--text-dim)', fontFamily: 'var(--font)' }}>
                    {u.id}
                  </td>
                  <td style={{ color: 'var(--green)' }}>{u.username}</td>
                  <td style={{ fontSize: '11px' }}>{u.email || '—'}</td>
                  <td style={{ fontSize: '11px' }}>{u.phone_number ?? '—'}</td>
                  <td><StatusBadge status={userStatus(u)} /></td>
                  <td style={{ fontSize: '11px', color: 'var(--text-dim)' }}>
                    {new Date(u.created_at).toLocaleDateString()}
                  </td>
                  <td>
                    <div className="actions">
                      <button
                        className="btn"
                        onClick={() => navigate(`/users/${u.id}`)}
                        title="View detail"
                      >
                        VIEW
                      </button>
                      {userStatus(u) === 'active' ? (
                        <button
                          className="btn btn-warning"
                          onClick={() => openBanModal(u)}
                        >
                          BAN
                        </button>
                      ) : (
                        <button
                          className="btn"
                          onClick={async () => {
                            try {
                              await apiUnbanUser(u.id)
                              setSuccess(`User ${u.username} unbanned.`)
                              load(q, offset)
                            } catch (e) {
                              setError(e instanceof Error ? e.message : 'Unban failed')
                            }
                          }}
                        >
                          UNBAN
                        </button>
                      )}
                      <button
                        className="btn"
                        onClick={() => handleForceLogout(u)}
                        title="Force logout"
                      >
                        F-LOGOUT
                      </button>
                      <button
                        className="btn"
                        onClick={() => handleImpersonate(u)}
                        title="Impersonate user"
                      >
                        IMPERSONATE
                      </button>
                      <button
                        className="btn btn-danger"
                        onClick={() => handleDelete(u)}
                        title="Delete user"
                      >
                        DELETE
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Pagination */}
      <div className="pagination">
        <button
          className="btn"
          disabled={currentPage === 1}
          onClick={() => setOffset(offset - PAGE_SIZE)}
        >
          &lt; PREV
        </button>
        <span>PAGE {currentPage}</span>
        <button
          className="btn"
          disabled={!hasMore}
          onClick={() => setOffset(offset + PAGE_SIZE)}
        >
          NEXT &gt;
        </button>
      </div>

      {/* Ban modal */}
      {banModal && (
        <Modal title={`BAN USER: ${banModal.username}`} onClose={() => setBanModal(null)} width={440}>
          <div style={{ marginBottom: '16px', fontSize: '12px', color: 'var(--text-dim)' }}>
            User: <span style={{ color: 'var(--green)' }}>{banModal.username}</span>
            <br />
            ID: <span style={{ color: 'var(--text-dim)', fontSize: '11px' }}>{banModal.id}</span>
          </div>
          <div className="form-group">
            <label>BAN REASON</label>
            <textarea
              rows={4}
              value={banReason}
              onChange={(e) => setBanReason(e.target.value)}
              placeholder="Enter reason for ban..."
              style={{ resize: 'vertical' }}
            />
          </div>
          <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
            <button className="btn" onClick={() => setBanModal(null)}>
              CANCEL
            </button>
            <button
              className="btn btn-danger"
              onClick={handleBan}
              disabled={banLoading || !banReason.trim()}
            >
              {banLoading ? <span className="loading">BANNING</span> : '[CONFIRM BAN]'}
            </button>
          </div>
        </Modal>
      )}
    </div>
  )
}
