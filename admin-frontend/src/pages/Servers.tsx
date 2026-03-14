import { useState, useEffect, useCallback } from 'react'
import { apiGetServers, apiDisbandServer, Server } from '../api'

const PAGE_SIZE = 50

export default function Servers() {
  const [servers, setServers] = useState<Server[]>([])
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
      const res = await apiGetServers(query, PAGE_SIZE, off)
      setServers(res ?? [])
      setHasMore((res ?? []).length === PAGE_SIZE)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load servers')
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

  const handleDisband = async (s: Server) => {
    if (
      !confirm(
        `DISBAND server "${s.name}"?\n\nThis will permanently delete the server and all its channels and messages.\n\nThis is IRREVERSIBLE.`
      )
    )
      return
    try {
      const res = await apiDisbandServer(s.id)
      setSuccess(`Server "${s.name}" disbanded. ${res.members_notified} member(s) notified.`)
      load(q, offset)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Disband failed')
    }
  }

  const currentPage = Math.floor(offset / PAGE_SIZE) + 1

  return (
    <div>
      <div className="page-header">
        <h1>SERVER MANAGEMENT</h1>
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
          placeholder="Search server name..."
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
        <span style={{ fontSize: '11px', color: 'var(--text-dim)' }}>
          {servers.length} server{servers.length !== 1 ? 's' : ''}
        </span>
      </form>

      {/* Table */}
      {loading ? (
        <div style={{ padding: '32px', textAlign: 'center' }}>
          <span className="loading">LOADING</span>
        </div>
      ) : (
        <div style={{ overflowX: 'auto' }}>
          <table className="data-table">
            <thead>
              <tr>
                <th>ID</th>
                <th>NAME</th>
                <th>OWNER ID</th>
                <th>CREATED</th>
                <th>ACTIONS</th>
              </tr>
            </thead>
            <tbody>
              {servers.length === 0 ? (
                <tr>
                  <td colSpan={5} style={{ textAlign: 'center', padding: '32px', color: 'var(--text-dim)' }}>
                    [NO SERVERS FOUND]
                  </td>
                </tr>
              ) : servers.map((s) => (
                <tr key={s.id}>
                  <td style={{ fontSize: '11px', color: 'var(--text-dim)' }}>
                    {s.id}
                  </td>
                  <td style={{ color: 'var(--green)', fontWeight: 'bold' }}>{s.name}</td>
                  <td style={{ fontSize: '11px', color: 'var(--text-dim)' }}>{s.owner_id}</td>
                  <td style={{ fontSize: '11px', color: 'var(--text-dim)', whiteSpace: 'nowrap' }}>
                    {new Date(s.created_at).toLocaleDateString()}
                  </td>
                  <td>
                    <button
                      className="btn btn-danger"
                      onClick={() => handleDisband(s)}
                    >
                      DISBAND
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

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
    </div>
  )
}
