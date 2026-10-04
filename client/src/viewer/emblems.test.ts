import { describe, expect, it } from 'vitest';
import { EmblemCache, emblemURL } from './emblems';

describe('emblemURL', () => {
  it('serves emblems from the WebSocket host over http or https', () => {
    expect(emblemURL('ws://localhost:8300/ws', 12n, 3)).toBe(
      'http://localhost:8300/emblems/12/3.png',
    );
    expect(emblemURL('wss://play.example.com/ws', 7n, 1)).toBe(
      'https://play.example.com/emblems/7/1.png',
    );
  });

  it('gives no URL without an emblem or a server', () => {
    expect(emblemURL('ws://localhost:8300/ws', 12n, 0)).toBeUndefined();
    expect(emblemURL('ws://localhost:8300/ws', 0n, 1)).toBeUndefined();
    expect(emblemURL('not a url', 12n, 1)).toBeUndefined();
    expect(emblemURL('ftp://localhost/ws', 12n, 1)).toBeUndefined();
  });
});

/** Lets pending promise callbacks run. */
async function settle(): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, 0);
  });
}

describe('EmblemCache', () => {
  it('loads each emblem version once and gives its URL after it loads', async () => {
    const asked: string[] = [];
    let finish: (ok: boolean) => void = () => {};
    const cache = new EmblemCache('ws://localhost:8300/ws', async (url) => {
      asked.push(url);
      return new Promise((resolve) => {
        finish = resolve;
      });
    });
    let loads = 0;
    cache.onLoad = () => {
      loads++;
    };
    expect(cache.get(12n, 3)).toBeUndefined();
    expect(cache.get(12n, 3)).toBeUndefined();
    expect(asked).toEqual(['http://localhost:8300/emblems/12/3.png']);
    finish(true);
    await settle();
    expect(loads).toBe(1);
    expect(cache.get(12n, 3)).toBe('http://localhost:8300/emblems/12/3.png');
    // A new version is a new image.
    expect(cache.get(12n, 4)).toBeUndefined();
    expect(asked).toHaveLength(2);
  });

  it('shows nothing for a failed load or version 0, and does not ask again', async () => {
    const asked: string[] = [];
    const cache = new EmblemCache('ws://localhost:8300/ws', async (url) => {
      asked.push(url);
      return Promise.resolve(false);
    });
    expect(cache.get(5n, 1)).toBeUndefined();
    await settle();
    expect(cache.get(5n, 1)).toBeUndefined();
    expect(cache.get(5n, 0)).toBeUndefined();
    expect(asked).toHaveLength(1);
  });
});
