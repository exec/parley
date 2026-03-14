import { useState, useEffect, useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  apiGetMessages,
  apiGetMessageContext,
  apiDeleteMessage,
  apiBanUser,
  apiForceLogout,
  Message,
  MessageContextResponse,
} from '../api'
import Modal from '../components/Modal'

const PAGE_SIZE = 50

// Find the target message in the context response
function findTarget(ctx: MessageContextResponse): Message | undefined {
  return ctx.messages.find((m) => m.id === ctx.message_id)
}

function ContextViewer({
  ctx,
  onDelete,
}: {
  ctx: MessageContextResponse
  onDelete: (id: number) => void
}) {
  const target = findTarget(ctx)
  const [banModal, setBanModal] = useState(false)
  const [banReason, setBanReason] = useState('')
  const [status, setStatus] = useState('')

  const handleBan = async () => {
    if (!target) return
    try {
      await apiBanUser(target.author_id, banReason)
      setStatus(`[OK] User ${target.author_username} banned.`)
      setBanModal(false)
    } catch (e) {
      setStatus(`[ERROR] ${e instanceof Error ? e.message : 'Ban failed'}`)
    }
  }

  const handleForceLogout = async () => {
    if (!target) return
    try {
      await apiForceLogout(target.author_id)
      setStatus('[OK] Force logout sent.')
    } catch (e) {
      setStatus(`[ERROR] ${e instanceof Error ? e.message : 'Failed'}`)
    }
  }

  return (
    <>
      {/* Author info bar */}
      {target && (
        <div
          style={{
            padding: '8px 12px',
            background: '#000',
            border: '1px solid var(--border)',
            marginBottom: '8px',
            fontSize: '11px',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            flexWrap: 'wrap',
            gap: '8px',
          }}
        >
          <div>
            <span style={{ color: 'var(--text-dim)' }}>AUTHOR: </span>
            <span style={{ color: 'var(--green)' }}>{target.author_username}</span>
            <span style={{ color: 'var(--text-dim)', marginLeft: '12px' }}>ID: </span>
            <span style={{ fontSize: '10px', color: 'var(--text-dim)' }}>{target.author_id}</span>
          </div>
          <div style={{ display: 'flex', gap: '6px' }}>
            <button
              className="btn btn-warning"
              onClick={() => { setBanModal(true); setBanReason('') }}
            >
              [BAN USER]
            </button>
            <button className="btn" onClick={handleForceLogout}>
              [FORCE LOGOUT]
            </button>
          </div>
        </div>
      )}

      {status && (
        <div
          className={status.startsWith('[OK]') ? 'alert alert-success' : 'alert alert-error'}
          style={{ marginBottom: '8px' }}
        >
          {status}
        </div>
      )}

      {/* Message list */}
      <div className="msg-context-list">
        {ctx.messages.map((msg) => {
          const isTarget = msg.id === ctx.message_id
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
                onClick={() => onDelete(msg.id)}
              >
                DEL
              </button>
            </div>
          )
        })}
      </div>

      {/* Ban sub-modal */}
      {banModal && target && (
        <div
          style={{
            marginTop: '12px',
            padding: '12px',
            border: '1px solid #5a1a1a',
            background: 'rgba(255,68,68,0.05)',
          }}
        >
          <div
            style={{
              fontSize: '11px',
              color: 'var(--red)',
              textTransform: 'uppercase',
              letterSpacing: '0.1em',
              marginBottom: '8px',
            }}
          >
            BAN: {target.author_username}
          </div>
          <div className="form-group">
            <label>REASON</label>
            <textarea
              rows={2}
              value={banReason}
              onChange={(e) => setBanReason(e.target.value)}
              placeholder="Ban reason..."
              style={{ resize: 'none' }}
            />
          </div>
          <div style={{ display: 'flex', gap: '6px', justifyContent: 'flex-end' }}>
            <button className="btn" onClick={() => setBanModal(false)}>CANCEL</button>
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

  // Context modal
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
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load(q, userId, offset)
  }, [load, q, userId, offset])

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
    try {
      const c = await apiGetMessageContext(msg.id)
      setCtx(c)
    } catch (e) {
      setCtxError(e instanceof Error ? e.message : 'Failed to load context')
    } finally {
      setCtxLoading(false)
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Delete this message?')) return
    try {
      await apiDeleteMessage(id)
      // refresh context if open
      if (ctx) {
        const refreshed = await apiGetMessageContext(ctx.message_id).catch(() => null)
        if (refreshed) setCtx(refreshed)
      }
      load(q, userId, offset)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Delete failed')
    }
  }

  const currentPage = Math.floor(offset / PAGE_SIZE) + 1

  return (
    <div>
      <div className="page-header">
        <h1>MESSAGE SEARCH</h1>
      </div>

      {error && (
        <div className="alert alert-error" onClick={() => setError('')} style={{ cursor: 'pointer' }}>
          [ERROR] {error}
        </div>
      )}

      {/* Search form */}
      <form onSubmit={handleSearch}>
        <div style={{ display: 'flex', gap: '8px', marginBottom: '16px', flexWrap: 'wrap', alignItems: 'flex-end' }}>
          <div style={{ flex: '2 1 240px' }}>
            <label
              style={{
                display: 'block',
                fontSize: '10px',
                textTransform: 'uppercase',
                letterSpacing: '0.1em',
                color: 'var(--text-dim)',
                marginBottom: '4px',
              }}
            >
              Content
            </label>
            <input
              type="search"
              placeholder="Search message content..."
              value={q}
              onChange={(e) => setQ(e.target.value)}
            />
          </div>
          <div style={{ flex: '1 1 200px' }}>
            <label
              style={{
                display: 'block',
                fontSize: '10px',
                textTransform: 'uppercase',
                letterSpacing: '0.1em',
                color: 'var(--text-dim)',
                marginBottom: '4px',
              }}
            >
              User ID
            </label>
            <input
              type="number"
              placeholder="Filter by user ID..."
              value={userId}
              onChange={(e) => setUserId(e.target.value)}
            />
          </div>
          <button type="submit" className="btn btn-primary" style={{ height: '32px' }}>
            [SEARCH]
          </button>
          {(q || userId) && (
            <button
              type="button"
              className="btn"
              style={{ height: '32px' }}
              onClick={() => {
                setQ('')
                setUserId('')
                setOffset(0)
                setSearchParams({})
                load('', '', 0)
              }}
            >
              [CLEAR]
            </button>
          )}
        </div>
        <div style={{ fontSize: '11px', color: 'var(--text-dim)', marginBottom: '12px' }}>
          {messages.length} result{messages.length !== 1 ? 's' : ''}
        </div>
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
                <th>USER</th>
                <th>CONTENT</th>
                <th>CHANNEL ID</th>
                <th>CREATED</th>
                <th>ACTIONS</th>
              </tr>
            </thead>
            <tbody>
              {messages.length === 0 ? (
                <tr>
                  <td colSpan={6} style={{ textAlign: 'center', padding: '32px', color: 'var(--text-dim)' }}>
                    [NO MESSAGES FOUND]
                  </td>
                </tr>
              ) : messages.map((msg) => (
                <tr key={msg.id}>
                  <td style={{ fontSize: '11px', color: 'var(--text-dim)' }}>
                    {msg.id}
                  </td>
                  <td style={{ color: 'var(--green)', whiteSpace: 'nowrap' }}>{msg.author_username}</td>
                  <td>
                    <div
                      className="truncate"
                      style={{ maxWidth: '300px', fontSize: '12px' }}
                      title={msg.content}
                    >
                      {msg.content || <em style={{ color: 'var(--text-dim)' }}>[no content]</em>}
                    </div>
                  </td>
                  <td style={{ fontSize: '11px', color: 'var(--text-dim)' }}>
                    {msg.channel_id}
                  </td>
                  <td style={{ fontSize: '11px', color: 'var(--text-dim)', whiteSpace: 'nowrap' }}>
                    {new Date(msg.created_at).toLocaleString()}
                  </td>
                  <td>
                    <div className="actions">
                      <button
                        className="btn"
                        onClick={() => handleViewContext(msg)}
                      >
                        CONTEXT
                      </button>
                      <button
                        className="btn btn-danger"
                        onClick={() => handleDelete(msg.id)}
                      >
                        DELETE
                      </button>
                    </div>
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

      {/* Context modal */}
      {(ctx || ctxLoading || ctxError) && (
        <Modal
          title="MESSAGE CONTEXT"
          onClose={() => { setCtx(null); setCtxError('') }}
          width={720}
        >
          {ctxLoading && (
            <div style={{ textAlign: 'center', padding: '32px' }}>
              <span className="loading">LOADING CONTEXT</span>
            </div>
          )}
          {ctxError && (
            <div className="alert alert-error">[ERROR] {ctxError}</div>
          )}
          {ctx && (
            <ContextViewer
              ctx={ctx}
              onDelete={handleDelete}
            />
          )}
        </Modal>
      )}
    </div>
  )
}
