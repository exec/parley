import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiGetStats, Stats } from '../api'

interface StatDef {
  key: keyof Stats
  label: string
  color: string
  bgColor: string
  icon: string
  link?: string
  warnIfPositive?: boolean
}

const STATS: StatDef[] = [
  { key: 'total_users',     label: 'Total Users',     color: 'var(--accent)',  bgColor: 'var(--accent-soft)',  icon: '👥', link: '/users' },
  { key: 'new_users_today', label: 'New Today',       color: 'var(--green)',   bgColor: 'var(--green-soft)',   icon: '✨' },
  { key: 'total_servers',   label: 'Servers',         color: 'var(--accent)',  bgColor: 'var(--accent-soft)',  icon: '🌐', link: '/servers' },
  { key: 'total_messages',  label: 'Messages',        color: 'var(--purple)',  bgColor: 'var(--purple-soft)',  icon: '💬', link: '/messages' },
  { key: 'open_reports',    label: 'Open Reports',    color: 'var(--yellow)',  bgColor: 'var(--yellow-soft)',  icon: '🚩', link: '/reports', warnIfPositive: true },
  { key: 'banned_users',    label: 'Banned Users',    color: 'var(--red)',     bgColor: 'var(--red-soft)',     icon: '🚫', link: '/users', warnIfPositive: true },
]

function StatCard({ def, value, onClick }: { def: StatDef; value: number; onClick?: () => void }) {
  const isWarn = def.warnIfPositive && value > 0
  return (
    <div
      onClick={onClick}
      style={{
        background: 'var(--bg-surface)',
        border: `1px solid ${isWarn ? (def.color + '40') : 'var(--border)'}`,
        borderRadius: '12px',
        padding: '20px 22px',
        flex: '1 1 160px',
        minWidth: '150px',
        cursor: onClick ? 'pointer' : 'default',
        transition: 'border-color 0.15s, transform 0.12s',
        position: 'relative',
        overflow: 'hidden',
      }}
      onMouseEnter={e => { if (onClick) { (e.currentTarget as HTMLDivElement).style.transform = 'translateY(-1px)'; (e.currentTarget as HTMLDivElement).style.borderColor = def.color + '66' } }}
      onMouseLeave={e => { (e.currentTarget as HTMLDivElement).style.transform = ''; (e.currentTarget as HTMLDivElement).style.borderColor = isWarn ? def.color + '40' : 'var(--border)' }}
    >
      {isWarn && (
        <div style={{
          position: 'absolute',
          inset: 0,
          background: `${def.bgColor}`,
          opacity: 0.5,
          pointerEvents: 'none',
        }} />
      )}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '12px', position: 'relative' }}>
        <div style={{
          width: '36px',
          height: '36px',
          borderRadius: '9px',
          background: def.bgColor,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontSize: '16px',
        }}>
          {def.icon}
        </div>
        {isWarn && value > 0 && (
          <span style={{
            fontSize: '10px',
            fontWeight: '600',
            color: def.color,
            background: def.bgColor,
            padding: '2px 7px',
            borderRadius: '100px',
            border: `1px solid ${def.color}40`,
          }}>
            Action needed
          </span>
        )}
      </div>
      <div style={{ fontSize: '11px', fontWeight: '600', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '6px', position: 'relative' }}>
        {def.label}
      </div>
      <div style={{ fontSize: '28px', fontWeight: '700', color: def.color, letterSpacing: '-0.02em', position: 'relative' }}>
        {value.toLocaleString()}
      </div>
    </div>
  )
}

export default function Dashboard() {
  const navigate = useNavigate()
  const [stats, setStats] = useState<Stats | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    apiGetStats()
      .then(s => setStats(s))
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  return (
    <div>
      <div className="page-header">
        <h1>Dashboard</h1>
        <p style={{ color: 'var(--text-secondary)', fontSize: '14px', marginTop: '4px' }}>
          Platform overview and key metrics
        </p>
      </div>

      {error && <div className="alert alert-error">{error}</div>}

      {loading ? (
        <div style={{ display: 'flex', gap: '14px', flexWrap: 'wrap' }}>
          {STATS.map(def => (
            <div key={def.key} style={{
              background: 'var(--bg-surface)',
              border: '1px solid var(--border)',
              borderRadius: '12px',
              padding: '20px 22px',
              flex: '1 1 160px',
              minWidth: '150px',
              height: '118px',
              animation: 'pulse 1.5s ease-in-out infinite',
            }} />
          ))}
          <style>{`@keyframes pulse { 0%,100% { opacity:.5 } 50% { opacity:.85 } }`}</style>
        </div>
      ) : stats && (
        <>
          {/* Stat cards */}
          <div style={{ display: 'flex', gap: '14px', flexWrap: 'wrap', marginBottom: '28px' }}>
            {STATS.map(def => (
              <StatCard
                key={def.key}
                def={def}
                value={stats[def.key]}
                onClick={def.link ? () => navigate(def.link!) : undefined}
              />
            ))}
          </div>

          {/* Quick actions */}
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', gap: '16px' }}>
            <div className="card">
              <div className="card-title">Quick Navigation</div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                {[
                  { label: 'Review open reports', link: '/reports', badge: stats.open_reports, color: 'var(--yellow)' },
                  { label: 'Manage users', link: '/users', badge: undefined, color: 'var(--accent)' },
                  { label: 'Search messages', link: '/messages', badge: undefined, color: 'var(--purple)' },
                  { label: 'Manage servers', link: '/servers', badge: undefined, color: 'var(--accent)' },
                  { label: 'Platform bots', link: '/bots', badge: undefined, color: 'var(--purple)' },
                ].map(item => (
                  <button
                    key={item.link}
                    onClick={() => navigate(item.link)}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      padding: '9px 12px',
                      background: 'var(--bg-elevated)',
                      border: '1px solid var(--border)',
                      borderRadius: '7px',
                      color: 'var(--text-secondary)',
                      fontSize: '13px',
                      fontWeight: '500',
                      fontFamily: 'var(--font)',
                      cursor: 'pointer',
                      transition: 'background 0.12s, color 0.12s',
                      textAlign: 'left',
                    }}
                    onMouseEnter={e => { e.currentTarget.style.background = 'var(--bg-overlay)'; e.currentTarget.style.color = 'var(--text)' }}
                    onMouseLeave={e => { e.currentTarget.style.background = 'var(--bg-elevated)'; e.currentTarget.style.color = 'var(--text-secondary)' }}
                  >
                    <span>{item.label}</span>
                    {item.badge !== undefined && item.badge > 0 && (
                      <span style={{
                        background: 'var(--yellow-soft)',
                        color: 'var(--yellow)',
                        fontSize: '11px',
                        fontWeight: '700',
                        padding: '1px 7px',
                        borderRadius: '100px',
                      }}>
                        {item.badge}
                      </span>
                    )}
                  </button>
                ))}
              </div>
            </div>

            <div className="card">
              <div className="card-title">Platform Health</div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                {[
                  { label: 'Open reports', value: stats.open_reports, max: 20, color: stats.open_reports > 0 ? 'var(--yellow)' : 'var(--green)', status: stats.open_reports === 0 ? 'All clear' : `${stats.open_reports} pending` },
                  { label: 'Banned users', value: stats.banned_users, max: 50, color: stats.banned_users > 10 ? 'var(--red)' : 'var(--accent)', status: `${stats.banned_users.toLocaleString()} accounts` },
                  { label: 'User growth', value: stats.new_users_today, max: 100, color: 'var(--green)', status: `+${stats.new_users_today} today` },
                ].map(item => (
                  <div key={item.label}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '5px' }}>
                      <span style={{ fontSize: '12px', color: 'var(--text-secondary)', fontWeight: '500' }}>{item.label}</span>
                      <span style={{ fontSize: '12px', color: item.color, fontWeight: '600' }}>{item.status}</span>
                    </div>
                    <div style={{ height: '4px', background: 'var(--bg-elevated)', borderRadius: '2px', overflow: 'hidden' }}>
                      <div style={{
                        height: '100%',
                        width: `${Math.min(100, (item.value / item.max) * 100)}%`,
                        background: item.color,
                        borderRadius: '2px',
                        transition: 'width 0.6s ease',
                      }} />
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
