import { apiClient } from './client';
import { DmChannel, DmMessage } from './types';

export async function getDmChannels(): Promise<DmChannel[]> {
  return apiClient.get<DmChannel[]>('/dms');
}

export async function openDmChannel(userId: string): Promise<DmChannel> {
  return apiClient.post<DmChannel>('/dms', { user_id: userId });
}

export async function getDmMessages(
  dmChannelId: string,
  limit = 50,
  offset = 0
): Promise<DmMessage[]> {
  const queryString = `?limit=${limit}&offset=${offset}`;
  return apiClient.get<DmMessage[]>(`/dms/${dmChannelId}/messages${queryString}`);
}

export async function sendDmMessage(
  dmChannelId: string,
  content: string,
  attachmentUrl?: string
): Promise<DmMessage> {
  return apiClient.post<DmMessage>(`/dms/${dmChannelId}/messages`, {
    content,
    attachment_url: attachmentUrl || '',
  });
}