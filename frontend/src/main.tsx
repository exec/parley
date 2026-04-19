import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import App from './App';
import './index.css';

// When VITE_SITE_URL is baked in (Tauri build targeting a remote site, or any
// cross-origin bundle), redirect relative `/api/...` fetches to the absolute
// URL. apiClient already does this for its own requests; this catch-all
// covers the handful of direct fetch() calls for unauthenticated auth routes.
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