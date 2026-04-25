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
  attachmentType?: string,
  parentId?: string
): Promise<DmMessage> {
  return apiClient.post<DmMessage>(`/dms/${dmChannelId}/messages`, {
    content,
    attachment_url: attachmentUrl || '',
    attachment_name: attachmentName || '',
    attachment_type: attachmentType || '',
    // Stringify — DmMessage.ID serializes as a JSON number from the Go side,
    // so without this the backend's *string Decode on parent_id 400s.
    parent_id: parentId != null ? String(parentId) : null,
  });
}

export async function deleteDmMessage(dmChannelId: string, messageId: string): Promise<void> {
  return apiClient.delete(`/dms/${dmChannelId}/messages/${messageId}`);
}

export async function toggleDmReaction(dmChannelId: string, messageId: string, emoji: string): Promise<void> {
  return apiClient.post(`/dms/${dmChannelId}/messages/${messageId}/reactions`, { emoji });
}

export async function createGroupDm(userIds: string[], name?: string): Promise<DmChannel> {
  return apiClient.post<DmChannel>('/dms', { user_ids: userIds, name });
}

export async function addDmMembers(dmChannelId: string, userIds: string[]): Promise<void> {
  await apiClient.post(`/dms/${dmChannelId}/members`, { user_ids: userIds });
}

export async function kickDmMember(dmChannelId: string, userId: string): Promise<void> {
  await apiClient.delete(`/dms/${dmChannelId}/members/${userId}`);
}

export async function leaveDm(dmChannelId: string, transferTo?: string): Promise<void> {
  await apiClient.post(`/dms/${dmChannelId}/leave`, transferTo ? { transfer_to: transferTo } : {});
}

export async function renameDmGroup(dmChannelId: string, name: string): Promise<void> {
  await apiClient.patch(`/dms/${dmChannelId}`, { name });
}

export async function setDmGroupAvatar(dmChannelId: string, avatarUrl: string): Promise<void> {
  await apiClient.patch(`/dms/${dmChannelId}`, { avatar_url: avatarUrl });
}

export async function clearDmGroupAvatar(dmChannelId: string): Promise<void> {
  await apiClient.patch(`/dms/${dmChannelId}`, { clear_avatar: true });
}

export async function transferDmOwnership(dmChannelId: string, userId: string): Promise<void> {
  await apiClient.post(`/dms/${dmChannelId}/transfer-ownership`, { new_owner_id: userId });
}