// The map generator of procedural worlds, from the shared Go package
// compiled to WebAssembly with TinyGo (shared/gen/wasm). The server runs
// the same code, so a grid's checksum matches the server's
// (docs/specs/generator-v1.md). The client loads the module only for a
// procedural world.

import genWasm from './gen.wasm?url';

/** A generated cell grid, row by row from y = 0. */
export interface GeneratedGrid {
  width: number;
  height: number;
  /** The generator version that made it. */
  version: number;
  /** GAT cell types, one per cell. */
  types: Uint8Array;
  /** Four corner heights per cell, Q16.16 in cells, up positive. */
  heights: Int32Array;
  /** The water level, Q16.16 in cells. */
  water: number;
  /** The grid's SHA-256 checksum, as lowercase hex. */
  checksum: string;
  /**
   * The field as a WMAP map, for a version with fields
   * (docs/specs/generator-v2.md). Empty for a version without them.
   */
  wmap: Uint8Array<ArrayBuffer>;
  /** The attempt that made the field, 0 for its own node. */
  attempt: number;
  /**
   * The map's named places in their canonical encoding, for version 3
   * and later (docs/specs/generator-v3.md). The client does not read
   * them; the checks hash them.
   */
  places: Uint8Array<ArrayBuffer>;
}

/** The kinds of maps generateMap makes, as the module numbers them. */
export const MAP_KINDS = { dungeon: 1, town: 2 } as const;

/** A kind of map that generateMap makes. */
export type MapKind = keyof typeof MAP_KINDS;

interface Exports {
  memory: WebAssembly.Memory;
  _initialize?: () => void;
  key: () => number;
  rng_version: (version: number) => number;
  latest: () => number;
  generate: (version: number, width: number, height: number, biome: number) => number;
  types: () => number;
  heights: () => number;
  water: () => number;
  checksum: () => number;
  biomes: (version: number) => number;
  wmap: () => number;
  wmap_size: () => number;
  attempt: () => number;
  params: () => number;
  generate_map: (version: number, kind: number, length: number) => number;
  width: () => number;
  height: () => number;
  places: () => number;
  places_size: () => number;
}

const FUNCTIONS = [
  'key',
  'rng_version',
  'latest',
  'generate',
  'types',
  'heights',
  'water',
  'checksum',
  'biomes',
  'wmap',
  'wmap_size',
  'attempt',
  'params',
  'generate_map',
  'width',
  'height',
  'places',
  'places_size',
] as const;

/** The longest params a dungeon floor or a town takes, gen.MaxParams. */
const MAX_PARAMS = 16 << 10;

function isExports(v: unknown): v is Exports {
  if (typeof v !== 'object' || v === null) {
    return false;
  }
  return (
    'memory' in v &&
    v.memory instanceof WebAssembly.Memory &&
    FUNCTIONS.every((f) => typeof Reflect.get(v, f) === 'function')
  );
}

export function hex(bytes: Uint8Array): string {
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

/** Reads bytes from lowercase hex digits. */
export function bytesFromHex(text: string): Uint8Array {
  if (!/^(?:[0-9a-f]{2})*$/.test(text)) {
    throw new Error('want an even number of lowercase hex digits');
  }
  const out = new Uint8Array(text.length / 2);
  for (let i = 0; i < out.length; i++) {
    out[i] = Number.parseInt(text.slice(2 * i, 2 * i + 2), 16);
  }
  return out;
}

/** Reads a 32-byte key from 64 hex digits. */
export function keyFromHex(text: string): Uint8Array {
  if (!/^[0-9a-f]{64}$/.test(text)) {
    throw new Error('a node key is 64 lowercase hex digits');
  }
  const out = new Uint8Array(32);
  for (let i = 0; i < 32; i++) {
    out[i] = Number.parseInt(text.slice(2 * i, 2 * i + 2), 16);
  }
  return out;
}

export class Generator {
  private constructor(private readonly e: Exports) {}

  /** Loads the module from the client's build. */
  static async load(): Promise<Generator> {
    const res = await fetch(genWasm);
    if (!res.ok) {
      throw new Error(`gen.wasm: HTTP ${res.status}`);
    }
    return Generator.fromBytes(await res.arrayBuffer());
  }

  static async fromBytes(bytes: BufferSource): Promise<Generator> {
    const { instance } = await WebAssembly.instantiate(bytes, {});
    const e: unknown = instance.exports;
    if (!isExports(e)) {
      throw new Error('gen.wasm: missing exports');
    }
    // TinyGo names the start-up function this way.
    // oxlint-disable-next-line no-underscore-dangle
    e._initialize?.();
    return new Generator(e);
  }

  /** The module's memory in bytes, which only grows. */
  get memoryBytes(): number {
    return this.e.memory.buffer.byteLength;
  }

  /** The newest generator version in the module. */
  get latest(): number {
    return this.e.latest();
  }

  /**
   * Whether the module has a generator version, on the rng spec version
   * the server named, such as "rng-v1".
   */
  supports(version: number, rng: string): boolean {
    const n = this.e.rng_version(version);
    return n > 0 && rng === `rng-v${n}`;
  }

  /** The number of biomes of a version, 0 for a version without fields. */
  biomes(version: number): number {
    return Math.max(this.e.biomes(version), 0);
  }

  /**
   * Generates a map from its public node key: a field of the biome for a
   * version with fields, else a grid.
   */
  generate(
    version: number,
    key: Uint8Array,
    width: number,
    height: number,
    biome = 0,
  ): GeneratedGrid {
    if (key.length !== 32) {
      throw new Error('a node key is 32 bytes');
    }
    new Uint8Array(this.e.memory.buffer, this.e.key(), 32).set(key);
    const code = this.e.generate(version, width, height, biome);
    switch (code) {
      case 0:
        break;
      case -1:
        throw new Error(`gen.wasm: no generator version ${version}`);
      case -3:
        throw new Error(`gen.wasm: no valid field after every attempt`);
      case -4:
        throw new Error(`gen.wasm: no biome ${biome} in version ${version}`);
      default:
        throw new Error(`gen.wasm: invalid size ${width} × ${height}`);
    }
    return this.result(version);
  }

  /**
   * Generates a dungeon floor or a town from its public node key and its
   * params, the bytes the server sends as hex (docs/specs/generator-v3.md).
   */
  generateMap(version: number, key: Uint8Array, kind: MapKind, params: Uint8Array): GeneratedGrid {
    if (key.length !== 32) {
      throw new Error('a node key is 32 bytes');
    }
    if (params.length > MAX_PARAMS) {
      throw new Error('gen.wasm: the params are too long');
    }
    new Uint8Array(this.e.memory.buffer, this.e.key(), 32).set(key);
    new Uint8Array(this.e.memory.buffer, this.e.params(), params.length).set(params);
    const code = this.e.generate_map(version, MAP_KINDS[kind], params.length);
    switch (code) {
      case 0:
        break;
      case -1:
        throw new Error(`gen.wasm: no ${kind} in generator version ${version}`);
      case -3:
        throw new Error(`gen.wasm: no valid ${kind} after every attempt`);
      case -4:
        throw new Error(`gen.wasm: the ${kind}'s tileset or props are not in version ${version}`);
      default:
        throw new Error(`gen.wasm: invalid ${kind} params`);
    }
    return this.result(version);
  }

  /** Copies the last map out of the module's memory. */
  private result(version: number): GeneratedGrid {
    // The memory can grow during generate, so views are made after it.
    const width = this.e.width();
    const height = this.e.height();
    const cells = width * height;
    const buffer = this.e.memory.buffer;
    return {
      width,
      height,
      version,
      types: new Uint8Array(buffer, this.e.types(), cells).slice(),
      heights: new Int32Array(buffer, this.e.heights(), 4 * cells).slice(),
      water: this.e.water(),
      checksum: hex(new Uint8Array(buffer, this.e.checksum(), 32)),
      wmap: new Uint8Array(buffer, this.e.wmap(), this.e.wmap_size()).slice(),
      attempt: this.e.attempt(),
      places: new Uint8Array(buffer, this.e.places(), this.e.places_size()).slice(),
    };
  }
}

/** A map to generate: a field, a dungeon floor or a town. */
export interface MapRequest {
  version: number;
  rng: string;
  /** The map's public node, 64 hex digits. */
  key: string;
  width: number;
  height: number;
  biome: number;
  kind: 'field' | MapKind;
  /** A dungeon floor's or a town's params, as hex. */
  params: string;
}

/** Runs a request on a loaded module. */
export function runRequest(g: Generator, r: MapRequest): GeneratedGrid {
  if (r.kind === 'field') {
    return g.generate(r.version, keyFromHex(r.key), r.width, r.height, r.biome);
  }
  const out = g.generateMap(r.version, keyFromHex(r.key), r.kind, bytesFromHex(r.params));
  if (out.width !== r.width || out.height !== r.height) {
    throw new Error(`the ${r.kind} is ${out.width} × ${out.height}, not ${r.width} × ${r.height}`);
  }
  return out;
}

/** The world fields generatorFor reads, as the connection's WorldValues. */
export interface GeneratedWorld {
  mode: 'classic' | 'procedural';
  generatorVersion: number;
  rngVersion: string;
}

/**
 * Loads the generator of a procedural world, or nothing for a classic
 * world, which never fetches the module. It fails when the module lacks
 * the world's generator version or rng spec, such as a client older than
 * the server.
 */
export async function generatorFor(
  world: GeneratedWorld,
  load: () => Promise<Generator> = async () => Generator.load(),
): Promise<Generator | undefined> {
  if (world.mode !== 'procedural') {
    return undefined;
  }
  const g = await load();
  if (!g.supports(world.generatorVersion, world.rngVersion)) {
    throw new Error(
      `this client has map generators up to ${g.latest}, but the world uses generator ${world.generatorVersion} with ${world.rngVersion}`,
    );
  }
  return g;
}
