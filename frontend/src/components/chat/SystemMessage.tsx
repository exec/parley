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

  const actor = nameOr(event.actor_display_name, event.actor_user_id);
  switch (event.type) {
    case 'group_created':
      return `${actor} created the group`;
    case 'member_added':
      return `${actor} added ${nameOr(event.target_display_name, event.target_user_id)}`;
    case 'member_left':
      return `${actor} left the group`;
    case 'member_kicked':
      return `${actor} removed ${nameOr(event.target_display_name, event.target_user_id)}`;
    case 'group_name_changed':
      return `${actor} renamed the group to "${event.new_name}"`;
    case 'owner_transferred':
      return `${actor} transferred ownership to ${nameOr(event.new_owner_display_name, event.new_owner_user_id)}`;
  }
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}
