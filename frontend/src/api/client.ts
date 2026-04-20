import { ApiError } from './types';

// When the bundle is not served same-origin with the API (e.g. the Tauri
// desktop shell, where the webview origin is tauri://localhost), the API
// calls need an absolute URL. VITE_SITE_URL, when set at build time, points
// at the deployed Parley site. For the normal web build it is either unset
// (relative `/api` still works) or set to the same origin (absolute URL is
// equivalent to relative).
const SITE_URL = (import.meta.env.VITE_SITE_URL as string) || '';
const DEFAULT_BASE_URL = SITE_URL ? `${SITE_URL.replace(/\/$/, '')}/api` : '/api';

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

    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    }

    return headers;
  }

  private static isRedirecting = false;

  private async handleResponse<T>(response: Response): Promise<T> {
    if (response.status === 401) {
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
    });

    return this.handleResponse<T>(response);
  }

  async post<T>(endpoint: string, data?: unknown): Promise<T> {
    const response = await fetch(`${this.baseUrl}${endpoint}`, {
      method: 'POST',
      headers: this.getHeaders(),
      body: data ? JSON.stringify(data) : undefined,
    });

    return this.handleResponse<T>(response);
  }

  async put<T>(endpoint: string, data?: unknown): Promise<T> {
    const response = await fetch(`${this.baseUrl}${endpoint}`, {
      method: 'PUT',
      headers: this.getHeaders(),
      body: data ? JSON.stringify(data) : undefined,
    });

    return this.handleResponse<T>(response);
  }

  async patch<T>(endpoint: string, data?: unknown): Promise<T> {
    const response = await fetch(`${this.baseUrl}${endpoint}`, {
      method: 'PATCH',
      headers: this.getHeaders(),
      body: data ? JSON.stringify(data) : undefined,
    });

    return this.handleResponse<T>(response);
  }

  async delete<T>(endpoint: string): Promise<T> {
    const response = await fetch(`${this.baseUrl}${endpoint}`, {
      method: 'DELETE',
      headers: this.getHeaders(),
    });

    return this.handleResponse<T>(response);
  }
}

export const apiClient = new ApiClient();

// Initialize token synchronously from localStorage so it's available before any useEffect runs
const _storedToken = localStorage.getItem('token');
if (_storedToken) {
  apiClient.setToken(_storedToken);
}

export default ApiClient;