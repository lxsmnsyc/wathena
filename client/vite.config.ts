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

export default defineConfig({
  plugins: [tables(), serviceWorker()],
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
