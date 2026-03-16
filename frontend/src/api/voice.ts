import { apiClient } from './client';

export interface VoiceToken {
  token: string;
  url: string;
}

export interface VoiceParticipant {
  user_id: string;
  username: string;
  avatar_url?: string;
}

export async function getVoiceToken(channelId: string): Promise<VoiceToken> {
  return apiClient.get<VoiceToken>(`/channels/${channelId}/voice/token`);
}

export async function joinVoiceChannel(channelId: string): Promise<void> {
  return apiClient.post<void>(`/channels/${channelId}/voice/join`, {});
}

export async function leaveVoiceChannel(channelId: string): Promise<void> {
  return apiClient.post<void>(`/channels/${channelId}/voice/leave`, {});
}

export async function getVoiceParticipants(channelId: string): Promise<VoiceParticipant[]> {
  return apiClient.get<VoiceParticipant[]>(`/channels/${channelId}/voice/participants`);
}

export async function muteVoiceParticipant(channelId: string, targetUserId: string): Promise<void> {
  return apiClient.post<void>(`/channels/${channelId}/voice/participants/${targetUserId}/mute`, {});
}

export async function kickVoiceParticipant(channelId: string, targetUserId: string): Promise<void> {
  return apiClient.post<void>(`/channels/${channelId}/voice/participants/${targetUserId}/kick`, {});
}
