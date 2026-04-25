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
  const actor = resolve(event.actor_user_id).displayName;
  switch (event.type) {
    case 'group_created':
      return `${actor} created the group`;
    case 'member_added':
      return `${actor} added ${resolve(event.target_user_id).displayName}`;
    case 'member_left':
      return `${actor} left the group`;
    case 'member_kicked':
      return `${actor} removed ${resolve(event.target_user_id).displayName}`;
    case 'group_name_changed':
      return `${actor} renamed the group to "${event.new_name}"`;
    case 'owner_transferred':
      return `${actor} transferred ownership to ${resolve(event.new_owner_user_id).displayName}`;
  }
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}
