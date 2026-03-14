import { apiClient } from './client';
import { PublicUser } from './types';

export async function searchUsers(query: string): Promise<PublicUser[]> {
  const queryString = `?q=${encodeURIComponent(query)}`;
  return apiClient.get<PublicUser[]>(`/users/search${queryString}`);
}

export async function getUser(userId: string): Promise<PublicUser> {
  return apiClient.get<PublicUser>(`/users/${userId}`);
}