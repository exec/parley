import { useState, useEffect, useCallback } from 'react'
import { apiGetServers, apiDisbandServer, apiGenerateInvite, Server } from '../api'
import { SITE_URL } from '../config'

const PAGE_SIZE = 50

export default function Servers() {
  const [servers, setServers] = useState<Server[]>([])
  const [loading, setLoading] = useState(false)
  const [hasMore, setHasMore] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [q, setQ] = useState('')
  const [offset, setOffset] = useState(0)
  const [inviteCode, setInviteCode] = useState<{ serverId: number; code: string } | null>(null)

  const load = useCallback(async (query: string, off: number) => {
    setLoading(true)
    setError('')
    try {
      const res = await apiGetServers(query, PAGE_SIZE, off)
      setServers(res ?? [])
      setHasMore((res ?? []).length === PAGE_SIZE)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load servers')
    } finally { setLoading(false) }
  }, [])

  useEffect(() => { load(q, offset) }, [load, q, offset])

  const handleSearch = (e: React.FormEvent) => { e.preventDefault(); setOffset(0); load(q, 0) }

  const handleGenerateInvite = async (s: Server) => {
    try {
      const res = await apiGenerateInvite(s.id)
      setInviteCode({ serverId: s.id, code: res.code })
    } catch (e) { setError(e instanceof Error ? e.message : 'Failed to generate invite') }
  }

  const handleDisband = async (s: Server) => {
    if (!confirm(`Disband server "${s.name}"?\n\nThis permanently deletes all channels and messages. This is IRREVERSIBLE.`)) return
    try {
      const res = await apiDisbandServer(s.id)
      setSuccess(`"${s.name}" disbanded. ${res.members_notified} member(s) notified.`)
      load(q, offset)
    } catch (e) { setError(e instanceof Error ? e.message : 'Disband failed') }
  }

  const currentPage = Math.floor(offset / PAGE_SIZE) + 1
  const fmt = (d: string) => new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
  const inviteURL = inviteCode ? `${SITE_URL}/invite/${inviteCode.code}` : ''

  return (
    <div>
      <div className="page-header">
        <h1>Servers</h1>
        <p style={{ color: 'var(--text-secondary)', fontSize: '14px', marginTop: '4px' }}>Browse and manage platform servers</p>
      </div>

      {error && <div className="alert alert-error" onClick={() => setError('')} style={{ cursor: 'pointer' }}>{error}</div>}
      {success && <div className="alert alert-success" onClick={() => setSuccess('')} style={{ cursor: 'pointer' }}>{success}</div>}

      {inviteCode && (
        <div className="alert alert-info" style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap' }}>
          <span style={{ fontWeight: '500' }}>Invite link generated:</span>
          <code style={{
            flex: 1,
            background: 'rgba(0,0,0,0.2)',
            padding: '4px 10px',
            borderRadius: '5px',
            fontFamily: 'var(--mono)',
            fontSize: '12px',
            userSelect: 'all',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}>
            {inviteURL}
          </code>
          <button className="btn btn-primary" onClick={() => navigator.clipboard.writeText(inviteURL)}>Copy</button>
          <button className="btn" onClick={() => setInviteCode(null)}>Dismiss</button>
        </div>
      )}

      {/* Search */}
      <form onSubmit={handleSearch} className="search-bar">
        <input type="search" placeholder="Search server name…" value={q} onChange={e => setQ(e.target.value)} style={{ maxWidth: '360px' }} />
        <button type="submit" className="btn btn-primary">Search</button>
        {q && <button type="button" className="btn" onClick={() => { setQ(''); setOffset(0) }}>Clear</button>}
        <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
          {servers.length} server{servers.length !== 1 ? 's' : ''}
        </span>
      </form>

      <div className="table-card">
        {loading ? (
          <div style={{ padding: '48px', textAlign: 'center', color: 'var(--text-secondary)' }}>
            <div className="loading-spinner" style={{ margin: '0 auto 10px' }} />Loading servers…
          </div>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table className="data-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Name</th>
                  <th>Owner ID</th>
                  <th>Created</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {servers.length === 0 ? (
                  <tr><td colSpan={5} style={{ textAlign: 'center', padding: '48px', color: 'var(--text-muted)' }}>No servers found</td></tr>
                ) : servers.map(s => (
                  <tr key={s.id}>
                    <td><span className="mono" style={{ color: 'var(--text-muted)' }}>{s.id}</span></td>
                    <td style={{ fontWeight: '600', color: 'var(--text)' }}>{s.name}</td>
                    <td><span className="mono" style={{ color: 'var(--text-muted)' }}>{s.owner_id}</span></td>
                    <td><span className="mono" style={{ color: 'var(--text-muted)' }}>{fmt(s.created_at)}</span></td>
                    <td>
                      <div className="actions">
                        <button className="btn btn-primary" onClick={() => handleGenerateInvite(s)}>Generate invite</button>
                        <button className="btn btn-danger" onClick={() => handleDisband(s)}>Disband</button>
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
