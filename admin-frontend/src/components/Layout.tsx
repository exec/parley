import React, { useState, useEffect } from 'react'
import { NavLink, useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { apiGetStats } from '../api'

/* ---- SVG Icons ---- */
const IcoDash = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
    <rect x="1" y="1" width="6" height="6" rx="1.5" fill="currentColor" opacity="0.8"/>
    <rect x="9" y="1" width="6" height="6" rx="1.5" fill="currentColor" opacity="0.8"/>
    <rect x="1" y="9" width="6" height="6" rx="1.5" fill="currentColor" opacity="0.8"/>
    <rect x="9" y="9" width="6" height="6" rx="1.5" fill="currentColor" opacity="0.8"/>
  </svg>
)
const IcoUsers = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
    <circle cx="6" cy="5" r="2.5" fill="currentColor" opacity="0.8"/>
    <path d="M1 13.5c0-2.76 2.24-5 5-5h1c2.76 0 5 2.24 5 5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" opacity="0.8"/>
    <circle cx="12.5" cy="5" r="2" fill="currentColor" opacity="0.5"/>
    <path d="M14.5 13.5c0-1.93-1.34-3.55-3.16-4" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" opacity="0.5"/>
  </svg>
)
const IcoMessages = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
    <path d="M2 3a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H5l-3 2V3Z" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" opacity="0.8"/>
    <line x1="5" y1="6" x2="11" y2="6" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" opacity="0.5"/>
    <line x1="5" y1="8.5" x2="9" y2="8.5" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" opacity="0.5"/>
  </svg>
)
const IcoFlag = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
    <path d="M3.5 2v12" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" opacity="0.8"/>
    <path d="M3.5 2l9 3.5-9 3.5" fill="rgba(currentColor,0.15)" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" opacity="0.8"/>
  </svg>
)
const IcoServer = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
    <rect x="1.5" y="2" width="13" height="5" rx="1.5" stroke="currentColor" strokeWidth="1.4" opacity="0.8"/>
    <rect x="1.5" y="9" width="13" height="5" rx="1.5" stroke="currentColor" strokeWidth="1.4" opacity="0.8"/>
    <circle cx="12.5" cy="4.5" r="1" fill="currentColor" opacity="0.8"/>
    <circle cx="12.5" cy="11.5" r="1" fill="currentColor" opacity="0.8"/>
  </svg>
)
const IcoBot = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
    <rect x="2" y="5.5" width="12" height="8" rx="2" stroke="currentColor" strokeWidth="1.4" opacity="0.8"/>
    <circle cx="5.5" cy="9.5" r="1.2" fill="currentColor" opacity="0.8"/>
    <circle cx="10.5" cy="9.5" r="1.2" fill="currentColor" opacity="0.8"/>
    <line x1="8" y1="2" x2="8" y2="5.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" opacity="0.8"/>
    <circle cx="8" cy="1.5" r="1" fill="currentColor" opacity="0.7"/>
    <line x1="6" y1="13.5" x2="10" y2="13.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" opacity="0.5"/>
  </svg>
)
const IcoSettings = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
    <circle cx="8" cy="8" r="2.2" stroke="currentColor" strokeWidth="1.4" opacity="0.8"/>
    <path d="M8 1.5v2M8 12.5v2M1.5 8h2M12.5 8h2M3.4 3.4l1.41 1.41M11.19 11.19l1.41 1.41M3.4 12.6l1.41-1.41M11.19 4.81l1.41-1.41" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" opacity="0.7"/>
  </svg>
)
const IcoLogout = () => (
  <svg width="15" height="15" viewBox="0 0 16 16" fill="none">
    <path d="M6 2H3a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h3M10 11l3-3-3-3M13 8H6" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" opacity="0.8"/>
  </svg>
)

const NAV_ITEMS: { to: string; label: string; Icon: React.FC }[] = [
  { to: '/',         label: 'Dashboard', Icon: IcoDash },
  { to: '/users',    label: 'Users',     Icon: IcoUsers },
  { to: '/messages', label: 'Messages',  Icon: IcoMessages },
  { to: '/reports',  label: 'Reports',   Icon: IcoFlag },
  { to: '/servers',  label: 'Servers',   Icon: IcoServer },
  { to: '/bots',     label: 'Bots',      Icon: IcoBot },
  { to: '/settings', label: 'Settings',  Icon: IcoSettings },
]

function NavItem({ to, label, Icon, badge }: { to: string; label: string; Icon: React.FC; badge?: number }) {
  return (
    <NavLink
      to={to}
      end={to === '/'}
      style={({ isActive }) => ({
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
        padding: '8px 12px',
        marginBottom: '2px',
        borderRadius: '8px',
        color: isActive ? '#fff' : 'var(--text-secondary)',
        background: isActive ? 'rgba(6, 182, 212, 0.14)' : 'transparent',
        textDecoration: 'none',
        fontSize: '13px',
        fontWeight: isActive ? '600' : '500',
        transition: 'all 0.12s',
        borderLeft: isActive ? '2px solid var(--accent)' : '2px solid transparent',
      })}
      className="nav-link"
    >
      <Icon />
      <span style={{ flex: 1 }}>{label}</span>
      {badge !== undefined && badge > 0 && (
        <span style={{
          background: 'var(--yellow)',
          color: '#000',
          fontSize: '10px',
          fontWeight: '700',
          padding: '1px 6px',
          borderRadius: '100px',
          minWidth: '18px',
          textAlign: 'center',
        }}>
          {badge}
        </span>
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
    apiGetStats().then(s => setOpenReports(s.open_reports)).catch(() => {})
  }, [])

  const handleLogout = () => { logout(); navigate('/login') }

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden', background: 'var(--bg)' }}>
      {/* Sidebar */}
      <div style={{
        width: '216px',
        flexShrink: 0,
        background: 'var(--bg-surface)',
        borderRight: '1px solid var(--border)',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
      }}>
        {/* Logo */}
        <div style={{ padding: '18px 14px 14px', borderBottom: '1px solid var(--border)' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <div style={{
              width: '32px',
              height: '32px',
              borderRadius: '9px',
              background: 'linear-gradient(135deg, var(--accent) 0%, #0284c7 100%)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: '15px',
              fontWeight: '800',
              color: '#fff',
              flexShrink: 0,
              boxShadow: '0 2px 8px rgba(6,182,212,0.35)',
            }}>P</div>
            <div>
              <div style={{ fontSize: '14px', fontWeight: '700', color: 'var(--text)', letterSpacing: '-0.01em' }}>
                Parley
              </div>
              <div style={{ fontSize: '10px', color: 'var(--text-muted)', fontWeight: '500', letterSpacing: '0.05em', textTransform: 'uppercase' }}>
                Admin
              </div>
            </div>
          </div>
        </div>

        {/* Nav */}
        <nav style={{ flex: 1, padding: '10px 8px', overflowY: 'auto' }}>
          {NAV_ITEMS.map(({ to, label, Icon }) => (
            <NavItem key={to} to={to} label={label} Icon={Icon}
              badge={to === '/reports' ? openReports : undefined} />
          ))}
        </nav>

        {/* Bottom */}
        <div style={{ borderTop: '1px solid var(--border)', padding: '10px 8px' }}>
          <div style={{
            padding: '8px 10px',
            borderRadius: '8px',
            background: 'var(--bg-elevated)',
            marginBottom: '6px',
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
          }}>
            <div style={{
              width: '28px',
              height: '28px',
              borderRadius: '50%',
              background: 'var(--accent-soft)',
              border: '1px solid rgba(6,182,212,0.3)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: '11px',
              fontWeight: '700',
              color: 'var(--accent)',
              flexShrink: 0,
            }}>
              {(username ?? 'A')[0].toUpperCase()}
            </div>
            <div style={{ overflow: 'hidden', flex: 1 }}>
              <div style={{ fontSize: '12px', fontWeight: '600', color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {username ?? 'Admin'}
              </div>
              <div style={{ fontSize: '10px', color: 'var(--text-muted)', fontFamily: 'var(--mono)' }}>
                {time.toTimeString().slice(0, 8)}
              </div>
            </div>
          </div>
          <button
            onClick={handleLogout}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              width: '100%',
              padding: '7px 10px',
              background: 'transparent',
              border: 'none',
              color: 'var(--red)',
              cursor: 'pointer',
              fontSize: '13px',
              fontWeight: '500',
              fontFamily: 'var(--font)',
              borderRadius: '8px',
              transition: 'background 0.12s',
            }}
            onMouseEnter={e => (e.currentTarget.style.background = 'var(--red-soft)')}
            onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
          >
            <IcoLogout />
            Sign out
          </button>
        </div>
      </div>

      {/* Main area */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        {/* Top bar */}
        <div style={{
          height: '44px',
          borderBottom: '1px solid var(--border)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-end',
          padding: '0 28px',
          background: 'var(--bg-surface)',
          flexShrink: 0,
          gap: '20px',
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '12px', color: 'var(--text-muted)' }}>
            <span style={{ width: '6px', height: '6px', borderRadius: '50%', background: 'var(--green)', display: 'inline-block' }} />
            All systems operational
          </div>
          <div style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
            {time.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
          </div>
        </div>

        {/* Page content */}
        <main style={{ flex: 1, overflow: 'auto', padding: '28px 32px' }}>
          {children}
        </main>
      </div>
    </div>
  )
}
