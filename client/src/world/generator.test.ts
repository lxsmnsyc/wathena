import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { Generator, bytesFromHex, generatorFor, keyFromHex, runRequest } from './generator';

// The WebAssembly module must give the same grids as the Go package it is
// built from. This runs the golden maps of shared/gen through it in Node.

const repo = new URL('../../../', import.meta.url);
const wasm = readFileSync(new URL('client/src/world/gen.wasm', repo));

interface Golden {
  version: number;
  rng: string;
  maps: {
    key: string;
    width: number;
    height: number;
    biome?: number;
    water: number;
    counts: number[];
    checksum: string;
    attempt?: number;
    wmap?: string;
    kind?: 'dungeon' | 'town';
    params?: string;
    name?: string;
    places?: string;
  }[];
}

function golden(version: number): Golden {
  return JSON.parse(
    readFileSync(new URL(`shared/gen/testdata/golden-v${version}.json`, repo), 'utf8'),
  ) as Golden;
}

describe('Generator', () => {
  it('matches the golden checksums of every version', async () => {
    const g = await Generator.fromBytes(wasm);
    expect(g.latest).toBeGreaterThanOrEqual(1);
    let checked = 0;
    for (let v = 1; v <= g.latest; v++) {
      const want = golden(v);
      expect(g.supports(v, want.rng)).toBe(true);
      for (const m of want.maps) {
        const began = performance.now();
        const grid =
          m.kind === undefined
            ? g.generate(v, keyFromHex(m.key), m.width, m.height, m.biome ?? 0)
            : g.generateMap(v, keyFromHex(m.key), m.kind, bytesFromHex(m.params ?? ''));
        const took = performance.now() - began;
        if (m.width * m.height >= 400 * 400 || m.kind !== undefined) {
          console.info(
            `gen v${v} ${m.kind ?? 'field'} ${m.name ?? ''} ${m.width}x${m.height} in Node: ${took.toFixed(1)} ms`,
          );
        }
        const counts = [0, 0, 0, 0];
        for (const t of grid.types) {
          counts[[0, 1, 3, 5].indexOf(t)]! += 1;
        }
        const wmap =
          grid.wmap.length === 0 ? undefined : createHash('sha256').update(grid.wmap).digest('hex');
        const places = v >= 3 ? createHash('sha256').update(grid.places).digest('hex') : undefined;
        expect({
          checksum: grid.checksum,
          water: grid.water,
          counts,
          attempt: grid.attempt,
          wmap,
          places,
          width: grid.width,
        }).toEqual({
          checksum: m.checksum,
          water: m.water,
          counts: m.counts,
          attempt: m.attempt ?? 0,
          wmap: m.wmap,
          places: m.places,
          width: m.width,
        });
        expect(grid.heights.length).toBe(4 * m.width * m.height);
        checked++;
      }
    }
    expect(checked).toBeGreaterThanOrEqual(36);
  });

  it('refuses unknown versions, sizes and keys', async () => {
    const g = await Generator.fromBytes(wasm);
    expect(g.supports(g.latest + 1, 'rng-v1')).toBe(false);
    expect(g.supports(1, 'rng-v2')).toBe(false);
    const key = new Uint8Array(32);
    expect(() => g.generate(g.latest + 1, key, 4, 4)).toThrow(/no generator version/);
    expect(() => g.generate(1, key, 0, 4)).toThrow(/invalid size/);
    expect(() => g.generate(1, key, 1025, 4)).toThrow(/invalid size/);
    expect(() => g.generate(1, new Uint8Array(31), 4, 4)).toThrow(/32 bytes/);
    expect(() => g.generate(2, key, 31, 64)).toThrow(/invalid size/);
    expect(() => g.generate(2, key, 64, 64, 99)).toThrow(/no biome 99/);
    expect(g.biomes(1)).toBe(0);
    expect(g.biomes(2)).toBe(6);
    expect(() => g.generateMap(2, key, 'town', new Uint8Array([2, 1]))).toThrow(/no town/);
    expect(() => g.generateMap(3, key, 'town', new Uint8Array([2, 1, 0]))).toThrow(
      /invalid town params/,
    );
    expect(() => g.generateMap(3, key, 'dungeon', new Uint8Array(17 << 10))).toThrow(/too long/);
    expect(() => bytesFromHex('abc')).toThrow(/hex digits/);
    expect(() => keyFromHex('00')).toThrow(/64 lowercase hex/);
    // A refused call leaves the module usable.
    expect(g.generate(1, key, 4, 4).types.length).toBe(16);
  });
});

describe('runRequest', () => {
  it('makes a town from its params and checks its size', async () => {
    const g = await Generator.fromBytes(wasm);
    const town = golden(3).maps.find((m) => m.kind === 'town')!;
    const request = {
      version: 3,
      rng: 'rng-v1',
      key: town.key,
      width: town.width,
      height: town.height,
      biome: 0,
      kind: 'town',
      params: town.params ?? '',
    } as const;
    expect(runRequest(g, request).checksum).toBe(town.checksum);
    expect(() => runRequest(g, { ...request, width: town.width + 2 })).toThrow(/not/);
  });
});

describe('generatorFor', () => {
  it('loads the module only for a procedural world it supports', async () => {
    let loads = 0;
    const load = async (): Promise<Generator> => {
      loads++;
      return Generator.fromBytes(wasm);
    };
    const classic = { mode: 'classic', generatorVersion: 0, rngVersion: '' } as const;
    expect(await generatorFor(classic, load)).toBeUndefined();
    expect(loads).toBe(0);
    const g = await generatorFor(
      { mode: 'procedural', generatorVersion: 1, rngVersion: 'rng-v1' },
      load,
    );
    expect(g?.latest).toBeGreaterThanOrEqual(1);
    expect(loads).toBe(1);
    await expect(
      generatorFor({ mode: 'procedural', generatorVersion: 999, rngVersion: 'rng-v1' }, load),
    ).rejects.toThrow(/generator 999/);
  });
});
