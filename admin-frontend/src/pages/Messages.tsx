import { useState, useEffect, useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  apiGetMessages, apiGetMessageContext, apiDeleteMessage,
  apiBanUser, apiForceLogout, Message, MessageContextResponse,
} from '../api'
import Modal from '../components/Modal'

const PAGE_SIZE = 50

function findTarget(ctx: MessageContextResponse): Message | undefined {
  return ctx.messages.find(m => m.id === ctx.message_id)
}

function ContextViewer({ ctx, onDelete }: { ctx: MessageContextResponse; onDelete: (id: number) => void }) {
  const target = findTarget(ctx)
  const [banModal, setBanModal] = useState(false)
  const [banReason, setBanReason] = useState('')
  const [status, setStatus] = useState('')

  const handleBan = async () => {
    if (!target) return
    try { await apiBanUser(target.author_id, banReason); setStatus(`User ${target.author_username} banned.`); setBanModal(false) }
    catch (e) { setStatus(`Error: ${e instanceof Error ? e.message : 'Ban failed'}`) }
  }

  const handleForceLogout = async () => {
    if (!target) return
    try { await apiForceLogout(target.author_id); setStatus('Force logout sent.') }
    catch (e) { setStatus(`Error: ${e instanceof Error ? e.message : 'Failed'}`) }
  }

  return (
    <>
      {target && (
        <div style={{
          padding: '10px 14px',
          background: 'var(--bg-elevated)',
          borderRadius: '8px',
          border: '1px solid var(--border)',
          marginBottom: '10px',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          flexWrap: 'wrap',
          gap: '8px',
        }}>
          <div style={{ fontSize: '13px' }}>
            <span style={{ color: 'var(--text-secondary)' }}>Author: </span>
            <span style={{ fontWeight: '600', color: 'var(--text)' }}>{target.author_username}</span>
            <span style={{ color: 'var(--text-muted)', fontSize: '12px', marginLeft: '10px', fontFamily: 'var(--mono)' }}>
              #{target.author_id}
            </span>
          </div>
          <div style={{ display: 'flex', gap: '6px' }}>
            <button className="btn btn-warning" onClick={() => { setBanModal(true); setBanReason('') }}>Ban user</button>
            <button className="btn" onClick={handleForceLogout}>Force logout</button>
          </div>
        </div>
      )}

      {status && (
        <div className={`alert ${status.startsWith('Error') ? 'alert-error' : 'alert-success'}`} style={{ marginBottom: '10px' }}>
          {status}
        </div>
      )}

      <div className="msg-context-list">
        {ctx.messages.map(msg => {
          const isTarget = msg.id === ctx.message_id
          return (
            <div key={msg.id} className={`msg-context-item${isTarget ? ' target' : ''}`}>
              <div className="msg-meta">
                <div style={{ fontWeight: isTarget ? '600' : undefined, color: isTarget ? 'var(--accent)' : undefined }}>
                  {msg.author_username}
                </div>
                <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--mono)' }}>
                  {new Date(msg.created_at).toLocaleTimeString()}
                </div>
              </div>
              <div className="msg-body">{msg.content}</div>
              <button className="btn btn-danger" style={{ fontSize: '11px', padding: '3px 8px' }} onClick={() => onDelete(msg.id)}>
                Delete
              </button>
            </div>
          )
        })}
      </div>

      {banModal && target && (
        <div style={{
          marginTop: '12px',
          padding: '14px',
          borderRadius: '8px',
          border: '1px solid rgba(248,113,113,0.25)',
          background: 'var(--red-soft)',
        }}>
          <div style={{ fontSize: '13px', fontWeight: '600', color: 'var(--red)', marginBottom: '10px' }}>
            Ban: {target.author_username}
          </div>
          <div className="form-group">
            <label>Reason</label>
            <textarea rows={2} value={banReason} onChange={e => setBanReason(e.target.value)} placeholder="Ban reason…" />
          </div>
          <div style={{ display: 'flex', gap: '6px', justifyContent: 'flex-end' }}>
            <button className="btn" onClick={() => setBanModal(false)}>Cancel</button>
            <button className="btn btn-danger" onClick={handleBan} disabled={!banReason.trim()}>Confirm ban</button>
          </div>
        </div>
      )}
    </>
  )
}

export default function Messages() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [messages, setMessages] = useState<Message[]>([])
  const [loading, setLoading] = useState(false)
  const [hasMore, setHasMore] = useState(false)
  const [error, setError] = useState('')
  const [q, setQ] = useState(searchParams.get('q') ?? '')
  const [userId, setUserId] = useState(searchParams.get('user_id') ?? '')
  const [offset, setOffset] = useState(0)
  const [ctx, setCtx] = useState<MessageContextResponse | null>(null)
  const [ctxLoading, setCtxLoading] = useState(false)
  const [ctxError, setCtxError] = useState('')

  const load = useCallback(async (query: string, uid: string, off: number) => {
    setLoading(true)
    setError('')
    try {
      const uidNum = uid ? Number(uid) : undefined
      const res = await apiGetMessages(query || undefined, uidNum, PAGE_SIZE, off)
      setMessages(res ?? [])
      setHasMore((res ?? []).length === PAGE_SIZE)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load messages')
    } finally { setLoading(false) }
  }, [])

  useEffect(() => { load(q, userId, offset) }, [load, q, userId, offset])

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault()
    setOffset(0)
    const params: Record<string, string> = {}
    if (q) params.q = q
    if (userId) params.user_id = userId
    setSearchParams(params)
    load(q, userId, 0)
  }

  const handleViewContext = async (msg: Message) => {
    setCtxLoading(true)
    setCtxError('')
    setCtx(null)
    try { setCtx(await apiGetMessageContext(msg.id)) }
    catch (e) { setCtxError(e instanceof Error ? e.message : 'Failed to load context') }
    finally { setCtxLoading(false) }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Delete this message?')) return
    try {
      await apiDeleteMessage(id)
      if (ctx) {
        const refreshed = await apiGetMessageContext(ctx.message_id).catch(() => null)
        if (refreshed) setCtx(refreshed)
      }
      load(q, userId, offset)
    } catch (e) { setError(e instanceof Error ? e.message : 'Delete failed') }
  }

  const currentPage = Math.floor(offset / PAGE_SIZE) + 1

  return (
    <div>
      <div className="page-header">
        <h1>Messages</h1>
        <p style={{ color: 'var(--text-secondary)', fontSize: '14px', marginTop: '4px' }}>Search and moderate platform messages</p>
      </div>

      {error && <div className="alert alert-error" onClick={() => setError('')} style={{ cursor: 'pointer' }}>{error}</div>}

      {/* Search form */}
      <form onSubmit={handleSearch}>
        <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', flexWrap: 'wrap', alignItems: 'flex-end' }}>
          <div style={{ flex: '2 1 240px' }}>
            <label style={{ display: 'block', fontSize: '12px', fontWeight: '500', color: 'var(--text-secondary)', marginBottom: '5px' }}>
              Content
            </label>
            <input type="search" placeholder="Search message content…" value={q} onChange={e => setQ(e.target.value)} />
          </div>
          <div style={{ flex: '1 1 180px' }}>
            <label style={{ display: 'block', fontSize: '12px', fontWeight: '500', color: 'var(--text-secondary)', marginBottom: '5px' }}>
              User ID
            </label>
            <input type="number" placeholder="Filter by user ID…" value={userId} onChange={e => setUserId(e.target.value)} />
          </div>
          <button type="submit" className="btn btn-primary">Search</button>
          {(q || userId) && (
            <button type="button" className="btn" onClick={() => { setQ(''); setUserId(''); setOffset(0); setSearchParams({}); load('', '', 0) }}>
              Clear
            </button>
          )}
        </div>
        <div style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '14px' }}>
          {messages.length} result{messages.length !== 1 ? 's' : ''}
        </div>
      </form>

      <div className="table-card">
        {loading ? (
          <div style={{ padding: '48px', textAlign: 'center', color: 'var(--text-secondary)' }}>
            <div className="loading-spinner" style={{ margin: '0 auto 10px' }} />Loading messages…
          </div>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table className="data-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>User</th>
                  <th>Content</th>
                  <th>Channel ID</th>
                  <th>Date</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {messages.length === 0 ? (
                  <tr><td colSpan={6} style={{ textAlign: 'center', padding: '48px', color: 'var(--text-muted)' }}>No messages found</td></tr>
                ) : messages.map(msg => (
                  <tr key={msg.id}>
                    <td><span className="mono" style={{ color: 'var(--text-muted)' }}>{msg.id}</span></td>
                    <td style={{ fontWeight: '600', color: 'var(--text)', whiteSpace: 'nowrap' }}>{msg.author_username}</td>
                    <td>
                      <div className="truncate" style={{ maxWidth: '300px', fontSize: '13px' }} title={msg.content}>
                        {msg.content || <em style={{ color: 'var(--text-muted)' }}>no content</em>}
                      </div>
                    </td>
                    <td><span className="mono" style={{ color: 'var(--text-muted)' }}>{msg.channel_id}</span></td>
                    <td><span className="mono" style={{ color: 'var(--text-muted)', fontSize: '12px' }}>
                      {new Date(msg.created_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}
                    </span></td>
                    <td>
                      <div className="actions">
                        <button className="btn" onClick={() => handleViewContext(msg)}>Context</button>
                        <button className="btn btn-danger" onClick={() => handleDelete(msg.id)}>Delete</button>
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

      {(ctx || ctxLoading || ctxError) && (
        <Modal title="Message Context" onClose={() => { setCtx(null); setCtxError('') }} width={720}>
          {ctxLoading && (
            <div style={{ textAlign: 'center', padding: '32px', color: 'var(--text-secondary)' }}>
              <div className="loading-spinner" style={{ margin: '0 auto 10px' }} />Loading…
            </div>
          )}
          {ctxError && <div className="alert alert-error">{ctxError}</div>}
          {ctx && <ContextViewer ctx={ctx} onDelete={handleDelete} />}
        </Modal>
      )}
    </div>
  )
}
