import React from 'react'

export interface Column<T> {
  key: string
  header: string
  render?: (row: T) => React.ReactNode
  style?: React.CSSProperties
}

interface TableProps<T> {
  columns: Column<T>[]
  data: T[]
  keyField: keyof T
  loading?: boolean
  emptyMessage?: string
  onRowClick?: (row: T) => void
}

export default function Table<T>({
  columns,
  data,
  keyField,
  loading,
  emptyMessage = 'NO DATA',
  onRowClick,
}: TableProps<T>) {
  if (loading) {
    return (
      <div style={{ padding: '24px', textAlign: 'center' }}>
        <span className="loading">LOADING</span>
      </div>
    )
  }

  return (
    <div style={{ overflowX: 'auto' }}>
      <table className="data-table">
        <thead>
          <tr>
            {columns.map((col) => (
              <th key={col.key} style={col.style}>{col.header}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.length === 0 ? (
            <tr>
              <td
                colSpan={columns.length}
                style={{
                  textAlign: 'center',
                  padding: '32px',
                  color: 'var(--text-dim)',
                  letterSpacing: '0.1em',
                }}
              >
                [{emptyMessage}]
              </td>
            </tr>
          ) : (
            data.map((row) => (
              <tr
                key={String(row[keyField])}
                onClick={onRowClick ? () => onRowClick(row) : undefined}
                style={onRowClick ? { cursor: 'pointer' } : undefined}
              >
                {columns.map((col) => (
                  <td key={col.key} style={col.style}>
                    {col.render
                      ? col.render(row)
                      : String((row as Record<string, unknown>)[col.key] ?? '')}
                  </td>
                ))}
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  )
}
