import { ApiError } from './types';

// When the bundle is not served same-origin with the API (e.g. the Tauri
// desktop shell, where the webview origin is tauri://localhost), the API
// calls need an absolute URL. VITE_SITE_URL, when set at build time, points
// at the deployed Parley site. For the normal web build it is either unset
// (relative `/api` still works) or set to the same origin (absolute URL is
// equivalent to relative).
const SITE_URL = (import.meta.env.VITE_SITE_URL as string) || '';
const DEFAULT_BASE_URL = SITE_URL ? `${SITE_URL.replace(/\/$/, '')}/api` : '/api';

// The desktop shell is loaded from tauri://localhost, so requests to
// parley.byexec.com are cross-site — SameSite=Lax cookies don't ship there.
// On desktop we keep the legacy Bearer-header flow (token in localStorage,
// Authorization header on every request). On web we lean on the HttpOnly
// session cookie set by /auth/login & friends and never touch the token
// from JS, so an XSS can't exfiltrate it.
export const IS_DESKTOP =
  typeof window !== 'undefined' && window.location.protocol === 'tauri:';

// Build an absolute API URL for endpoints that bypass apiClient (multipart
// uploads, etc.). Paths should start with a slash.
export function apiUrl(path: string): string {
  return `${DEFAULT_BASE_URL}${path.startsWith('/') ? path : `/${path}`}`;
}

class ApiClient {
  private baseUrl: string;
  private token: string | null = null;

  constructor(baseUrl: string = DEFAULT_BASE_URL) {
    this.baseUrl = baseUrl;
  }

  setToken(token: string | null): void {
    this.token = token;
  }

  getToken(): string | null {
    return this.token;
  }

  private getHeaders(): HeadersInit {
    const headers: HeadersInit = {
      'Content-Type': 'application/json',
    };

    // If a token is in memory, attach it. On web this happens only for the
    // admin impersonation flow (Impersonate.tsx) where a one-shot Bearer
    // token is delivered via URL — normal web sessions never set the token
    // and rely entirely on the HttpOnly session cookie. Desktop always has
    // the token in memory because cross-origin SameSite=Lax cookies don't
    // ship to tauri://localhost.
    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    }

    return headers;
  }

  private static isRedirecting = false;

  private async handleResponse<T>(response: Response): Promise<T> {
    if (response.status === 401) {
      // Clear any in-JS state. Web's HttpOnly session cookie isn't
      // reachable from JS — the server clears it via /auth/logout, and
      // otherwise the browser drops it at expiry. Desktop's Bearer
      // token + the impersonation token (if any) are both wiped here.
      localStorage.removeItem('token');
      localStorage.removeItem('user');
      this.token = null;
      if (!ApiClient.isRedirecting) {
        ApiClient.isRedirecting = true;
        window.location.href = '/login';
      }
      throw new Error('Session expired');
    }

    if (!response.ok) {
      const error: ApiError = await response.json().catch(() => ({
        message: response.statusText || 'An error occurred',
        code: response.status.toString(),
      }));
      throw error;
    }

    if (response.status === 204) {
      return undefined as T;
    }

    return response.json();
  }

  async get<T>(endpoint: string): Promise<T> {
    const response = await fetch(`${this.baseUrl}${endpoint}`, {
      method: 'GET',
      headers: this.getHeaders(),
      credentials: 'include',
    });

    return this.handleResponse<T>(response);
  }

  async post<T>(endpoint: string, data?: unknown): Promise<T> {
    const response = await fetch(`${this.baseUrl}${endpoint}`, {
      method: 'POST',
      headers: this.getHeaders(),
      credentials: 'include',
      body: data ? JSON.stringify(data) : undefined,
    });

    return this.handleResponse<T>(response);
  }

  async put<T>(endpoint: string, data?: unknown): Promise<T> {
    const response = await fetch(`${this.baseUrl}${endpoint}`, {
      method: 'PUT',
      headers: this.getHeaders(),
      credentials: 'include',
      body: data ? JSON.stringify(data) : undefined,
    });

    return this.handleResponse<T>(response);
  }

  async patch<T>(endpoint: string, data?: unknown): Promise<T> {
    const response = await fetch(`${this.baseUrl}${endpoint}`, {
      method: 'PATCH',
      headers: this.getHeaders(),
      credentials: 'include',
      body: data ? JSON.stringify(data) : undefined,
    });

    return this.handleResponse<T>(response);
  }

  async delete<T>(endpoint: string, data?: unknown): Promise<T> {
    const response = await fetch(`${this.baseUrl}${endpoint}`, {
      method: 'DELETE',
      headers: this.getHeaders(),
      credentials: 'include',
      body: data ? JSON.stringify(data) : undefined,
    });

    return this.handleResponse<T>(response);
  }
}

export const apiClient = new ApiClient();

// Hydrate any stored token synchronously so the first render's requests
// are authenticated.
//   - Desktop always stores the JWT (Tauri webview can't share cookies
//     cross-origin with parley.byexec.com).
//   - Web normally does NOT store anything here; the HttpOnly session
//     cookie carries auth. The exception is the admin impersonation flow
//     (Impersonate.tsx) which receives a one-shot Bearer token via URL
//     and stashes it for the duration of the impersonation session.
const _storedToken = localStorage.getItem('token');
if (_storedToken) {
  apiClient.setToken(_storedToken);
}

export default ApiClient;