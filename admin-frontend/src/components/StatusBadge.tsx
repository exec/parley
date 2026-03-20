interface StatusBadgeProps {
  status: string
}

const statusMap: Record<string, { label: string; cls: string }> = {
  active:    { label: 'Active',     cls: 'badge badge-active' },
  banned:    { label: 'Banned',     cls: 'badge badge-banned' },
  open:      { label: 'Open',       cls: 'badge badge-open' },
  resolved:  { label: 'Resolved',   cls: 'badge badge-resolved' },
  dismissed: { label: 'Dismissed',  cls: 'badge badge-dismissed' },
  bot:       { label: 'Bot',        cls: 'badge badge-bot' },
  system:    { label: 'System',     cls: 'badge badge-system' },
}

export default function StatusBadge({ status }: StatusBadgeProps) {
  const cfg = statusMap[status.toLowerCase()] ?? { label: status, cls: 'badge badge-dismissed' }
  return <span className={cfg.cls}>{cfg.label}</span>
}
