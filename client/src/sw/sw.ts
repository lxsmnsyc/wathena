/// <reference lib="webworker" />
// The Service Worker keeps downloaded assets in the Cache Storage API, as
// strategy section 10.1, "Loading in the client", describes:
//
// - Files named by a content hash never change, so they are served from
//   the cache first. Before caching one, the worker checks its SHA-256
//   against the hash in its name, so a bad proxy or CDN cannot poison the
//   cache.
// - manifest.json changes between builds, so it comes from the network
//   first, and from the cache only when the network fails.
// - Mods' builds, under mods/<mod id>/<version>/ (decision 0033), are
//   cached the same way as core's.
// - The cache has a byte budget. When it is over, the files used least
//   recently are removed, which also clears out old builds.
//
// This file has no imports or exports, so browsers can run it as a
// classic script. Its code is in a function, so it adds no globals.

(() => {
  if (!(self instanceof ServiceWorkerGlobalScope)) {
    return;
  }
  const worker: ServiceWorkerGlobalScope = self;

  /** The asset build's URL, from the registration's ?assets= parameter. */
  const ASSETS = new URL(
    new URL(worker.location.href).searchParams.get('assets') ?? '/assets/',
    worker.location.origin,
  ).href;

  const CACHE = 'wathena-assets-v1';
  const DB = 'wathena-assets';
  const STORE = 'files';

  /** Bytes of assets to keep. */
  const BUDGET = 1024 * 1024 * 1024;

  /** How long a file's last use may be out of date, to limit writes. */
  const TOUCH_MS = 60 * 60 * 1000;

  /** "<2 hex>/<64 hex>.<ext>", with the first two digits of the hash. */
  const HASHED = /^([0-9a-f]{2})\/([0-9a-f]{64})\.[a-z0-9]+$/;

  /** A mod's build: "mods/<mod id>/<version>/", then the file. */
  const MOD = /^(mods\/[a-z0-9._-]+\/[^/]+\/)(.*)$/;

  interface FileRecord {
    url: string;
    size: number;
    used: number;
  }

  function isRecord(v: unknown): v is FileRecord {
    return (
      typeof v === 'object' &&
      v !== null &&
      'url' in v &&
      typeof v.url === 'string' &&
      'size' in v &&
      typeof v.size === 'number' &&
      'used' in v &&
      typeof v.used === 'number'
    );
  }

  worker.addEventListener('install', (event) => {
    event.waitUntil(worker.skipWaiting());
  });

  worker.addEventListener('activate', (event) => {
    event.waitUntil(worker.clients.claim());
  });

  worker.addEventListener('fetch', (event) => {
    const { request } = event;
    if (request.method !== 'GET' || !request.url.startsWith(ASSETS)) {
      return;
    }
    let path = request.url.slice(ASSETS.length).split('?')[0] ?? '';
    // A mod's files are kept apart from core's, so evicting one does not
    // drop the other.
    let scope = '';
    const mod = MOD.exec(path);
    if (mod !== null) {
      scope = mod[1]!;
      path = mod[2]!;
    }
    if (path === 'manifest.json') {
      event.respondWith(networkFirst(request));
      return;
    }
    const m = HASHED.exec(path);
    if (m === null || m[1] !== m[2]!.slice(0, 2)) {
      return;
    }
    event.respondWith(cacheFirst(request, m[2]!, scope + m[2]!, event.clientId));
  });

  async function networkFirst(request: Request): Promise<Response> {
    const cache = await caches.open(CACHE);
    try {
      const res = await fetch(request, { cache: 'no-cache' });
      if (res.ok) {
        await cache.put(request.url, res.clone());
      }
      return res;
    } catch (error) {
      const hit = await cache.match(request.url);
      if (hit !== undefined) {
        return hit;
      }
      throw error;
    }
  }

  async function cacheFirst(
    request: Request,
    hash: string,
    key: string,
    clientId: string,
  ): Promise<Response> {
    const cache = await caches.open(CACHE);
    const hit = await cache.match(request.url);
    if (hit !== undefined) {
      touch(key, request.url);
      return hit;
    }
    const res = await fetch(request);
    if (!res.ok) {
      return res;
    }
    // The browser has removed any Brotli encoding, and the hash is of the
    // decoded content.
    const body = await res.arrayBuffer();
    if ((await sha256(body)) !== hash) {
      return new Response('content does not match its hash', { status: 502 });
    }
    const copy = new Response(body, {
      headers: { 'Content-Type': res.headers.get('Content-Type') ?? 'application/octet-stream' },
    });
    await cache.put(request.url, copy.clone());
    // Pages cannot see the bytes the worker downloads, so it tells them:
    // the size on the network when the server gives it, which is smaller
    // for Brotli files.
    const sent = Number(res.headers.get('Content-Length')) || body.byteLength;
    worker.clients
      .get(clientId)
      .then((client) => {
        client?.postMessage({ type: 'wathena-downloaded', bytes: sent });
      })
      .catch(() => undefined);
    await record(key, { url: request.url, size: body.byteLength, used: Date.now() });
    return copy;
  }

  async function sha256(data: ArrayBuffer): Promise<string> {
    const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', data));
    let out = '';
    for (const b of digest) {
      out += b.toString(16).padStart(2, '0');
    }
    return out;
  }

  // Records of cached files: size and last use, for eviction.

  let db: Promise<IDBDatabase> | undefined;

  async function open(): Promise<IDBDatabase> {
    db ??= new Promise((resolve, reject) => {
      const req = indexedDB.open(DB, 1);
      req.onupgradeneeded = () => {
        req.result.createObjectStore(STORE).createIndex('used', 'used');
      };
      req.onsuccess = () => {
        resolve(req.result);
      };
      req.onerror = () => {
        reject(req.error ?? new Error('indexedDB'));
      };
    });
    return db;
  }

  async function done(tx: IDBTransaction): Promise<void> {
    return new Promise((resolve, reject) => {
      tx.oncomplete = () => {
        resolve();
      };
      tx.onerror = () => {
        reject(tx.error ?? new Error('indexedDB'));
      };
      tx.onabort = tx.onerror;
    });
  }

  /** Last uses written recently, so repeated hits do not write each time. */
  const touched = new Map<string, number>();

  function touch(hash: string, url: string): void {
    const now = Date.now();
    if (now - (touched.get(hash) ?? 0) < TOUCH_MS) {
      return;
    }
    touched.set(hash, now);
    open()
      .then(async (d) => {
        const tx = d.transaction(STORE, 'readwrite');
        const store = tx.objectStore(STORE);
        const req = store.get(hash);
        req.onsuccess = () => {
          const r: unknown = req.result;
          store.put({ url, size: isRecord(r) ? r.size : 0, used: now } satisfies FileRecord, hash);
        };
        await done(tx);
      })
      .catch(() => undefined);
  }

  let total: number | undefined;

  async function record(hash: string, r: FileRecord): Promise<void> {
    const d = await open();
    const tx = d.transaction(STORE, 'readwrite');
    tx.objectStore(STORE).put(r, hash);
    await done(tx);
    touched.set(hash, r.used);
    total ??= await sumSizes(d);
    total += r.size;
    if (total > BUDGET) {
      total = await evict(d);
    }
  }

  async function sumSizes(d: IDBDatabase): Promise<number> {
    const tx = d.transaction(STORE, 'readonly');
    const req = tx.objectStore(STORE).getAll();
    await done(tx);
    const all: unknown[] = req.result;
    return all.reduce<number>((sum, r) => sum + (isRecord(r) ? r.size : 0), 0);
  }

  /** Removes the least recently used files until the cache is at 90% of its budget. */
  async function evict(d: IDBDatabase): Promise<number> {
    let size = await sumSizes(d);
    const cache = await caches.open(CACHE);
    const tx = d.transaction(STORE, 'readwrite');
    const store = tx.objectStore(STORE);
    const drop: string[] = [];
    await new Promise<void>((resolve) => {
      const req = store.index('used').openCursor();
      req.onsuccess = () => {
        const cursor = req.result;
        if (cursor === null || size <= BUDGET * 0.9) {
          resolve();
          return;
        }
        const r: unknown = cursor.value;
        if (isRecord(r)) {
          size -= r.size;
          drop.push(r.url);
        }
        if (typeof cursor.primaryKey === 'string') {
          touched.delete(cursor.primaryKey);
        }
        cursor.delete();
        cursor.continue();
      };
      req.onerror = () => {
        resolve();
      };
    });
    await done(tx);
    await Promise.all(drop.map(async (url) => cache.delete(url)));
    return size;
  }
})();
