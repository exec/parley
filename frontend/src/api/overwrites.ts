import { apiClient } from './client';
import type { PermOverwrite } from '../lib/permissions';

export async function getOverwrites(channelId: string): Promise<PermOverwrite[]> {
  const data = await apiClient.get<any[]>(`/channels/${channelId}/overwrites`);
  // Convert allow/deny from number to bigint
  return (data ?? []).map((ow: any) => ({
    ...ow,
    id: String(ow.id),
    channel_id: String(ow.channel_id),
    target_id: String(ow.target_id),
    allow: BigInt(ow.allow),
    deny: BigInt(ow.deny),
  }));
}

export async function upsertOverwrite(channelId: string, data: {
  target_type: number; target_id: string; allow: bigint; deny: bigint;
}): Promise<void> {
  await apiClient.put<void>(`/channels/${channelId}/overwrites`, {
    ...data,
    allow: Number(data.allow),
    deny: Number(data.deny),
  });
}

export async function deleteOverwrite(channelId: string, overwriteId: string): Promise<void> {
  await apiClient.delete<void>(`/channels/${channelId}/overwrites/${overwriteId}`);
}

export async function getMyChannelPermissions(channelId: string): Promise<bigint> {
  const data = await apiClient.get<{ permissions: number }>(`/channels/${channelId}/my-permissions`);
  return BigInt(data.permissions);
}
