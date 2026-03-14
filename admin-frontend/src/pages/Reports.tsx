import { useState, useEffect, useCallback } from 'react'
import {
  apiGetReports,
  apiGetReport,
  apiDeleteMessage,
  apiResolveReport,
  apiBanUser,
  Report,
  Message,
  ReportDetailResponse,
} from '../api'
import StatusBadge from '../components/StatusBadge'
import Modal from '../components/Modal'

const STATUS_TABS = ['ALL', 'OPEN', 'RESOLVED', 'DISMISSED'] as const
type StatusFilter = (typeof STATUS_TABS)[number]

const PAGE_SIZE = 50

function ReportDetail({
  reportId,
  onUpdated,
}: {
  reportId: number
  onClose?: () => void
  onUpdated: () => void
}) {
  const [detail, setDetail] = useState<ReportDetailResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [status, setStatus] = useState('')
  const [note, setNote] = useState('')
  const [resolving, setResolving] = useState(false)
  const [banReason, setBanReason] = useState('')
  const [showBanForm, setShowBanForm] = useState(false)

  const loadDetail = useCallback(() => {
    setLoading(true)
    apiGetReport(reportId)
      .then(setDetail)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [reportId])

  useEffect(() => {
    loadDetail()
  }, [loadDetail])

  const handleResolve = async (s: 'resolved' | 'dismissed') => {
    if (!detail) return
    setResolving(true)
    try {
      await apiResolveReport(detail.report.id, s, note)
      setStatus(`[OK] Report marked as ${s}.`)
      setDetail((d) => d ? { ...d, report: { ...d.report, status: s } } : d)
      onUpdated()
    } catch (e) {
      setStatus(`[ERROR] ${e instanceof Error ? e.message : 'Failed'}`)
    } finally {
      setResolving(false)
    }
  }

  const handleDeleteMessage = async (msgId: number) => {
    if (!confirm('Delete this message?')) return
    try {
      await apiDeleteMessage(msgId)
      setStatus('[OK] Message deleted.')
      loadDetail()
    } catch (e) {
      setStatus(`[ERROR] ${e instanceof Error ? e.message : 'Delete failed'}`)
    }
  }

  const handleBan = async () => {
    if (!detail?.report.reported_user_id) return
    try {
      await apiBanUser(detail.report.reported_user_id, banReason)
      setStatus(`[OK] User ${detail.report.reported_username} banned.`)
      setShowBanForm(false)
      onUpdated()
    } catch (e) {
      setStatus(`[ERROR] ${e instanceof Error ? e.message : 'Ban failed'}`)
    }
  }

  const fmt = (d?: string) => d ? new Date(d).toLocaleString() : '—'

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: '32px' }}>
        <span className="loading">LOADING REPORT</span>
      </div>
    )
  }

  if (!detail) {
    return <div className="alert alert-error">[ERROR] {error || 'Report not found'}</div>
  }

  const { report, context, target_message_id } = detail

  return (
    <div>
      {status && (
        <div
          className={status.startsWith('[OK]') ? 'alert alert-success' : 'alert alert-error'}
          style={{ marginBottom: '12px' }}
        >
          {status}
        </div>
      )}

      {/* Report metadata */}
      <div className="detail-grid" style={{ marginBottom: '16px' }}>
        <span className="detail-label">Report ID</span>
        <span className="detail-value" style={{ fontSize: '11px' }}>{report.id}</span>

        <span className="detail-label">Category</span>
        <span className="detail-value" style={{ color: 'var(--yellow)' }}>{report.category_name}</span>

        <span className="detail-label">Status</span>
        <span className="detail-value"><StatusBadge status={report.status} /></span>

        <span className="detail-label">Reporter</span>
        <span className="detail-value">{report.reporter_username || '—'}</span>

        <span className="detail-label">Reported User</span>
        <span className="detail-value" style={{ color: 'var(--red)' }}>{report.reported_username || '—'}</span>

        <span className="detail-label">Description</span>
        <span className="detail-value" style={{ fontSize: '12px' }}>{report.description || '—'}</span>

        <span className="detail-label">Created</span>
        <span className="detail-value">{fmt(report.created_at)}</span>

        {report.resolution_note && (
          <>
            <span className="detail-label">Resolution Note</span>
            <span className="detail-value">{report.resolution_note}</span>
          </>
        )}
      </div>

      {/* Action buttons */}
      {report.status === 'open' && (
        <div
          style={{
            padding: '12px',
            border: '1px solid var(--border)',
            background: '#000',
            marginBottom: '16px',
          }}
        >
          <div
            style={{
              fontSize: '10px',
              color: 'var(--text-dim)',
              textTransform: 'uppercase',
              letterSpacing: '0.1em',
              marginBottom: '10px',
            }}
          >
            // RESOLUTION NOTE
          </div>
          <textarea
            rows={2}
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder="Optional resolution note..."
            style={{ resize: 'none', marginBottom: '10px' }}
          />
          <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap' }}>
            <button
              className="btn btn-primary"
              onClick={() => handleResolve('resolved')}
              disabled={resolving}
            >
              [RESOLVE]
            </button>
            <button
              className="btn"
              onClick={() => handleResolve('dismissed')}
              disabled={resolving}
            >
              [DISMISS]
            </button>
            {report.reported_user_id && (
              <button
                className="btn btn-warning"
                onClick={() => { setShowBanForm(!showBanForm); setBanReason('') }}
              >
                [BAN REPORTED USER]
              </button>
            )}
            {report.reported_message_id && (
              <button
                className="btn btn-danger"
                onClick={() => report.reported_message_id && handleDeleteMessage(report.reported_message_id)}
              >
                [DELETE MESSAGE]
              </button>
            )}
          </div>

          {showBanForm && report.reported_user_id && (
            <div
              style={{
                marginTop: '10px',
                padding: '10px',
                border: '1px solid #5a1a1a',
                background: 'rgba(255,68,68,0.05)',
              }}
            >
              <div style={{ fontSize: '11px', color: 'var(--red)', marginBottom: '6px' }}>
                BAN: {report.reported_username}
              </div>
              <input
                type="text"
                placeholder="Ban reason..."
                value={banReason}
                onChange={(e) => setBanReason(e.target.value)}
                style={{ marginBottom: '6px' }}
              />
              <div style={{ display: 'flex', gap: '6px' }}>
                <button className="btn" onClick={() => setShowBanForm(false)}>CANCEL</button>
                <button
                  className="btn btn-danger"
                  onClick={handleBan}
                  disabled={!banReason.trim()}
                >
                  [CONFIRM BAN]
                </button>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Message context */}
      {context && context.length > 0 && (
        <div>
          <div
            style={{
              fontSize: '11px',
              color: 'var(--text-dim)',
              textTransform: 'uppercase',
              letterSpacing: '0.1em',
              marginBottom: '8px',
            }}
          >
            // REPORTED MESSAGE CONTEXT
          </div>
          <div className="msg-context-list">
            {context.map((msg: Message) => {
              const isTarget = msg.id === target_message_id
              return (
                <div key={msg.id} className={`msg-context-item${isTarget ? ' target' : ''}`}>
                  <div className="msg-meta">
                    <div style={{ color: isTarget ? 'var(--green)' : 'var(--text-dim)' }}>
                      {msg.author_username}
                    </div>
                    <div style={{ fontSize: '10px', color: 'var(--green-dark)' }}>
                      {new Date(msg.created_at).toLocaleString()}
                    </div>
                  </div>
                  <div className="msg-body">{msg.content}</div>
                  <button
                    className="btn btn-danger"
                    style={{ fontSize: '10px', padding: '2px 6px' }}
                    onClick={() => handleDeleteMessage(msg.id)}
                  >
                    DEL
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
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load(statusFilter, offset)
  }, [load, statusFilter, offset])

  const handleTabChange = (tab: StatusFilter) => {
    setStatusFilter(tab)
    setOffset(0)
    load(tab, 0)
  }

  const currentPage = Math.floor(offset / PAGE_SIZE) + 1

  return (
    <div>
      <div className="page-header">
        <h1>REPORTS</h1>
      </div>

      {error && (
        <div className="alert alert-error" onClick={() => setError('')} style={{ cursor: 'pointer' }}>
          [ERROR] {error}
        </div>
      )}

      {/* Tab bar */}
      <div className="tab-bar">
        {STATUS_TABS.map((tab) => (
          <button
            key={tab}
            className={`tab-item${statusFilter === tab ? ' active' : ''}`}
            onClick={() => handleTabChange(tab)}
          >
            {tab}
          </button>
        ))}
        <span
          style={{
            marginLeft: 'auto',
            alignSelf: 'center',
            fontSize: '11px',
            color: 'var(--text-dim)',
            paddingRight: '8px',
          }}
        >
          {reports.length} result{reports.length !== 1 ? 's' : ''}
        </span>
      </div>

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
                <th>CATEGORY</th>
                <th>REPORTER</th>
                <th>REPORTED USER</th>
                <th>STATUS</th>
                <th>CREATED</th>
                <th>ACTION</th>
              </tr>
            </thead>
            <tbody>
              {reports.length === 0 ? (
                <tr>
                  <td colSpan={7} style={{ textAlign: 'center', padding: '32px', color: 'var(--text-dim)' }}>
                    [NO REPORTS]
                  </td>
                </tr>
              ) : reports.map((r) => (
                <tr key={r.id} style={{ cursor: 'pointer' }} onClick={() => setSelectedId(r.id)}>
                  <td style={{ fontSize: '11px', color: 'var(--text-dim)' }}>
                    {r.id}
                  </td>
                  <td style={{ color: 'var(--yellow)' }}>{r.category_name}</td>
                  <td>{r.reporter_username || '—'}</td>
                  <td style={{ color: r.status === 'open' ? 'var(--red)' : 'var(--text-dim)' }}>
                    {r.reported_username || '—'}
                  </td>
                  <td><StatusBadge status={r.status} /></td>
                  <td style={{ fontSize: '11px', color: 'var(--text-dim)', whiteSpace: 'nowrap' }}>
                    {new Date(r.created_at).toLocaleDateString()}
                  </td>
                  <td onClick={(e) => e.stopPropagation()}>
                    <button
                      className="btn"
                      onClick={() => setSelectedId(r.id)}
                    >
                      VIEW
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

      {/* Report detail modal */}
      {selectedId !== null && (
        <Modal
          title="REPORT DETAIL"
          onClose={() => setSelectedId(null)}
          width={760}
        >
          <ReportDetail
            reportId={selectedId}
            onClose={() => setSelectedId(null)}
            onUpdated={() => load(statusFilter, offset)}
          />
        </Modal>
      )}
    </div>
  )
}
