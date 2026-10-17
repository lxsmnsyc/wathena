// The rotation check of decision 0045: a waserver built with -tags
// procedural runs a --dev world layout with a town, two fields and one
// dungeon, which rotates as in a new world, in UTC.
//
// - Delver warps into the rotating dungeon's first floor, which the page
//   generates from its public node.
// - The server clock moves to 9 minutes before the next rotation. The
//   chat log shows the warning.
// - At the time, the server moves Delver to the town, and the page loads
//   it. The old floor is gone from the server.
// - Delver warps into the new first floor, which carries the new date in
//   its map ID, and the page generates it with the server's checksum.

import type { Browser, Page } from 'playwright-core';
import { gameURL, hook, sleep, startGame } from './game-check.ts';
import type { FieldContext } from './field-check.ts';
import { nthLoad } from './places-check.ts';

type Check = (browser: string, name: string, ok: boolean, detail: string) => void;

const WEEKDAYS = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday'];

/**
 * The next time of a rotation after now, in UTC, from its rule as the
 * server logs it: "daily at 04:00" or "weekly on friday at 20:00".
 */
export function nextRotation(rule: string, now: Date): Date {
  const m = /^(?:daily|weekly on (\w+)) at (\d\d):(\d\d)$/.exec(rule);
  if (m === null) {
    throw new Error(`a rotation rule ${rule}`);
  }
  const day = m[1] === undefined ? -1 : WEEKDAYS.indexOf(m[1]);
  for (let i = 0; i <= 8; i++) {
    const t = new Date(
      Date.UTC(
        now.getUTCFullYear(),
        now.getUTCMonth(),
        now.getUTCDate() + i,
        Number(m[2]),
        Number(m[3]),
      ),
    );
    if (t > now && (day < 0 || t.getUTCDay() === day)) {
      return t;
    }
  }
  throw new Error(`no time of ${rule} after ${now.toISOString()}`);
}

/** Waits for a line of the server's log that a pattern matches. */
async function logMatch(log: () => string, re: RegExp, timeout = 30_000): Promise<RegExpExecArray> {
  const end = Date.now() + timeout;
  for (;;) {
    const found = re.exec(log());
    if (found !== null) {
      return found;
    }
    if (Date.now() > end) {
      throw new Error(`no log line ${re.source}`);
    }
    await sleep(200);
  }
}

/** Types a line into the chat and sends it. */
async function say(page: Page, text: string): Promise<void> {
  await page.fill('#chat-input', text);
  await page.press('#chat-input', 'Enter');
}

/** POSTs to the server's debug address. */
async function debugPost(debug: string, path: string): Promise<void> {
  const res = await fetch(`${debug}${path}`, { method: 'POST' });
  if (!res.ok) {
    throw new Error(`${path}: HTTP ${res.status} ${await res.text()}`);
  }
}

/** Runs the rotation check. Chromium only, as the other game checks. */
export async function checkRotation(
  name: string,
  browser: Browser,
  ctx: FieldContext,
  check: Check,
): Promise<void> {
  const game = await startGame(ctx.repo, ctx.client, ctx.dir, ctx.wactl, ctx.build, {
    layout: true,
    fieldSize: 64,
    config: 'time_zone: UTC\nprocedural:\n  layout: {grid: [2, 1], towns: 1, dungeons: 1}\n',
    name: 'rotation',
  });
  const c = (label: string, ok: boolean, detail: string): void => {
    check(name, `rotation ${label}`, ok, detail);
  };
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  try {
    const rotating = await logMatch(
      game.log,
      /msg="rotating dungeon" dungeon=(\S+) rotation="([^"]+)"/,
    );
    const started = await logMatch(game.log, /change="the rotation of \S+ to (\d{4}-\d\d-\d\d)"/);
    const dungeon = rotating[1] ?? '';
    const rule = rotating[2] ?? '';
    const floor = `gen:dungeon:${dungeon}:${started[1] ?? ''}:1`;
    const page = await context.newPage();
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.goto(gameURL(game.base, 'Delver'));
    await nthLoad(page, 1);
    await debugPost(game.debug, '/debug/wathena/staff?name=Delver&role=admin');
    await say(page, '@hide');
    await say(page, `@warp ${floor}`);
    const first = await nthLoad(page, 2);
    c(
      'old floor',
      first.map === floor && first.kind === 'dungeon' && first.source === 'generated',
      `${first.map} ${first.kind}: ${first.source}${first.reason === undefined ? '' : ` (${first.reason})`}, ${first.ms.toFixed(0)} ms`,
    );

    // The server clock goes to 9 minutes before the next rotation.
    const now = new Date();
    const next = nextRotation(rule, now);
    const lead = next.getTime() - now.getTime() - 9 * 60_000;
    if (lead > 0) {
      await debugPost(
        game.debug,
        `/debug/wathena/clock?advance=${String(Math.floor(lead / 1000))}s`,
      );
    }
    await page.waitForFunction(
      () =>
        document.querySelector('#chat-log')?.textContent.includes('closes for new floors in') ===
        true,
      undefined,
      { timeout: 15_000 },
    );
    const warning =
      (await page.locator('#chat-log').textContent())?.match(
        /[^\n]*closes for new floors in[^\n]*/,
      )?.[0] ?? '';
    c('warning', warning.includes('Anyone still here then goes to'), warning.trim().slice(0, 160));

    // At the time, the page follows the server to the town.
    await debugPost(game.debug, '/debug/wathena/clock?advance=600s');
    const town = await nthLoad(page, 3);
    const rotated = await logMatch(
      game.log,
      /msg="world changed" change="the rotation of \S+ to (\d{4}-\d\d-\d\d)"/,
    );
    const date = rotated[1] ?? '';
    c(
      'moved to the town',
      town.map.startsWith('gen:town:') && town.source === 'generated',
      `${town.map}: ${town.source}, the new date ${date}`,
    );
    const old = await fetch(`${game.game}/gen/maps/${encodeURIComponent(floor)}.json`);
    c('old floor gone', old.status === 404, `${floor}: HTTP ${old.status}`);

    // The new floor carries the date, and the page generates it.
    const fresh = `gen:dungeon:${dungeon}:${date}:1`;
    await say(page, `@warp ${fresh}`);
    const loaded = await nthLoad(page, 4);
    const shown = await hook<string>(page, 'displayName').catch(() => '');
    c(
      'new floor',
      loaded.map === fresh &&
        loaded.kind === 'dungeon' &&
        loaded.source === 'generated' &&
        date > (started[1] ?? ''),
      `${loaded.map} ${loaded.kind}: ${loaded.source}${loaded.reason === undefined ? '' : ` (${loaded.reason})`}, ${loaded.ms.toFixed(0)} ms, "${shown}"`,
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
