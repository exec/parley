import React, { useEffect } from 'react'

interface ModalProps {
  title: string
  onClose: () => void
  children: React.ReactNode
  width?: number | string
}

export default function Modal({ title, onClose, children, width = 640 }: ModalProps) {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose])

  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(0,0,0,0.7)',
        backdropFilter: 'blur(4px)',
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: '24px',
        animation: 'fadeIn 0.12s ease',
      }}
      onClick={e => { if (e.target === e.currentTarget) onClose() }}
    >
      <style>{`@keyframes fadeIn { from { opacity: 0 } to { opacity: 1 } }
        @keyframes slideUp { from { opacity: 0; transform: translateY(12px) } to { opacity: 1; transform: translateY(0) } }`}
      </style>
      <div
        style={{
          background: 'var(--bg-surface)',
          border: '1px solid var(--border-bright)',
          borderRadius: '12px',
          width: typeof width === 'number' ? `${width}px` : width,
          maxWidth: '100%',
          maxHeight: '90vh',
          display: 'flex',
          flexDirection: 'column',
          boxShadow: 'var(--shadow-lg)',
          animation: 'slideUp 0.15s ease',
        }}
      >
        {/* Header */}
        <div style={{
          padding: '16px 20px',
          borderBottom: '1px solid var(--border)',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          flexShrink: 0,
        }}>
          <span style={{ fontSize: '15px', fontWeight: '600', color: 'var(--text)' }}>
            {title}
          </span>
          <button
            onClick={onClose}
            title="Close [ESC]"
            style={{
              background: 'var(--bg-elevated)',
              border: '1px solid var(--border)',
              color: 'var(--text-secondary)',
              cursor: 'pointer',
              width: '28px',
              height: '28px',
              borderRadius: '6px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: '16px',
              lineHeight: 1,
              transition: 'background 0.12s, color 0.12s',
            }}
            onMouseEnter={e => { e.currentTarget.style.background = 'var(--red-soft)'; e.currentTarget.style.color = 'var(--red)' }}
            onMouseLeave={e => { e.currentTarget.style.background = 'var(--bg-elevated)'; e.currentTarget.style.color = 'var(--text-secondary)' }}
          >
            ×
          </button>
        </div>
        {/* Body */}
        <div style={{ padding: '20px', overflowY: 'auto', flex: 1 }}>
          {children}
        </div>
      </div>
    </div>
  )
}
