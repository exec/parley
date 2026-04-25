import { describe, it, expect, beforeEach, vi } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useLocalVolumes } from './useLocalVolumes';

// Node 25+ has a built-in localStorage that lacks Web Storage API methods.
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

describe('useLocalVolumes', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', createStorageMock());
  });

  it('defaults to 100 (unity) for unknown users', () => {
    const { result } = renderHook(() => useLocalVolumes());
    expect(result.current.getVolume('42')).toBe(100);
  });

  it('persists set values to localStorage', () => {
    const { result } = renderHook(() => useLocalVolumes());
    act(() => result.current.setVolume('42', 50));
    expect(result.current.getVolume('42')).toBe(50);
    const raw = localStorage.getItem('parley.localVolumes');
    expect(raw).toBeTruthy();
    expect(JSON.parse(raw!)).toEqual({ '42': 50 });
  });

  it('toggleMute swaps between 0 and last non-zero (default 100)', () => {
    const { result } = renderHook(() => useLocalVolumes());
    act(() => result.current.toggleMute('42'));
    expect(result.current.getVolume('42')).toBe(0);
    act(() => result.current.toggleMute('42'));
    expect(result.current.getVolume('42')).toBe(100);
    act(() => result.current.setVolume('42', 80));
    act(() => result.current.toggleMute('42'));
    expect(result.current.getVolume('42')).toBe(0);
    act(() => result.current.toggleMute('42'));
    expect(result.current.getVolume('42')).toBe(80);
  });

  it('clamps to 0..200', () => {
    const { result } = renderHook(() => useLocalVolumes());
    act(() => result.current.setVolume('42', -10));
    expect(result.current.getVolume('42')).toBe(0);
    act(() => result.current.setVolume('42', 999));
    expect(result.current.getVolume('42')).toBe(200);
  });
});
