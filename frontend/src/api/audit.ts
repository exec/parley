import { apiClient } from './client';

export interface AuditLogEntry {
  id: string;
  server_id: string;
  actor_id?: string;
  actor_username: string;
  actor_avatar_url?: string;
  action: string;
  target_id?: string;
  target_type?: string;
  target_name?: string;
  target_avatar_url?: string;
  changes?: Record<string, unknown>;
  reason?: string;
  created_at: string;
}

export async function getAuditLog(
  serverId: string,
  params: { limit?: number; offset?: number; action?: string; actorId?: string; target?: string }
): Promise<{ logs: AuditLogEntry[]; total: number }> {
  const qs = new URLSearchParams();
  if (params.limit !== undefined) qs.set('limit', String(params.limit));
  if (params.offset !== undefined) qs.set('offset', String(params.offset));
  if (params.action !== undefined && params.action !== '') qs.set('action', params.action);
  if (params.actorId !== undefined && params.actorId !== '') qs.set('actor_id', params.actorId);
  if (params.target !== undefined && params.target !== '') qs.set('target', params.target);
  const queryString = qs.toString();
  const url = `/servers/${serverId}/audit-log${queryString ? `?${queryString}` : ''}`;
  return apiClient.get<{ logs: AuditLogEntry[]; total: number }>(url);
}
