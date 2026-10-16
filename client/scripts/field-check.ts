// The generated field check of decision 0039: a waserver built with
// -tags procedural runs a --dev procedural world with two fields, and
// players walk a field in the browser.
//
// - Walker's page generates the field with gen.wasm in a worker, and the
//   checksum matches the server's.
// - Walker walks three legs on the field, and the server agrees on each
//   cell.
// - Fallback's page, with ?nogen, downloads the server's map, which has
//   the same cells as Walker's.

import type { Browser, Page } from 'playwright-core';
import { gameURL, hook, selfCell, settle, startGame, walkedTo } from './game-check.ts';

/** What the check needs from browser-check.ts. */
export interface FieldContext {
  repo: string;
  client: string;
  dir: string;
  wactl: string;
  build: string;
}

type Check = (browser: string, name: string, ok: boolean, detail: string) => void;

/** A field load as the viewer records it. */
interface FieldLoad {
  map: string;
  biome: string;
  source: 'generated' | 'downloaded';
  reason?: string;
  generateMs?: number;
  ms: number;
}

/** Waits for the page's first field load and returns it. */
async function firstField(page: Page): Promise<FieldLoad> {
  await page.waitForFunction(
    () => {
      const v: unknown = Reflect.get(globalThis, 'viewer');
      const f: unknown =
        typeof v === 'object' && v !== null ? Reflect.get(v, 'fieldLoads') : undefined;
      const l: unknown = typeof f === 'function' ? Reflect.apply(f, v, []) : [];
      return Array.isArray(l) && l.length > 0;
    },
    null,
    { timeout: 60_000 },
  );
  const loads = await hook<FieldLoad[]>(page, 'fieldLoads');
  return loads[0]!;
}

/** The map's size and a hash of its cell flags and heights, in the page. */
async function mapPrint(page: Page): Promise<{ width: number; height: number; hash: string }> {
  return page.evaluate(async () => {
    const v: unknown = Reflect.get(globalThis, 'viewer');
    const f: unknown = typeof v === 'object' && v !== null ? Reflect.get(v, 'map') : undefined;
    const m: unknown = typeof f === 'function' ? Reflect.apply(f, v, []) : undefined;
    if (typeof m !== 'object' || m === null) {
      return { width: 0, height: 0, hash: '' };
    }
    const flags: unknown = Reflect.get(m, 'cellFlags');
    const heights: unknown = Reflect.get(m, 'cellHeights');
    if (!(flags instanceof Uint8Array) || !(heights instanceof Float32Array)) {
      return { width: 0, height: 0, hash: '' };
    }
    const both = new Uint8Array(flags.length + heights.byteLength);
    both.set(flags);
    both.set(new Uint8Array(heights.buffer, heights.byteOffset, heights.byteLength), flags.length);
    const d = new Uint8Array(await crypto.subtle.digest('SHA-256', both));
    return {
      width: Number(Reflect.get(m, 'width')),
      height: Number(Reflect.get(m, 'height')),
      hash: Array.from(d, (b) => b.toString(16).padStart(2, '0')).join(''),
    };
  });
}

/**
 * A walkable cell 5 to 9 cells from the player, in one of 8 directions,
 * whose middle is on the canvas, or undefined.
 */
async function walkTarget(page: Page, leg: number): Promise<[number, number] | undefined> {
  return page.evaluate((turn) => {
    const v: unknown = Reflect.get(globalThis, 'viewer');
    const call = (name: string, ...args: unknown[]): unknown => {
      const f: unknown = typeof v === 'object' && v !== null ? Reflect.get(v, name) : undefined;
      return typeof f === 'function' ? Reflect.apply(f, v, args) : undefined;
    };
    const m: unknown = call('map');
    const me: unknown = call('selfCell');
    if (typeof m !== 'object' || m === null || !Array.isArray(me)) {
      return undefined;
    }
    const width = Number(Reflect.get(m, 'width'));
    const height = Number(Reflect.get(m, 'height'));
    const flags: unknown = Reflect.get(m, 'cellFlags');
    if (!(flags instanceof Uint8Array)) {
      return undefined;
    }
    const canvas = document.querySelector('canvas')?.getBoundingClientRect();
    const dirs = [
      [1, 0],
      [1, 1],
      [0, 1],
      [-1, 1],
      [-1, 0],
      [-1, -1],
      [0, -1],
      [1, -1],
    ];
    for (let k = 0; k < 8; k++) {
      const [dx, dy] = dirs[(k + turn * 3) % 8]!;
      // The way there is a straight line of walkable cells, so the
      // server's walk path, which has a length limit, finds it.
      const open = (x: number, y: number): boolean =>
        x >= 0 && y >= 0 && x < width && y < height && ((flags[y * width + x] ?? 0) & 1) !== 0;
      let clear = 0;
      while (
        clear < 9 &&
        open(Number(me[0]) + dx! * (clear + 1), Number(me[1]) + dy! * (clear + 1))
      ) {
        clear++;
      }
      for (let d = clear; d >= 5; d--) {
        const x = Number(me[0]) + dx! * d;
        const y = Number(me[1]) + dy! * d;
        if (!open(x, y)) {
          continue;
        }
        const at: unknown = call('screenOfCell', x, y);
        if (
          canvas !== undefined &&
          Array.isArray(at) &&
          Number(at[0]) > canvas.left + 40 &&
          Number(at[0]) < canvas.right - 40 &&
          Number(at[1]) > canvas.top + 80 &&
          Number(at[1]) < canvas.bottom - 80
        ) {
          return [x, y];
        }
      }
    }
    return undefined;
  }, leg);
}

/** Runs the field check. Chromium only, as the other game checks. */
export async function checkFields(
  name: string,
  browser: Browser,
  ctx: FieldContext,
  check: Check,
): Promise<void> {
  const game = await startGame(ctx.repo, ctx.client, ctx.dir, ctx.wactl, ctx.build, {
    fields: '0:0:grassland,1:0:desert',
    fieldSize: 160,
    name: 'fields',
  });
  const c = (label: string, ok: boolean, detail: string): void => {
    check(name, `field ${label}`, ok, detail);
  };
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  try {
    const walker = await context.newPage();
    const errors: string[] = [];
    walker.on('pageerror', (e) => errors.push(e.message));
    await walker.goto(gameURL(game.base, 'Walker'));
    const load = await firstField(walker);
    c(
      'generated',
      load.map === 'gen:field:0:0' && load.source === 'generated' && load.biome === 'grassland',
      `${load.map} ${load.biome}: ${load.source}${load.reason === undefined ? '' : ` (${load.reason})`}, generator ${load.generateMs?.toFixed(0) ?? '-'} ms in the worker, ${load.ms.toFixed(0)} ms in all`,
    );
    const print = await mapPrint(walker);
    c('size', print.width === 160 && print.height === 160, `${print.width}×${print.height}`);

    // Three legs of a walk, each confirmed by the server's position.
    await walker.waitForFunction(
      () => {
        const v: unknown = Reflect.get(globalThis, 'viewer');
        const f: unknown =
          typeof v === 'object' && v !== null ? Reflect.get(v, 'selfCell') : undefined;
        return typeof f === 'function' && Array.isArray(Reflect.apply(f, v, []));
      },
      null,
      { timeout: 30_000 },
    );
    const start = await selfCell(walker);
    const legs: string[] = [];
    let walked = true;
    for (let leg = 0; leg < 3 && walked; leg++) {
      await settle(walker, 30_000);
      const from = await selfCell(walker);
      const to = await walkTarget(walker, leg);
      if (to === undefined) {
        walked = false;
        legs.push(`no walkable cell in reach of ${from}`);
        break;
      }
      const at = await hook<[number, number]>(walker, 'screenOfCell', to[0], to[1]);
      await walker.mouse.click(at[0], at[1]);
      const got = await walkedTo(
        walker,
        (cell) => Math.abs(cell[0] - to[0]) <= 1 && Math.abs(cell[1] - to[1]) <= 1,
        20_000,
      ).catch(() => undefined);
      legs.push(`${from} to ${got ?? 'nowhere'} (asked ${to})`);
      walked = got !== undefined;
    }
    const end = await selfCell(walker);
    c(
      'walk',
      walked && (end[0] !== start[0] || end[1] !== start[1]),
      `from ${start}: ${legs.join(', ')}`,
    );

    // A client without the generator downloads the same map.
    const fallback = await context.newPage();
    fallback.on('pageerror', (e) => errors.push(e.message));
    await fallback.goto(`${gameURL(game.base, 'Fallback')}&nogen`);
    const dl = await firstField(fallback);
    const dlPrint = await mapPrint(fallback);
    c(
      'download',
      dl.source === 'downloaded' && dlPrint.hash === print.hash && print.hash !== '',
      `${dl.source} in ${dl.ms.toFixed(0)} ms (${dl.reason ?? ''}); cells ${dlPrint.hash.slice(0, 16)}, generated ${print.hash.slice(0, 16)}`,
    );
    c('page errors', errors.length === 0, errors.join('; ') || 'none');
    c('server errors', game.errors.length === 0, game.errors.slice(0, 3).join('; ') || 'none');
  } finally {
    await context.close();
    await game.stop();
  }
}
