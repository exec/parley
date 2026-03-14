import React, { useEffect } from 'react'

interface ModalProps {
  title: string
  onClose: () => void
  children: React.ReactNode
  width?: number | string
}

export default function Modal({ title, onClose, children, width = 640 }: ModalProps) {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose])

  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(0,0,0,0.82)',
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: '20px',
      }}
      onClick={(e) => { if (e.target === e.currentTarget) onClose() }}
    >
      <div
        style={{
          background: 'var(--bg-secondary)',
          border: '1px solid var(--green-dark)',
          width: typeof width === 'number' ? `${width}px` : width,
          maxWidth: '100%',
          maxHeight: '90vh',
          display: 'flex',
          flexDirection: 'column',
          boxShadow: '0 0 30px rgba(50,205,50,0.12)',
        }}
      >
        {/* Modal header */}
        <div
          style={{
            padding: '10px 16px',
            borderBottom: '1px solid var(--green-dark)',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            background: 'var(--bg)',
          }}
        >
          <span
            style={{
              fontSize: '12px',
              textTransform: 'uppercase',
              letterSpacing: '0.1em',
              color: 'var(--green)',
            }}
          >
            // {title}
          </span>
          <button
            onClick={onClose}
            style={{
              background: 'none',
              border: 'none',
              color: 'var(--text-dim)',
              cursor: 'pointer',
              fontSize: '16px',
              fontFamily: 'var(--font)',
              padding: '0 4px',
              lineHeight: 1,
            }}
            title="Close [ESC]"
          >
            [X]
          </button>
        </div>
        {/* Modal body */}
        <div style={{ padding: '16px', overflowY: 'auto', flex: 1 }}>
          {children}
        </div>
      </div>
    </div>
  )
}
