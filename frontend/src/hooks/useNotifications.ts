import { useRef, useEffect, useCallback } from 'react';
import {
  requestNotificationPermission,
  sendDesktopNotification,
} from '../lib/tauri';

const SOUND_URL = 'https://raw.githubusercontent.com/exec/parley/main/assets/audio/ping.mp3';

export function useNotifications() {
  const audioRef = useRef<HTMLAudioElement | null>(null);

  useEffect(() => {
    const audio = new Audio(SOUND_URL);
    audio.volume = 0.4;
    audio.preload = 'auto';
    audioRef.current = audio;

    // Browsers block audio playback until a user gesture has occurred.
    // Playing muted + immediately pausing on the first interaction "unlocks"
    // the element so that later play() calls (triggered by WS events) succeed.
    const unlock = () => {
      audio.muted = true;
      audio.play()
        .then(() => { audio.pause(); audio.currentTime = 0; audio.muted = false; })
        .catch(() => { audio.muted = false; });
    };

    document.addEventListener('click', unlock, { once: true });
    document.addEventListener('keydown', unlock, { once: true });

    return () => {
      document.removeEventListener('click', unlock);
      document.removeEventListener('keydown', unlock);
    };
  }, []);

  const requestPermission = useCallback(async () => {
    await requestNotificationPermission();
  }, []);

  const notify = useCallback((title: string, body: string, icon?: string, onClick?: () => void) => {
    const audio = audioRef.current;
    if (audio) {
      audio.currentTime = 0;
      audio.play().catch(() => {});
    }

    // sendDesktopNotification rechecks permission at call time and no-ops
    // when not granted, so we don't have to cache a (stale-prone) value.
    if (document.hidden) {
      void sendDesktopNotification({ title, body, icon, onClick });
    }
  }, []);

  return { requestPermission, notify };
}
