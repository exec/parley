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
      const { sendNotification, isPermissionGranted } = await import('@tauri-apps/plugin-notification');
      // Re-check at call time so late grants (e.g. via the Settings tab)
      // take effect without waiting for the hook to rehydrate its ref.
      // Calling sendNotification before permission is granted triggers a
      // system prompt on macOS, stealing what should be a real notification.
      if (!(await isPermissionGranted())) return;
      sendNotification({ title, body });
    } catch {
      // plugin missing — silently drop
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

// Copy text to the OS clipboard. In Tauri the browser Notification-gesture
// rules break after an `await` (WebKit clears the user-activation flag), so
// `navigator.clipboard.writeText` throws NotAllowedError — the plugin route
// bypasses that. Falls back to the browser API on the web.
export async function copyToClipboard(text: string): Promise<void> {
  if (isTauri()) {
    try {
      const { writeText } = await import('@tauri-apps/plugin-clipboard-manager');
      await writeText(text);
      return;
    } catch {
      // fall through to the browser API
    }
  }
  await navigator.clipboard.writeText(text);
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
