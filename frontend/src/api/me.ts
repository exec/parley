import { apiClient, apiUrl } from './client';
import { ApiError } from './types';

export interface DeleteBlocker { id: string; name: string }

export interface DeleteBlockersError {
  error: 'has_blockers';
  blocking_servers: DeleteBlocker[];
  blocking_group_dms: DeleteBlocker[];
}

export type DeleteAccountErrorKind = 'invalid_confirmation' | 'has_blockers' | 'server_error' | 'unknown';

export class DeleteAccountError extends Error {
  kind: DeleteAccountErrorKind;
  status: number;
  blockers?: DeleteBlockersError;
  constructor(kind: DeleteAccountErrorKind, status: number, message: string, blockers?: DeleteBlockersError) {
    super(message);
    this.kind = kind;
    this.status = status;
    this.blockers = blockers;
  }
}

function authHeaders(): HeadersInit {
  const token = apiClient.getToken();
  const h: Record<string, string> = {};
  if (token) h['Authorization'] = `Bearer ${token}`;
  return h;
}

export async function deleteAccount(confirmUsername: string): Promise<void> {
  const res = await fetch(apiUrl('/me'), {
    method: 'DELETE',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...authHeaders() },
    body: JSON.stringify({ confirm_username: confirmUsername }),
  });
  if (res.status === 204) return;
  const body: unknown = await res.json().catch(() => null);
  if (res.status === 409 && body && typeof body === 'object' && (body as { error?: string }).error === 'has_blockers') {
    throw new DeleteAccountError('has_blockers', 409, 'Account has blockers', body as DeleteBlockersError);
  }
  if (res.status === 400) {
    throw new DeleteAccountError('invalid_confirmation', 400, "Username didn't match");
  }
  if (res.status >= 500) {
    throw new DeleteAccountError('server_error', res.status, "Couldn't delete — try again later");
  }
  const msg = (body && typeof body === 'object' && 'message' in body)
    ? String((body as ApiError).message)
    : `Delete failed (${res.status})`;
  throw new DeleteAccountError('unknown', res.status, msg);
}

// Custom fetch path: apiClient.get<T> assumes a JSON parse, but the export
// endpoint returns a downloadable JSON blob we hand straight to the browser.
export async function exportMyData(): Promise<{ blob: Blob; filename: string | null }> {
  const res = await fetch(apiUrl('/me/export'), {
    method: 'GET',
    credentials: 'include',
    headers: authHeaders(),
  });
  if (!res.ok) {
    const err: ApiError = await res.json().catch(() => ({
      message: res.statusText || 'Export failed',
      code: res.status.toString(),
    }));
    throw err;
  }
  const blob = await res.blob();
  const filename = parseFilename(res.headers.get('Content-Disposition'));
  return { blob, filename };
}

function parseFilename(disposition: string | null): string | null {
  if (!disposition) return null;
  const match = /filename\*?=(?:UTF-8'')?"?([^";]+)"?/i.exec(disposition);
  return match ? decodeURIComponent(match[1]) : null;
}
