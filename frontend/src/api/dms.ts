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
  before?: string
): Promise<DmMessage[]> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (before) params.append('before', before);
  return apiClient.get<DmMessage[]>(`/dms/${dmChannelId}/messages?${params}`);
}

export async function sendDmMessage(
  dmChannelId: string,
  content: string,
  attachmentUrl?: string,
  attachmentName?: string,
  attachmentType?: string
): Promise<DmMessage> {
  return apiClient.post<DmMessage>(`/dms/${dmChannelId}/messages`, {
    content,
    attachment_url: attachmentUrl || '',
    attachment_name: attachmentName || '',
    attachment_type: attachmentType || '',
  });
}