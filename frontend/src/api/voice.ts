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

export async function getVoiceToken(vc: string): Promise<VoiceToken> {
  return apiClient.get<VoiceToken>(`/voice/${vc}/token`);
}

export async function joinVoiceChannel(vc: string): Promise<void> {
  return apiClient.post<void>(`/voice/${vc}/join`, {});
}

export async function leaveVoiceChannel(vc: string): Promise<void> {
  return apiClient.post<void>(`/voice/${vc}/leave`, {});
}

export async function refreshVoiceHeartbeat(vc: string): Promise<void> {
  return apiClient.post<void>(`/voice/${vc}/heartbeat`, {});
}

export async function getVoiceParticipants(vc: string): Promise<VoiceParticipant[]> {
  return apiClient.get<VoiceParticipant[]>(`/voice/${vc}/participants`);
}

export async function muteVoiceParticipant(vc: string, targetUserId: string): Promise<void> {
  return apiClient.post<void>(`/voice/${vc}/participants/${targetUserId}/mute`, {});
}

export async function kickVoiceParticipant(vc: string, targetUserId: string): Promise<void> {
  return apiClient.post<void>(`/voice/${vc}/participants/${targetUserId}/kick`, {});
}

export function serverVc(channelId: string | number): string {
  return `s:${channelId}`;
}

export function dmVc(dmChannelId: string | number): string {
  return `dm:${dmChannelId}`;
}
