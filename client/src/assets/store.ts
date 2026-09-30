// Loads assets by name from a build, as defined in docs/specs/assets-v1.md.
// Every file is checked against the SHA-256 the manifest gives for it, so
// a bad proxy or CDN cannot change what the client uses.

export const CONTRACT = 'assets-v1';

export type AssetType =
  | 'sprite'
  | 'palette'
  | 'texture'
  | 'model'
  | 'map'
  | 'effect'
  | 'sound'
  | 'music';

export interface Entry {
  /** SHA-256 of the content, lowercase hex. */
  h: string;
  t: AssetType;
  /** Size in bytes, before any transfer encoding. */
  s: number;
}

interface Manifest {
  contract: string;
  build: string;
  shards: string[];
}

const EXTENSIONS: Record<AssetType, string> = {
  sprite: 'wspr',
  palette: 'wpal',
  texture: 'webp',
  model: 'glb',
  map: 'wmap',
  effect: 'weff',
  sound: 'webm',
  music: 'mp3',
};

const MB = 1024 * 1024;

/** The largest file of each type the client accepts. */
const LIMITS: Record<AssetType, number> = {
  sprite: 32 * MB,
  palette: 4096,
  texture: 64 * MB,
  model: 16 * MB,
  map: 64 * MB,
  effect: 8 * MB,
  sound: 8 * MB,
  music: 32 * MB,
};

/** The largest manifest shard the client accepts. */
const MAX_SHARD_BYTES = 16 * MB;

export class AssetError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'AssetError';
  }
}

const HASH = /^[0-9a-f]{64}$/;

function isEntry(v: unknown): v is Entry {
  if (typeof v !== 'object' || v === null || !('h' in v) || !('t' in v) || !('s' in v)) {
    return false;
  }
  const { h, t, s } = v;
  return (
    typeof h === 'string' &&
    HASH.test(h) &&
    typeof t === 'string' &&
    Object.hasOwn(EXTENSIONS, t) &&
    typeof s === 'number' &&
    Number.isSafeInteger(s) &&
    s >= 0
  );
}

function isManifest(v: unknown): v is Manifest {
  if (
    typeof v !== 'object' ||
    v === null ||
    !('contract' in v) ||
    !('build' in v) ||
    !('shards' in v)
  ) {
    return false;
  }
  const { build, shards } = v;
  return (
    typeof build === 'string' &&
    Array.isArray(shards) &&
    shards.length === 256 &&
    shards.every((h) => typeof h === 'string' && HASH.test(h))
  );
}

export async function sha256(data: BufferSource): Promise<string> {
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', data));
  let out = '';
  for (const b of digest) {
    out += b.toString(16).padStart(2, '0');
  }
  return out;
}

const encoder = new TextEncoder();

/** The manifest shard of a name: the first byte of its SHA-256. */
export async function shardOf(name: string): Promise<number> {
  const digest = await crypto.subtle.digest('SHA-256', encoder.encode(name));
  return new Uint8Array(digest)[0] ?? 0;
}

export interface StoreStats {
  /** Files downloaded and checked. */
  files: number;
  /** Bytes of content, after the browser removes any transfer encoding. */
  bytes: number;
}

export type Fetch = (url: string, init?: RequestInit) => Promise<Response>;

/**
 * An asset build on a server. Names resolve through the manifest's shards,
 * which load when first needed.
 */
export class AssetStore {
  readonly build: string;
  readonly stats: StoreStats = { files: 0, bytes: 0 };

  private readonly shards = new Map<number, Promise<Map<string, Entry>>>();
  private readonly files = new Map<string, Promise<Uint8Array<ArrayBuffer>>>();
  private readonly missing = new Set<string>();

  private constructor(
    private readonly base: string,
    private readonly manifest: Manifest,
    private readonly fetcher: Fetch,
  ) {
    this.build = manifest.build;
  }

  /** Loads a build's manifest. base is the build's URL, ending in "/". */
  static async open(
    base: string,
    fetcher: Fetch = async (url, init) => fetch(url, init),
  ): Promise<AssetStore> {
    const res = await fetcher(`${base}manifest.json`, { cache: 'no-cache' });
    if (!res.ok) {
      throw new AssetError(`manifest.json: HTTP ${res.status}`);
    }
    const m: unknown = await res.json();
    if (!isManifest(m)) {
      throw new AssetError('manifest.json: invalid');
    }
    if (m.contract !== CONTRACT) {
      throw new AssetError(`manifest.json: contract ${m.contract}`);
    }
    return new AssetStore(base, m, fetcher);
  }

  /** Looks up a name. Undefined means the build does not have it. */
  async entry(name: string): Promise<Entry | undefined> {
    const shard = await this.shard(await shardOf(name));
    return shard.get(name);
  }

  /**
   * Downloads an asset and checks it. Undefined means the build does not
   * have it, which is logged once per name. A type other than expected
   * is an error.
   */
  async bytes(name: string, expected: AssetType): Promise<Uint8Array<ArrayBuffer> | undefined> {
    const e = await this.entry(name);
    if (e === undefined) {
      if (!this.missing.has(name)) {
        this.missing.add(name);
        console.warn(`asset missing: ${name}`);
      }
      return undefined;
    }
    if (e.t !== expected) {
      throw new AssetError(`${name}: is a ${e.t}, not a ${expected}`);
    }
    // Names that share content share one download.
    let file = this.files.get(e.h);
    if (file === undefined) {
      file = this.download(name, e);
      this.files.set(e.h, file);
      // A failed download can be tried again.
      file.catch(() => this.files.delete(e.h));
    }
    return file;
  }

  /** The URL of a stored file. */
  url(e: Entry): string {
    return `${this.base}${e.h.slice(0, 2)}/${e.h}.${EXTENSIONS[e.t]}`;
  }

  private async download(name: string, e: Entry): Promise<Uint8Array<ArrayBuffer>> {
    if (e.s > LIMITS[e.t]) {
      throw new AssetError(`${name}: ${e.s} bytes is over the limit`);
    }
    const data = await this.fetchChecked(this.url(e), e.h, e.s);
    this.stats.files++;
    this.stats.bytes += data.byteLength;
    return data;
  }

  private async shard(index: number): Promise<Map<string, Entry>> {
    let shard = this.shards.get(index);
    if (shard === undefined) {
      shard = this.loadShard(index);
      this.shards.set(index, shard);
      shard.catch(() => this.shards.delete(index));
    }
    return shard;
  }

  private async loadShard(index: number): Promise<Map<string, Entry>> {
    const h = this.manifest.shards[index] ?? '';
    const data = await this.fetchChecked(`${this.base}${h.slice(0, 2)}/${h}.json`, h, undefined);
    const raw: unknown = JSON.parse(new TextDecoder().decode(data));
    if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) {
      throw new AssetError(`shard ${index}: invalid`);
    }
    const out = new Map<string, Entry>();
    for (const [name, e] of Object.entries(raw)) {
      if (!isEntry(e)) {
        throw new AssetError(`shard ${index}: invalid entry ${name}`);
      }
      out.set(name, { h: e.h, t: e.t, s: e.s });
    }
    return out;
  }

  /** Fetches a file and checks its size, when known, and hash. */
  private async fetchChecked(
    url: string,
    hash: string,
    size: number | undefined,
  ): Promise<Uint8Array<ArrayBuffer>> {
    const res = await this.fetcher(url);
    if (!res.ok) {
      throw new AssetError(`${url}: HTTP ${res.status}`);
    }
    const data = new Uint8Array(await res.arrayBuffer());
    if (size === undefined ? data.byteLength > MAX_SHARD_BYTES : data.byteLength !== size) {
      throw new AssetError(`${url}: ${data.byteLength} bytes, want ${size}`);
    }
    if ((await sha256(data)) !== hash) {
      throw new AssetError(`${url}: content does not match its hash`);
    }
    return data;
  }
}
