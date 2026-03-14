import { apiClient } from './client';
import { Server, ServerMember } from './types';

export interface Invite {
  id: string;
  server_id: string;
  code: string;
  created_by: string;
  created_at: string;
}

export async function getServers(): Promise<Server[]> {
  return apiClient.get<Server[]>('/servers');
}

export async function getServer(id: string): Promise<Server> {
  return apiClient.get<Server>(`/servers/${id}`);
}

export async function createServer(name: string, iconURL?: string): Promise<Server> {
  return apiClient.post<Server>('/servers', {
    name,
    icon_url: iconURL,
  });
}

export async function updateServer(
  id: string,
  name: string,
  iconURL?: string
): Promise<Server> {
  return apiClient.put<Server>(`/servers/${id}`, {
    name,
    icon_url: iconURL,
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
    user_id: userId,
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

export async function createInvite(serverId: string): Promise<Invite> {
  return apiClient.post<Invite>(`/servers/${serverId}/invites`, {});
}

export async function getInvite(code: string): Promise<{ invite: Invite; server: Server }> {
  return apiClient.get<{ invite: Invite; server: Server }>(`/invites/${code}`);
}

export async function joinServerByInvite(code: string): Promise<Server> {
  const response = await apiClient.get<{ server: Server; message?: string }>(`/invites/${code}`);
  return response.server;
}