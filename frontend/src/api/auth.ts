import { apiClient } from './client';
import { AuthResponse, User } from './types';

export async function login(email: string, password: string): Promise<AuthResponse> {
  const response = await apiClient.post<AuthResponse>('/auth/login', {
    email,
    password,
  });

  apiClient.setToken(response.token);
  return response;
}

export async function register(
  username: string,
  email: string,
  password: string
): Promise<AuthResponse> {
  const response = await apiClient.post<AuthResponse>('/auth/register', {
    username,
    email,
    password,
  });

  apiClient.setToken(response.token);
  return response;
}

export async function logout(): Promise<void> {
  try {
    await apiClient.post('/auth/logout');
  } finally {
    apiClient.setToken(null);
  }
}

export async function getCurrentUser(): Promise<User> {
  return apiClient.get<User>('/auth/me');
}

export function setAuthToken(token: string): void {
  apiClient.setToken(token);
}

export function clearAuthToken(): void {
  apiClient.setToken(null);
}

export interface UpdateProfileRequest {
  username?: string;
  current_password?: string;
  new_password?: string;
  avatar_url?: string;
  banner_url?: string;
}

export async function updateProfile(req: UpdateProfileRequest): Promise<User> {
  return apiClient.put<User>('/auth/profile', req);
}

export async function changeEmail(newEmail: string, password: string): Promise<User> {
  return apiClient.put<User>('/auth/email', { new_email: newEmail, password });
}

export async function verifyEmail(token: string): Promise<{ message: string }> {
  return apiClient.get<{ message: string }>(`/auth/verify-email?token=${encodeURIComponent(token)}`);
}

export async function resendVerification(): Promise<{ message: string }> {
  return apiClient.post<{ message: string }>('/auth/resend-verification', {});
}