import { apiClient } from './client';
import { Role } from './types';

export async function getServerRoles(serverId: string): Promise<Role[]> {
  return apiClient.get<Role[]>(`/servers/${serverId}/roles`);
}

export async function createServerRole(serverId: string, name: string, color: string, permissions = 0): Promise<Role> {
  return apiClient.post<Role>(`/servers/${serverId}/roles`, { name, color, permissions });
}

export async function deleteServerRole(serverId: string, roleId: string): Promise<void> {
  return apiClient.delete<void>(`/servers/${serverId}/roles/${roleId}`);
}

export async function updateServerRole(serverId: string, roleId: string, name: string, color: string, permissions: number, hoist: boolean, position: number): Promise<Role> {
  return apiClient.patch<Role>(`/servers/${serverId}/roles/${roleId}`, { name, color, permissions, hoist, position });
}

export async function getMemberRoles(serverId: string, userId: string): Promise<Role[]> {
  return apiClient.get<Role[]>(`/servers/${serverId}/members/${userId}/roles`);
}

export async function assignRoleToMember(serverId: string, userId: string, roleId: string): Promise<void> {
  return apiClient.post<void>(`/servers/${serverId}/members/${userId}/roles`, { role_id: roleId });
}

export async function removeRoleFromMember(serverId: string, userId: string, roleId: string): Promise<void> {
  return apiClient.delete<void>(`/servers/${serverId}/members/${userId}/roles/${roleId}`);
}
