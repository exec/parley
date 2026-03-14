import React, { useState, useEffect } from 'react'
import { NavLink, useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { apiGetStats } from '../api'

function NavItem({
  to,
  label,
  badge,
}: {
  to: string
  label: string
  badge?: number
}) {
  return (
    <NavLink
      to={to}
      style={({ isActive }) => ({
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '8px 16px',
        color: isActive ? '#000' : 'var(--green)',
        background: isActive ? 'var(--green)' : 'transparent',
        textDecoration: 'none',
        fontSize: '12px',
        fontWeight: isActive ? '700' : '500',
        textTransform: 'uppercase',
        letterSpacing: '0.08em',
        borderLeft: isActive ? '3px solid var(--green)' : '3px solid transparent',
        transition: 'all 0.1s',
        fontFamily: 'var(--font)',
      })}
      className="nav-link"
    >
      {({ isActive }) => (
        <>
          <span>{isActive ? '> ' : '  '}{label}</span>
          {badge !== undefined && badge > 0 && (
            <span
              style={{
                background: isActive ? '#000' : 'var(--yellow)',
                color: isActive ? 'var(--yellow)' : '#000',
                fontSize: '10px',
                padding: '1px 5px',
                minWidth: '20px',
                textAlign: 'center',
                fontFamily: 'var(--font)',
              }}
            >
              {badge}
            </span>
          )}
        </>
      )}
    </NavLink>
  )
}

export default function Layout({ children }: { children: React.ReactNode }) {
  const { username, logout } = useAuth()
  const navigate = useNavigate()
  const [time, setTime] = useState(new Date())
  const [openReports, setOpenReports] = useState(0)

  useEffect(() => {
    const interval = setInterval(() => setTime(new Date()), 1000)
    return () => clearInterval(interval)
  }, [])

  useEffect(() => {
    apiGetStats()
      .then((s) => setOpenReports(s.open_reports))
      .catch(() => {})
  }, [])

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  const timeStr = time.toTimeString().slice(0, 8)
  const dateStr = time.toISOString().slice(0, 10)

  return (
    <div
      style={{
        display: 'flex',
        height: '100vh',
        overflow: 'hidden',
        background: 'var(--bg)',
      }}
    >
      {/* Sidebar */}
      <div
        style={{
          width: '200px',
          flexShrink: 0,
          background: 'var(--bg)',
          borderRight: '1px solid var(--border)',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        {/* Logo */}
        <div
          style={{
            padding: '16px',
            borderBottom: '1px solid var(--border)',
            background: 'var(--bg-secondary)',
          }}
        >
          <div
            style={{
              fontSize: '14px',
              color: 'var(--green)',
              letterSpacing: '0.15em',
              fontWeight: 'bold',
            }}
          >
            PARLEY
          </div>
          <div
            style={{
              fontSize: '10px',
              color: 'var(--green-dim)',
              letterSpacing: '0.1em',
              marginTop: '2px',
            }}
          >
            ADMIN CONSOLE v1.0
          </div>
        </div>

        {/* Nav links */}
        <nav style={{ flex: 1, paddingTop: '8px', overflowY: 'auto' }}>
          <NavItem to="/" label="Dashboard" />
          <NavItem to="/users" label="Users" />
          <NavItem to="/messages" label="Messages" />
          <NavItem to="/reports" label="Reports" badge={openReports} />
          <NavItem to="/servers" label="Servers" />
          <NavItem to="/settings" label="Settings" />
        </nav>

        {/* Bottom section */}
        <div style={{ borderTop: '1px solid var(--border)' }}>
          <div
            style={{
              padding: '8px 16px',
              fontSize: '10px',
              color: 'var(--green-dark)',
              borderBottom: '1px solid var(--border)',
            }}
          >
            <div>{dateStr}</div>
            <div style={{ color: 'var(--green-dim)' }}>{timeStr}</div>
          </div>
          <button
            onClick={handleLogout}
            style={{
              display: 'block',
              width: '100%',
              padding: '10px 16px',
              background: 'transparent',
              border: 'none',
              color: 'var(--red)',
              cursor: 'pointer',
              textAlign: 'left',
              fontSize: '12px',
              textTransform: 'uppercase',
              letterSpacing: '0.08em',
              fontFamily: 'var(--font)',
              borderLeft: '3px solid transparent',
            }}
            onMouseEnter={(e) => {
              ;(e.target as HTMLButtonElement).style.background = 'rgba(255,68,68,0.08)'
            }}
            onMouseLeave={(e) => {
              ;(e.target as HTMLButtonElement).style.background = 'transparent'
            }}
          >
            {'  '}LOGOUT
          </button>
        </div>
      </div>

      {/* Main area */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        {/* Top bar */}
        <div
          style={{
            height: '36px',
            borderBottom: '1px solid var(--border)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'flex-end',
            padding: '0 20px',
            background: 'var(--bg-secondary)',
            flexShrink: 0,
          }}
        >
          <span
            style={{
              fontSize: '11px',
              color: 'var(--text-dim)',
              letterSpacing: '0.08em',
            }}
          >
            PARLEY_ADMIN //{' '}
            <span style={{ color: 'var(--green)' }}>{username ?? 'unknown'}</span>
            @system &nbsp;|&nbsp; {timeStr}
          </span>
        </div>

        {/* Page content */}
        <main
          style={{
            flex: 1,
            overflow: 'auto',
            padding: '24px',
          }}
        >
          {children}
        </main>
      </div>
    </div>
  )
}
