import { useState, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { apiLogin } from '../api'

const ASCII_LOGO = `
 ██████╗  █████╗ ██████╗ ██╗     ███████╗██╗   ██╗
 ██╔══██╗██╔══██╗██╔══██╗██║     ██╔════╝╚██╗ ██╔╝
 ██████╔╝███████║██████╔╝██║     █████╗   ╚████╔╝
 ██╔═══╝ ██╔══██║██╔══██╗██║     ██╔══╝    ╚██╔╝
 ██║     ██║  ██║██║  ██║███████╗███████╗   ██║
 ╚═╝     ╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝╚══════╝   ╚═╝
`.trim()

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
    <div
      style={{
        minHeight: '100vh',
        background: 'var(--bg)',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        padding: '20px',
      }}
    >
      {/* ASCII Logo */}
      <pre
        style={{
          color: 'var(--green)',
          fontSize: '11px',
          lineHeight: '1.3',
          marginBottom: '8px',
          textAlign: 'center',
          textShadow: '0 0 8px rgba(50,205,50,0.4)',
        }}
      >
        {ASCII_LOGO}
      </pre>

      <div
        style={{
          color: 'var(--green-dim)',
          fontSize: '11px',
          letterSpacing: '0.3em',
          textTransform: 'uppercase',
          marginBottom: '40px',
        }}
      >
        ADMIN CONSOLE — AUTHORIZED ACCESS ONLY
      </div>

      {/* Login box */}
      <div
        style={{
          width: '100%',
          maxWidth: '420px',
          border: '1px solid var(--green-dark)',
          background: 'var(--bg-secondary)',
          boxShadow: '0 0 24px rgba(50,205,50,0.08)',
        }}
      >
        {/* Box header */}
        <div
          style={{
            padding: '10px 16px',
            borderBottom: '1px solid var(--green-dark)',
            background: 'var(--bg)',
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
          }}
        >
          <span style={{ color: 'var(--green-dim)', fontSize: '11px', letterSpacing: '0.1em' }}>
            // AUTHENTICATE
          </span>
          <span className="cursor-blink" />
        </div>

        <form onSubmit={handleSubmit} style={{ padding: '24px' }}>
          {error && (
            <div className="alert alert-error" style={{ marginBottom: '16px' }}>
              [ERROR] {error}
            </div>
          )}

          <div className="form-group">
            <label htmlFor="username">USERNAME</label>
            <div style={{ display: 'flex', alignItems: 'center' }}>
              <span
                style={{
                  color: 'var(--green)',
                  fontFamily: 'var(--font)',
                  fontSize: '13px',
                  padding: '6px 8px',
                  background: '#000',
                  border: '1px solid var(--green-dark)',
                  borderRight: 'none',
                }}
              >
                &gt;_
              </span>
              <input
                id="username"
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="enter username"
                autoComplete="username"
                autoFocus
                style={{ flex: 1 }}
              />
            </div>
          </div>

          <div className="form-group">
            <label htmlFor="password">PASSWORD</label>
            <div style={{ display: 'flex', alignItems: 'center' }}>
              <span
                style={{
                  color: 'var(--green)',
                  fontFamily: 'var(--font)',
                  fontSize: '13px',
                  padding: '6px 8px',
                  background: '#000',
                  border: '1px solid var(--green-dark)',
                  borderRight: 'none',
                }}
              >
                &gt;_
              </span>
              <input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="enter password"
                autoComplete="current-password"
                style={{ flex: 1 }}
              />
            </div>
          </div>

          <button
            type="submit"
            disabled={loading}
            className="btn btn-primary"
            style={{
              width: '100%',
              padding: '10px',
              fontSize: '13px',
              letterSpacing: '0.15em',
              marginTop: '8px',
              justifyContent: 'center',
            }}
          >
            {loading ? (
              <span className="loading">AUTHENTICATING</span>
            ) : (
              '[ AUTHENTICATE ]'
            )}
          </button>
        </form>

        <div
          style={{
            borderTop: '1px solid var(--green-dark)',
            padding: '8px 16px',
            fontSize: '10px',
            color: 'var(--green-dark)',
            letterSpacing: '0.05em',
          }}
        >
          WARNING: Unauthorized access is prohibited. All activity is logged.
        </div>
      </div>

      {/* Boot-up text */}
      <div
        style={{
          marginTop: '32px',
          fontSize: '10px',
          color: 'var(--green-dark)',
          letterSpacing: '0.08em',
          textAlign: 'center',
          lineHeight: '1.8',
        }}
      >
        <div>PARLEY ADMIN CONSOLE v1.0.0</div>
        <div>CONNECTION: SECURE</div>
      </div>
    </div>
  )
}
