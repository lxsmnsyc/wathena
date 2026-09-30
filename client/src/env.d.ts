/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** The asset build's URL, ending in "/". Defaults to /assets/. */
  readonly VITE_ASSETS_URL?: string;
}

/** Client data tables. src/data/tables.ts checks their shape. */
declare module '*.yml' {
  const table: unknown;
  export default table;
}
