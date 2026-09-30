// Pathfinding for the client, from the shared Go package compiled to
// WebAssembly with TinyGo (shared/path/wasm). It gives the same paths as
// the server, which runs the same code.

import { SHOOTABLE, WALKABLE, type WMap } from '../formats/wmap';
import pathWasm from './path.wasm?url';

/** rAthena's walk directions: 0 is north, turning counterclockwise. */
export type Dir = number;

/** Milliseconds per cell at RO's default walk speed. */
export const DEFAULT_SPEED = 150;

const STEPS: readonly (readonly [number, number])[] = [
  [0, 1],
  [-1, 1],
  [-1, 0],
  [-1, -1],
  [0, -1],
  [1, -1],
  [1, 0],
  [1, 1],
];

/** The cell offset of one step. */
export function step(d: Dir): [number, number] {
  const s = STEPS[d];
  return s === undefined ? [0, 0] : [s[0], s[1]];
}

/** How long one step takes: diagonal steps take 14/10 as long, as in rAthena. */
export function stepTime(d: Dir, speed: number): number {
  return d % 2 === 1 ? Math.floor((speed * 14) / 10) : speed;
}

/** A sprite facing (0 south, clockwise) for a walk direction. */
export function facing(d: Dir): number {
  return (((4 - d) % 8) + 8) % 8;
}

interface Exports {
  memory: WebAssembly.Memory;
  _initialize?: () => void;
  grid: (width: number, height: number) => number;
  steps: () => number;
  search: (x0: number, y0: number, x1: number, y1: number, easy: number) => number;
  shoot: (x0: number, y0: number, x1: number, y1: number) => number;
}

function isExports(v: unknown): v is Exports {
  return (
    typeof v === 'object' &&
    v !== null &&
    'memory' in v &&
    v.memory instanceof WebAssembly.Memory &&
    'grid' in v &&
    typeof v.grid === 'function' &&
    'steps' in v &&
    typeof v.steps === 'function' &&
    'search' in v &&
    typeof v.search === 'function' &&
    'shoot' in v &&
    typeof v.shoot === 'function'
  );
}

export class Pathfinder {
  private constructor(private readonly e: Exports) {}

  /** Loads the module from the client's build. */
  static async load(): Promise<Pathfinder> {
    const res = await fetch(pathWasm);
    return Pathfinder.fromBytes(await res.arrayBuffer());
  }

  static async fromBytes(bytes: BufferSource): Promise<Pathfinder> {
    const { instance } = await WebAssembly.instantiate(bytes, {});
    const e: unknown = instance.exports;
    if (!isExports(e)) {
      throw new Error('path.wasm: missing exports');
    }
    // TinyGo names the start-up function this way.
    // oxlint-disable-next-line no-underscore-dangle
    e._initialize?.();
    return new Pathfinder(e);
  }

  /** Sets the cells, as WMAP flags, row by row from y = 0. */
  setCells(width: number, height: number, flags: Uint8Array): void {
    const at = this.e.grid(width, height);
    if (at === 0) {
      throw new Error(`path.wasm: invalid size ${width} × ${height}`);
    }
    const cells = new Uint8Array(this.e.memory.buffer, at, width * height);
    for (let i = 0; i < cells.length; i++) {
      cells[i] = (flags[i] ?? 0) & (WALKABLE | SHOOTABLE);
    }
  }

  /** Sets the cells from a map. */
  setMap(map: WMap): void {
    this.setCells(map.width, map.height, map.cellFlags);
  }

  /**
   * Finds a walk path, as rAthena's path_search. easy tries only the
   * direct path that monsters use. Undefined means no path.
   */
  search(x0: number, y0: number, x1: number, y1: number, easy = false): Dir[] | undefined {
    const n = this.e.search(x0, y0, x1, y1, easy ? 1 : 0);
    if (n < 0) {
      return undefined;
    }
    return [...new Int8Array(this.e.memory.buffer, this.e.steps(), n)];
  }

  /** Whether a projectile can fly between two cells. */
  shoot(x0: number, y0: number, x1: number, y1: number): boolean {
    return this.e.shoot(x0, y0, x1, y1) === 1;
  }
}
