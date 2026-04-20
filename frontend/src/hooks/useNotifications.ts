import { useRef, useEffect, useCallback } from 'react';
import {
  isTauri,
  getNotificationPermission,
  requestNotificationPermission,
  sendDesktopNotification,
  type NotifyPermission,
} from '../lib/tauri';

const SOUND_URL = import.meta.env.VITE_CDN_HOST
  ? `${import.meta.env.VITE_CDN_HOST}/audio/noti.mp3`
  : 'https://parley-prod.nyc3.cdn.digitaloceanspaces.com/audio/noti.mp3';

export function useNotifications() {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const permissionRef = useRef<NotifyPermission>('default');
  const tauriRef = useRef<boolean>(false);

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

    tauriRef.current = isTauri();
    void getNotificationPermission().then(p => {
      permissionRef.current = p;
    });

    return () => {
      document.removeEventListener('click', unlock);
      document.removeEventListener('keydown', unlock);
    };
  }, []);

  const requestPermission = useCallback(async () => {
    if (permissionRef.current === 'default') {
      permissionRef.current = await requestNotificationPermission();
    }
  }, []);

  const notify = useCallback((title: string, body: string, icon?: string, onClick?: () => void) => {
    const audio = audioRef.current;
    if (audio) {
      audio.currentTime = 0;
      audio.play().catch(() => {});
    }

    if (document.hidden && permissionRef.current === 'granted') {
      void sendDesktopNotification({ title, body, icon, onClick });
    }
  }, []);

  return { requestPermission, notify };
}
