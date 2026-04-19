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

type DeepLinkHandler = (url: string) => void;

export async function onDeepLink(handler: DeepLinkHandler): Promise<() => void> {
  if (!isTauri()) return () => {};
  const { listen } = await import('@tauri-apps/api/event');
  const unlisten = await listen<string>('deep-link', (event) => {
    handler(event.payload);
  });
  return unlisten;
}
