import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { decodeWMap } from '../formats/wmap';
import {
  type FieldGenerator,
  type FieldInfo,
  isGenerated,
  loadField,
  loadTilesets,
} from './fields';
import { Generator, hex, keyFromHex } from './generator';

// The download fallback of decision 0039: the client uses its own field
// when the checksum matches, and the server's map otherwise.

const repo = new URL('../../../', import.meta.url);
const wasm = readFileSync(new URL('client/src/world/gen.wasm', repo));
const key = '11'.repeat(32);

/** A server with one field, made by the module, and its counts of requests. */
async function server(): Promise<{
  info: FieldInfo;
  wmap: Uint8Array<ArrayBuffer>;
  fetcher: (url: string) => Promise<Response>;
  hits: string[];
}> {
  const g = await Generator.fromBytes(wasm);
  const f = g.generate(2, keyFromHex(key), 64, 64, 1);
  const info: FieldInfo = {
    id: 'gen:field:0:0',
    version: 2,
    rng: 'rng-v1',
    key,
    width: 64,
    height: 64,
    biome: 1,
    biomeName: 'forest',
    checksum: f.checksum,
    wmap: '/gen/maps/gen:field:0:0.wmap',
    wmapSha256: createHash('sha256').update(f.wmap).digest('hex'),
    wmapBytes: f.wmap.length,
  };
  const hits: string[] = [];
  const respond = (url: string): Response => {
    hits.push(url);
    if (url === 'http://game/gen/maps/gen%3Afield%3A0%3A0.json') {
      return Response.json(info);
    }
    if (url === `http://game${info.wmap}`) {
      return new Response(f.wmap);
    }
    if (url === 'http://game/gen/tilesets.json') {
      return Response.json({
        version: 1,
        aliases: {
          'texture/gen/forest/grass': 'texture/kro/grass',
          other: 'x',
          'model/gen/a/b': 7,
        },
      });
    }
    return new Response('', { status: 404 });
  };
  const fetcher = async (url: string): Promise<Response> => Promise.resolve(respond(url));

  return { info, wmap: f.wmap, fetcher, hits };
}

/** The module on the page, as the worker runs it. */
const local: FieldGenerator = async (info) => {
  const g = await Generator.fromBytes(wasm);
  return g.generate(info.version, keyFromHex(info.key), info.width, info.height, info.biome);
};

describe('loadField', () => {
  it('uses the generated field when the checksums match', async () => {
    const s = await server();
    const f = await loadField('http://game', 'gen:field:0:0', local, s.fetcher);
    expect(f.source).toBe('generated');
    expect(f.wmap).toEqual(s.wmap);
    expect(s.hits).toHaveLength(1);
    expect(decodeWMap(f.wmap).width).toBe(64);
  });

  it('downloads the map when the checksum differs', async () => {
    const s = await server();
    const wrong: FieldGenerator = async (info) => ({
      ...(await local(info)),
      checksum: '0'.repeat(64),
    });
    const f = await loadField('http://game', 'gen:field:0:0', wrong, s.fetcher);
    expect(f.source).toBe('downloaded');
    expect(f.reason).toMatch(/checksum/);
    expect(f.wmap).toEqual(s.wmap);
    expect(s.hits.at(-1)).toBe(`http://game${s.info.wmap}`);
  });

  it('downloads the map when the module is missing or fails', async () => {
    const s = await server();
    const missing = await loadField('http://game', 'gen:field:0:0', undefined, s.fetcher);
    expect(missing.source).toBe('downloaded');
    expect(missing.reason).toMatch(/no map generator/);
    const failing: FieldGenerator = async () => Promise.reject(new Error('gen.wasm: HTTP 404'));
    const failed = await loadField('http://game', 'gen:field:0:0', failing, s.fetcher);
    expect(failed.source).toBe('downloaded');
    expect(failed.reason).toMatch(/HTTP 404/);
  });

  it('refuses a download without the SHA-256 the server gave, and bad info', async () => {
    const s = await server();
    const bad = async (url: string): Promise<Response> =>
      url.endsWith('.wmap') ? new Response(new Uint8Array([1, 2, 3])) : s.fetcher(url);
    await expect(loadField('http://game', 'gen:field:0:0', undefined, bad)).rejects.toThrow(
      /SHA-256/,
    );
    await expect(loadField('http://game', 'gen:field:5:5', local, s.fetcher)).rejects.toThrow(
      /HTTP 404/,
    );
    const odd = async (url: string): Promise<Response> =>
      url.endsWith('.json') ? Response.json({ ...s.info, key: 'zz' }) : s.fetcher(url);
    await expect(loadField('http://game', 'gen:field:0:0', local, odd)).rejects.toThrow(
      /invalid info/,
    );
  });
});

describe('loadTilesets', () => {
  it('keeps the generated names that map to a name', async () => {
    const s = await server();
    const t = await loadTilesets('http://game', s.fetcher);
    expect([...t]).toEqual([['texture/gen/forest/grass', 'texture/kro/grass']]);
  });
});

describe('isGenerated', () => {
  it('tells field IDs from asset names', () => {
    expect(isGenerated('gen:field:3:-1')).toBe(true);
    expect(isGenerated('map/prontera')).toBe(false);
    expect(isGenerated('placeholder/map')).toBe(false);
  });
});

it('hex writes lowercase digits', () => {
  expect(hex(new Uint8Array([0, 255, 16]))).toBe('00ff10');
});
