import { apiClient } from './client';
import { AppNotification } from './types';

export async function getNotifications(limit = 50): Promise<AppNotification[]> {
  return apiClient.get<AppNotification[]>(`/notifications?limit=${limit}`);
}

export async function markAllRead(): Promise<void> {
  return apiClient.patch('/notifications/read-all');
}

export async function markRead(id: string): Promise<void> {
  return apiClient.patch(`/notifications/${id}/read`);
}
