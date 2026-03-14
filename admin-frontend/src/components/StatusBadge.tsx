interface StatusBadgeProps {
  status: string
}

const statusMap: Record<string, { label: string; cls: string }> = {
  active:    { label: 'ACTIVE',     cls: 'badge badge-active' },
  banned:    { label: 'BANNED',     cls: 'badge badge-banned' },
  open:      { label: 'OPEN',       cls: 'badge badge-open' },
  resolved:  { label: 'RESOLVED',   cls: 'badge badge-resolved' },
  dismissed: { label: 'DISMISSED',  cls: 'badge badge-dismissed' },
}

export default function StatusBadge({ status }: StatusBadgeProps) {
  const cfg = statusMap[status.toLowerCase()] ?? { label: status.toUpperCase(), cls: 'badge badge-resolved' }
  return <span className={cfg.cls}>[{cfg.label}]</span>
}
