// The procedural content check of decision 0044: `wactl world new`, built
// with -tags procedural, makes a small procedural world in a new
// database, and a waserver built with the tag runs it with development
// sessions.
//
// - Visitor starts in the first town, which the page generates.
// - Visitor walks to the Kafra Employee, a service NPC that stands at the
//   town's storage place, and talks to her.
// - Visitor walks to the quest board, which stands at the quest_board
//   place that the world's content added, and reads the town's quests,
//   which the world chose from its fields' spawns.
//
// The database comes from WATHENA_TEST_DATABASE_URL, a role that can make
// databases. tools/testdb makes one for the run and drops it at the end.

import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { writeFileSync } from 'node:fs';
import { join } from 'node:path';
import type { Browser, Page } from 'playwright-core';
import { gameURL, selfCell, sleep, startGame, unitScreen, units } from './game-check.ts';
import type { FieldContext } from './field-check.ts';
import { type InfoPlace, nthLoad, placesOf, walk } from './places-check.ts';

type Check = (browser: string, name: string, ok: boolean, detail: string) => void;

/** The world's layout: 2 × 2 fields of 64 cells, two towns and a dungeon. */
const WORLD = 'procedural:\n  field_size: 64\n  layout: {grid: [2, 2], towns: 2, dungeons: 1}\n';

/** The first place with a tag. */
function find(list: InfoPlace[], tag: string): InfoPlace {
  const found = list.find((p) => p.tags.includes(tag));
  if (found === undefined) {
    throw new Error(`no place tagged ${tag}`);
  }
  return found;
}

/** Waits for the dialog window to show a text, and returns its body. */
async function dialogSays(page: Page, text: string): Promise<string> {
  try {
    await page.locator(`#dialog-window:has-text("${text}")`).waitFor({ timeout: 15_000 });
    return (await page.textContent('#dialog-window')) ?? '';
  } catch (error) {
    return `${(await page.textContent('#dialog-window').catch(() => '')) ?? ''} ${String(error).split('\n')[0]}`;
  }
}

/** Closes the dialog window with its buttons, up to five steps. */
async function closeDialog(page: Page): Promise<void> {
  for (let i = 0; i < 5 && (await page.locator('#dialog-window').isVisible()); i++) {
    const cancel = page.locator('#dialog-window button:visible', { hasText: /^(Close|Cancel)$/ });
    if ((await cancel.count()) > 0) {
      await cancel
        .first()
        .click({ timeout: 5000 })
        .catch(() => undefined);
    } else {
      await page
        .locator('#dialog-window .dialog-controls button:visible')
        .first()
        .click({ timeout: 5000 })
        .catch(() => undefined);
    }
    await sleep(300);
  }
}

/**
 * A walkable cell 4 to 6 cells from a place, with no unit within 2
 * cells, so the clicks of the walk never land on an NPC's sprite.
 */
async function standingCell(page: Page, place: InfoPlace): Promise<[number, number]> {
  const cell = await page.evaluate(
    (to) => {
      const v: unknown = Reflect.get(globalThis, 'viewer');
      const call = (name: string): unknown => {
        const f: unknown = typeof v === 'object' && v !== null ? Reflect.get(v, name) : undefined;
        return typeof f === 'function' ? Reflect.apply(f, v, []) : undefined;
      };
      const m: unknown = call('map');
      const others: unknown = call('units');
      if (typeof m !== 'object' || m === null) {
        return undefined;
      }
      const w = Number(Reflect.get(m, 'width'));
      const h = Number(Reflect.get(m, 'height'));
      const flags: unknown = Reflect.get(m, 'cellFlags');
      if (!(flags instanceof Uint8Array)) {
        return undefined;
      }
      const busy = (x: number, y: number): boolean =>
        Array.isArray(others) &&
        others.some((u: unknown) => {
          const c: unknown =
            typeof u === 'object' && u !== null ? Reflect.get(u, 'cell') : undefined;
          return (
            Array.isArray(c) && Math.abs(Number(c[0]) - x) <= 2 && Math.abs(Number(c[1]) - y) <= 2
          );
        });
      for (let d = 4; d <= 6; d++) {
        for (let dy = -d; dy <= d; dy++) {
          for (let dx = -d; dx <= d; dx++) {
            const x = to[0] + dx;
            const y = to[1] + dy;
            if (
              Math.max(Math.abs(dx), Math.abs(dy)) === d &&
              x >= 0 &&
              y >= 0 &&
              x < w &&
              y < h &&
              ((flags[y * w + x] ?? 0) & 1) !== 0 &&
              !busy(x, y)
            ) {
              return [x, y] as [number, number];
            }
          }
        }
      }
      return undefined;
    },
    [place.x, place.y] as [number, number],
  );
  if (cell === undefined) {
    throw new Error(`no free cell near ${place.x},${place.y}`);
  }
  return cell;
}

/**
 * Walks near an NPC at a place, checks it stands there, and clicks it
 * until its dialog shows a text, at most five times, since a click may
 * land on a sprite beside it. It returns where the NPC stands and what
 * the dialog shows.
 */
async function visit(
  page: Page,
  place: InfoPlace,
  npc: string,
  text: string,
): Promise<[string, string]> {
  // A town's square has many NPCs, and a click of the walk may land on a
  // sprite and open its dialog. The walk keeps 4 cells from units where
  // it can, then 3 and 2 in a crowded square, and goes on after a stray
  // dialog closes.
  let last: unknown;
  let arrived = false;
  for (const clearance of [4, 4, 3, 3, 2, 2]) {
    try {
      await walk(
        page,
        await standingCell(page, place),
        1,
        async () => Promise.resolve(false),
        clearance,
      );
      arrived = true;
      break;
    } catch (error) {
      last = error;
      await closeDialog(page);
    }
  }
  if (!arrived) {
    // The last way: the connection's walk, as a click on the ground
    // sends it, toward the cell, without a click that may hit a sprite.
    const cell = await standingCell(page, place);
    for (let i = 0; i < 20 && !arrived; i++) {
      await page.evaluate(([x, y]) => {
        const v: unknown = Reflect.get(globalThis, 'viewer');
        const get: unknown =
          typeof v === 'object' && v !== null ? Reflect.get(v, 'session') : undefined;
        const session: unknown = typeof get === 'function' ? Reflect.apply(get, v, []) : undefined;
        const conn: unknown =
          typeof session === 'object' && session !== null
            ? Reflect.get(session, 'connection')
            : undefined;
        const walkTo: unknown =
          typeof conn === 'object' && conn !== null ? Reflect.get(conn, 'walkTo') : undefined;
        if (typeof walkTo === 'function') {
          Reflect.apply(walkTo, conn, [x, y]);
        }
      }, cell);
      await sleep(1500);
      const at = await selfCell(page);
      arrived = Math.max(Math.abs(at[0] - cell[0]), Math.abs(at[1] - cell[1])) <= 1;
    }
    if (!arrived) {
      throw last;
    }
  }
  const unit = (await units(page)).find((u) => u.name === npc);
  const at = `${npc} at ${unit?.cell?.join(',') ?? 'nowhere'}, its place at ${place.x},${place.y}`;
  if (unit?.cell?.[0] !== place.x || unit.cell[1] !== place.y) {
    throw new Error(at);
  }
  let said = '';
  for (let i = 0; i < 5 && !said.includes(text); i++) {
    await closeDialog(page);
    const screen = await unitScreen(page, (u) => u.name === npc);
    await page.mouse.click(screen[0], screen[1]);
    said = await dialogSays(page, text);
  }
  return [at, said];
}

/** The units the viewer shows, for the report of a failed run. */
async function unitsOf(page: Page): Promise<string> {
  const list = await units(page).catch(() => []);
  return list.map((u) => `${u.name}@${u.cell?.join(',') ?? '?'}`).join(' ');
}

/** Runs the content check. Chromium only, as the other game checks. */
export default async function checkContent(
  name: string,
  browser: Browser,
  ctx: FieldContext,
  check: Check,
): Promise<void> {
  const c = (label: string, ok: boolean, detail: string): void => {
    check(name, `content ${label}`, ok, detail);
  };
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
  const config = join(ctx.dir, 'content-world.yml');
  writeFileSync(config, WORLD);
  try {
    const password = `Content-${randomBytes(9).toString('base64url')}`;
    execFileSync(
      wactl,
      [
        'world',
        'new',
        '--mode',
        'procedural',
        '--config',
        config,
        '--admin',
        'Admin',
        '--admin-password-file',
        '-',
      ],
      {
        input: `${password}\n`,
        env: { ...process.env, WATHENA_MIGRATE_DATABASE_URL: dbURL() },
        stdio: ['pipe', 'ignore', 'inherit'],
      },
    );
    const game = await startGame(ctx.repo, ctx.client, ctx.dir, ctx.wactl, ctx.build, {
      procedural: true,
      name: 'content',
      env: { WATHENA_DATABASE_URL: dbURL('wathena_game') },
    });
    const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
    try {
      const page = await context.newPage();
      const errors: string[] = [];
      page.on('pageerror', (e) => errors.push(e.message));
      await page.goto(gameURL(game.base, 'Visitor'));
      const town = await nthLoad(page, 1);
      c(
        'town',
        town.map === 'gen:town:t1' && town.kind === 'town' && town.source === 'generated',
        `${town.map} ${town.kind}: ${town.source}, ${town.ms.toFixed(0)} ms`,
      );
      const places = await placesOf(game.game, 'gen:town:t1');
      await sleep(1000);

      // A service NPC: the Kafra Employee at the storage place.
      let [at, said] = await visit(
        page,
        find(places, 'storage'),
        'Kafra Employee',
        'Kafra Corporation',
      );
      c(
        'kafra',
        said.includes('Kafra Corporation'),
        `${at}: ${said.replace(/\s+/g, ' ').trim().slice(0, 120)}`,
      );
      await closeDialog(page);

      // The quest board at its place, with the town's quests.
      [at, said] = await visit(page, find(places, 'quest_board'), 'Quest Board', 'Quests of');
      await page
        .locator('#dialog-window .dialog-controls button:visible')
        .first()
        .click({ timeout: 5000 })
        .catch(() => undefined);
      const options = page.locator('#dialog-window .dialog-menu button');
      await options
        .first()
        .waitFor({ timeout: 15_000 })
        .catch(() => undefined);
      const n = await options.count();
      const first = n > 0 ? ((await options.first().textContent()) ?? '') : '';
      c(
        'quest board',
        said.includes('Quests of') && n >= 2,
        `${at}: ${said.replace(/\s+/g, ' ').trim().slice(0, 80)}, ${n} options, the first "${first}"`,
      );
      await closeDialog(page);
      c('page errors', errors.length === 0, errors.join('; ') || 'none');
      c('server errors', game.errors.length === 0, game.errors.slice(0, 3).join('; ') || 'none');
    } catch (error) {
      const page = context.pages()[0];
      const shots = process.env.SCREENSHOTS;
      if (page !== undefined && shots !== undefined) {
        await page.screenshot({ path: join(shots, 'content-run.png') }).catch(() => undefined);
      }
      const seen = page === undefined ? '' : await unitsOf(page);
      c('run', false, `${String(error).split('\n')[0] ?? ''} [units ${seen}]`);
    } finally {
      await context.close();
      await game.stop();
    }
  } finally {
    execFileSync(testdb, ['drop', db], { stdio: 'inherit' });
  }
}
