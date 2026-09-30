import { describe, expect, it, vi } from 'vitest';
import { AssetError, AssetStore, type Entry, type Fetch, sha256, shardOf } from './store';

const encoder = new TextEncoder();

/** An in-memory build, served by a fake fetch that counts requests. */
async function fakeBuild(assets: Record<string, { t: Entry['t']; data: Uint8Array<ArrayBuffer> }>) {
  const files = new Map<string, Uint8Array<ArrayBuffer>>();
  const shards: Record<string, Entry>[] = Array.from({ length: 256 }, () => ({}));
  const ext: Record<string, string> = { sprite: 'wspr', texture: 'webp', map: 'wmap' };
  for (const [name, a] of Object.entries(assets)) {
    const h = await sha256(a.data);
    shards[await shardOf(name)]![name] = { h, t: a.t, s: a.data.byteLength };
    files.set(`${h.slice(0, 2)}/${h}.${ext[a.t]}`, a.data);
  }
  const hashes: string[] = [];
  for (const s of shards) {
    const data = encoder.encode(JSON.stringify(s));
    const h = await sha256(data);
    hashes.push(h);
    files.set(`${h.slice(0, 2)}/${h}.json`, data);
  }
  files.set(
    'manifest.json',
    encoder.encode(JSON.stringify({ contract: 'assets-v1', build: 'test', shards: hashes })),
  );
  const requests: string[] = [];
  const fetcher: Fetch = async (url) => {
    requests.push(url);
    const data = files.get(url.replace(/^\/b\//, ''));
    return Promise.resolve(
      data === undefined ? new Response('not found', { status: 404 }) : new Response(data.slice()),
    );
  };
  return { files, requests, fetcher };
}

describe('shardOf', () => {
  it('uses the first byte of SHA-256', async () => {
    expect(await shardOf('sprite/mob/poring')).toBe(230);
    expect(await shardOf('texture/워터/water000')).toBe(9);
    expect(await shardOf('')).toBe(227);
  });
});

describe('AssetStore', () => {
  const sprite = encoder.encode('WSPR sprite');

  it('loads and checks assets', async () => {
    const b = await fakeBuild({
      'sprite/mob/poring': { t: 'sprite', data: sprite },
      'sprite/mob/poring2': { t: 'sprite', data: sprite },
    });
    const store = await AssetStore.open('/b/', b.fetcher);
    expect(store.build).toBe('test');
    expect(await store.bytes('sprite/mob/poring', 'sprite')).toEqual(sprite);
    // Names with the same content share one download.
    await store.bytes('sprite/mob/poring2', 'sprite');
    expect(b.requests.filter((r) => r.endsWith('.wspr'))).toHaveLength(1);
    expect(store.stats).toEqual({ files: 1, bytes: sprite.byteLength });
  });

  it('reports missing names once', async () => {
    const b = await fakeBuild({});
    const store = await AssetStore.open('/b/', b.fetcher);
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
    expect(await store.bytes('sprite/mob/gone', 'sprite')).toBeUndefined();
    expect(await store.bytes('sprite/mob/gone', 'sprite')).toBeUndefined();
    expect(warn).toHaveBeenCalledTimes(1);
    warn.mockRestore();
  });

  it('rejects content that does not match its hash', async () => {
    const b = await fakeBuild({ 'sprite/mob/poring': { t: 'sprite', data: sprite } });
    const store = await AssetStore.open('/b/', b.fetcher);
    for (const [k, v] of b.files) {
      if (k.endsWith('.wspr')) {
        const changed = v.slice();
        changed[0] = 0;
        b.files.set(k, changed);
      }
    }
    await expect(store.bytes('sprite/mob/poring', 'sprite')).rejects.toThrow(AssetError);
  });

  it('rejects a changed shard', async () => {
    const b = await fakeBuild({ 'sprite/mob/poring': { t: 'sprite', data: sprite } });
    const store = await AssetStore.open('/b/', b.fetcher);
    for (const k of b.files.keys()) {
      if (k.endsWith('.json') && k !== 'manifest.json') {
        b.files.set(k, encoder.encode('{}'));
      }
    }
    await expect(store.entry('sprite/mob/poring')).rejects.toThrow(/hash/);
  });

  it('rejects the wrong type', async () => {
    const b = await fakeBuild({ 'sprite/mob/poring': { t: 'sprite', data: sprite } });
    const store = await AssetStore.open('/b/', b.fetcher);
    await expect(store.bytes('sprite/mob/poring', 'map')).rejects.toThrow(/not a map/);
  });

  it('rejects unknown contracts', async () => {
    const b = await fakeBuild({});
    b.files.set(
      'manifest.json',
      encoder.encode(JSON.stringify({ contract: 'assets-v2', build: 'x', shards: [] })),
    );
    await expect(AssetStore.open('/b/', b.fetcher)).rejects.toThrow(AssetError);
  });
});
