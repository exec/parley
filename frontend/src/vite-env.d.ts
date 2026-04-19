/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_CDN_HOST: string;
  readonly VITE_SITE_URL: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

declare const __APP_VERSION__: string;
declare const __APP_COMMIT__: string;
