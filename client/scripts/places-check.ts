// The town and dungeon check of decision 0040: a waserver built with
// -tags procedural runs a --dev procedural world with a field, a town
// and a crypt of two floors, and the Market Town sample mod, whose Town
// Guide stands on every town square by its place.
//
// - Walker starts on the field, which the page generates, and walks to
//   the field's south portal, which leads to town t1.
// - The page generates the town with the server's checksum. Walker walks
//   to the Town Guide on the square and talks to it: the guide finds the
//   nearest blacksmith with wa.places.
// - Walker walks to the town's other portal, which leads to the crypt's
//   first floor, and the page generates the floor.
//
// The map info at /gen/maps/<id>.json gives each map's places, so the
// check knows where the portals and the square are.

import type { Browser, Page } from 'playwright-core';
import {
  gameURL,
  hook,
  selfCell,
  settle,
  sleep,
  startGame,
  unitScreen,
  units,
} from './game-check.ts';
import type { FieldContext } from './field-check.ts';

type Check = (browser: string, name: string, ok: boolean, detail: string) => void;

/** A field load as the viewer records it. */
export interface MapLoad {
  map: string;
  kind: string;
  biome: string;
  source: 'generated' | 'downloaded';
  reason?: string;
  generateMs?: number;
  ms: number;
}

/** A named place of a map's info. */
export interface InfoPlace {
  tags: string[];
  x: number;
  y: number;
}

export async function loads(page: Page): Promise<MapLoad[]> {
  return hook<MapLoad[]>(page, 'fieldLoads');
}

/** Waits until the page has loaded n generated maps, and returns the last. */
export async function nthLoad(page: Page, n: number, timeout = 60_000): Promise<MapLoad> {
  const end = Date.now() + timeout;
  for (;;) {
    const l = await loads(page).catch(() => []);
    if (l.length >= n) {
      return l[n - 1]!;
    }
    if (Date.now() > end) {
      throw new Error(`the page loaded ${l.length} maps, not ${n}`);
    }
    await sleep(200);
  }
}

/** The places of a generated map, from the server's map info. */
export async function placesOf(game: string, id: string): Promise<InfoPlace[]> {
  const res = await fetch(`${game}/gen/maps/${encodeURIComponent(id)}.json`);
  if (!res.ok) {
    throw new Error(`${id}: HTTP ${res.status}`);
  }
  const info: unknown = await res.json();
  const list: unknown =
    typeof info === 'object' && info !== null ? Reflect.get(info, 'places') : [];
  // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- the server's Info.
  return (Array.isArray(list) ? list : []) as InfoPlace[];
}

/**
 * The next cell to click toward a target: a breadth-first path over the
 * map's walkable cells along the 4 axes, round the cells within 2 of
 * other units when it can, since a click there picks the unit, and the
 * farthest cell of it up to 7 steps ahead whose middle is well inside
 * the canvas. It returns why when there is none.
 */
async function nextStep(
  page: Page,
  target: [number, number],
  clearance: number,
): Promise<[number, number] | string> {
  return page.evaluate(
    ([to, clear]) => {
      const v: unknown = Reflect.get(globalThis, 'viewer');
      const call = (name: string, ...args: unknown[]): unknown => {
        const f: unknown = typeof v === 'object' && v !== null ? Reflect.get(v, name) : undefined;
        return typeof f === 'function' ? Reflect.apply(f, v, args) : undefined;
      };
      const m: unknown = call('map');
      const me: unknown = call('selfCell');
      if (typeof m !== 'object' || m === null || !Array.isArray(me)) {
        return 'no map or no player';
      }
      const w = Number(Reflect.get(m, 'width'));
      const h = Number(Reflect.get(m, 'height'));
      const flags: unknown = Reflect.get(m, 'cellFlags');
      if (!(flags instanceof Uint8Array)) {
        return 'no cell flags';
      }
      const others: unknown = call('units');
      const near = (x: number, y: number): boolean =>
        Array.isArray(others) &&
        others.some((u: unknown) => {
          if (typeof u !== 'object' || u === null || Reflect.get(u, 'self') === true) {
            return false;
          }
          const cell: unknown = Reflect.get(u, 'cell');
          return (
            Array.isArray(cell) &&
            Math.abs(Number(cell[0]) - x) <= clear &&
            Math.abs(Number(cell[1]) - y) <= clear
          );
        });
      const [sx, sy] = [Number(me[0]), Number(me[1])];
      const start = sy * w + sx;
      const goal = to[1] * w + to[0];
      const pathTo = (avoid: boolean): number[] | undefined => {
        const open = (x: number, y: number): boolean =>
          x >= 0 &&
          y >= 0 &&
          x < w &&
          y < h &&
          ((flags[y * w + x] ?? 0) & 1) !== 0 &&
          (!avoid ||
            !near(x, y) ||
            Math.max(Math.abs(x - to[0]), Math.abs(y - to[1])) <= clear ||
            Math.max(Math.abs(x - sx), Math.abs(y - sy)) <= 1);
        const prev = new Int32Array(w * h).fill(-1);
        prev[start] = start;
        const queue = [start];
        for (let q = 0; q < queue.length && prev[goal] === -1; q++) {
          const c = queue[q]!;
          const x = c % w;
          const y = Math.floor(c / w);
          for (const [dx, dy] of [
            [1, 0],
            [-1, 0],
            [0, 1],
            [0, -1],
          ]) {
            const nx = x + dx!;
            const ny = y + dy!;
            if (open(nx, ny) && prev[ny * w + nx] === -1) {
              prev[ny * w + nx] = c;
              queue.push(ny * w + nx);
            }
          }
        }
        if (prev[goal] === -1) {
          return undefined;
        }
        const out: number[] = [];
        for (let c = goal; c !== start; c = prev[c]!) {
          out.push(c);
        }
        return out.reverse();
      };
      const path = pathTo(true) ?? pathTo(false);
      if (path === undefined) {
        return 'no path';
      }
      const canvas = document.querySelector('canvas')?.getBoundingClientRect();
      for (let i = Math.min(path.length, 7) - 1; i >= 0; i--) {
        const c = path[i]!;
        const x = c % w;
        const y = Math.floor(c / w);
        if (near(x, y) && i < path.length - 1) {
          continue;
        }
        const at: unknown = call('screenOfCell', x, y);
        if (
          canvas !== undefined &&
          Array.isArray(at) &&
          Number(at[0]) > canvas.left + 60 &&
          Number(at[0]) < canvas.right - 60 &&
          Number(at[1]) > canvas.top + 100 &&
          Number(at[1]) < canvas.bottom - 100
        ) {
          return [x, y];
        }
      }
      return `no cell of the first 7 of a path of ${path.length} is clear on the screen`;
    },
    [target, clearance] as const,
  );
}

/**
 * Walks toward a cell in legs of up to 7 cells, each a click on the
 * ground, until the player stands within reach of it, or a warp loads
 * another map. It returns the legs. The clicks keep clearance cells
 * from other units where they can, 2 by default.
 */
export async function walk(
  page: Page,
  target: [number, number],
  reach: number,
  until: () => Promise<boolean>,
  clearance = 2,
): Promise<string[]> {
  const legs: string[] = [];
  for (let leg = 0; leg < 40; leg++) {
    if (await until()) {
      return legs;
    }
    await settle(page, 30_000).catch(() => undefined);
    if (await until()) {
      return legs;
    }
    const from = await selfCell(page);
    if (Math.max(Math.abs(from[0] - target[0]), Math.abs(from[1] - target[1])) <= reach) {
      return legs;
    }
    const to = await nextStep(page, target, clearance);
    if (typeof to === 'string') {
      throw new Error(`no way from ${from} to ${target}: ${to}, after ${legs.join(', ')}`);
    }
    const at = await hook<[number, number]>(page, 'screenOfCell', to[0], to[1]);
    await page.mouse.click(at[0], at[1]);
    const end = Date.now() + 10_000;
    for (;;) {
      if (await until()) {
        return [...legs, `${from} to ${to}`];
      }
      const c = await selfCell(page).catch(() => from);
      if (Math.abs(c[0] - to[0]) <= 1 && Math.abs(c[1] - to[1]) <= 1) {
        break;
      }
      if (Date.now() > end) {
        throw new Error(`the player stayed at ${c} on the way from ${from} to ${to}`);
      }
      await sleep(100);
    }
    legs.push(`${from} to ${to}`);
  }
  throw new Error(`the walk to ${target} took over 40 legs: ${legs.join(', ')}`);
}

/** The first place with a tag. */
function find(list: InfoPlace[], tag: string, nth = 0): InfoPlace {
  const found = list.filter((p) => p.tags.includes(tag))[nth];
  if (found === undefined) {
    throw new Error(`no place tagged ${tag}`);
  }
  return found;
}

/** Runs the town and dungeon check. Chromium only, as the other game checks. */
export async function checkPlaces(
  name: string,
  browser: Browser,
  ctx: FieldContext,
  check: Check,
): Promise<void> {
  const game = await startGame(ctx.repo, ctx.client, ctx.dir, ctx.wactl, ctx.build, {
    fields: '0:0:grassland',
    fieldSize: 64,
    towns: 1,
    dungeons: 'crypt:2',
    mods: ['mods/examples/market-town'],
    name: 'places',
  });
  const c = (label: string, ok: boolean, detail: string): void => {
    check(name, `places ${label}`, ok, detail);
  };
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  try {
    const page = await context.newPage();
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.goto(gameURL(game.base, 'Visitor'));
    const field = await nthLoad(page, 1);
    c(
      'field',
      field.map === 'gen:field:0:0' && field.source === 'generated',
      `${field.map} ${field.source}`,
    );
    const fieldPlaces = await placesOf(game.game, 'gen:field:0:0');
    const townPlaces = await placesOf(game.game, 'gen:town:t1');

    // The field's south portal leads to the town.
    const south = find(fieldPlaces, 'portal:south');
    const inTown = async (): Promise<boolean> => (await loads(page)).length >= 2;
    let legs = await walk(page, [south.x, south.y], 0, inTown);
    const town = await nthLoad(page, 2);
    c(
      'town',
      town.map === 'gen:town:t1' &&
        town.kind === 'town' &&
        town.source === 'generated' &&
        town.biome === 'hamlet',
      `${town.map} ${town.kind} ${town.biome}: ${town.source}${town.reason === undefined ? '' : ` (${town.reason})`}, generator ${town.generateMs?.toFixed(0) ?? '-'} ms in the worker, ${town.ms.toFixed(0)} ms in all, after ${legs.length} legs`,
    );

    // The Town Guide stands on the square, at its place.
    const square = find(townPlaces, 'square');
    await sleep(1000);
    legs = await walk(page, [square.x, square.y], 3, async () => Promise.resolve(false));
    const guide = (await units(page)).find((u) => u.name === 'Town Guide');
    c(
      'guide at its place',
      guide?.cell?.[0] === square.x && guide.cell[1] === square.y,
      `the guide at ${guide?.cell ?? 'nowhere'}, the square at ${square.x},${square.y}`,
    );
    const at = await unitScreen(page, (u) => u.name === 'Town Guide');
    await page.mouse.click(at[0], at[1]);
    const smith = find(townPlaces, 'blacksmith');
    const said = `The nearest blacksmith is in town:t1 at ${smith.x}, ${smith.y}.`;
    let text = '';
    try {
      await page.locator(`#dialog-window:has-text("${said}")`).waitFor({ timeout: 15_000 });
      text = (await page.textContent('#dialog-window .dialog-body')) ?? '';
    } catch (error) {
      text = `${(await page.textContent('#dialog-window').catch(() => '')) ?? ''} ${String(error).split('\n')[0]}`;
    }
    c('talk', text.includes(said), text.replace(/\s+/g, ' ').trim().slice(0, 160));
    for (let i = 0; i < 5 && (await page.locator('#dialog-window').isVisible()); i++) {
      await page.locator('#dialog-window .dialog-controls button:visible').first().click();
      await sleep(300);
    }

    // The town's second portal leads to the crypt's first floor.
    const gate = find(townPlaces, 'portal', 1);
    const inCrypt = async (): Promise<boolean> => (await loads(page)).length >= 3;
    legs = await walk(page, [gate.x, gate.y], 0, inCrypt);
    const floor = await nthLoad(page, 3);
    c(
      'dungeon',
      floor.map === 'gen:dungeon:crypt:1' &&
        floor.kind === 'dungeon' &&
        floor.source === 'generated' &&
        floor.biome === 'crypt',
      `${floor.map} ${floor.kind} ${floor.biome}: ${floor.source}${floor.reason === undefined ? '' : ` (${floor.reason})`}, generator ${floor.generateMs?.toFixed(0) ?? '-'} ms in the worker, ${floor.ms.toFixed(0)} ms in all, after ${legs.length} legs`,
    );
    c('page errors', errors.length === 0, errors.join('; ') || 'none');
    c('server errors', game.errors.length === 0, game.errors.slice(0, 3).join('; ') || 'none');
  } catch (error) {
    c('run', false, String(error).split('\n')[0] ?? '');
  } finally {
    await context.close();
    await game.stop();
  }
}
