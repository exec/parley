import { apiClient } from './client';
import { DmChannel, DmMessage } from './types';

export async function getDmChannels(): Promise<DmChannel[]> {
  return apiClient.get('/dms');
}

export async function openDmChannel(userId: string): Promise<DmChannel> {
  return apiClient.post('/dms', { user_id: userId });
}

export async function getDmMessages(dmId: string, limit = 50, offset = 0): Promise<DmMessage[]> {
  return apiClient.get(`/dms/${dmId}/messages?limit=${limit}&offset=${offset}`);
}

export async function sendDmMessage(dmId: string, content: string): Promise<DmMessage> {
  return apiClient.post(`/dms/${dmId}/messages`, { content });
}