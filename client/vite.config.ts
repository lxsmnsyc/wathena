import { type Plugin, defineConfig } from 'vite';
import { parse } from 'yaml';

/** Imports the client data tables, YAML files in data/, as JSON. */
function tables(): Plugin {
  return {
    name: 'wathena-tables',
    transform(code, id) {
      if (!id.endsWith('.yml')) {
        return null;
      }
      return { code: `export default ${JSON.stringify(parse(code))};`, map: null };
    },
  };
}

// In development, /assets is forwarded to `wactl assets serve`, which
// serves a build the way the game server will.
const assets = process.env.WATHENA_ASSETS ?? 'http://127.0.0.1:8200';

export default defineConfig({
  plugins: [tables()],
  server: {
    proxy: {
      '/assets': {
        target: assets,
        rewrite: (path) => path.replace(/^\/assets/, ''),
      },
    },
  },
  build: {
    target: 'es2024',
    // three.js alone is about 600 kB.
    chunkSizeWarningLimit: 1024,
  },
});
