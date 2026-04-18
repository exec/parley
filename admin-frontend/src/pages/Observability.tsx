// Embedded Grafana panels scoped to Parley logs only.
//
// Grafana is reverse-proxied by parley-admin's nginx at /grafana/ so only one
// SSH tunnel is needed:
//
//   ssh -L 8081:10.10.10.15:80 eqr      # admin nginx (this page + /grafana/)
//
// Scope is enforced server-side via the `parley-embed` dashboard (only
// {host=~"parley-.*"} streams), so there's no way for DMZ / gitwise / wg-vpn
// traffic to leak through this page.

const GRAFANA_URL =
  (import.meta.env.VITE_GRAFANA_URL as string) || '/grafana'
const DASH = 'parley-embed'

function panelSrc(id: number, extra: Record<string, string> = {}): string {
  const params = new URLSearchParams({
    panelId: String(id),
    theme: 'dark',
    from: 'now-1h',
    to: 'now',
    kiosk: '',
    orgId: '1',
    refresh: '10s',
    ...extra,
  })
  return `${GRAFANA_URL}/d-solo/${DASH}/${DASH}?${params.toString()}`
}

function PanelFrame({
  title,
  panelId,
  height,
  range,
}: {
  title: string
  panelId: number
  height: number
  range?: { from: string; to: string }
}) {
  const src = panelSrc(panelId, range ?? {})
  return (
    <div
      style={{
        background: 'var(--panel, #111)',
        borderRadius: 10,
        padding: 12,
        border: '1px solid rgba(255,255,255,0.06)',
      }}
    >
      <div
        style={{
          fontSize: 13,
          fontWeight: 600,
          color: 'var(--text-secondary, #aaa)',
          marginBottom: 8,
        }}
      >
        {title}
      </div>
      <iframe
        src={src}
        width="100%"
        height={height}
        frameBorder="0"
        style={{ borderRadius: 6, background: '#000' }}
        title={title}
      />
    </div>
  )
}

export default function Observability() {
  return (
    <div style={{ padding: 20, maxWidth: 1400, margin: '0 auto' }}>
      <h1 style={{ fontSize: 22, marginBottom: 4 }}>Observability</h1>
      <p style={{ color: 'var(--text-secondary, #888)', fontSize: 13, marginBottom: 20 }}>
        Live Parley logs and account-level security events, scoped to{' '}
        <code>{'{host=~"parley-.*"}'}</code> only. Grafana is reverse-proxied
        through the admin nginx at <code>/grafana</code> — the same SSH tunnel
        you used to reach this page already covers it.
      </p>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr', gap: 16 }}>
        <PanelFrame title="Live Parley logs" panelId={1} height={420} />
        <PanelFrame title="Audit events (login / ban / rate-limited)" panelId={2} height={360} />
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
          <PanelFrame
            title="Top spammy accounts (last 24h)"
            panelId={3}
            height={280}
            range={{ from: 'now-24h', to: 'now' }}
          />
          <PanelFrame
            title="Ban-evasion attempts (last 24h)"
            panelId={4}
            height={280}
            range={{ from: 'now-24h', to: 'now' }}
          />
        </div>
      </div>
    </div>
  )
}
