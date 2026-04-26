import React from 'react';
import type { SystemEvent } from '../../api/types';
import './SystemMessage.css';

interface Props {
  event: SystemEvent;
  resolveUser: (userId: string) => { displayName: string };
  createdAt: string;
}

export const SystemMessage: React.FC<Props> = ({ event, resolveUser, createdAt }) => {
  const text = renderEvent(event, resolveUser);
  return (
    <div className="system-message" role="note">
      <span className="system-message-text">{text}</span>
      <span className="system-message-time">{formatTime(createdAt)}</span>
    </div>
  );
};

function renderEvent(
  event: SystemEvent,
  resolve: (id: string) => { displayName: string }
): string {
  // Prefer the snapshotted display name in the event payload (always up to
  // date for new events); fall back to live resolution for older events
  // emitted before names were embedded.
  const nameOr = (embedded: string | undefined, userId: string) =>
    embedded && embedded.length > 0 ? embedded : resolve(userId).displayName;

  switch (event.type) {
    case 'group_created': {
      const actor = nameOr(event.actor_display_name, event.actor_user_id);
      return `${actor} created the group`;
    }
    case 'member_added': {
      const actor = nameOr(event.actor_display_name, event.actor_user_id);
      return `${actor} added ${nameOr(event.target_display_name, event.target_user_id)}`;
    }
    case 'member_left': {
      const actor = nameOr(event.actor_display_name, event.actor_user_id);
      return `${actor} left the group`;
    }
    case 'member_kicked': {
      const actor = nameOr(event.actor_display_name, event.actor_user_id);
      return `${actor} removed ${nameOr(event.target_display_name, event.target_user_id)}`;
    }
    case 'group_name_changed': {
      const actor = nameOr(event.actor_display_name, event.actor_user_id);
      return `${actor} renamed the group to "${event.new_name}"`;
    }
    case 'owner_transferred': {
      const actor = nameOr(event.actor_display_name, event.actor_user_id);
      return `${actor} transferred ownership to ${nameOr(event.new_owner_display_name, event.new_owner_user_id)}`;
    }
    case 'call_started':
      return `${nameOr(event.actor_display_name, event.actor_user_id)} started a call.`;
    case 'call_ended':
      return `Call ended · ${formatDuration(event.duration_ms)}`;
    case 'call_missed':
      return `Missed call from ${nameOr(event.caller_display_name, event.caller_user_id)}.`;
    case 'call_declined':
      return `${nameOr(event.decliner_display_name, event.decliner_user_id)} declined the call.`;
  }
}

function formatDuration(ms: number): string {
  const totalSec = Math.max(0, Math.round(ms / 1000));
  const m = Math.floor(totalSec / 60);
  const s = totalSec % 60;
  if (m === 0) return `${s}s`;
  return `${m}m ${s}s`;
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}
