import { apiClient } from './client';

export interface UserTheme {
  id: number;
  name: string;
  css: string;
  background_url: string | null;
  share_token: string | null;
  created_at: string;
}

export interface UserPreferences {
  active_theme: string;
  active_custom_theme_id: number | null;
  custom_themes: UserTheme[];
}

export interface NewTheme { name: string; css: string; background_url?: string | null; }

export const getPreferences = () => apiClient.get<UserPreferences>('/me/preferences');
export const setBuiltinTheme = (id: string) => apiClient.put('/me/preferences/theme', { theme: id });
export const setCustomTheme = (id: number) => apiClient.put('/me/preferences/theme', { theme: 'custom', custom_theme_id: id });
export const createTheme = (t: NewTheme) => apiClient.post<UserTheme>('/me/themes', t);
export const updateTheme = (id: number, t: NewTheme) => apiClient.put<UserTheme>(`/me/themes/${id}`, t);
export const deleteTheme = (id: number) => apiClient.delete(`/me/themes/${id}`);
export const shareTheme = (id: number) => apiClient.post<{ share_url: string }>(`/me/themes/${id}/share`);
export const getPublicTheme = (token: string) => apiClient.get<UserTheme>(`/themes/${token}`);
export const installTheme = (token: string) => apiClient.post<UserTheme>(`/me/themes/install/${token}`);
