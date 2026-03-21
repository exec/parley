import { apiClient } from './client';
import { Server, ServerMember } from './types';

export interface Invite {
  id: string;
  server_id: string;
  code: string;
  created_by: string;
  creator_username: string;
  created_at: string;
  max_uses?: number;
  expires_at?: string;
  use_count: number;
  is_active: boolean;
}

export interface InviteMember {
  user_id: string;
  username: string;
  display_name: string;
  avatar_url: string;
  joined_at: string;
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
  iconURL?: string,
  description?: string,
  isPublic?: boolean,
): Promise<Server> {
  return apiClient.put<Server>(`/servers/${id}`, {
    name,
    icon_url: iconURL,
    description: description ?? '',
    is_public: isPublic ?? false,
  });
}

export async function deleteServer(id: string): Promise<void> {
  return apiClient.delete<void>(`/servers/${id}`);
}

export async function leaveServer(serverId: string): Promise<void> {
  return apiClient.delete<void>(`/servers/${serverId}/leave`);
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

export async function createInvite(serverId: string, options?: { max_uses?: number; expires_in?: string }): Promise<Invite> {
  return apiClient.post<Invite>(`/servers/${serverId}/invites`, options ?? {});
}

export async function listServerInvites(serverId: string): Promise<Invite[]> {
  return apiClient.get<Invite[]>(`/servers/${serverId}/invites`);
}

export async function revokeInvite(serverId: string, code: string): Promise<void> {
  return apiClient.delete<void>(`/servers/${serverId}/invites/${code}`);
}

export async function getInviteMembers(serverId: string, code: string): Promise<InviteMember[]> {
  return apiClient.get<InviteMember[]>(`/servers/${serverId}/invites/${code}/members`);
}

export async function getInvite(code: string): Promise<{ server: Server }> {
  return apiClient.get<{ server: Server }>(`/invites/${code}`);
}

export async function joinServerByInvite(code: string): Promise<Server> {
  const response = await apiClient.post<{ server: Server; message?: string }>(`/invites/${code}`, {});
  return response.server;
}

export async function setVanityURL(serverId: string, vanityUrl: string): Promise<Server> {
  return apiClient.put<Server>(`/servers/${serverId}/vanity`, { vanity_url: vanityUrl });
}

export async function kickMember(serverId: string, userId: string): Promise<void> {
  return apiClient.post<void>(`/servers/${serverId}/members/${userId}/kick`, {});
}

export async function banMember(serverId: string, userId: string, reason?: string): Promise<void> {
  return apiClient.post<void>(`/servers/${serverId}/members/${userId}/ban`, { reason: reason ?? '' });
}

export interface ServerBan {
  user_id: string;
  username: string;
  avatar_url: string;
  reason: string;
  banned_at: string;
}

export async function listServerBans(serverId: string): Promise<ServerBan[]> {
  return apiClient.get<ServerBan[]>(`/servers/${serverId}/bans`);
}

export async function unbanMember(serverId: string, userId: string): Promise<void> {
  return apiClient.delete<void>(`/servers/${serverId}/bans/${userId}`);
}

export async function getMyPermissions(serverId: string): Promise<number> {
  const result = await apiClient.get<{ permissions: number }>(`/servers/${serverId}/my-permissions`);
  return result.permissions;
}