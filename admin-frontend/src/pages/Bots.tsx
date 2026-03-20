import { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { apiGetBots, apiBanUser, apiUnbanUser, Bot } from '../api'

const PAGE_SIZE = 50

export default function Bots() {
  const [bots, setBots] = useState<Bot[]>([])
  const [loading, setLoading] = useState(false)
  const [hasMore, setHasMore] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [q, setQ] = useState('')
  const [offset, setOffset] = useState(0)

  const load = useCallback(async (query: string, off: number) => {
    setLoading(true)
    setError('')
    try {
      const res = await apiGetBots(query || undefined, PAGE_SIZE, off)
      setBots(res ?? [])
      setHasMore((res ?? []).length === PAGE_SIZE)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load bots')
    } finally { setLoading(false) }
  }, [])

  useEffect(() => { load(q, offset) }, [load, q, offset])

  const handleSearch = (e: React.FormEvent) => { e.preventDefault(); setOffset(0); load(q, 0) }

  const handleBan = async (bot: Bot) => {
    if (!confirm(`Ban bot "${bot.username}"?`)) return
    try {
      await apiBanUser(bot.id, 'Admin action')
      setSuccess(`"${bot.username}" banned.`)
      load(q, offset)
    } catch (e) { setError(e instanceof Error ? e.message : 'Ban failed') }
  }

  const handleUnban = async (bot: Bot) => {
    if (!confirm(`Unban bot "${bot.username}"?`)) return
    try {
      await apiUnbanUser(bot.id)
      setSuccess(`"${bot.username}" unbanned.`)
      load(q, offset)
    } catch (e) { setError(e instanceof Error ? e.message : 'Unban failed') }
  }

  const currentPage = Math.floor(offset / PAGE_SIZE) + 1
  const fmt = (d: string) => new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })

  return (
    <div>
      <div className="page-header">
        <h1>Bots</h1>
        <p style={{ color: 'var(--text-secondary)', fontSize: '14px', marginTop: '4px' }}>Registered bot accounts on the platform</p>
      </div>

      {error && <div className="alert alert-error" onClick={() => setError('')} style={{ cursor: 'pointer' }}>{error}</div>}
      {success && <div className="alert alert-success" onClick={() => setSuccess('')} style={{ cursor: 'pointer' }}>{success}</div>}

      <form onSubmit={handleSearch} className="search-bar">
        <input
          type="search"
          placeholder="Search bot name or ID…"
          value={q}
          onChange={e => setQ(e.target.value)}
          style={{ maxWidth: '360px' }}
        />
        <button type="submit" className="btn btn-primary">Search</button>
        {q && (
          <button type="button" className="btn" onClick={() => { setQ(''); setOffset(0) }}>Clear</button>
        )}
        <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
          {bots.length} bot{bots.length !== 1 ? 's' : ''}
        </span>
      </form>

      <div className="table-card">
        {loading ? (
          <div style={{ padding: '48px', textAlign: 'center', color: 'var(--text-secondary)' }}>
            <div className="loading-spinner" style={{ margin: '0 auto 10px' }} />Loading bots…
          </div>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table className="data-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Username</th>
                  <th>Owner</th>
                  <th>Status</th>
                  <th>Created</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {bots.length === 0 ? (
                  <tr>
                    <td colSpan={6} style={{ textAlign: 'center', padding: '48px', color: 'var(--text-muted)' }}>
                      No bots found
                    </td>
                  </tr>
                ) : bots.map(bot => (
                  <tr key={bot.id}>
                    <td><span className="mono" style={{ color: 'var(--text-muted)' }}>{bot.id}</span></td>
                    <td>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                        <span style={{ fontWeight: '600', color: 'var(--text)' }}>{bot.username}</span>
                        <span className="badge badge-bot">BOT</span>
                      </div>
                    </td>
                    <td>
                      {bot.owner_id ? (
                        <Link
                          to={`/users/${bot.owner_id}`}
                          style={{ color: 'var(--accent)', fontSize: '13px' }}
                        >
                          {bot.owner_username || `#${bot.owner_id}`}
                        </Link>
                      ) : (
                        <span style={{ color: 'var(--text-muted)', fontSize: '13px' }}>—</span>
                      )}
                    </td>
                    <td>
                      {bot.banned_at
                        ? <span className="badge badge-banned">Banned</span>
                        : <span className="badge badge-active">Active</span>}
                    </td>
                    <td>
                      <span className="mono" style={{ color: 'var(--text-muted)', fontSize: '12px' }}>
                        {fmt(bot.created_at)}
                      </span>
                    </td>
                    <td>
                      <div className="actions">
                        <Link to={`/users/${bot.id}`} className="btn">View</Link>
                        {bot.banned_at ? (
                          <button className="btn" onClick={() => handleUnban(bot)}>Unban</button>
                        ) : (
                          <button className="btn btn-warning" onClick={() => handleBan(bot)}>Ban</button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="pagination">
        <button className="btn" disabled={currentPage === 1} onClick={() => setOffset(offset - PAGE_SIZE)}>← Prev</button>
        <span>Page {currentPage}</span>
        <button className="btn" disabled={!hasMore} onClick={() => setOffset(offset + PAGE_SIZE)}>Next →</button>
      </div>
    </div>
  )
}
