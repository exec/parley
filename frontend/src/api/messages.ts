import { apiClient } from './client';
import { Message, ForwardedMessage } from './types';

export interface MessageVersion {
  id: string;
  message_id: string;
  content: string;
  edited_at: string;
}

export async function getMessageVersions(messageId: string): Promise<MessageVersion[]> {
  const res = await fetch(`/api/messages/${messageId}/versions`, {
    headers: { Authorization: `Bearer ${localStorage.getItem('token')}` },
  });
  if (!res.ok) throw new Error('Failed to fetch versions');
  return res.json();
}

export interface GetMessagesParams {
  limit?: number;
  offset?: number;
  before?: string;
  after?: string;
  around?: string;
}

export async function getMessages(
  channelId: string,
  params?: GetMessagesParams
): Promise<Message[]> {
  const queryParams = new URLSearchParams();

  if (params?.limit) {
    queryParams.append('limit', params.limit.toString());
  }
  if (params?.offset) {
    queryParams.append('offset', params.offset.toString());
  }
  if (params?.before) {
    queryParams.append('before', params.before);
  }
  if (params?.after) {
    queryParams.append('after', params.after);
  }
  if (params?.around) {
    queryParams.append('around', params.around);
  }

  const queryString = queryParams.toString();
  const endpoint = `/channels/${channelId}/messages${queryString ? `?${queryString}` : ''}`;

  return apiClient.get<Message[]>(endpoint);
}

export async function sendMessage(channelId: string, content: string, nonce?: string, attachmentUrl?: string, attachmentName?: string, attachmentType?: string, parentId?: string): Promise<Message> {
  return apiClient.post<Message>(`/channels/${channelId}/messages`, {
    content,
    nonce,
    attachment_url: attachmentUrl || '',
    attachment_name: attachmentName || '',
    attachment_type: attachmentType || '',
    ...(parentId ? { parent_id: parentId } : {}),
  });
}

export async function editMessage(id: string, content: string): Promise<Message> {
  return apiClient.put<Message>(`/messages/${id}`, {
    content,
  });
}

export async function deleteMessage(id: string): Promise<void> {
  return apiClient.delete<void>(`/messages/${id}`);
}

export async function getMessage(id: string): Promise<Message> {
  return apiClient.get<Message>(`/messages/${id}`);
}

export async function toggleReaction(messageId: string, emoji: string): Promise<void> {
  return apiClient.post<void>(`/messages/${messageId}/reactions`, { emoji });
}

export async function getPinnedMessages(channelId: string): Promise<Message[]> {
  return apiClient.get<Message[]>(`/channels/${channelId}/pins`);
}

export async function pinMessage(channelId: string, messageId: string): Promise<void> {
  return apiClient.post<void>(`/channels/${channelId}/pins/${messageId}`);
}

export async function unpinMessage(channelId: string, messageId: string): Promise<void> {
  return apiClient.delete<void>(`/channels/${channelId}/pins/${messageId}`);
}

export async function forwardToChannel(channelId: string, fwd: ForwardedMessage): Promise<Message> {
  return apiClient.post<Message>(`/channels/${channelId}/forward`, fwd);
}

export async function forwardToDm(dmChannelId: string, fwd: ForwardedMessage): Promise<unknown> {
  return apiClient.post<unknown>(`/dms/${dmChannelId}/forward`, fwd);
}