// The kRO browser check of review decision 55: the classic world of the
// full kRO run in Chromium. `wactl world new --mode classic`, built with
// -tags procedural, makes the world in a new database, and a waserver
// built with the tag runs every map of a full kRO asset build with
// `mode: renewal` and development sessions.
//
// - Pilgrim's page loads Prontera from the kRO build.
// - Pilgrim walks with clicks on the ground.
// - Pilgrim clicks a converted NPC beside the fountain, and its dialog
//   opens.
//
// It runs only when WATHENA_KRO_ASSETS names a full kRO asset build, made
// with `wactl assets build --source placeholder --source grf:<client>`,
// the variable the end-to-end kRO check reads (e2e/kro_test.go). The
// database comes from WATHENA_TEST_DATABASE_URL, as content-check.ts.

import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { join } from 'node:path';
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
import { walk } from './places-check.ts';

type Check = (browser: string, name: string, ok: boolean, detail: string) => void;

/** The variable that names the kRO asset build. */
export const KRO_ASSETS = 'WATHENA_KRO_ASSETS';

/** Every map, new characters in Prontera, as a classic world ships. */
const CLASSIC = 'classic:\n  start_map: prontera\n  maps: all\nmode: renewal\n';

/**
 * The NPC the check talks to: Man from Morocc at prontera,163,204, a
 * converted dialog NPC of npc/cities/prontera.txt near the start cell,
 * whose lines start with [Morocc Traveler].
 */
const NPC = { name: 'Man from Morocc', x: 163, y: 204, says: '[Morocc Traveler]' };

/**
 * Waits until the page shows its first map, and returns the map's name,
 * from the map field, and the status line, which says how many files it
 * loaded and how long it took.
 */
async function firstMap(page: Page, timeout = 180_000): Promise<[string, string]> {
  const end = Date.now() + timeout;
  for (;;) {
    const shown = await page.inputValue('#map').catch(() => '');
    const line = (await page.textContent('#status').catch(() => '')) ?? '';
    if (shown !== '' && line.startsWith('done')) {
      return [shown, line];
    }
    if (Date.now() > end) {
      throw new Error(`the page showed no map: ${line}`);
    }
    await sleep(250);
  }
}

/**
 * A walkable cell 4 to 6 cells from a cell, from the page's map, so the
 * clicks of the walk do not land on the NPC there.
 */
async function cellNear(page: Page, to: [number, number]): Promise<[number, number]> {
  const cell = await page.evaluate((at) => {
    const v: unknown = Reflect.get(globalThis, 'viewer');
    const f: unknown = typeof v === 'object' && v !== null ? Reflect.get(v, 'map') : undefined;
    const m: unknown = typeof f === 'function' ? Reflect.apply(f, v, []) : undefined;
    if (typeof m !== 'object' || m === null) {
      return undefined;
    }
    const w = Number(Reflect.get(m, 'width'));
    const h = Number(Reflect.get(m, 'height'));
    const flags: unknown = Reflect.get(m, 'cellFlags');
    if (!(flags instanceof Uint8Array)) {
      return undefined;
    }
    for (let d = 4; d <= 6; d++) {
      for (let dy = -d; dy <= d; dy++) {
        for (let dx = -d; dx <= d; dx++) {
          const x = at[0] + dx;
          const y = at[1] + dy;
          if (
            Math.max(Math.abs(dx), Math.abs(dy)) === d &&
            x >= 0 &&
            y >= 0 &&
            x < w &&
            y < h &&
            ((flags[y * w + x] ?? 0) & 1) !== 0
          ) {
            return [x, y] as [number, number];
          }
        }
      }
    }
    return undefined;
  }, to);
  if (cell === undefined) {
    throw new Error(`no walkable cell near ${to.join(',')}`);
  }
  return cell;
}

/** Waits for the dialog window to show text, and returns its body. */
async function dialogText(page: Page): Promise<string> {
  try {
    await page.locator('#dialog-window').waitFor({ state: 'visible', timeout: 15_000 });
    await sleep(300);
    return (await page.textContent('#dialog-window')) ?? '';
  } catch (error) {
    return String(error).split('\n')[0] ?? '';
  }
}

/** Runs the kRO check. Chromium only, as the other game checks. */
export async function checkKRO(
  name: string,
  browser: Browser,
  ctx: FieldContext,
  check: Check,
): Promise<void> {
  const c = (label: string, ok: boolean, detail: string): void => {
    check(name, `kro ${label}`, ok, detail);
  };
  const build = process.env[KRO_ASSETS];
  if (build === undefined || build === '') {
    console.log(`skip ${name} kro: set ${KRO_ASSETS} to a full kRO asset build`);
    return;
  }
  const url = process.env.WATHENA_TEST_DATABASE_URL;
  if (url === undefined || url === '') {
    c('database', false, 'set WATHENA_TEST_DATABASE_URL to a PostgreSQL 18 server');
    return;
  }
  const testdb = join(ctx.dir, 'testdb');
  execFileSync('go', ['build', '-o', testdb, './tools/testdb'], {
    cwd: ctx.repo,
    stdio: 'inherit',
  });
  const wactl = join(ctx.dir, 'wactl-procedural');
  execFileSync('go', ['build', '-tags', 'procedural', '-o', wactl, './tools/wactl'], {
    cwd: ctx.repo,
    stdio: 'inherit',
  });
  const db = execFileSync(testdb, ['create'], { encoding: 'utf8' }).trim();
  const dbURL = (role?: string): string => {
    const u = new URL(url);
    u.pathname = `/${db}`;
    u.searchParams.delete('options');
    const base = u.toString();
    return role === undefined
      ? base
      : `${base}${base.includes('?') ? '&' : '?'}options=-c%20role%3D${role}`;
  };
  try {
    const password = `Classic-${randomBytes(9).toString('base64url')}`;
    execFileSync(
      wactl,
      ['world', 'new', '--mode', 'classic', '--admin', 'Admin', '--admin-password-file', '-'],
      {
        input: `${password}\n`,
        env: { ...process.env, WATHENA_MIGRATE_DATABASE_URL: dbURL() },
        stdio: ['pipe', 'ignore', 'inherit'],
      },
    );
    const game = await startGame(ctx.repo, ctx.client, ctx.dir, ctx.wactl, build, {
      classic: true,
      name: 'kro',
      config: CLASSIC,
      startJob: '0:20:10',
      startSeconds: 600,
      env: { WATHENA_DATABASE_URL: dbURL('wathena_game') },
    });
    const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
    try {
      const page = await context.newPage();
      const errors: string[] = [];
      page.on('pageerror', (e) => errors.push(e.message));
      await page.goto(gameURL(game.base, 'Pilgrim'));

      const [shown, line] = await firstMap(page);
      c('prontera', shown.endsWith('prontera'), `${shown}: ${line}`);

      // The camera starts far out over a town this size. The wheel brings it
      // in, so a click on the ground lands on a cell and not on a sprite.
      await settle(page, 30_000).catch(() => undefined);
      const far = await hook<number>(page, 'distance');
      for (let i = 0; i < 6; i++) {
        await page.mouse.move(640, 400);
        await page.mouse.wheel(0, -300);
        await sleep(200);
      }
      await settle(page, 30_000).catch(() => undefined);
      const near = await hook<number>(page, 'distance');
      console.log(`info chromium kro camera distance ${far.toFixed(1)} to ${near.toFixed(1)}`);
      const start = await selfCell(page);
      const legs = await walk(page, await cellNear(page, [NPC.x, NPC.y]), 1, async () =>
        Promise.resolve(false),
      );
      const at = await selfCell(page);
      c(
        'walk',
        legs.length > 0 && (at[0] !== start[0] || at[1] !== start[1]),
        `from ${start.join(',')} to ${at.join(',')} in ${legs.length} legs`,
      );

      let said = '';
      for (let i = 0; i < 5 && !said.includes(NPC.says); i++) {
        const screen = await unitScreen(page, (u) => u.name === NPC.name);
        await page.mouse.click(screen[0], screen[1]);
        said = await dialogText(page);
      }
      const npc = (await units(page)).find((u) => u.name === NPC.name);
      c(
        'dialog',
        said.includes(NPC.says),
        `${NPC.name} at ${npc?.cell?.join(',') ?? 'nowhere'}: ${said.replace(/\s+/g, ' ').trim().slice(0, 120)}`,
      );
      c('page errors', errors.length === 0, errors.join('; ') || 'none');
      c('server errors', game.errors.length === 0, game.errors.slice(0, 3).join('; ') || 'none');
    } catch (error) {
      const page = context.pages()[0];
      const shots = process.env.SCREENSHOTS;
      if (page !== undefined && shots !== undefined) {
        await page.screenshot({ path: join(shots, 'kro-run.png') }).catch(() => undefined);
      }
      c('run', false, String(error).split('\n')[0] ?? '');
    } finally {
      await context.close();
      await game.stop();
    }
  } finally {
    execFileSync(testdb, ['drop', db], { stdio: 'inherit' });
  }
}
