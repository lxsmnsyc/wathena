import { readFileSync } from 'node:fs';
import { inflateSync } from 'node:zlib';
import { describe, expect, it } from 'vitest';
import { SHOOTABLE, WALKABLE } from '../formats/wmap';
import { Pathfinder, facing, stepTime } from './pathfinder';

// The WebAssembly module must give rAthena's results, like the Go
// package it is built from. This runs the reference queries through it.

const repo = new URL('../../../', import.meta.url);
const wasm = readFileSync(new URL('client/src/world/path.wasm', repo));

const OPEN = WALKABLE | SHOOTABLE;
const GAT: Record<number, number> = { 0: OPEN, 2: OPEN, 3: OPEN, 4: OPEN, 5: SHOOTABLE, 6: OPEN };

function cells(mode: string, map: string): { width: number; height: number; flags: Uint8Array } {
  const b = readFileSync(new URL(`shared/path/testdata/${mode}-${map}.cells`, repo));
  const width = b.readUInt16LE(4);
  const height = b.readUInt16LE(6);
  const types = inflateSync(b.subarray(8));
  return { width, height, flags: Uint8Array.from(types, (t) => GAT[t] ?? 0) };
}

interface Reference {
  maps: {
    map: string;
    walk: { from: number[]; to: number[]; flag: number; found: boolean; path?: number[] }[];
    shoot: { from: number[]; to: number[]; clear: boolean }[];
  }[];
}

describe('Pathfinder', () => {
  it('matches rAthena on the reference maps', async () => {
    const p = await Pathfinder.fromBytes(wasm);
    let checked = 0;
    for (const mode of ['renewal', 'pre-renewal']) {
      const ref = JSON.parse(
        readFileSync(new URL(`reference/rathena/${mode}/pathfinding.json`, repo), 'utf8'),
      ) as Reference;
      for (const m of ref.maps) {
        const c = cells(mode, m.map);
        p.setCells(c.width, c.height, c.flags);
        for (const w of m.walk) {
          const got = p.search(w.from[0]!, w.from[1]!, w.to[0]!, w.to[1]!, (w.flag & 1) !== 0);
          expect(got).toEqual(w.found ? w.path : undefined);
          checked++;
        }
        for (const s of m.shoot) {
          expect(p.shoot(s.from[0]!, s.from[1]!, s.to[0]!, s.to[1]!)).toBe(s.clear);
          checked++;
        }
      }
    }
    expect(checked).toBe(9000);
  });

  it('times steps and turns directions into sprite facings', () => {
    expect(stepTime(0, 150)).toBe(150);
    expect(stepTime(7, 150)).toBe(210);
    // North faces away from the camera, and south toward it.
    expect([0, 1, 2, 3, 4, 5, 6, 7].map(facing)).toEqual([4, 3, 2, 1, 0, 7, 6, 5]);
  });
});
