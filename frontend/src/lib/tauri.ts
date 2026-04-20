// Utilities for running inside a Tauri webview. Safe to import in browser
// builds: helpers guard on isTauri() and dynamic-import the tauri modules so
// the web bundle doesn't pull them in.

export function isTauri(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;
}

export function randomState(): string {
  const bytes = new Uint8Array(24);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

export async function openInBrowser(url: string): Promise<void> {
  if (!isTauri()) {
    window.open(url, '_blank', 'noopener');
    return;
  }
  const { open } = await import('@tauri-apps/plugin-shell');
  await open(url);
}

export type NotifyPermission = 'granted' | 'denied' | 'default';

export async function getNotificationPermission(): Promise<NotifyPermission> {
  if (isTauri()) {
    try {
      const { isPermissionGranted } = await import('@tauri-apps/plugin-notification');
      return (await isPermissionGranted()) ? 'granted' : 'default';
    } catch {
      return 'default';
    }
  }
  if (typeof Notification === 'undefined') return 'denied';
  return Notification.permission;
}

export async function requestNotificationPermission(): Promise<NotifyPermission> {
  if (isTauri()) {
    try {
      const { requestPermission } = await import('@tauri-apps/plugin-notification');
      const r = await requestPermission();
      return r === 'granted' ? 'granted' : r === 'denied' ? 'denied' : 'default';
    } catch {
      return 'denied';
    }
  }
  if (typeof Notification === 'undefined') return 'denied';
  return Notification.requestPermission();
}

export interface SendNotificationOptions {
  title: string;
  body: string;
  icon?: string;
  onClick?: () => void;
}

export async function sendDesktopNotification(opts: SendNotificationOptions): Promise<void> {
  const { title, body, icon, onClick } = opts;
  if (isTauri()) {
    try {
      const { sendNotification } = await import('@tauri-apps/plugin-notification');
      sendNotification({ title, body });
    } catch {
      // plugin missing or permission not granted — silently drop
    }
    return;
  }
  if (typeof Notification === 'undefined' || Notification.permission !== 'granted') return;
  try {
    const n = new Notification(title, { body, icon, silent: true });
    if (onClick) {
      n.onclick = () => {
        window.focus();
        onClick();
        n.close();
      };
    }
  } catch {
    // Firefox may throw if called outside a gesture context
  }
}

type DeepLinkHandler = (url: string) => void;

export async function onDeepLink(handler: DeepLinkHandler): Promise<() => void> {
  if (!isTauri()) return () => {};
  const { listen } = await import('@tauri-apps/api/event');
  const unlisten = await listen<string>('deep-link', (event) => {
    handler(event.payload);
  });
  return unlisten;
}
