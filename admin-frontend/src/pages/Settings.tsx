import { useState, useEffect, FormEvent } from 'react'
import { apiGetCategories, apiCreateCategory, apiDeleteCategory, Category } from '../api'

export default function Settings() {
  const [categories, setCategories] = useState<Category[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [newCatName, setNewCatName] = useState('')
  const [addLoading, setAddLoading] = useState(false)

  const loadCategories = () => {
    setLoading(true)
    apiGetCategories()
      .then(setCategories)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    loadCategories()
  }, [])

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
    } catch (ex) {
      setError(ex instanceof Error ? ex.message : 'Create failed')
    } finally {
      setAddLoading(false)
    }
  }

  const handleDelete = async (cat: Category) => {
    if (!confirm(`Delete category "${cat.name}"?`)) return
    try {
      await apiDeleteCategory(cat.id)
      setSuccess(`Category "${cat.name}" deleted.`)
      loadCategories()
    } catch (ex) {
      setError(ex instanceof Error ? ex.message : 'Delete failed')
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>SETTINGS</h1>
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

      <div style={{ maxWidth: '600px' }}>
        {/* Report categories */}
        <div className="panel">
          <div className="panel-title">// REPORT CATEGORIES</div>

          {loading ? (
            <div style={{ padding: '16px', textAlign: 'center' }}>
              <span className="loading">LOADING</span>
            </div>
          ) : (
            <>
              <div style={{ marginBottom: '16px' }}>
                {categories.length === 0 ? (
                  <div
                    style={{
                      padding: '16px',
                      textAlign: 'center',
                      color: 'var(--text-dim)',
                      fontSize: '12px',
                      border: '1px solid var(--border)',
                      background: '#000',
                    }}
                  >
                    [NO CATEGORIES DEFINED]
                  </div>
                ) : (
                  <table className="data-table">
                    <thead>
                      <tr>
                        <th>ID</th>
                        <th>NAME</th>
                        <th>ACTION</th>
                      </tr>
                    </thead>
                    <tbody>
                      {categories.map((cat) => (
                        <tr key={cat.id}>
                          <td style={{ fontSize: '11px', color: 'var(--text-dim)' }}>
                            {cat.id}
                          </td>
                          <td style={{ color: 'var(--yellow)' }}>{cat.name}</td>
                          <td>
                            <button
                              className="btn btn-danger"
                              style={{ fontSize: '11px', padding: '3px 8px' }}
                              onClick={() => handleDelete(cat)}
                            >
                              DELETE
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              {/* Add new category */}
              <form onSubmit={handleAdd}>
                <div
                  style={{
                    fontSize: '10px',
                    color: 'var(--text-dim)',
                    textTransform: 'uppercase',
                    letterSpacing: '0.1em',
                    marginBottom: '8px',
                  }}
                >
                  // ADD NEW CATEGORY
                </div>
                <div style={{ display: 'flex', gap: '8px' }}>
                  <input
                    type="text"
                    placeholder="Category name..."
                    value={newCatName}
                    onChange={(e) => setNewCatName(e.target.value)}
                    style={{ flex: 1 }}
                  />
                  <button
                    type="submit"
                    className="btn btn-primary"
                    disabled={addLoading || !newCatName.trim()}
                  >
                    {addLoading ? (
                      <span className="loading">ADDING</span>
                    ) : (
                      '[ADD]'
                    )}
                  </button>
                </div>
              </form>
            </>
          )}
        </div>

        {/* System info */}
        <div className="panel" style={{ marginTop: '16px' }}>
          <div className="panel-title">// SYSTEM INFO</div>
          <div
            style={{
              fontFamily: 'var(--font)',
              fontSize: '11px',
              color: 'var(--text-dim)',
              lineHeight: '2',
              padding: '8px',
              background: '#000',
              border: '1px solid var(--border)',
            }}
          >
            <div>
              <span style={{ color: 'var(--green)' }}>SYSTEM:</span> PARLEY ADMIN CONSOLE
            </div>
            <div>
              <span style={{ color: 'var(--green)' }}>VERSION:</span> 1.0.0
            </div>
            <div>
              <span style={{ color: 'var(--green)' }}>BUILD:</span> production
            </div>
            <div>
              <span style={{ color: 'var(--green)' }}>API:</span> /api
            </div>
            <div>
              <span style={{ color: 'var(--green)' }}>STATUS:</span>{' '}
              <span style={{ color: 'var(--green)' }}>ONLINE</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
