import { apiClient } from './client';
import { Server, ServerMember } from './types';

export async function getServers(): Promise<Server[]> {
  return apiClient.get<Server[]>('/servers');
}

export async function getServer(id: string): Promise<Server> {
  return apiClient.get<Server>(`/servers/${id}`);
}

export async function createServer(name: string, iconURL?: string): Promise<Server> {
  return apiClient.post<Server>('/servers', {
    name,
    icon: iconURL,
  });
}

export async function updateServer(
  id: string,
  name: string,
  iconURL?: string
): Promise<Server> {
  return apiClient.put<Server>(`/servers/${id}`, {
    name,
    icon: iconURL,
  });
}

export async function deleteServer(id: string): Promise<void> {
  return apiClient.delete<void>(`/servers/${id}`);
}

export async function getMembers(serverId: string): Promise<ServerMember[]> {
  return apiClient.get<ServerMember[]>(`/servers/${serverId}/members`);
}

export async function addMember(
  serverId: string,
  userId: string,
  nickname?: string
): Promise<void> {
  return apiClient.post<void>(`/servers/${serverId}/members`, {
    userId,
    nickname,
  });
}

export async function removeMember(serverId: string, userId: string): Promise<void> {
  return apiClient.delete<void>(`/servers/${serverId}/members/${userId}`);
}

export async function updateMemberNickname(
  serverId: string,
  userId: string,
  nickname: string
): Promise<ServerMember> {
  return apiClient.put<ServerMember>(`/servers/${serverId}/members/${userId}`, {
    nickname,
  });
}