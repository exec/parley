import { Message } from './types';
import { apiClient } from './client';

export interface SearchParams {
  q?: string;
  from?: string;   // userID
  in?: string;     // channelID
  limit?: number;
  before?: string; // messageID cursor
}

export async function searchMessages(serverId: string, params: SearchParams): Promise<Message[]> {
  const query = new URLSearchParams();
  if (params.q) query.set('q', params.q);
  if (params.from) query.set('from', params.from);
  if (params.in) query.set('in', params.in);
  if (params.limit) query.set('limit', String(params.limit));
  if (params.before) query.set('before', params.before);

  return apiClient.get<Message[]>(`/servers/${serverId}/messages/search?${query}`);
}
