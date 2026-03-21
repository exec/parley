import { apiClient } from './client';
import { ServerCategory, PublicServer } from './types';

export async function getServerCategories(): Promise<ServerCategory[]> {
  return apiClient.get<ServerCategory[]>('/server-categories');
}

export async function discoverServers(params: {
  page?: number;
  categoryId?: number;
  q?: string;
} = {}): Promise<{ servers: PublicServer[]; total: number }> {
  const search = new URLSearchParams();
  if (params.page) search.set('page', String(params.page));
  if (params.categoryId != null) search.set('category_id', String(params.categoryId));
  if (params.q) search.set('q', params.q);
  const qs = search.toString();
  return apiClient.get<{ servers: PublicServer[]; total: number }>(
    `/discover${qs ? '?' + qs : ''}`,
  );
}

export async function getServerCategoryAssignments(serverId: string): Promise<ServerCategory[]> {
  return apiClient.get<ServerCategory[]>(`/servers/${serverId}/categories`);
}

export async function setServerCategories(serverId: string, categoryIds: number[]): Promise<ServerCategory[]> {
  return apiClient.put<ServerCategory[]>(`/servers/${serverId}/categories`, {
    category_ids: categoryIds,
  });
}
