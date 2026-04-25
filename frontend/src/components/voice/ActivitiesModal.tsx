import React from 'react';
import { list } from '../../activities/registry';
import { startActivity } from '../../api/activities';

interface Props {
  vc: string;
  open: boolean;
  onClose: () => void;
}

export const ActivitiesModal: React.FC<Props> = ({ vc, open, onClose }) => {
  if (!open) return null;
  const items = list();
  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <h2 className="modal-title">Activities</h2>
          <button className="modal-close" onClick={onClose} aria-label="Close">✕</button>
        </div>
        <div className="modal-body">
          {items.length === 0 ? (
            <p style={{ color: 'var(--parley-text-muted)' }}>Activities coming soon.</p>
          ) : (
            <ul style={{ listStyle: 'none', padding: 0, margin: 0 }}>
              {items.map(it => (
                <li key={it.type}>
                  <button
                    style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%', padding: '8px 0', background: 'none', border: 'none', color: 'var(--parley-text-normal)', cursor: 'pointer', fontSize: 14 }}
                    onClick={async () => { await startActivity(vc, it.type); onClose(); }}
                  >
                    {it.icon}
                    {it.displayName}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
        <div className="modal-actions">
          <button className="modal-btn" onClick={onClose}>Close</button>
        </div>
      </div>
    </div>
  );
};
