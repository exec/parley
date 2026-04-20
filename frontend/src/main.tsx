import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import App from './App';
import './index.css';

// When VITE_SITE_URL is baked in (Tauri build targeting a remote site, or any
// cross-origin bundle), redirect relative `/api/...` fetches to the absolute
// URL. apiClient already does this for its own requests; this catch-all
// covers the handful of direct fetch() calls for unauthenticated auth routes.
// Flag the document when running inside a Tauri webview so CSS can carve out
// space for native window chrome (e.g. macOS traffic-light buttons overlap
// content because we use titleBarStyle: "Overlay").
if (typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window) {
  document.documentElement.dataset.tauri = '1';
  const ua = navigator.userAgent;
  // iOS UA contains "Mac" ("CPU iPhone OS ... like Mac OS X"), so iOS/Android
  // must be matched before the macOS check.
  const platform = /iPad|iPhone|iPod/.test(ua) ? 'ios'
    : /Android/.test(ua) ? 'android'
    : /Mac/i.test(ua) ? 'macos'
    : /Windows/i.test(ua) ? 'windows'
    : 'linux';
  document.documentElement.dataset.tauriPlatform = platform;
}

const __SITE_URL = (import.meta.env.VITE_SITE_URL as string) || '';
if (__SITE_URL && typeof window !== 'undefined') {
  const __origFetch = window.fetch.bind(window);
  const __base = __SITE_URL.replace(/\/$/, '');
  window.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
    if (typeof input === 'string' && input.startsWith('/api/')) {
      return __origFetch(__base + input, init);
    }
    return __origFetch(input as any, init);
  }) as typeof window.fetch;
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </React.StrictMode>
);