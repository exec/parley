import { useEffect, useState, useCallback } from 'react';

const STORAGE_KEY = 'parley.localVolumes';
const PRE_MUTE_KEY = 'parley.preMuteVolumes';

type VolumeMap = Record<string, number>;

function readMap(key: string): VolumeMap {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed === 'object') return parsed as VolumeMap;
  } catch {
    // ignored
  }
  return {};
}

function writeMap(key: string, map: VolumeMap) {
  localStorage.setItem(key, JSON.stringify(map));
}

function clamp(n: number): number {
  if (!Number.isFinite(n)) return 100;
  if (n < 0) return 0;
  if (n > 200) return 200;
  return Math.round(n);
}

export interface UseLocalVolumesReturn {
  getVolume: (userID: string | number | bigint) => number;
  setVolume: (userID: string | number | bigint, value: number) => void;
  toggleMute: (userID: string | number | bigint) => void;
}

export function useLocalVolumes(): UseLocalVolumesReturn {
  const [version, setVersion] = useState(0);

  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key === STORAGE_KEY || e.key === PRE_MUTE_KEY) {
        setVersion(v => v + 1);
      }
    };
    window.addEventListener('storage', onStorage);
    return () => window.removeEventListener('storage', onStorage);
  }, []);

  const getVolume = useCallback((userID: string | number | bigint): number => {
    const key = String(userID);
    const map = readMap(STORAGE_KEY);
    return key in map ? map[key] : 100;
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [version]);

  const setVolume = useCallback((userID: string | number | bigint, value: number) => {
    const key = String(userID);
    const v = clamp(value);
    const map = readMap(STORAGE_KEY);
    map[key] = v;
    writeMap(STORAGE_KEY, map);
    setVersion(x => x + 1);
  }, []);

  const toggleMute = useCallback((userID: string | number | bigint) => {
    const key = String(userID);
    const map = readMap(STORAGE_KEY);
    const pre = readMap(PRE_MUTE_KEY);
    const current = key in map ? map[key] : 100;
    if (current === 0) {
      const restored = pre[key] ?? 100;
      map[key] = restored;
    } else {
      pre[key] = current;
      map[key] = 0;
      writeMap(PRE_MUTE_KEY, pre);
    }
    writeMap(STORAGE_KEY, map);
    setVersion(x => x + 1);
  }, []);

  return { getVolume, setVolume, toggleMute };
}
