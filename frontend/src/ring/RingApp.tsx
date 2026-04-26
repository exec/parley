import React, { useEffect } from 'react';
import { Phone, PhoneOff } from 'lucide-react';
import { emit, listen } from '@tauri-apps/api/event';
import { getCurrentWindow } from '@tauri-apps/api/window';

interface Props {
  ringId: string;
  vc: string;
  callerUsername: string;
  callerDisplayName: string;
  callerAvatarUrl: string;
  groupName: string;
}

export const RingApp: React.FC<Props> = ({ ringId, callerUsername, callerDisplayName, callerAvatarUrl, groupName }) => {
  const name = callerDisplayName || callerUsername;

  useEffect(() => {
    // If the main window resolves the ring elsewhere, it emits "ring:dismiss" with our id.
    const unsub = listen<{ ring_id: string }>('ring:dismiss', e => {
      if (e.payload.ring_id === ringId) {
        getCurrentWindow().close().catch(() => {});
      }
    });
    return () => { unsub.then(fn => fn()).catch(() => {}); };
  }, [ringId]);

  const accept = async () => {
    await emit('ring:accept', { ring_id: ringId });
    await getCurrentWindow().close();
  };
  const decline = async () => {
    await emit('ring:decline', { ring_id: ringId });
    await getCurrentWindow().close();
  };

  return (
    <div className="ring-app">
      {callerAvatarUrl
        ? <img src={callerAvatarUrl} alt="" className="ring-avatar" />
        : <div className="ring-avatar ring-avatar--placeholder" />}
      <p className="ring-text">
        Incoming call from <strong>{name}</strong>{groupName ? <> in <strong>{groupName}</strong></> : null}…
      </p>
      <div className="ring-buttons">
        <button className="ring-btn ring-btn--decline" onClick={decline} aria-label="Decline call">
          <PhoneOff size={20} /> Decline
        </button>
        <button className="ring-btn ring-btn--accept" onClick={accept} aria-label="Accept call">
          <Phone size={20} /> Accept
        </button>
      </div>
    </div>
  );
};
