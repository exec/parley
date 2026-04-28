import { apiClient } from './client';

export interface APIKeyInfo {
  id: number;
  key_prefix: string;
  user_id: number;
  owner_id: number;
  name: string;
  is_bot: boolean;
  bot_username?: string;
  created_at: string;
  last_used_at?: string;
}

export interface CreateKeyResponse {
  id: number;
  key: string;
  key_prefix: string;
  name: string;
  type: 'bot' | 'user';
  bot_username?: string;
  bot_user_id?: number;
}

export async function listAPIKeys(): Promise<APIKeyInfo[]> {
  return apiClient.get<APIKeyInfo[]>('/developer/keys');
}

export const KNOWN_SCOPES = [
  'full',
  'messages:read',
  'messages:write',
  'commands:write',
  'interactions:respond',
  'profile:write',
  'servers:read',
  'developer:manage',
] as const;

export async function createAPIKey(
  type: 'bot' | 'user',
  name: string,
  botUsername?: string,
  scopes: string[] = ['full'],
): Promise<CreateKeyResponse> {
  return apiClient.post<CreateKeyResponse>('/developer/keys', {
    type,
    name,
    bot_username: botUsername,
    scopes,
  });
}

export async function revokeAPIKey(id: number): Promise<void> {
  return apiClient.delete<void>(`/developer/keys/${id}`);
}

export async function renameBotUser(botId: number, username: string): Promise<void> {
  return apiClient.patch<void>(`/developer/bots/${botId}`, { username });
}
