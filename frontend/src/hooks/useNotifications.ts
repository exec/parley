import { useRef, useEffect, useCallback } from 'react';

const SOUND_URL = 'https://parley-prod.nyc3.cdn.digitaloceanspaces.com/audio/noti.mp3';

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
    if (typeof Notification === 'undefined') return;
    if (Notification.permission === 'default') {
      await Notification.requestPermission();
    }
  }, []);

  const notify = useCallback((title: string, body: string, icon?: string) => {
    const audio = audioRef.current;
    if (audio) {
      audio.currentTime = 0;
      audio.play().catch(() => {});
    }

    if (
      document.hidden &&
      typeof Notification !== 'undefined' &&
      Notification.permission === 'granted'
    ) {
      try {
        new Notification(title, { body, icon, silent: true });
      } catch {
        // Firefox may throw if called outside a gesture context
      }
    }
  }, []);

  return { requestPermission, notify };
}
