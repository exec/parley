import { apiClient } from './client';

export interface RegistrationInvite {
  code: string;
  invitee_username?: string;
  created_at: string;
  used_at?: string;
}

export interface InvitesListResponse {
  invite_count: number;
  invites: RegistrationInvite[];
}

export async function listMyInvites(): Promise<InvitesListResponse> {
  return apiClient.get<InvitesListResponse>('/invites');
}

export async function createMyInvite(): Promise<{ code: string }> {
  return apiClient.post<{ code: string }>('/invites', {});
}
