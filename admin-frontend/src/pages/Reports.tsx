import { useState, useEffect, useCallback } from 'react'
import {
  apiGetReports, apiGetReport, apiDeleteMessage, apiResolveReport,
  apiBanUser, Report, Message, ReportDetailResponse,
} from '../api'
import StatusBadge from '../components/StatusBadge'
import Modal from '../components/Modal'

const STATUS_TABS = ['ALL', 'OPEN', 'RESOLVED', 'DISMISSED'] as const
type StatusFilter = (typeof STATUS_TABS)[number]
const PAGE_SIZE = 50

function ReportDetail({ reportId, onUpdated }: { reportId: number; onClose?: () => void; onUpdated: () => void }) {
  const [detail, setDetail] = useState<ReportDetailResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [note, setNote] = useState('')
  const [resolving, setResolving] = useState(false)
  const [status, setStatus] = useState('')
  const [banReason, setBanReason] = useState('')
  const [showBanForm, setShowBanForm] = useState(false)

  const loadDetail = useCallback(() => {
    setLoading(true)
    apiGetReport(reportId).then(setDetail).catch(e => setError(e.message)).finally(() => setLoading(false))
  }, [reportId])

  useEffect(() => { loadDetail() }, [loadDetail])

  const handleResolve = async (s: 'resolved' | 'dismissed') => {
    if (!detail) return
    setResolving(true)
    try {
      await apiResolveReport(detail.report.id, s, note)
      setStatus(`Report marked as ${s}.`)
      setDetail(d => d ? { ...d, report: { ...d.report, status: s } } : d)
      onUpdated()
    } catch (e) { setStatus(`Error: ${e instanceof Error ? e.message : 'Failed'}`) }
    finally { setResolving(false) }
  }

  const handleDeleteMessage = async (msgId: number) => {
    if (!confirm('Delete this message?')) return
    try { await apiDeleteMessage(msgId); setStatus('Message deleted.'); loadDetail() }
    catch (e) { setStatus(`Error: ${e instanceof Error ? e.message : 'Delete failed'}`) }
  }

  const handleBan = async () => {
    if (!detail?.report.reported_user_id) return
    try {
      await apiBanUser(detail.report.reported_user_id, banReason)
      setStatus(`User ${detail.report.reported_username} banned.`)
      setShowBanForm(false)
      onUpdated()
    } catch (e) { setStatus(`Error: ${e instanceof Error ? e.message : 'Ban failed'}`) }
  }

  const fmt = (d?: string) => d ? new Date(d).toLocaleString('en-US', { dateStyle: 'medium', timeStyle: 'short' }) : '—'

  if (loading) return (
    <div style={{ textAlign: 'center', padding: '40px', color: 'var(--text-secondary)' }}>
      <div className="loading-spinner" style={{ margin: '0 auto 10px' }} />Loading…
    </div>
  )
  if (!detail) return <div className="alert alert-error">{error || 'Report not found'}</div>

  const { report, context, target_message_id } = detail
  const isOpen = report.status === 'open'

  return (
    <div>
      {status && (
        <div className={`alert ${status.startsWith('Error') ? 'alert-error' : 'alert-success'}`} style={{ marginBottom: '14px' }}>
          {status}
        </div>
      )}

      {/* Metadata */}
      <div className="detail-grid" style={{ marginBottom: '18px' }}>
        <span className="detail-label">Report ID</span>
        <span className="detail-value mono" style={{ color: 'var(--text-muted)' }}>{report.id}</span>
        <span className="detail-label">Category</span>
        <span className="detail-value" style={{ color: 'var(--yellow)', fontWeight: '600' }}>{report.category_name}</span>
        <span className="detail-label">Status</span>
        <span className="detail-value"><StatusBadge status={report.status} /></span>
        <span className="detail-label">Reporter</span>
        <span className="detail-value">{report.reporter_username || '—'}</span>
        <span className="detail-label">Reported user</span>
        <span className="detail-value" style={{ color: isOpen ? 'var(--red)' : undefined, fontWeight: isOpen ? '600' : undefined }}>
          {report.reported_username || '—'}
        </span>
        <span className="detail-label">Description</span>
        <span className="detail-value">{report.description || '—'}</span>
        <span className="detail-label">Created</span>
        <span className="detail-value mono" style={{ color: 'var(--text-secondary)' }}>{fmt(report.created_at)}</span>
        {report.resolution_note && (
          <>
            <span className="detail-label">Resolution note</span>
            <span className="detail-value">{report.resolution_note}</span>
          </>
        )}
      </div>

      {/* Actions */}
      {isOpen && (
        <div style={{
          padding: '16px',
          borderRadius: '8px',
          border: '1px solid var(--border)',
          background: 'var(--bg-elevated)',
          marginBottom: '18px',
        }}>
          <div style={{ fontSize: '11px', fontWeight: '600', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '10px' }}>
            Resolution note (optional)
          </div>
          <textarea
            rows={2}
            value={note}
            onChange={e => setNote(e.target.value)}
            placeholder="Add a note before resolving…"
            style={{ marginBottom: '10px' }}
          />
          <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap' }}>
            <button className="btn btn-success" onClick={() => handleResolve('resolved')} disabled={resolving}>
              Resolve
            </button>
            <button className="btn" onClick={() => handleResolve('dismissed')} disabled={resolving}>
              Dismiss
            </button>
            {report.reported_user_id && (
              <button className="btn btn-warning" onClick={() => { setShowBanForm(!showBanForm); setBanReason('') }}>
                Ban reported user
              </button>
            )}
            {report.reported_message_id && (
              <button className="btn btn-danger"
                onClick={() => report.reported_message_id && handleDeleteMessage(report.reported_message_id)}>
                Delete message
              </button>
            )}
          </div>

          {showBanForm && report.reported_user_id && (
            <div style={{
              marginTop: '12px',
              padding: '12px',
              borderRadius: '7px',
              border: '1px solid rgba(248,113,113,0.25)',
              background: 'var(--red-soft)',
            }}>
              <div style={{ fontSize: '12px', fontWeight: '600', color: 'var(--red)', marginBottom: '8px' }}>
                Ban: {report.reported_username}
              </div>
              <input
                type="text"
                placeholder="Ban reason…"
                value={banReason}
                onChange={e => setBanReason(e.target.value)}
                style={{ marginBottom: '8px' }}
              />
              <div style={{ display: 'flex', gap: '6px' }}>
                <button className="btn" onClick={() => setShowBanForm(false)}>Cancel</button>
                <button className="btn btn-danger" onClick={handleBan} disabled={!banReason.trim()}>Confirm ban</button>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Message context */}
      {context && context.length > 0 && (
        <div>
          <div style={{ fontSize: '11px', fontWeight: '600', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>
            Message context
          </div>
          <div className="msg-context-list">
            {context.map((msg: Message) => {
              const isTarget = msg.id === target_message_id
              return (
                <div key={msg.id} className={`msg-context-item${isTarget ? ' target' : ''}`}>
                  <div className="msg-meta">
                    <div style={{ fontWeight: isTarget ? '600' : '400', color: isTarget ? 'var(--accent)' : undefined }}>
                      {msg.author_username}
                    </div>
                    <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--mono)' }}>
                      {new Date(msg.created_at).toLocaleTimeString()}
                    </div>
                  </div>
                  <div className="msg-body">{msg.content}</div>
                  <button className="btn btn-danger" style={{ fontSize: '11px', padding: '3px 8px' }}
                    onClick={() => handleDeleteMessage(msg.id)}>
                    Delete
                  </button>
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}

export default function Reports() {
  const [reports, setReports] = useState<Report[]>([])
  const [loading, setLoading] = useState(false)
  const [hasMore, setHasMore] = useState(false)
  const [error, setError] = useState('')
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('OPEN')
  const [offset, setOffset] = useState(0)
  const [selectedId, setSelectedId] = useState<number | null>(null)

  const load = useCallback(async (filter: StatusFilter, off: number) => {
    setLoading(true)
    setError('')
    try {
      const statusParam = filter === 'ALL' ? undefined : filter.toLowerCase()
      const res = await apiGetReports(statusParam, PAGE_SIZE, off)
      setReports(res ?? [])
      setHasMore((res ?? []).length === PAGE_SIZE)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load reports')
    } finally { setLoading(false) }
  }, [])

  useEffect(() => { load(statusFilter, offset) }, [load, statusFilter, offset])

  const handleTabChange = (tab: StatusFilter) => { setStatusFilter(tab); setOffset(0); load(tab, 0) }
  const currentPage = Math.floor(offset / PAGE_SIZE) + 1
  const fmt = (d: string) => new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })

  return (
    <div>
      <div className="page-header">
        <h1>Reports</h1>
        <p style={{ color: 'var(--text-secondary)', fontSize: '14px', marginTop: '4px' }}>Review and action user-submitted reports</p>
      </div>

      {error && <div className="alert alert-error" onClick={() => setError('')} style={{ cursor: 'pointer' }}>{error}</div>}

      {/* Status tabs */}
      <div className="tab-bar">
        {STATUS_TABS.map(tab => (
          <button key={tab} className={`tab-item${statusFilter === tab ? ' active' : ''}`} onClick={() => handleTabChange(tab)}>
            {tab.charAt(0) + tab.slice(1).toLowerCase()}
          </button>
        ))}
        <span style={{ marginLeft: 'auto', alignSelf: 'center', fontSize: '12px', color: 'var(--text-muted)', paddingRight: '4px' }}>
          {reports.length} result{reports.length !== 1 ? 's' : ''}
        </span>
      </div>

      {/* Table */}
      <div className="table-card">
        {loading ? (
          <div style={{ padding: '48px', textAlign: 'center', color: 'var(--text-secondary)' }}>
            <div className="loading-spinner" style={{ margin: '0 auto 10px' }} />Loading reports…
          </div>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table className="data-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Category</th>
                  <th>Reporter</th>
                  <th>Reported user</th>
                  <th>Status</th>
                  <th>Date</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {reports.length === 0 ? (
                  <tr><td colSpan={7} style={{ textAlign: 'center', padding: '48px', color: 'var(--text-muted)' }}>No reports found</td></tr>
                ) : reports.map(r => (
                  <tr key={r.id} style={{ cursor: 'pointer' }} onClick={() => setSelectedId(r.id)}>
                    <td><span className="mono" style={{ color: 'var(--text-muted)' }}>{r.id}</span></td>
                    <td><span style={{ color: 'var(--yellow)', fontWeight: '500' }}>{r.category_name}</span></td>
                    <td>{r.reporter_username || '—'}</td>
                    <td style={{ fontWeight: r.status === 'open' ? '600' : undefined, color: r.status === 'open' ? 'var(--red)' : undefined }}>
                      {r.reported_username || '—'}
                    </td>
                    <td><StatusBadge status={r.status} /></td>
                    <td><span className="mono" style={{ color: 'var(--text-muted)' }}>{fmt(r.created_at)}</span></td>
                    <td onClick={e => e.stopPropagation()}>
                      <button className="btn" onClick={() => setSelectedId(r.id)}>Review</button>
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

      {selectedId !== null && (
        <Modal title="Report Detail" onClose={() => setSelectedId(null)} width={760}>
          <ReportDetail reportId={selectedId} onClose={() => setSelectedId(null)} onUpdated={() => load(statusFilter, offset)} />
        </Modal>
      )}
    </div>
  )
}
