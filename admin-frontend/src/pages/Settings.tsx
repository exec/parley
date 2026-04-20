import { useState, useEffect, FormEvent } from 'react'
import { apiGetCategories, apiCreateCategory, apiDeleteCategory, apiBulkAddInvites, Category } from '../api'

export default function Settings() {
  const [categories, setCategories] = useState<Category[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [newCatName, setNewCatName] = useState('')
  const [addLoading, setAddLoading] = useState(false)
  const [bulkInviteCount, setBulkInviteCount] = useState(1)
  const [bulkInviteLoading, setBulkInviteLoading] = useState(false)

  const handleBulkInvites = async () => {
    if (bulkInviteCount < 1 || bulkInviteCount > 10) {
      setError('Invite count must be between 1 and 10.')
      return
    }
    if (!confirm(`Grant ${bulkInviteCount} invite${bulkInviteCount === 1 ? '' : 's'} to every active user?`)) return
    setBulkInviteLoading(true)
    setError('')
    try {
      const res = await apiBulkAddInvites(bulkInviteCount)
      setSuccess(`Added ${res.count_added} to ${res.users_updated} user${res.users_updated === 1 ? '' : 's'}.`)
    } catch (ex) {
      setError(ex instanceof Error ? ex.message : 'Bulk add failed')
    } finally {
      setBulkInviteLoading(false)
    }
  }

  const loadCategories = () => {
    setLoading(true)
    apiGetCategories().then(setCategories).catch(e => setError(e.message)).finally(() => setLoading(false))
  }

  useEffect(() => { loadCategories() }, [])

  const handleAdd = async (e: FormEvent) => {
    e.preventDefault()
    if (!newCatName.trim()) return
    setAddLoading(true)
    setError('')
    try {
      await apiCreateCategory(newCatName.trim())
      setSuccess(`Category "${newCatName.trim()}" created.`)
      setNewCatName('')
      loadCategories()
    } catch (ex) { setError(ex instanceof Error ? ex.message : 'Create failed') }
    finally { setAddLoading(false) }
  }

  const handleDelete = async (cat: Category) => {
    if (!confirm(`Delete category "${cat.name}"?`)) return
    try { await apiDeleteCategory(cat.id); setSuccess(`"${cat.name}" deleted.`); loadCategories() }
    catch (ex) { setError(ex instanceof Error ? ex.message : 'Delete failed') }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Settings</h1>
        <p style={{ color: 'var(--text-secondary)', fontSize: '14px', marginTop: '4px' }}>Platform configuration and report categories</p>
      </div>

      {error && <div className="alert alert-error" onClick={() => setError('')} style={{ cursor: 'pointer' }}>{error}</div>}
      {success && <div className="alert alert-success" onClick={() => setSuccess('')} style={{ cursor: 'pointer' }}>{success}</div>}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))', gap: '16px', alignItems: 'start' }}>
        {/* Report categories */}
        <div className="card">
          <div className="card-title">Report Categories</div>

          {loading ? (
            <div style={{ padding: '24px', textAlign: 'center', color: 'var(--text-secondary)' }}>
              <div className="loading-spinner" style={{ margin: '0 auto 8px' }} />Loading…
            </div>
          ) : (
            <>
              {categories.length === 0 ? (
                <div style={{ padding: '20px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>
                  No categories defined yet.
                </div>
              ) : (
                <div className="table-card" style={{ marginBottom: '16px' }}>
                  <table className="data-table">
                    <thead>
                      <tr><th>ID</th><th>Name</th><th></th></tr>
                    </thead>
                    <tbody>
                      {categories.map(cat => (
                        <tr key={cat.id}>
                          <td><span className="mono" style={{ color: 'var(--text-muted)' }}>{cat.id}</span></td>
                          <td style={{ fontWeight: '500', color: 'var(--yellow)' }}>{cat.name}</td>
                          <td>
                            <button className="btn btn-danger" style={{ fontSize: '12px', padding: '4px 10px' }} onClick={() => handleDelete(cat)}>
                              Delete
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}

              <form onSubmit={handleAdd}>
                <div style={{ fontSize: '12px', fontWeight: '500', color: 'var(--text-secondary)', marginBottom: '8px' }}>
                  Add new category
                </div>
                <div style={{ display: 'flex', gap: '8px' }}>
                  <input
                    type="text"
                    placeholder="Category name…"
                    value={newCatName}
                    onChange={e => setNewCatName(e.target.value)}
                    style={{ flex: 1 }}
                  />
                  <button type="submit" className="btn btn-primary" disabled={addLoading || !newCatName.trim()}>
                    {addLoading ? 'Adding…' : 'Add'}
                  </button>
                </div>
              </form>
            </>
          )}
        </div>

        {/* Bulk invites */}
        <div className="card">
          <div className="card-title">Registration Invites</div>
          <div style={{ fontSize: '13px', color: 'var(--text-secondary)', marginBottom: '12px', lineHeight: 1.45 }}>
            Grant invite credits to every active, non-system, non-bot user. Capped at 10 per click.
          </div>
          <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
            <input
              type="number"
              min={1}
              max={10}
              value={bulkInviteCount}
              onChange={e => setBulkInviteCount(Math.max(1, Math.min(10, Number(e.target.value) || 1)))}
              style={{ width: '80px' }}
              disabled={bulkInviteLoading}
            />
            <button
              className="btn btn-primary"
              onClick={handleBulkInvites}
              disabled={bulkInviteLoading}
              style={{ flex: 1 }}
            >
              {bulkInviteLoading ? 'Granting…' : `Grant ${bulkInviteCount} to everyone`}
            </button>
          </div>
        </div>

        {/* System info */}
        <div className="card">
          <div className="card-title">System Information</div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
            {[
              { label: 'Console', value: 'Parley Admin' },
              { label: 'Version', value: '2.0.0' },
              { label: 'Environment', value: 'Production' },
              { label: 'API base', value: '/api' },
              { label: 'Status', value: 'Operational', valueColor: 'var(--green)' },
            ].map(item => (
              <div key={item.label} style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                padding: '8px 0',
                borderBottom: '1px solid var(--border)',
              }}>
                <span style={{ fontSize: '13px', color: 'var(--text-secondary)', fontWeight: '500' }}>{item.label}</span>
                <span style={{ fontSize: '13px', fontFamily: 'var(--mono)', color: item.valueColor ?? 'var(--text)' }}>
                  {item.value}
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
