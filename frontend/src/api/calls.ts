import { apiClient } from './client';

export interface RingCaller {
  user_id: number;
  username: string;
  display_name: string;
  avatar_url: string;
}

export interface ActiveRing {
  ring_id: string;
  vc: string;
  caller: RingCaller;
  started_at_ms: number;
}

export interface InCallEntry {
  dm_channel_id: string;
  participant_count: number;
}

export interface ActiveCallsResponse {
  rings: ActiveRing[];
  in_call: InCallEntry[];
}

export async function ringDm(dmChannelId: string | number): Promise<{ ring_id: string }> {
  return apiClient.post<{ ring_id: string }>(`/dms/${dmChannelId}/call/ring`, {});
}

export async function startGcCall(dmChannelId: string | number): Promise<void> {
  return apiClient.post<void>(`/dms/${dmChannelId}/call/start`, {});
}

export async function acceptCall(dmChannelId: string | number, ringId: string): Promise<void> {
  return apiClient.post<void>(`/dms/${dmChannelId}/call/accept`, { ring_id: ringId });
}

export async function declineCall(dmChannelId: string | number, ringId: string): Promise<void> {
  return apiClient.post<void>(`/dms/${dmChannelId}/call/decline`, { ring_id: ringId });
}

export async function cancelCall(dmChannelId: string | number, ringId: string): Promise<void> {
  return apiClient.post<void>(`/dms/${dmChannelId}/call/cancel`, { ring_id: ringId });
}

export async function getActiveCalls(): Promise<ActiveCallsResponse> {
  return apiClient.get<ActiveCallsResponse>('/calls/active');
}
