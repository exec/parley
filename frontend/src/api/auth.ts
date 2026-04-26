import { apiClient, IS_DESKTOP } from './client';
import { AuthResponse, User } from './types';

// Web users get an HttpOnly session cookie set by the server; the JWT in the
// response body is for desktop only. Storing the token in localStorage on
// web is exactly the XSS exfiltration target we're trying to remove.
function adoptToken(response: AuthResponse): AuthResponse {
  if (IS_DESKTOP) {
    apiClient.setToken(response.token);
    localStorage.setItem('token', response.token);
  }
  return response;
}

export async function login(email: string, password: string): Promise<AuthResponse> {
  return adoptToken(await apiClient.post<AuthResponse>('/auth/login', { email, password }));
}

export async function register(
  username: string,
  email: string,
  password: string
): Promise<AuthResponse> {
  return adoptToken(await apiClient.post<AuthResponse>('/auth/register', { username, email, password }));
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
  display_name?: string;
  current_password?: string;
  new_password?: string;
  avatar_url?: string;
  banner_url?: string;
  bio?: string;
  phone?: string;
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

export async function verifyPhone(code: string): Promise<{ message: string }> {
  return apiClient.post<{ message: string }>('/auth/verify-phone', { code });
}

export async function resendPhone(): Promise<{ message: string }> {
  return apiClient.post<{ message: string }>('/auth/resend-phone', {});
}

export async function changePhone(phone: string, password: string): Promise<User> {
  return apiClient.put<User>('/auth/phone', { phone, password });
}

export async function getMyPhone(): Promise<{ phone_number: string; phone_verified: boolean }> {
  return apiClient.get('/auth/me/phone');
}

export async function getWsTicket(): Promise<string> {
  const data = await apiClient.post<{ ticket: string }>('/auth/ws-ticket', {});
  return data.ticket;
}