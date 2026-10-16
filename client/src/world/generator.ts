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
}

interface Exports {
  memory: WebAssembly.Memory;
  _initialize?: () => void;
  key: () => number;
  rng_version: (version: number) => number;
  latest: () => number;
  generate: (version: number, width: number, height: number) => number;
  types: () => number;
  heights: () => number;
  water: () => number;
  checksum: () => number;
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
] as const;

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

function hex(bytes: Uint8Array): string {
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
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

  /** Generates a map's grid from its public node key. */
  generate(version: number, key: Uint8Array, width: number, height: number): GeneratedGrid {
    if (key.length !== 32) {
      throw new Error('a node key is 32 bytes');
    }
    new Uint8Array(this.e.memory.buffer, this.e.key(), 32).set(key);
    const code = this.e.generate(version, width, height);
    if (code === -1) {
      throw new Error(`gen.wasm: no generator version ${version}`);
    }
    if (code !== 0) {
      throw new Error(`gen.wasm: invalid size ${width} × ${height}`);
    }
    // The memory can grow during generate, so views are made after it.
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
    };
  }
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
