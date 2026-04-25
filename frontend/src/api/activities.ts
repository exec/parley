import { apiClient } from './client';

export interface ActivityRecord {
  type: string;
  started_by: string;
  started_at_ms: number;
  params?: unknown;
}

export async function startActivity(vc: string, type: string, params?: unknown): Promise<void> {
  return apiClient.post<void>(`/voice/${vc}/activity/start`, { type, params });
}

export async function endActivity(vc: string): Promise<void> {
  return apiClient.post<void>(`/voice/${vc}/activity/end`, {});
}

export async function getActivity(vc: string): Promise<ActivityRecord | null> {
  try {
    const r = await apiClient.get<ActivityRecord>(`/voice/${vc}/activity`);
    return r ?? null;
  } catch (err) {
    console.error('getActivity failed', err);
    return null;
  }
}
