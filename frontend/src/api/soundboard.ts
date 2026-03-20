import { apiClient } from './client';

export interface Sound {
  id: string;
  server_id: string;
  uploader_id: string;
  name: string;
  emoji?: string;
  file_url: string;
  created_at: string;
}

export interface SoundWithServer extends Sound {
  server_name: string;
}

export async function listServerSounds(serverId: string): Promise<Sound[]> {
  return apiClient.get<Sound[]>(`/servers/${serverId}/soundboard`);
}

export async function uploadSound(serverId: string, file: File, name: string, emoji: string): Promise<Sound> {
  const formData = new FormData();
  formData.append('file', file);
  formData.append('name', name);
  if (emoji) {
    formData.append('emoji', emoji);
  }

  const token = apiClient.getToken();
  const headers: HeadersInit = {};
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const response = await fetch(`/api/servers/${serverId}/soundboard`, {
    method: 'POST',
    headers,
    body: formData,
  });

  if (response.status === 401) {
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    apiClient.setToken(null);
    window.location.href = '/login';
    throw new Error('Session expired');
  }

  if (!response.ok) {
    const error = await response.json().catch(() => ({
      message: response.statusText || 'An error occurred',
      code: response.status.toString(),
    }));
    throw error;
  }

  return response.json();
}

export async function updateSound(serverId: string, soundId: string, name: string, emoji: string): Promise<Sound> {
  return apiClient.patch<Sound>(`/servers/${serverId}/soundboard/${soundId}`, { name, emoji });
}

export async function deleteSound(serverId: string, soundId: string): Promise<void> {
  return apiClient.delete<void>(`/servers/${serverId}/soundboard/${soundId}`);
}

export async function listAllSounds(): Promise<SoundWithServer[]> {
  return apiClient.get<SoundWithServer[]>('/soundboard');
}

export async function playSound(channelId: string, soundId: string, durationMs: number): Promise<void> {
  return apiClient.post<void>(`/channels/${channelId}/soundboard/play`, {
    sound_id: soundId,
    duration_ms: durationMs,
  });
}
