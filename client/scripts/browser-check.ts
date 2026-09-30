// Runs the client in real browsers and checks what unit tests cannot:
//
// - audio: each browser decodes the encoder's Opus test sounds to their
//   exact length (src/audio/testdata).
// - render: the map viewer draws a map with sprites and no page errors.
// - cache: on a second visit, the Service Worker serves every asset.
//
// By default it builds the placeholder pack with wactl and serves it, so
// it needs no game files. Set WATHENA_ASSETS to an asset server, such as
// `wactl assets serve`, and MAP to a map name to check other builds.
//
//   pnpm check:browsers
//   BROWSERS=chromium,firefox,webkit pnpm check:browsers
//
// Browsers come from `playwright-core install`, or CHROME_PATH for an
// installed Chrome. SCREENSHOTS names a folder to save each browser's
// render in.

import { type ChildProcess, execFileSync, spawn } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { type Browser, type Page, chromium, firefox, webkit } from 'playwright-core';
import { createServer } from 'vite';

const repo = join(import.meta.dirname, '..', '..');
const client = join(repo, 'client');
const map = process.env.MAP ?? 'placeholder/map';
const browsers = (process.env.BROWSERS ?? 'chromium').split(',');

const failures: string[] = [];

function check(browser: string, name: string, ok: boolean, detail: string): void {
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${browser} ${name}: ${detail}`);
  if (!ok) {
    failures.push(`${browser} ${name}`);
  }
}

async function waitFor(url: string, seconds: number): Promise<void> {
  for (let i = 0; i < seconds * 10; i++) {
    try {
      if ((await fetch(url)).ok) {
        return;
      }
    } catch {
      // Not listening yet.
    }
    await new Promise((resolve) => {
      setTimeout(resolve, 100);
    });
  }
  throw new Error(`${url} did not answer`);
}

/** Builds and serves the placeholder pack, unless WATHENA_ASSETS is set. */
async function assetServer(): Promise<{ url: string; stop: () => void }> {
  const given = process.env.WATHENA_ASSETS;
  if (given !== undefined) {
    return { url: given, stop: () => undefined };
  }
  const dir = mkdtempSync(join(tmpdir(), 'wathena-check-'));
  const wactl = join(dir, 'wactl');
  execFileSync('go', ['build', '-o', wactl, './tools/wactl'], { cwd: repo, stdio: 'inherit' });
  execFileSync(wactl, ['assets', 'build', '--source', 'placeholder', '--out', join(dir, 'build')], {
    stdio: 'inherit',
  });
  const addr = '127.0.0.1:8299';
  const server: ChildProcess = spawn(
    wactl,
    ['assets', 'serve', '--build', join(dir, 'build'), '--addr', addr],
    {
      stdio: 'ignore',
    },
  );
  const url = `http://${addr}`;
  await waitFor(`${url}/manifest.json`, 10);
  return {
    url,
    stop: () => {
      server.kill();
      rmSync(dir, { recursive: true, force: true });
    },
  };
}

async function launch(name: string): Promise<Browser> {
  switch (name) {
    case 'chromium':
      return chromium.launch({
        ...(process.env.CHROME_PATH === undefined
          ? {}
          : { executablePath: process.env.CHROME_PATH }),
        // Headless Linux has no GPU, so WebGL runs on SwiftShader.
        args: ['--enable-unsafe-swiftshader', '--ignore-gpu-blocklist'],
      });
    case 'firefox':
      return firefox.launch();
    case 'webkit':
      return webkit.launch();
    default:
      throw new Error(`unknown browser ${name}`);
  }
}

async function loaded(page: Page): Promise<string> {
  await page.waitForFunction(
    () => /done|Error/.test(document.querySelector<HTMLOutputElement>('#status')?.value ?? ''),
    null,
    { timeout: 90_000 },
  );
  return page.evaluate(() => document.querySelector<HTMLOutputElement>('#status')?.value ?? '');
}

interface TestSound {
  file: string;
  channels: number;
  frames: number;
}

function isTestSound(v: unknown): v is TestSound {
  return (
    typeof v === 'object' &&
    v !== null &&
    'file' in v &&
    typeof v.file === 'string' &&
    'channels' in v &&
    typeof v.channels === 'number' &&
    'frames' in v &&
    typeof v.frames === 'number'
  );
}

/** The test sounds and their lengths, which the Go encoder's tests write. */
function sounds(): TestSound[] {
  const list: unknown = JSON.parse(
    readFileSync(join(client, 'src/audio/testdata/sounds.json'), 'utf8'),
  );
  if (!Array.isArray(list) || !list.every(isTestSound)) {
    throw new Error('sounds.json: invalid');
  }
  return list;
}

async function checkAudio(name: string, browser: Browser, base: string): Promise<void> {
  const page = await browser.newPage();
  await page.goto(`${base}/src/audio/testdata/sounds.json`);
  const results = await page.evaluate(async (list) => {
    const out: { file: string; want: string; got: string }[] = [];
    for (const s of list) {
      const data = await (await fetch(`/src/audio/testdata/${s.file}`)).arrayBuffer();
      const b = await new OfflineAudioContext(1, 1, 48000).decodeAudioData(data);
      out.push({
        file: s.file,
        want: `${s.frames} frames, ${s.channels} ch`,
        got: `${b.length} frames, ${b.numberOfChannels} ch`,
      });
    }
    return out;
  }, sounds());
  for (const r of results) {
    check(name, `audio ${r.file}`, r.got === r.want, r.got);
  }
  await page.close();
}

async function checkRender(name: string, browser: Browser, base: string): Promise<void> {
  const page = await browser.newPage({ viewport: { width: 800, height: 600 } });
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto(`${base}/?map=${encodeURIComponent(map)}&characters=5&drops=&nosw`);
  const status = await loaded(page);
  check(name, 'render status', status.startsWith('done'), status);
  check(name, 'render errors', errors.length === 0, errors.join('; ') || 'none');
  // A drawn scene has many colors. A blank or failed canvas has few.
  const png = await page.screenshot();
  if (process.env.SCREENSHOTS !== undefined) {
    mkdirSync(process.env.SCREENSHOTS, { recursive: true });
    writeFileSync(join(process.env.SCREENSHOTS, `${name}-render.png`), png);
  }
  const shot = png.toString('base64');
  const colors = await page.evaluate(async (data) => {
    const img = new Image();
    img.src = `data:image/png;base64,${data}`;
    await img.decode();
    const c = document.createElement('canvas');
    c.width = img.width;
    c.height = img.height;
    const g = c.getContext('2d');
    if (g === null) {
      return 0;
    }
    g.drawImage(img, 0, 0);
    const d = g.getImageData(0, 0, c.width, c.height).data;
    const seen = new Set<number>();
    for (let i = 0; i < d.length; i += 4 * 7) {
      seen.add((d[i]! << 16) | (d[i + 1]! << 8) | d[i + 2]!);
    }
    return seen.size;
  }, shot);
  check(name, 'render pixels', colors > 50, `${colors} colors`);
  await page.close();
}

async function checkCache(name: string, browser: Browser, base: string): Promise<void> {
  if (name === 'firefox') {
    // Playwright's Firefox does not use service workers for navigations
    // after the first, even with a minimal worker.
    console.log(`skip ${name} cache: not testable in Playwright's Firefox`);
    return;
  }
  const context = await browser.newContext({ serviceWorkers: 'allow' });
  const visit = async (): Promise<{ worker: number; network: number }> => {
    const page = await context.newPage();
    const counts = { worker: 0, network: 0 };
    page.on('response', (r) => {
      // Hashed asset files, not the manifest.
      if (/\/assets\/[0-9a-f]{2}\/[0-9a-f]{64}\./.test(r.url())) {
        counts[r.fromServiceWorker() ? 'worker' : 'network']++;
      }
    });
    await page.goto(`${base}/?map=${encodeURIComponent(map)}&characters=0&drops=`);
    await loaded(page);
    // The worker may still be installing after a fast first load.
    await page.evaluate(async () => {
      await navigator.serviceWorker.ready;
    });
    await page.close();
    return counts;
  };
  await visit();
  const second = await visit();
  check(
    name,
    'cache second visit',
    second.worker > 0 && second.network === 0,
    `${second.worker} from the worker, ${second.network} from the network`,
  );
  await context.close();
}

const assets = await assetServer();
process.env.WATHENA_ASSETS = assets.url;
const vite = await createServer({
  root: client,
  server: { port: 5190, strictPort: false },
  logLevel: 'error',
});
await vite.listen();
const base = (vite.resolvedUrls?.local[0] ?? 'http://localhost:5190/').replace(/\/$/, '');
try {
  for (const name of browsers) {
    const browser = await launch(name);
    try {
      await checkAudio(name, browser, base);
      await checkRender(name, browser, base);
      await checkCache(name, browser, base);
    } finally {
      await browser.close();
    }
  }
} finally {
  await vite.close();
  assets.stop();
}
if (failures.length > 0) {
  console.error(`${failures.length} checks failed`);
  process.exit(1);
}
