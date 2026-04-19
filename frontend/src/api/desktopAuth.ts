import { apiClient } from './client';

export async function issueDesktopCode(state: string): Promise<{ code: string }> {
  return apiClient.post<{ code: string }>('/auth/desktop/issue', { state });
}

export async function exchangeDesktopCode(
  code: string,
  state: string,
): Promise<{ user: any; token: string }> {
  const resp = await fetch('/api/auth/desktop/exchange', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code, state }),
  });
  const data = await resp.json().catch(() => ({}));
  if (!resp.ok) throw new Error(data.message || 'Desktop login failed');
  return data;
}
