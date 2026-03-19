import { apiClient } from './client';

export type StatusType = 'online' | 'dnd' | 'afk' | 'invisible';

export async function updateStatus(statusType: StatusType, statusText: string): Promise<void> {
  await apiClient.patch('/users/@me/status', { status_type: statusType, status_text: statusText });
}
