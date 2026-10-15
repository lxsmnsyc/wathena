import { readFileSync } from 'node:fs';
import { type Plugin, type ProxyOptions, defineConfig } from 'vite';
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

/**
 * Serves the Service Worker at /sw.js in development, so its scope is the
 * whole site. The build writes it there too.
 */
function serviceWorker(): Plugin {
  return {
    name: 'wathena-sw',
    configureServer(server) {
      server.middlewares.use('/sw.js', (_req, res, next) => {
        server
          .transformRequest('/src/sw/sw.ts')
          .then((result) => {
            if (result === null) {
              next();
              return;
            }
            res.setHeader('Content-Type', 'text/javascript');
            res.end(result.code);
          })
          .catch(next);
      });
    },
  };
}

// In development, /assets is forwarded to `wactl assets serve`, which
// serves a build the way the game server will.
const assets = process.env.WATHENA_ASSETS ?? 'http://127.0.0.1:8200';

// For a test on a phone (docs/decisions/0034-touch-input.md),
// WATHENA_GAME names a game server, such as http://127.0.0.1:8300, and
// the dev server forwards the game's WebSocket, client data, emblems,
// account pages and admin API to it, so the phone needs to reach the dev
// server alone. The account pages and the client then share one site,
// which the login cookie needs.
const game = process.env.WATHENA_GAME;
const gameProxy: Record<string, ProxyOptions> =
  game === undefined
    ? {}
    : {
        '/ws': { target: game, ws: true },
        '/client-data.json': { target: game },
        '/emblems': { target: game },
        '/account': { target: game },
        '/admin/api': { target: game },
      };

// Browsers give crypto.subtle, which checks every asset, only to secure
// pages. A phone on the local network needs HTTPS for that:
// WATHENA_HTTPS_CERT and WATHENA_HTTPS_KEY name a PEM certificate and key.
const cert = process.env.WATHENA_HTTPS_CERT;
const key = process.env.WATHENA_HTTPS_KEY;
const https =
  cert === undefined || key === undefined
    ? undefined
    : { cert: readFileSync(cert), key: readFileSync(key) };

export default defineConfig({
  plugins: [tables(), serviceWorker()],
  server: {
    ...(https === undefined ? {} : { https }),
    proxy: {
      '/assets': {
        target: assets,
        rewrite: (path) => path.replace(/^\/assets/, ''),
      },
      ...gameProxy,
    },
  },
  build: {
    target: 'es2024',
    // /assets/ is where the client loads game assets from, so the app's
    // own files go elsewhere.
    assetsDir: 'app',
    rolldownOptions: {
      // admin.html is the admin screens' page. Its script loads the
      // admin bundle only after the server confirms a staff role.
      input: { main: 'index.html', admin: 'admin.html', sw: 'src/sw/sw.ts' },
      output: {
        entryFileNames: (chunk) => (chunk.name === 'sw' ? 'sw.js' : 'app/[name]-[hash].js'),
      },
    },
    // three.js alone is about 600 kB.
    chunkSizeWarningLimit: 1024,
  },
});
