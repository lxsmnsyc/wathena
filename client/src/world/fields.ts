// Generated maps of procedural worlds: fields (decision 0039), dungeon
// floors and towns (decision 0040). The server sends each map's public
// node and checksum at /gen/maps/<id>.json, with the params of a floor or
// a town. The client generates the map with gen.wasm in a Web Worker,
// compares the checksum, and downloads the server's WMAP when they
// differ or the module cannot make the map.

import { type Fetch, sha256 } from '../assets/store';

/** Whether a map name is a generated map's ID, such as gen:field:3:-1. */
export function isGenerated(name: string): boolean {
  return name.startsWith('gen:');
}

/** What the server says about a generated map. */
export interface FieldInfo {
  id: string;
  /** field, dungeon or town. A server before version 3 sends none. */
  kind?: 'field' | 'dungeon' | 'town';
  /** A dungeon floor's or a town's params, as hex. */
  params?: string;
  /** A dungeon floor's style or a town's template. */
  name?: string;
  version: number;
  rng: string;
  /** The field's public node, 64 hex digits. */
  key: string;
  width: number;
  height: number;
  biome: number;
  biomeName: string;
  /** The grid's checksum, 64 hex digits. */
  checksum: string;
  /** The path of the field's WMAP on the server. */
  wmap: string;
  wmapSha256: string;
  wmapBytes: number;
}

const HEX64 = /^[0-9a-f]{64}$/;

function isFieldInfo(v: unknown): v is FieldInfo {
  if (typeof v !== 'object' || v === null) {
    return false;
  }
  const s = (k: string): unknown => Reflect.get(v, k);
  const num = (k: string): boolean => Number.isInteger(s(k));
  const str = (k: string): boolean => typeof s(k) === 'string';
  const kind = s('kind');
  const kindOK =
    kind === undefined ||
    kind === 'field' ||
    ((kind === 'dungeon' || kind === 'town') && /^(?:[0-9a-f]{2})+$/.test(String(s('params'))));
  return (
    kindOK &&
    str('id') &&
    num('version') &&
    str('rng') &&
    HEX64.test(String(s('key'))) &&
    num('width') &&
    num('height') &&
    num('biome') &&
    str('biomeName') &&
    HEX64.test(String(s('checksum'))) &&
    str('wmap') &&
    String(s('wmap')).startsWith('/gen/') &&
    HEX64.test(String(s('wmapSha256'))) &&
    num('wmapBytes')
  );
}

/** A field the generator made. */
export interface Generated {
  checksum: string;
  wmap: Uint8Array<ArrayBuffer>;
  attempt: number;
}

/** Makes a field from its info, such as gen.wasm in a worker. */
export type FieldGenerator = (info: FieldInfo) => Promise<Generated>;

/** A loaded field, and where its map came from. */
export interface LoadedField {
  info: FieldInfo;
  wmap: Uint8Array<ArrayBuffer>;
  source: 'generated' | 'downloaded';
  /** Why the client downloaded the map, when it did. */
  reason?: string;
  /** Milliseconds the generator took, when it ran. */
  generateMs?: number;
  /** Milliseconds from the start to the map's bytes. */
  ms: number;
}

/** The largest field map the client downloads, as WMAP's limit. */
const MAX_WMAP = 64 << 20;

/**
 * Loads a field: its info from the server, then its map from the
 * generator, or from the server when the generator fails or its checksum
 * differs. A downloaded map must have the SHA-256 the info gives.
 */
export async function loadField(
  origin: string,
  id: string,
  generate: FieldGenerator | undefined,
  fetcher: Fetch = async (url, init) => fetch(url, init),
): Promise<LoadedField> {
  const began = performance.now();
  const res = await fetcher(`${origin}/gen/maps/${encodeURIComponent(id)}.json`, {
    cache: 'no-cache',
  });
  if (!res.ok) {
    throw new Error(`field ${id}: HTTP ${res.status}`);
  }
  const info: unknown = await res.json();
  if (!isFieldInfo(info) || info.id !== id) {
    throw new Error(`field ${id}: invalid info`);
  }
  let reason = 'this client has no map generator';
  let generateMs: number | undefined;
  if (generate !== undefined) {
    const start = performance.now();
    try {
      const g = await generate(info);
      generateMs = performance.now() - start;
      if (g.checksum === info.checksum) {
        return {
          info,
          wmap: g.wmap,
          source: 'generated',
          generateMs,
          ms: performance.now() - began,
        };
      }
      reason = `the checksum ${g.checksum.slice(0, 16)} differs from the server's ${info.checksum.slice(0, 16)}`;
    } catch (error) {
      reason = `the generator failed: ${String(error)}`;
    }
  }
  console.warn(`field ${id}: downloading the map, since ${reason}`);
  if (info.wmapBytes > MAX_WMAP) {
    throw new Error(`field ${id}: ${info.wmapBytes} bytes is over the limit`);
  }
  const dl = await fetcher(`${origin}${info.wmap}`);
  if (!dl.ok) {
    throw new Error(`field ${id}: map download: HTTP ${dl.status}`);
  }
  const wmap = new Uint8Array(await dl.arrayBuffer());
  if ((await sha256(wmap)) !== info.wmapSha256) {
    throw new Error(`field ${id}: the downloaded map does not have its SHA-256`);
  }
  return {
    info,
    wmap,
    source: 'downloaded',
    reason,
    ...(generateMs === undefined ? {} : { generateMs }),
    ms: performance.now() - began,
  };
}

/**
 * Loads the tileset table: the asset each generated texture and model
 * name stands for. Names it lacks show placeholders.
 */
export async function loadTilesets(
  origin: string,
  fetcher: Fetch = async (url, init) => fetch(url, init),
): Promise<Map<string, string>> {
  const res = await fetcher(`${origin}/gen/tilesets.json`, { cache: 'no-cache' });
  if (!res.ok) {
    throw new Error(`tilesets: HTTP ${res.status}`);
  }
  const t: unknown = await res.json();
  const aliases: unknown =
    typeof t === 'object' && t !== null && Reflect.get(t, 'version') === 1
      ? Reflect.get(t, 'aliases')
      : undefined;
  if (typeof aliases !== 'object' || aliases === null) {
    throw new Error('tilesets: invalid');
  }
  const out = new Map<string, string>();
  for (const [from, to] of Object.entries(aliases)) {
    if (typeof to === 'string' && /^(texture|model)\/gen\//.test(from)) {
      out.set(from, to);
    }
  }
  return out;
}
