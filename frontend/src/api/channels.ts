import { apiClient } from './client';
import { Channel } from './types';

export async function getChannels(serverId: string): Promise<Channel[]> {
  return apiClient.get<Channel[]>(`/servers/${serverId}/channels`);
}

export async function createChannel(
  serverId: string,
  name: string,
  type: number
): Promise<Channel> {
  return apiClient.post<Channel>(`/servers/${serverId}/channels`, {
    name,
    type,
  });
}

export async function updateChannel(id: string, name: string): Promise<Channel> {
  return apiClient.put<Channel>(`/channels/${id}`, {
    name,
  });
}

export async function deleteChannel(id: string): Promise<void> {
  return apiClient.delete<void>(`/channels/${id}`);
}

export async function getChannel(id: string): Promise<Channel> {
  return apiClient.get<Channel>(`/channels/${id}`);
}