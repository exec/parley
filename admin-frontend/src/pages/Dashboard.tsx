import { useEffect, useState } from 'react'
import { apiGetStats, Stats } from '../api'

interface StatLine {
  key: keyof Stats
  label: string
  color?: string
  prefix?: string
}

const STAT_LINES: StatLine[] = [
  { key: 'total_users',    label: 'TOTAL USERS',     color: 'var(--green)' },
  { key: 'new_users_today', label: 'NEW TODAY',      color: 'var(--green)' },
  { key: 'banned_users',   label: 'BANNED USERS',    color: 'var(--red)' },
  { key: 'total_messages', label: 'TOTAL MESSAGES',  color: 'var(--green)' },
  { key: 'total_servers',  label: 'TOTAL SERVERS',   color: 'var(--green)' },
  { key: 'open_reports',   label: 'OPEN REPORTS',    color: 'var(--yellow)' },
]

function StatCard({
  label,
  value,
  color,
  warn,
}: {
  label: string
  value: number
  color: string
  warn?: boolean
}) {
  return (
    <div
      style={{
        background: 'var(--bg-secondary)',
        border: `1px solid ${warn && value > 0 ? color : 'var(--border)'}`,
        padding: '16px 20px',
        flex: '1 1 200px',
        minWidth: '180px',
        position: 'relative',
        overflow: 'hidden',
      }}
    >
      {warn && value > 0 && (
        <div
          style={{
            position: 'absolute',
            inset: 0,
            background: `${color}08`,
            pointerEvents: 'none',
          }}
        />
      )}
      <div
        style={{
          fontSize: '10px',
          color: 'var(--text-dim)',
          textTransform: 'uppercase',
          letterSpacing: '0.12em',
          marginBottom: '8px',
        }}
      >
        {label}
      </div>
      <div
        style={{
          fontSize: '28px',
          fontFamily: 'var(--font)',
          color,
          textShadow: `0 0 10px ${color}55`,
          letterSpacing: '0.05em',
        }}
      >
        {value.toLocaleString()}
      </div>
    </div>
  )
}

export default function Dashboard() {
  const [stats, setStats] = useState<Stats | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [bootLines, setBootLines] = useState<string[]>([])

  useEffect(() => {
    const lines = [
      'Initializing PARLEY admin console...',
      'Loading system modules...',
      'Establishing secure connection...',
      'Fetching system statistics...',
    ]
    let i = 0
    const timer = setInterval(() => {
      if (i < lines.length) {
        setBootLines((prev) => [...prev, lines[i]])
        i++
      } else {
        clearInterval(timer)
      }
    }, 180)
    return () => clearInterval(timer)
  }, [])

  useEffect(() => {
    apiGetStats()
      .then((s) => setStats(s))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  return (
    <div>
      <div className="page-header">
        <h1>SYSTEM STATUS</h1>
      </div>

      {/* Boot text */}
      <div
        style={{
          marginBottom: '24px',
          padding: '12px 16px',
          background: '#000',
          border: '1px solid var(--border)',
          fontSize: '11px',
          color: 'var(--green-dim)',
          lineHeight: '1.8',
        }}
      >
        {bootLines.map((line, i) => (
          <div key={i}>
            <span style={{ color: 'var(--green-dark)', marginRight: '8px' }}>[{String(i).padStart(2, '0')}]</span>
            {line}
            {i === bootLines.length - 1 && <span className="cursor-blink" />}
          </div>
        ))}
      </div>

      {error && <div className="alert alert-error">[ERROR] {error}</div>}

      {/* Terminal status output */}
      {!loading && stats && (
        <>
          <div
            style={{
              marginBottom: '24px',
              padding: '16px',
              background: '#000',
              border: '1px solid var(--border)',
              fontSize: '12px',
              lineHeight: '2',
              fontFamily: 'var(--font)',
            }}
          >
            <div style={{ color: 'var(--green-dim)', marginBottom: '8px' }}>
              $ parley-admin status --verbose
            </div>
            {STAT_LINES.map(({ key, label, color }) => {
              const val = stats[key]
              const isWarn = (key === 'open_reports' && val > 0) || (key === 'banned_users' && val > 0)
              return (
                <div key={key} style={{ display: 'flex', gap: '16px' }}>
                  <span
                    style={{
                      color: isWarn ? color : 'var(--green)',
                      minWidth: '36px',
                    }}
                  >
                    {isWarn ? '[!!]' : '[OK]'}
                  </span>
                  <span style={{ color: 'var(--text-dim)', minWidth: '160px' }}>
                    {label}
                  </span>
                  <span style={{ color: color ?? 'var(--green)', fontWeight: 'bold' }}>
                    {val.toLocaleString()}
                  </span>
                </div>
              )
            })}
          </div>

          {/* Stat cards */}
          <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap' }}>
            <StatCard label="Total Users" value={stats.total_users} color="var(--green)" />
            <StatCard label="New Today" value={stats.new_users_today} color="var(--green)" />
            <StatCard label="Banned Users" value={stats.banned_users} color="var(--red)" warn />
            <StatCard label="Total Messages" value={stats.total_messages} color="var(--green)" />
            <StatCard label="Total Servers" value={stats.total_servers} color="var(--green)" />
            <StatCard label="Open Reports" value={stats.open_reports} color="var(--yellow)" warn />
          </div>
        </>
      )}

      {loading && (
        <div style={{ textAlign: 'center', padding: '40px' }}>
          <span className="loading">LOADING SYSTEM DATA</span>
        </div>
      )}
    </div>
  )
}
