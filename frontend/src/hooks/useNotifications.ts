import { useRef, useEffect, useCallback } from 'react';
import {
  isTauri,
  requestNotificationPermission,
  sendDesktopNotification,
} from '../lib/tauri';

const SOUND_URL = 'https://raw.githubusercontent.com/exec/parley/main/assets/audio/ping.mp3';

export function useNotifications() {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  // True while the app is in the foreground (window visible + focused). OS
  // notifications are suppressed in that state so the in-app UI is the only
  // surface; otherwise we fall through to sendDesktopNotification. We track
  // this via focus/blur + visibilitychange because document.hidden alone
  // doesn't flip to true when the Tauri window is hidden via hide() on macOS.
  const inForegroundRef = useRef(true);

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

    const recomputeForeground = () => {
      inForegroundRef.current = !document.hidden && document.hasFocus();
    };
    recomputeForeground();

    const onFocus = () => { inForegroundRef.current = !document.hidden; };
    const onBlur = () => { inForegroundRef.current = false; };
    window.addEventListener('focus', onFocus);
    window.addEventListener('blur', onBlur);
    document.addEventListener('visibilitychange', recomputeForeground);

    // In Tauri on macOS, DOM focus/blur don't fire when the NSWindow loses
    // focus or is hidden (WKWebView keeps the web layer "focused"), so hook
    // into Tauri's window-level focus events directly as a second signal.
    let unlistenTauriFocus: (() => void) | null = null;
    if (isTauri()) {
      (async () => {
        try {
          const { getCurrentWindow } = await import('@tauri-apps/api/window');
          const w = getCurrentWindow();
          inForegroundRef.current = await w.isFocused();
          unlistenTauriFocus = await w.onFocusChanged(({ payload: focused }) => {
            inForegroundRef.current = focused;
          });
        } catch {
          // fall back to DOM listeners only
        }
      })();
    }

    return () => {
      document.removeEventListener('click', unlock);
      document.removeEventListener('keydown', unlock);
      window.removeEventListener('focus', onFocus);
      window.removeEventListener('blur', onBlur);
      document.removeEventListener('visibilitychange', recomputeForeground);
      if (unlistenTauriFocus) unlistenTauriFocus();
    };
  }, []);

  const requestPermission = useCallback(async () => {
    await requestNotificationPermission();
  }, []);

  const notify = useCallback((
    title: string,
    body: string,
    icon?: string,
    onClick?: () => void,
    // True when this message is for the channel/DM the user is currently
    // viewing. Only suppresses the ping when the window is also in the
    // foreground — otherwise we still fire so a minimized/backgrounded user
    // is alerted.
    forActiveChannel: boolean = false,
  ) => {
    if (forActiveChannel && inForegroundRef.current) return;

    const audio = audioRef.current;
    if (audio) {
      audio.currentTime = 0;
      audio.play().catch(() => {});
    }

    // sendDesktopNotification rechecks permission at call time and no-ops
    // when not granted, so we don't have to cache a (stale-prone) value.
    if (!inForegroundRef.current) {
      void sendDesktopNotification({ title, body, icon, onClick });
    }
  }, []);

  return { requestPermission, notify };
}
