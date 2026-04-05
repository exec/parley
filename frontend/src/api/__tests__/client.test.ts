import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Node 25+ has a built-in localStorage that lacks Web Storage API methods.
// Provide a compliant mock so client.ts module-level code works.
function createStorageMock() {
  const store = new Map<string, string>();
  return {
    getItem: vi.fn((key: string) => store.get(key) ?? null),
    setItem: vi.fn((key: string, val: string) => store.set(key, val)),
    removeItem: vi.fn((key: string) => store.delete(key)),
    clear: vi.fn(() => store.clear()),
    get length() { return store.size; },
    key: vi.fn((i: number) => [...store.keys()][i] ?? null),
  };
}

function mockFetchResponse(status: number, body?: unknown, statusText = '') {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText,
    json: vi.fn().mockResolvedValue(body),
    headers: new Headers(),
  } as unknown as Response;
}

describe('ApiClient', () => {
  let ApiClient: typeof import('../client').default;
  let client: InstanceType<typeof ApiClient>;
  let storageMock: ReturnType<typeof createStorageMock>;

  beforeEach(async () => {
    storageMock = createStorageMock();
    vi.stubGlobal('localStorage', storageMock);
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockFetchResponse(200, { ok: true })));
    vi.resetModules();
    const mod = await import('../client');
    ApiClient = mod.default;
    client = new ApiClient('http://test.local/api');
    (ApiClient as any).isRedirecting = false;
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  describe('token management', () => {
    it('starts with no token', () => {
      expect(client.getToken()).toBeNull();
    });

    it('stores and retrieves token', () => {
      client.setToken('abc123');
      expect(client.getToken()).toBe('abc123');
    });

    it('clears token', () => {
      client.setToken('abc123');
      client.setToken(null);
      expect(client.getToken()).toBeNull();
    });
  });

  describe('headers', () => {
    it('sends Content-Type header', async () => {
      await client.get('/test');
      expect(fetch).toHaveBeenCalledWith('http://test.local/api/test', expect.objectContaining({
        headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
      }));
    });

    it('sends Authorization header when token is set', async () => {
      client.setToken('mytoken');
      await client.get('/test');
      expect(fetch).toHaveBeenCalledWith('http://test.local/api/test', expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer mytoken' }),
      }));
    });

    it('omits Authorization header when no token', async () => {
      await client.get('/test');
      const call = vi.mocked(fetch).mock.calls[0];
      const headers = call[1]?.headers as Record<string, string>;
      expect(headers.Authorization).toBeUndefined();
    });
  });

  describe('HTTP methods', () => {
    it('GET sends correct method and no body', async () => {
      await client.get('/items');
      expect(fetch).toHaveBeenCalledWith('http://test.local/api/items', expect.objectContaining({
        method: 'GET',
      }));
    });

    it('POST sends JSON body', async () => {
      await client.post('/items', { name: 'test' });
      expect(fetch).toHaveBeenCalledWith('http://test.local/api/items', expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ name: 'test' }),
      }));
    });

    it('POST without data sends undefined body', async () => {
      await client.post('/items');
      expect(fetch).toHaveBeenCalledWith('http://test.local/api/items', expect.objectContaining({
        method: 'POST',
        body: undefined,
      }));
    });

    it('PUT sends correct method and body', async () => {
      await client.put('/items/1', { name: 'updated' });
      expect(fetch).toHaveBeenCalledWith('http://test.local/api/items/1', expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ name: 'updated' }),
      }));
    });

    it('PATCH sends correct method and body', async () => {
      await client.patch('/items/1', { name: 'patched' });
      expect(fetch).toHaveBeenCalledWith('http://test.local/api/items/1', expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({ name: 'patched' }),
      }));
    });

    it('DELETE sends correct method', async () => {
      await client.delete('/items/1');
      expect(fetch).toHaveBeenCalledWith('http://test.local/api/items/1', expect.objectContaining({
        method: 'DELETE',
      }));
    });
  });

  describe('response handling', () => {
    it('returns parsed JSON for successful responses', async () => {
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockFetchResponse(200, { id: 1, name: 'test' })));
      const result = await client.get<{ id: number; name: string }>('/items/1');
      expect(result).toEqual({ id: 1, name: 'test' });
    });

    it('returns undefined for 204 No Content', async () => {
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockFetchResponse(204)));
      const result = await client.delete('/items/1');
      expect(result).toBeUndefined();
    });

    it('throws ApiError for non-ok responses', async () => {
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockFetchResponse(400, { message: 'Bad request', code: '400' })));
      await expect(client.post('/items', {})).rejects.toEqual({
        message: 'Bad request',
        code: '400',
      });
    });

    it('throws fallback error when error response is not JSON', async () => {
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        statusText: 'Internal Server Error',
        json: vi.fn().mockRejectedValue(new Error('not json')),
      }));
      await expect(client.get('/fail')).rejects.toEqual({
        message: 'Internal Server Error',
        code: '500',
      });
    });

    it('uses fallback message when statusText is empty', async () => {
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        statusText: '',
        json: vi.fn().mockRejectedValue(new Error('not json')),
      }));
      await expect(client.get('/fail')).rejects.toEqual({
        message: 'An error occurred',
        code: '500',
      });
    });
  });

  describe('401 handling', () => {
    it('clears token and localStorage on 401', async () => {
      storageMock.setItem('token', 'old');
      storageMock.setItem('user', '{}');
      client.setToken('old');

      vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockFetchResponse(401)));
      await expect(client.get('/me')).rejects.toThrow('Session expired');

      expect(storageMock.removeItem).toHaveBeenCalledWith('token');
      expect(storageMock.removeItem).toHaveBeenCalledWith('user');
      expect(client.getToken()).toBeNull();
    });

    it('sets isRedirecting to prevent multiple redirects', async () => {
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockFetchResponse(401)));
      (ApiClient as any).isRedirecting = false;

      await client.get('/a').catch(() => {});
      expect((ApiClient as any).isRedirecting).toBe(true);
    });
  });

  describe('default base URL', () => {
    it('uses /api as default', async () => {
      const defaultClient = new ApiClient();
      await defaultClient.get('/test');
      expect(fetch).toHaveBeenCalledWith('/api/test', expect.anything());
    });
  });

  describe('module-level token initialization', () => {
    it('auto-loads token from localStorage on import', async () => {
      const freshStorage = createStorageMock();
      freshStorage.setItem('token', 'persisted-token');
      vi.stubGlobal('localStorage', freshStorage);
      vi.resetModules();
      const mod = await import('../client');
      expect(mod.apiClient.getToken()).toBe('persisted-token');
    });

    it('does not set token when localStorage is empty', async () => {
      const freshStorage = createStorageMock();
      vi.stubGlobal('localStorage', freshStorage);
      vi.resetModules();
      const mod = await import('../client');
      expect(mod.apiClient.getToken()).toBeNull();
    });
  });
});
