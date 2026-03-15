import { apiClient } from './client';
import { Channel } from './types';

export async function getChannels(serverId: string): Promise<Channel[]> {
  return apiClient.get<Channel[]>(`/servers/${serverId}/channels`);
}

export async function createChannel(
  serverId: string,
  name: string,
  type: number,
  topic?: string
): Promise<Channel> {
  return apiClient.post<Channel>(`/servers/${serverId}/channels`, {
    name,
    type,
    ...(topic ? { topic } : {}),
  });
}

export async function updateChannel(id: string, name: string, topic?: string): Promise<Channel> {
  return apiClient.put<Channel>(`/channels/${id}`, {
    name,
    ...(topic !== undefined ? { topic } : {}),
  });
}

export async function deleteChannel(id: string): Promise<void> {
  return apiClient.delete<void>(`/channels/${id}`);
}

export async function getChannel(id: string): Promise<Channel> {
  return apiClient.get<Channel>(`/channels/${id}`);
}
export interface ChannelOrder {
  id: string;
  position: number;
  parent_id: string | null;
}

export async function reorderChannels(serverId: string, orders: ChannelOrder[]): Promise<Channel[]> {
  return apiClient.patch<Channel[]>(`/servers/${serverId}/channels/reorder`, orders);
}
