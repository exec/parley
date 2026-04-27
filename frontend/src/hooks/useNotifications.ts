import { useRef, useEffect, useCallback } from 'react';
import {
  isTauri,
  requestNotificationPermission,
  sendDesktopNotification,
} from '../lib/tauri';
import type { ChannelKind, NotificationSetting } from '../api/types';

const SOUND_URL = 'https://raw.githubusercontent.com/exec/parley/main/assets/audio/ping.mp3';

// In-memory per-(kind, channelId) cache of notification settings. Populated by
// a single shared listener on parley:channel_notification (registered once
// when the first useNotifications hook mounts and cleaned up when the last
// unmounts), so we don't accumulate listeners across mount/unmount cycles.
const notificationSettingCache = new Map<string, NotificationSetting>();

function cacheKey(kind: ChannelKind, channelId: string): string {
  return `${kind}:${channelId}`;
}

let cacheListenerRefCount = 0;
let cacheListener: ((e: Event) => void) | null = null;

function attachCacheListener() {
  if (typeof window === 'undefined') return;
  cacheListenerRefCount += 1;
  if (cacheListener) return;
  cacheListener = (e: Event) => {
    const detail = (e as CustomEvent<{channel_kind: ChannelKind; channel_id: string; notification_setting: 0 | 1 | 2}>).detail;
    const map: Record<0 | 1 | 2, NotificationSetting> = { 0: 'ALL', 1: 'MENTIONS_ONLY', 2: 'MUTED' };
    notificationSettingCache.set(cacheKey(detail.channel_kind, detail.channel_id), map[detail.notification_setting]);
  };
  window.addEventListener('parley:channel_notification', cacheListener);
}

function detachCacheListener() {
  if (typeof window === 'undefined') return;
  cacheListenerRefCount = Math.max(0, cacheListenerRefCount - 1);
  if (cacheListenerRefCount === 0 && cacheListener) {
    window.removeEventListener('parley:channel_notification', cacheListener);
    cacheListener = null;
  }
}

export function getNotificationSettingForChannel(kind: ChannelKind, channelId: string): NotificationSetting {
  return notificationSettingCache.get(cacheKey(kind, channelId)) ?? 'ALL';
}

interface NotifyContext {
  channelKind: ChannelKind;
  channelId: string;
  authorId: string;
  mentions: string[];   // user ids @mentioned in the message
  currentUserId: string;
}

// shouldNotify decides whether a per-channel notification should fire client-side.
// Backend always fans out the message; this gate only suppresses toasts/sounds.
// Self-authored messages never notify; muted channels never notify; mentions-only
// channels notify only when the current user is in the mentions list.
export function shouldNotify(ctx: NotifyContext, setting: NotificationSetting): boolean {
  if (setting === 'MUTED') return false;
  if (ctx.authorId === ctx.currentUserId) return false;
  if (setting === 'MENTIONS_ONLY') return ctx.mentions.includes(ctx.currentUserId);
  return true;
}

export function useNotifications() {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  // True while the app is in the foreground (window visible + focused). OS
  // notifications are suppressed in that state so the in-app UI is the only
  // surface; otherwise we fall through to sendDesktopNotification. We track
  // this via focus/blur + visibilitychange because document.hidden alone
  // doesn't flip to true when the Tauri window is hidden via hide() on macOS.
  const inForegroundRef = useRef(true);

  useEffect(() => {
    attachCacheListener();
    const audio = new Audio(SOUND_URL);
    audio.volume = 0.4;
    audio.preload = 'auto';
    audioRef.current = audio;

    // Browsers block audio playback until a user gesture has occurred.
    // We unlock by playing a tiny silent WAV on first interaction — iOS
    // WKWebView ignores muted/volume during the initial few ms of a real
    // audio element's play(), so playing the ping itself produces an
    // audible blip. A dedicated silent clip sidesteps that entirely.
    const SILENT_WAV =
      'data:audio/wav;base64,UklGRigAAABXQVZFZm10IBIAAAABAAEARKwAAIhYAQACABAAAABkYXRhAgAAAAEA';
    const silent = new Audio(SILENT_WAV);
    const unlock = () => {
      silent.play().catch(() => {});
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

    // In Tauri on macOS, DOM focus/blur don't reliably fire when the NSWindow
    // loses focus or is hidden (WKWebView keeps the web layer "focused"). The
    // Rust side emits `parley:foreground` with a boolean payload when it
    // hides / reveals the main window; we also subscribe to onFocusChanged
    // as a belt-and-braces signal for raw focus changes.
    let unlistenTauriFocus: (() => void) | null = null;
    let unlistenForegroundEvent: (() => void) | null = null;
    let cancelled = false;
    if (isTauri()) {
      (async () => {
        try {
          const { getCurrentWindow } = await import('@tauri-apps/api/window');
          const { listen } = await import('@tauri-apps/api/event');
          const w = getCurrentWindow();
          inForegroundRef.current = await w.isFocused();
          const fnFocus = await w.onFocusChanged(({ payload: focused }) => {
            inForegroundRef.current = focused;
          });
          if (cancelled) { fnFocus(); } else { unlistenTauriFocus = fnFocus; }
          const fnFg = await listen<boolean>('parley:foreground', (event) => {
            inForegroundRef.current = event.payload;
          });
          if (cancelled) { fnFg(); } else { unlistenForegroundEvent = fnFg; }
        } catch {
          // fall back to DOM listeners only
        }
      })();
    }

    return () => {
      cancelled = true;
      detachCacheListener();
      document.removeEventListener('click', unlock);
      document.removeEventListener('keydown', unlock);
      window.removeEventListener('focus', onFocus);
      window.removeEventListener('blur', onBlur);
      document.removeEventListener('visibilitychange', recomputeForeground);
      if (unlistenTauriFocus) unlistenTauriFocus();
      if (unlistenForegroundEvent) unlistenForegroundEvent();
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
    // Optional per-channel gate. When provided, applies the shouldNotify
    // rules (mute / mentions-only / self-authored) before firing. Callers
    // that don't pass this fall through to the legacy behavior and always
    // ping (subject to forActiveChannel + foreground rules above).
    notifyGate?: { kind: ChannelKind; channelId: string; authorId: string; mentions: string[]; currentUserId: string },
  ) => {
    if (forActiveChannel && inForegroundRef.current) return;

    if (notifyGate) {
      const setting = getNotificationSettingForChannel(notifyGate.kind, notifyGate.channelId);
      if (!shouldNotify({
        channelKind: notifyGate.kind,
        channelId: notifyGate.channelId,
        authorId: notifyGate.authorId,
        mentions: notifyGate.mentions,
        currentUserId: notifyGate.currentUserId,
      }, setting)) {
        return;
      }
    }

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
