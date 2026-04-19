/// <reference types="vitest" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { readFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';

// API_TARGET can be overridden for Docker Compose dev where the api
// service is reachable by container name rather than localhost.
// e.g. API_TARGET=http://api:8081 docker compose up
const apiTarget = process.env.API_TARGET ?? 'http://localhost:8081';
const wsTarget = apiTarget.replace(/^http/, 'ws');

// Single source of truth for the app version lives in the Tauri config —
// read it at build time so the web bundle shows the same thing the desktop
// About dialog does.
const tauriVersion = (() => {
  try {
    const raw = readFileSync(new URL('../desktop/src-tauri/tauri.conf.json', import.meta.url), 'utf8');
    return JSON.parse(raw).version as string;
  } catch { return '0.0.0'; }
})();

const gitSha = (() => {
  try { return execFileSync('git', ['rev-parse', '--short', 'HEAD']).toString().trim(); }
  catch { return ''; }
})();

export default defineConfig({
  plugins: [react()],
  assetsInclude: ['**/*.wasm'],
  define: {
    __APP_VERSION__: JSON.stringify(tauriVersion),
    __APP_COMMIT__: JSON.stringify(gitSha),
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
    css: false,
  },
  server: {
    proxy: {
      '/api': {
        target: apiTarget,
        changeOrigin: true,
      },
      '/ws': {
        target: wsTarget,
        changeOrigin: true,
        ws: true,
      },
    },
  },
});