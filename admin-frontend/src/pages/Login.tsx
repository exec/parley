import { useState, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { apiLogin } from '../api'

export default function Login() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await apiLogin(username, password)
      login(res.token, res.username ?? username)
      navigate('/')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Authentication failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{
      minHeight: '100vh',
      background: 'var(--bg)',
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      padding: '24px',
      position: 'relative',
      overflow: 'hidden',
    }}>
      {/* Subtle background geometry */}
      <div style={{
        position: 'absolute',
        inset: 0,
        backgroundImage: `
          radial-gradient(ellipse 60% 40% at 50% 0%, rgba(6,182,212,0.07) 0%, transparent 70%),
          radial-gradient(ellipse 40% 30% at 80% 80%, rgba(6,182,212,0.04) 0%, transparent 60%)
        `,
        pointerEvents: 'none',
      }} />
      <div style={{
        position: 'absolute',
        inset: 0,
        backgroundImage: 'radial-gradient(rgba(255,255,255,0.03) 1px, transparent 1px)',
        backgroundSize: '32px 32px',
        pointerEvents: 'none',
      }} />

      {/* Logo */}
      <div style={{ textAlign: 'center', marginBottom: '36px', position: 'relative' }}>
        <div style={{
          width: '52px',
          height: '52px',
          borderRadius: '14px',
          background: 'linear-gradient(135deg, var(--accent) 0%, #0284c7 100%)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontSize: '22px',
          fontWeight: '800',
          color: '#fff',
          margin: '0 auto 14px',
          boxShadow: '0 4px 20px rgba(6,182,212,0.4)',
        }}>P</div>
        <h1 style={{ fontSize: '22px', fontWeight: '700', color: 'var(--text)', letterSpacing: '-0.02em', margin: 0 }}>
          Parley Admin
        </h1>
        <p style={{ fontSize: '13px', color: 'var(--text-muted)', marginTop: '4px' }}>
          Authorized access only
        </p>
      </div>

      {/* Login card */}
      <div style={{
        width: '100%',
        maxWidth: '380px',
        background: 'var(--bg-surface)',
        border: '1px solid var(--border-bright)',
        borderRadius: '14px',
        padding: '28px',
        boxShadow: 'var(--shadow-lg)',
        position: 'relative',
      }}>
        {error && (
          <div className="alert alert-error" style={{ marginBottom: '18px' }}>
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label htmlFor="username">Username</label>
            <input
              id="username"
              type="text"
              value={username}
              onChange={e => setUsername(e.target.value)}
              placeholder="Enter your username"
              autoComplete="username"
              autoFocus
            />
          </div>

          <div className="form-group">
            <label htmlFor="password">Password</label>
            <input
              id="password"
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              placeholder="Enter your password"
              autoComplete="current-password"
            />
          </div>

          <button
            type="submit"
            disabled={loading}
            style={{
              width: '100%',
              padding: '10px',
              background: loading ? 'rgba(6,182,212,0.15)' : 'var(--accent)',
              border: 'none',
              borderRadius: '8px',
              color: loading ? 'var(--accent)' : '#fff',
              fontSize: '14px',
              fontWeight: '600',
              fontFamily: 'var(--font)',
              cursor: loading ? 'not-allowed' : 'pointer',
              marginTop: '8px',
              transition: 'background 0.15s, opacity 0.15s',
              opacity: loading ? 0.7 : 1,
            }}
          >
            {loading ? (
              <span style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '8px' }}>
                <span className="loading-spinner" style={{ width: '14px', height: '14px' }} />
                Signing in…
              </span>
            ) : (
              'Sign in'
            )}
          </button>
        </form>
      </div>

      <p style={{ marginTop: '20px', fontSize: '11px', color: 'var(--text-muted)', textAlign: 'center' }}>
        All activity is monitored and logged.
      </p>
    </div>
  )
}
