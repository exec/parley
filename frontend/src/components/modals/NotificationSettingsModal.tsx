import React, { useState, useEffect } from 'react';
import './NotificationSettingsModal.css';

export const NOTIF_PREFS = {
  ALL: 'all',
  TAGS: 'tags',
  DIRECT: 'direct',
  NEVER: 'never',
} as const;
export type NotifPref = typeof NOTIF_PREFS[keyof typeof NOTIF_PREFS];

const STORAGE_KEY = 'parley_notification_prefs';

export function getNotifPref(serverId: string): NotifPref {
  try {
    const prefs = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
    return prefs[serverId] ?? 'tags';
  } catch {
    return 'tags';
  }
}

export function setNotifPref(serverId: string, pref: NotifPref): void {
  try {
    const prefs = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
    prefs[serverId] = pref;
    localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs));
  } catch {
    // ignore
  }
}

interface NotificationSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  serverId: string;
  serverName: string;
}

const OPTIONS: { value: NotifPref; label: string; description: string }[] = [
  {
    value: 'all',
    label: 'All Messages',
    description: 'You will be notified for every message sent in this server.',
  },
  {
    value: 'tags',
    label: 'All Tags',
    description: 'You will be notified for @everyone, @here, and direct @mentions.',
  },
  {
    value: 'direct',
    label: 'Direct Mentions Only',
    description: 'You will only be notified when someone directly @mentions you.',
  },
  {
    value: 'never',
    label: 'Never',
    description: 'You will receive no notifications from this server.',
  },
];

export const NotificationSettingsModal: React.FC<NotificationSettingsModalProps> = ({
  isOpen,
  onClose,
  serverId,
  serverName,
}) => {
  const [pref, setPref] = useState<NotifPref>('tags');

  useEffect(() => {
    if (isOpen) setPref(getNotifPref(serverId));
  }, [isOpen, serverId]);

  if (!isOpen) return null;

  const handleSave = () => {
    setNotifPref(serverId, pref);
    onClose();
  };

  return (
    <div className="notif-modal-overlay" onClick={onClose}>
      <div className="notif-modal" onClick={e => e.stopPropagation()}>
        <div className="notif-modal-header">
          <h2>Notification Settings</h2>
          <span className="notif-modal-server">{serverName}</span>
          <button className="notif-modal-close" onClick={onClose}>✕</button>
        </div>

        <div className="notif-modal-body">
          <p className="notif-modal-hint">
            Control which messages increment unread counts and trigger alerts for this server.
          </p>

          <div className="notif-options">
            {OPTIONS.map(opt => (
              <label key={opt.value} className={`notif-option${pref === opt.value ? ' selected' : ''}`}>
                <input
                  type="radio"
                  name="notif-pref"
                  value={opt.value}
                  checked={pref === opt.value}
                  onChange={() => setPref(opt.value)}
                />
                <div className="notif-option-text">
                  <span className="notif-option-label">{opt.label}</span>
                  <span className="notif-option-desc">{opt.description}</span>
                </div>
              </label>
            ))}
          </div>
        </div>

        <div className="notif-modal-footer">
          <button className="notif-btn-cancel" onClick={onClose}>Cancel</button>
          <button className="notif-btn-save" onClick={handleSave}>Save</button>
        </div>
      </div>
    </div>
  );
};
