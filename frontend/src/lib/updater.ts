import { isTauri } from './tauri';

export interface PendingUpdate {
  version: string;
  currentVersion: string;
  notes: string;
  install: () => Promise<void>;
}

// Silent availability check. Resolves to a handle when an update is available,
// or null if we're already up to date / not running in Tauri / the check fails.
// Failures are swallowed so a network blip on startup doesn't surface to the
// user — the banner is a nicety, not a critical path.
export async function checkForUpdate(): Promise<PendingUpdate | null> {
  if (!isTauri()) return null;
  try {
    const { check } = await import('@tauri-apps/plugin-updater');
    const update = await check();
    if (!update) return null;
    return {
      version: update.version,
      currentVersion: update.currentVersion,
      notes: update.body ?? '',
      install: async () => {
        await update.downloadAndInstall();
        // macOS / Linux auto-relaunch after replace; Windows runs its installer
        // and exits the app. Force a relaunch so the user lands back in Parley
        // on any platform where the plugin didn't already do it.
        const { relaunch } = await import('@tauri-apps/plugin-process');
        await relaunch();
      },
    };
  } catch {
    return null;
  }
}
