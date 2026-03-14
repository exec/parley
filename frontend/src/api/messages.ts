import { apiClient } from './client';
import { Message } from './types';

export interface GetMessagesParams {
  limit?: number;
  offset?: number;
  before?: string;
  after?: string;
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

  const queryString = queryParams.toString();
  const endpoint = `/channels/${channelId}/messages${queryString ? `?${queryString}` : ''}`;

  return apiClient.get<Message[]>(endpoint);
}

export async function sendMessage(channelId: string, content: string): Promise<Message> {
  return apiClient.post<Message>(`/channels/${channelId}/messages`, {
    content,
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