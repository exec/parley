/// <reference types="vitest" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// API_TARGET can be overridden for Docker Compose dev where the api
// service is reachable by container name rather than localhost.
// e.g. API_TARGET=http://api:8081 docker compose up
const apiTarget = process.env.API_TARGET ?? 'http://localhost:8081';
const wsTarget = apiTarget.replace(/^http/, 'ws');

export default defineConfig({
  plugins: [react()],
  optimizeDeps: {
    exclude: ['@ricky0123/vad-web'],
  },
  assetsInclude: ['**/*.wasm'],
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