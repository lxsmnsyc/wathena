// The online checks of browser-check.ts (decision 0034): a waserver with
// --dev sessions on the placeholder map and the end-to-end fixture mod,
// played with touch on phones and a tablet, and with a mouse.
//
// - Touch runs in emulated devices through the Chrome DevTools Protocol,
//   so they run in Chromium only. A phone plays the whole run: walking by
//   tap, by a held finger and by the joystick, pinch and twist, attack,
//   NPC talk, learning a skill, dragging it to the hotkey bar, casting it
//   with the cancel button, the inventory, equipping, dragging an item,
//   chat, and a trade with a second phone.
// - A tablet checks the layout and walks.
// - A desktop page checks that the mouse and keyboard work as before.
// - frameTimes measures frames at phone size with the CPU slowed down.

import { type ChildProcess, execFileSync, spawn } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { createServer as netServer } from 'node:net';
import { basename, join } from 'node:path';
import {
  type Browser,
  type BrowserContext,
  type CDPSession,
  type Page,
  devices,
} from 'playwright-core';
import { type ViteDevServer, createServer } from 'vite';

/** Waits for a time, in milliseconds. */
export async function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

export type Check = (name: string, ok: boolean, detail: string) => void;

/** A free TCP port on the loopback address. */
export async function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const s = netServer();
    s.once('error', reject);
    s.listen(0, '127.0.0.1', () => {
      const a = s.address();
      const port = typeof a === 'object' && a !== null ? a.port : 0;
      s.close(() => {
        resolve(port);
      });
    });
  });
}

/** The waserver config: the fixture's permissions, and fast chat. */
const CONFIG = `mods:
  approve:
    e2e.fixture: [player.give_items, player.give_zeny, player.warp, player.sanction, guild.manage, rules.override, mail.send, currencies.modify, shops.modify, events.schedule, events.control, player.expand_limits]
chat:
  new_character_period: 0s
  burst: 100
  refill: 10ms
`;

export interface Game {
  /** The Vite dev server's base URL, which forwards to the game. */
  base: string;
  /** The game server's own URL. */
  game: string;
  /** The server's debug address, with --dev's clock and staff endpoints. */
  debug: string;
  stop: () => Promise<void>;
  /** The end of the server's log. */
  log: () => string;
  /** The server's ERROR lines so far, up to 50. */
  errors: string[];
}

/** How startGame runs the server. */
export interface GameOptions {
  /**
   * Development sessions, where a name is a session token. Without them
   * the server serves the account pages and needs both databases.
   */
  dev?: boolean;
  /** --start-job, for development sessions. */
  startJob?: string;
  /** --start-items. */
  startItems?: string;
  /** More of the config, after the fixture's permissions and chat limits. */
  config?: string;
  /** More environment variables, such as the database URLs. */
  env?: Record<string, string>;
  /** Mods of the repository to install beside the fixture, such as mods/examples/lantern. */
  mods?: string[];
  /** A name for this server's files in dir, so two servers can share it. */
  name?: string;
  /** More lines of mods.approve, such as "    example.a: [player.give_zeny]\n". */
  approve?: string;
  /**
   * Generated fields to run, as --fields takes them, with a waserver
   * built with -tags procedural (decision 0039). The fields start, and
   * placeholder/map runs beside them.
   */
  fields?: string;
  /** --field-size, with fields. */
  fieldSize?: number;
  /** --towns and --dungeons, with fields (decision 0040). */
  towns?: number;
  dungeons?: string;
  /**
   * --layout: a --dev world layout built from the config's
   * procedural.layout, with a waserver built with -tags procedural
   * (decision 0041). The home area starts, and placeholder/map runs
   * beside the layout's maps. fieldSize sets --field-size.
   */
  layout?: boolean;
  /**
   * A waserver built with -tags procedural, for a procedural world that
   * `wactl world new` made in the database of env (decision 0044).
   * placeholder/map runs beside the world's maps, for the fixture's
   * events.
   */
  procedural?: boolean;
}

/**
 * Builds waserver into dir once, and returns its path. procedural builds
 * it with -tags procedural.
 */
function buildServer(repo: string, dir: string, procedural = false): string {
  const waserver = join(dir, procedural ? 'waserver-procedural' : 'waserver');
  if (!existsSync(waserver)) {
    execFileSync(
      'go',
      [
        'build',
        ...(procedural ? ['-tags', 'procedural'] : []),
        '-o',
        waserver,
        './server/cmd/waserver',
      ],
      { cwd: repo, stdio: 'inherit' },
    );
  }
  return waserver;
}

/**
 * Copies the fixture and the mods asked for into a mods folder of dir,
 * and builds their art.
 */
function installMods(repo: string, dir: string, wactl: string, extra: readonly string[]): string {
  const mods = join(dir, `mods-${extra.map((m) => basename(m)).join('-') || 'fixture'}`);
  if (existsSync(mods)) {
    return mods;
  }
  cpSync(join(repo, 'e2e', 'testdata', 'mods'), mods, { recursive: true });
  // The end-to-end world has a second map for the treasure hunt. This
  // world has one, so the hunt's chests go on it.
  const events = join(mods, 'fixture', 'data', 'events.yml');
  writeFileSync(
    events,
    readFileSync(events, 'utf8').replace('maps: [e2e_field]', 'maps: [placeholder/map]'),
  );
  const built = [join(mods, 'fixture')];
  for (const m of extra) {
    const to = join(mods, basename(m));
    cpSync(join(repo, m), to, {
      recursive: true,
      // A build in the mod's folder is the author's, not this check's.
      filter: (src) => !src.startsWith(join(repo, m, 'assets')),
    });
    built.push(to);
  }
  // Lantern Ghosts stand still in the checks, as the Dummies do. A ghost
  // that wanders may walk off before a click lands, or cover an NPC or a
  // Dummy. They still fight back when attacked.
  const ghosts = join(mods, 'lantern', 'data', 'monsters.yml');
  if (existsSync(ghosts)) {
    const yml = readFileSync(ghosts, 'utf8');
    const still = yml.replace('  ai: 1\n', '  ai: 1\n  modes: [no_random_walk]\n');
    if (still === yml) {
      throw new Error('the lantern mod has no Lantern Ghost of AI type 1');
    }
    writeFileSync(ghosts, still);
    // The ghosts stand on fixed cells, clear of the Dummies at y 15, the
    // Training Dummy that !trainer places at 16,9, the NPCs and the
    // cells the checks walk to. A ghost at a random cell could stand next
    // to the only Training Dummy, which then has no clear click
    // (decision 0037, "Lantern Ghosts on fixed cells").
    const script = join(mods, 'lantern', 'scripts', 'main.lua');
    const lua = readFileSync(script, 'utf8');
    const placed = lua.replace(
      'spawning.monsters { monster = GHOST, amount = 3, respawn = true }',
      [
        'spawning.monsters { monster = GHOST, amount = 1, x = 20, y = 11, respawn = true }',
        '    spawning.monsters { monster = GHOST, amount = 1, x = 21, y = 13, respawn = true }',
        '    spawning.monsters { monster = GHOST, amount = 1, x = 19, y = 15, respawn = true }',
      ].join('\n'),
    );
    if (placed === lua) {
      throw new Error('the lantern mod no longer places its ghosts with one spawn');
    }
    writeFileSync(script, placed);
  }
  // The Hunting Board's Hunt Master stands on the start map, and her
  // quest counts the fixture's Dummies, which stand still and come back
  // a second after they die (decision 0043).
  const board = join(mods, 'hunting-board');
  if (existsSync(board)) {
    const npcs = join(board, 'data', 'npcs.yml');
    const yml = readFileSync(npcs, 'utf8');
    const moved = yml.replace(
      '  map: prontera\n  x: 150\n  y: 180\n',
      '  map: placeholder/map\n  x: 9\n  y: 9\n',
    );
    const quests = join(board, 'data', 'quests.yml');
    const q = readFileSync(quests, 'utf8');
    const dummies = q.replaceAll('monster: PORING', 'monster: e2e.fixture:DUMMY');
    if (moved === yml || dummies === q) {
      throw new Error('the Hunting Board no longer places its NPC in Prontera or hunts Porings');
    }
    writeFileSync(npcs, moved);
    writeFileSync(quests, dummies);
  }
  // A mod without art, such as the Market Town, has nothing to build.
  for (const m of built.filter((d) => existsSync(join(d, 'art')))) {
    execFileSync(wactl, ['mod', 'build-art', m], { stdio: 'ignore' });
  }
  return mods;
}

/**
 * Builds waserver and the fixture's art in dir, starts the server on the
 * asset build, and a Vite dev server that forwards /assets and the game
 * to it, as a phone test on the local network does.
 */
export async function startGame(
  repo: string,
  client: string,
  dir: string,
  wactl: string,
  build: string,
  opts: GameOptions = {},
): Promise<Game> {
  const waserver = buildServer(
    repo,
    dir,
    opts.fields !== undefined || opts.layout === true || opts.procedural === true,
  );
  const mods = installMods(repo, dir, wactl, opts.mods ?? []);
  const config = join(dir, `waserver-${opts.name ?? 'game'}.yml`);
  writeFileSync(
    config,
    CONFIG.replace('  approve:\n', `  approve:\n${opts.approve ?? ''}`) + (opts.config ?? ''),
  );
  const port = await freePort();
  const debug = await freePort();
  const vitePort = await freePort();
  const game = `http://127.0.0.1:${port}`;
  const origin = `http://localhost:${vitePort}`;
  const dev = opts.dev ?? true;
  const server: ChildProcess = spawn(
    waserver,
    [
      ...['--config', config, '--assets', build, '--map', 'placeholder/map'],
      ...['--mode', 'renewal', '--addr', `127.0.0.1:${port}`, '--debug-addr', `127.0.0.1:${debug}`],
      ...['--origins', origin, '--shutdown-countdown', '0', '--mods', mods],
      ...['--start-items', opts.startItems ?? '501:10,909:20,1201:1,2101:1'],
      ...(opts.fields === undefined
        ? []
        : [
            '--fields',
            opts.fields,
            '--field-size',
            String(opts.fieldSize ?? 160),
            '--maps',
            'placeholder/map',
            ...(opts.towns === undefined ? [] : ['--towns', String(opts.towns)]),
            ...(opts.dungeons === undefined ? [] : ['--dungeons', opts.dungeons]),
          ]),
      ...(opts.layout === true
        ? ['--layout', '--field-size', String(opts.fieldSize ?? 160), '--maps', 'placeholder/map']
        : []),
      ...(opts.procedural === true ? ['--maps', 'placeholder/map'] : []),
      ...(dev
        ? ['--dev', '--seed', 'browser-check', '--start-job', opts.startJob ?? '1:50:40']
        : ['--play-url', `${origin}/`]),
    ],
    { stdio: ['ignore', 'ignore', 'pipe'], env: { ...process.env, ...opts.env } },
  );
  // A crash of the check must not leave the server running.
  process.once('exit', () => {
    server.kill('SIGKILL');
  });
  let log = '';
  let partial = '';
  const errors: string[] = [];
  const serving = new Promise<void>((resolve, reject) => {
    server.stderr?.on('data', (b: Buffer) => {
      const text = b.toString();
      const lines = (partial + text).split('\n');
      partial = lines.pop() ?? '';
      for (const line of lines) {
        if (line.includes('level=ERROR') && errors.length < 50) {
          errors.push(line);
        }
      }
      log += text;
      if (log.includes('msg=serving')) {
        resolve();
      }
      if (log.length > 100_000) {
        log = log.slice(-10_000);
      }
    });
    server.once('exit', (code) => {
      reject(new Error(`waserver exited with ${code}:\n${log.slice(-3000)}`));
    });
  });
  await Promise.race([
    serving,
    new Promise((_, reject) => {
      setTimeout(() => {
        reject(new Error(`waserver did not start in 120 seconds:\n${log.slice(-3000)}`));
      }, 120_000);
    }),
  ]);
  // vite.config.ts reads these when the server starts.
  process.env.WATHENA_ASSETS = `${game}/assets`;
  process.env.WATHENA_GAME = game;
  const vite: ViteDevServer = await createServer({
    root: client,
    server: { port: vitePort, strictPort: true, host: 'localhost' },
    logLevel: 'error',
  }).finally(() => {
    delete process.env.WATHENA_GAME;
  });
  await vite.listen();
  const stopServer = async (): Promise<void> => {
    if (server.exitCode !== null) {
      return;
    }
    const exited = new Promise((resolve) => {
      server.once('exit', resolve);
    });
    server.kill('SIGINT');
    await Promise.race([
      exited,
      new Promise((resolve) => {
        setTimeout(resolve, 15_000);
      }),
    ]);
    // exitCode changes when the process exits, after the await.
    if (Reflect.get(server, 'exitCode') === null) {
      server.kill('SIGKILL');
    }
  };
  return {
    base: origin,
    game,
    debug: `http://127.0.0.1:${debug}`,
    stop: async () => {
      await vite.close();
      await stopServer();
    },
    log: () => log,
    errors,
  };
}

/** The game page of a --dev character, through the dev server's proxy. */
export function gameURL(base: string, name: string): string {
  const ws = `${base.replace(/^http/, 'ws')}/ws`;
  return `${base}/?server=${encodeURIComponent(ws)}&name=${name}&nosw`;
}

export interface UnitView {
  id: number;
  name: string | undefined;
  npc: boolean;
  player: boolean;
  self: boolean;
  cell: [number, number] | undefined;
  walking: boolean;
}

/** Calls a function of the page's development hook, globalThis.viewer. */
export async function hook<T>(page: Page, fn: string, ...args: unknown[]): Promise<T> {
  const r: unknown = await page.evaluate(
    ([f, a]) => {
      const v: unknown = Reflect.get(globalThis, 'viewer');
      const call: unknown = typeof v === 'object' && v !== null ? Reflect.get(v, f) : undefined;
      if (typeof call !== 'function') {
        throw new Error(`viewer.${f} is missing`);
      }
      const out: unknown = Reflect.apply(call, v, a);
      return out;
    },
    [fn, args] as const,
  );
  // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- each hook returns the type its caller names.
  return r as T;
}

export async function selfCell(page: Page): Promise<[number, number]> {
  const c = await hook<[number, number] | undefined>(page, 'selfCell');
  if (c === undefined) {
    throw new Error('the player has no cell');
  }
  return c;
}

export async function units(page: Page): Promise<UnitView[]> {
  return hook<UnitView[]>(page, 'units');
}

/**
 * Waits until the view is still: the player stands and the camera, which
 * follows it, has reached it. A screen point read before then is where a
 * unit was, not where a tap finds it.
 */
export async function settle(page: Page, timeout = 15_000): Promise<void> {
  await page.waitForFunction(
    () => {
      const v: unknown = Reflect.get(globalThis, 'viewer');
      const f: unknown =
        typeof v === 'object' && v !== null ? Reflect.get(v, 'settled') : undefined;
      return typeof f === 'function' && Reflect.apply(f, v, []) === true;
    },
    null,
    { timeout, polling: 50 },
  );
}

/** Waits until the player's cell passes a test, and returns it. */
export async function walkedTo(
  page: Page,
  test: (cell: [number, number]) => boolean,
  timeout = 10_000,
): Promise<[number, number]> {
  const end = Date.now() + timeout;
  for (;;) {
    const c = await selfCell(page);
    if (test(c)) {
      return c;
    }
    if (Date.now() > end) {
      throw new Error(`the player stayed at ${c}`);
    }
    await sleep(100);
  }
}

/**
 * A unit's place on the screen, by a test on it, once the view is still
 * and the unit stands.
 */
export async function unitScreen(
  page: Page,
  test: (u: UnitView) => boolean,
): Promise<[number, number]> {
  await settle(page);
  for (let i = 0; i < 100 && (await units(page)).some((u) => test(u) && u.walking); i++) {
    await sleep(100);
  }
  const list = await units(page);
  const me = list.find((u) => u.self)?.cell ?? [0, 0];
  const size = page.viewportSize();
  const shown: { u: UnitView; at: [number, number] }[] = [];
  for (const u of list) {
    const at = await hook<[number, number] | undefined>(page, 'screenOf', u.id);
    if (at !== undefined) {
      shown.push({ u, at });
    }
  }
  // The nearest match on the screen, preferring one that no other unit
  // covers, since a tap picks the unit nearest the finger. The player
  // covers nothing: a tap never picks the player itself.
  const onScreen = ([x, y]: [number, number]): boolean =>
    size === null || (x >= 0 && y >= 0 && x <= size.width && y <= size.height);
  const found = shown
    .filter((s) => test(s.u))
    .map((s) => ({
      ...s,
      off: !onScreen(s.at),
      covered: shown.some(
        (o) =>
          o.u.id !== s.u.id && !o.u.self && Math.hypot(o.at[0] - s.at[0], o.at[1] - s.at[1]) < 45,
      ),
      d: Math.hypot((s.u.cell?.[0] ?? 0) - me[0], (s.u.cell?.[1] ?? 0) - me[1]),
    }))
    .sort(
      (a, b) => Number(a.off) - Number(b.off) || Number(a.covered) - Number(b.covered) || a.d - b.d,
    )[0];
  if (found === undefined) {
    throw new Error(`no unit found among ${list.map((u) => u.name).join(', ')}`);
  }
  const at = found.at;
  if (size !== null && (at[0] < 0 || at[1] < 0 || at[0] > size.width || at[1] > size.height)) {
    throw new Error(`unit ${found.u.name} is not on the screen: ${JSON.stringify(at)}`);
  }
  return at;
}

/** Waits for the game to show the player and the units around. */
export async function inGame(page: Page): Promise<void> {
  await page.waitForFunction(
    () => {
      const v: unknown = Reflect.get(globalThis, 'viewer');
      if (typeof v !== 'object' || v === null) {
        return false;
      }
      const fn: unknown = Reflect.get(v, 'units');
      if (typeof fn !== 'function') {
        return false;
      }
      const list: unknown = Reflect.apply(fn, v, []);
      return Array.isArray(list) && list.length > 3;
    },
    null,
    { timeout: 60_000 },
  );
  await settle(page);
}

/**
 * Fingers on a page, through the Chrome DevTools Protocol.
 *
 * Each event carries the time the finger moved, as a phone's events do.
 * The page reads gesture times from those times (event.timeStamp), not
 * from when it gets to the event. The protocol waits until the page has
 * handled each event, so on a busy machine a tap sent as two awaited
 * events would last as long as the page's slowest frame. Taps and double
 * taps send their events together, with times 60 ms apart that end now.
 */
export class Fingers {
  private readonly points = new Map<number, { x: number; y: number }>();

  private readonly cdp: CDPSession;

  /** The time of the last event, in seconds since 1970. */
  private last = 0;

  private constructor(cdp: CDPSession) {
    this.cdp = cdp;
  }

  static async on(page: Page): Promise<Fingers> {
    return new Fingers(await page.context().newCDPSession(page));
  }

  /** The time of a new event: now, and after the last event. */
  private stamp(): number {
    this.last = Math.max(Date.now() / 1000, this.last + 0.001);
    return this.last;
  }

  private async send(
    type: 'touchStart' | 'touchMove' | 'touchEnd',
    timestamp = this.stamp(),
  ): Promise<void> {
    const touchPoints = [...this.points].map(([id, p]) => ({
      x: p.x,
      y: p.y,
      id,
      radiusX: 2,
      radiusY: 2,
      force: 1,
    }));
    await this.cdp.send('Input.dispatchTouchEvent', { type, touchPoints, timestamp });
  }

  async down(id: number, x: number, y: number, timestamp?: number): Promise<void> {
    this.points.set(id, { x, y });
    await this.send('touchStart', timestamp);
  }

  async move(id: number, x: number, y: number): Promise<void> {
    this.points.set(id, { x, y });
    await this.send('touchMove');
  }

  async up(id: number, timestamp?: number): Promise<void> {
    this.points.delete(id);
    await this.send('touchEnd', timestamp);
  }

  /** Taps, at the given times after now, in seconds: down, up, down, up. */
  private async taps(x: number, y: number, times: number[]): Promise<void> {
    // The taps end now, so no event has a time in the future.
    const span = times.at(-1) ?? 0;
    const start = Math.max(Date.now() / 1000 - span, this.last + 0.001);
    const point = [{ x, y, id: 1, radiusX: 2, radiusY: 2, force: 1 }];
    this.last = start + span;
    await Promise.all(
      times.map(async (t, i) =>
        this.cdp.send('Input.dispatchTouchEvent', {
          type: i % 2 === 0 ? 'touchStart' : 'touchEnd',
          touchPoints: i % 2 === 0 ? point : [],
          timestamp: start + t,
        }),
      ),
    );
  }

  /** A tap of 60 ms. */
  async tap(x: number, y: number): Promise<void> {
    await this.taps(x, y, [0, 0.06]);
  }

  /** Two taps of 60 ms, 100 ms apart. */
  async doubleTap(x: number, y: number): Promise<void> {
    await this.taps(x, y, [0, 0.06, 0.16, 0.22]);
  }

  /** Holds a finger still for a time. */
  async press(x: number, y: number, ms: number): Promise<void> {
    const start = this.stamp();
    await this.down(1, x, y, start);
    await sleep(ms);
    await this.up(1, Math.max(this.stamp(), start + ms / 1000));
  }

  /** A long press, then a drag to a point, then a lift. */
  async longDrag(from: [number, number], to: [number, number]): Promise<void> {
    await this.down(1, from[0], from[1]);
    await sleep(700);
    const steps = 12;
    for (let i = 1; i <= steps; i++) {
      await this.move(
        1,
        from[0] + ((to[0] - from[0]) * i) / steps,
        from[1] + ((to[1] - from[1]) * i) / steps,
      );
      await sleep(20);
    }
    await this.up(1);
  }
}

/** The middle of an element. */
export async function middle(page: Page, selector: string): Promise<[number, number]> {
  await page.locator(selector).first().scrollIntoViewIfNeeded({ timeout: 10_000 });
  const box = await page.locator(selector).first().boundingBox();
  if (box === null) {
    throw new Error(`${selector} is not shown`);
  }
  return [box.x + box.width / 2, box.y + box.height / 2];
}

/**
 * Walks with the joystick toward a unit until it is on the open part of
 * the screen, clear of the panels and the controls.
 */
export async function approach(
  page: Page,
  f: Fingers,
  test: (u: UnitView) => boolean,
): Promise<void> {
  const size = page.viewportSize() ?? { width: 390, height: 660 };
  for (let i = 0; i < 8; i++) {
    const list = await units(page);
    const me = list.find((u) => u.self);
    const target = list.find(test);
    if (me === undefined || target === undefined) {
      throw new Error('the unit is not in sight');
    }
    const s = await hook<[number, number] | undefined>(page, 'screenOf', target.id);
    const m = await hook<[number, number] | undefined>(page, 'screenOf', me.id);
    if (s === undefined || m === undefined) {
      throw new Error('the unit is not shown');
    }
    // Below the top bar and any open panel.
    const top = await page.evaluate(() =>
      Math.max(
        90,
        ...[...document.querySelectorAll<HTMLElement>('#controls, .game-window')]
          .filter((e) => e.offsetParent !== null || e.id === 'controls')
          .map((e) => e.getBoundingClientRect().bottom + 30),
      ),
    );
    if (s[0] > 40 && s[0] < size.width - 40 && s[1] > top && s[1] < size.height - 260) {
      return;
    }
    const d = Math.hypot(s[0] - m[0], s[1] - m[1]) || 1;
    const [x, y] = await middle(page, '#joystick');
    await f.down(7, x, y);
    await f.move(7, x + ((s[0] - m[0]) / d) * 50, y + ((s[1] - m[1]) / d) * 50);
    await sleep(450);
    await f.up(7);
    await settle(page);
  }
}

/**
 * Marks the damage numbers on the page, so newHit sees only later ones.
 * Old numbers fade and go, so a count would miss a new one.
 */
export async function markHits(page: Page): Promise<void> {
  await page.evaluate(() => {
    for (const h of document.querySelectorAll<HTMLElement>('#hits .hit')) {
      h.dataset.seen = '1';
    }
  });
}

/** Waits for a damage number that came after markHits. */
export async function newHit(page: Page, timeout: number): Promise<void> {
  await page.waitForFunction(
    () => document.querySelector('#hits .hit:not([data-seen])') !== null,
    null,
    { timeout },
  );
}

/** Taps an element with a finger. */
export async function tapOn(page: Page, f: Fingers, selector: string): Promise<void> {
  await page.locator(selector).first().waitFor({ state: 'visible', timeout: 10_000 });
  const [x, y] = await middle(page, selector);
  await f.tap(x, y);
}

/** Opens a window by its button: on a small screen, through the menu. */
export async function openWindow(page: Page, f: Fingers, button: string): Promise<void> {
  if (!(await page.locator(button).isVisible())) {
    await tapOn(page, f, '#menu-toggle');
  }
  await tapOn(page, f, button);
}

/** Runs one step, and reports it as a check. */
export async function step(
  check: Check,
  name: string,
  run: () => Promise<string>,
  on: { errors: string[]; page: Page },
): Promise<void> {
  let ok = false;
  try {
    const detail = await run();
    const errs = on.errors.splice(0);
    ok = errs.length === 0;
    check(name, ok, ok ? detail : `page errors: ${errs.join('; ')}`);
  } catch (e) {
    const msg = e instanceof Error ? (e.message.split('\n')[0] ?? '') : String(e);
    const errs = on.errors.splice(0);
    const also = errs.length === 0 ? '' : `; page errors: ${errs.join('; ')}`;
    check(name, false, `${msg}${also} [${await pageState(on.page)}]`);
  }
  // SCREENSHOTS names a folder for a screenshot of each failed step.
  const shots = process.env.SCREENSHOTS;
  if (!ok && shots !== undefined) {
    mkdirSync(shots, { recursive: true });
    await on.page
      .screenshot({ path: join(shots, `${name.replaceAll(/\W+/g, '-')}.png`) })
      .catch(() => undefined);
  }
}

/**
 * What a failed step's report tells about the page: the player's cell,
 * the target mode, the units in sight and the end of the chat log.
 */
export async function pageState(page: Page): Promise<string> {
  try {
    const list = await units(page);
    const shown = list
      .filter((u) => !u.self)
      .map((u) => `${u.name ?? ''}#${u.id}@${u.cell?.join(',')}${u.walking ? ' walking' : ''}`)
      .join(', ');
    const me = list.find((u) => u.self);
    const targeting = await hook<string | undefined>(page, 'targeting');
    const chat = ((await page.locator('#chat-log').textContent()) ?? '').slice(-160);
    const items = (await hook<{ id: number; itemId: number }[]>(page, 'groundItems'))
      .map((g) => `${g.itemId}#${g.id}`)
      .join(' ');
    return `me@${me?.cell?.join(',')}${me?.walking === true ? ' walking' : ''}, targeting ${targeting ?? 'none'}; units: ${shown}; ground: ${items}; chat ends: ${chat}`;
  } catch (e) {
    return `no state: ${e instanceof Error ? e.message.split('\n')[0] : String(e)}`;
  }
}

export async function open(
  browser: Browser,
  options: Parameters<Browser['newContext']>[0],
  url: string,
): Promise<{ context: BrowserContext; page: Page; errors: string[]; frames: { count: number } }> {
  const context = await browser.newContext(options);
  const page = await context.newPage();
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));
  const frames = countFrames(page);
  await page.goto(url);
  await inGame(page);
  return { context, page, errors, frames };
}

/** Counts the WebSocket frames a page sends. */
function countFrames(page: Page): { count: number } {
  const c = { count: 0 };
  page.on('websocket', (ws) => {
    ws.on('framesent', () => {
      c.count++;
    });
  });
  return c;
}

/** The touch run on two phones, with a trade between them. */
export async function checkPhones(
  browser: Browser,
  base: string,
  check: Check,
  phone = 'iPhone 13',
  other = 'Pixel 7',
): Promise<void> {
  const aliceDevice = devices[phone];
  const bobDevice = devices[other];
  if (aliceDevice === undefined || bobDevice === undefined) {
    throw new Error('unknown device');
  }
  // The device profiles name WebKit for iPhones; the run is in Chromium.
  const { defaultBrowserType: _a, ...aliceOptions } = aliceDevice;
  const { defaultBrowserType: _b, ...bobOptions } = bobDevice;
  const alice = await open(browser, aliceOptions, gameURL(base, 'Alice'));
  const frames = alice.frames;
  const bob = await open(browser, bobOptions, gameURL(base, 'Bob'));
  const page = alice.page;
  const f = await Fingers.on(page);
  const g = await Fingers.on(bob.page);
  const tag = `${phone} touch`;
  const c: Check = (name, ok, detail) => {
    check(`${tag} ${name}`, ok, detail);
  };
  try {
    await step(
      c,
      'touch mode',
      async () => {
        await tapOn(page, f, '#hp');
        const touch = await page.evaluate(() => document.body.classList.contains('touch'));
        const joystick = await page.locator('#joystick').isVisible();
        const hotkeys = await page.locator('#hotkeys').boundingBox();
        const ratio = await page.evaluate(() => {
          const canvas = document.querySelector('canvas');
          return canvas === null ? 0 : canvas.width / canvas.clientWidth;
        });
        if (!touch || !joystick) {
          throw new Error(`touch class ${touch}, joystick shown ${joystick}`);
        }
        if (ratio > 2) {
          throw new Error(`the canvas draws at ${ratio} times its size`);
        }
        return `body.touch, joystick shown, hotkey bar ${hotkeys?.width}×${hotkeys?.height} px, canvas at ${ratio}× its CSS size (device ${aliceOptions.deviceScaleFactor}×)`;
      },
      alice,
    );

    await step(
      c,
      'tap to walk',
      async () => {
        const before = await selfCell(page);
        const me = await unitScreen(page, (u) => u.self);
        await f.tap(me[0], me[1] - 90);
        const after = await walkedTo(page, (cell) => cell[1] > before[1]);
        return `walked north from ${before} to ${after}`;
      },
      alice,
    );

    await step(
      c,
      'hold to walk',
      async () => {
        const before = await selfCell(page);
        const me = await unitScreen(page, (u) => u.self);
        // A finger held beside the player, away from other units, keeps
        // walking that way.
        const list = await units(page);
        const spots: [number, number][] = [];
        for (const u of list) {
          const s = await hook<[number, number] | undefined>(page, 'screenOf', u.id);
          if (s !== undefined && !u.self) {
            spots.push(s);
          }
        }
        const free = [
          [0, 80],
          [80, 80],
          [-80, 80],
          [100, 0],
          [-100, 0],
        ]
          .map(([dx, dy]): [number, number] => [me[0] + (dx ?? 0), me[1] + (dy ?? 0)])
          .find(([x, y]) => spots.every((s) => Math.hypot(s[0] - x, s[1] - y) > 60));
        if (free === undefined) {
          throw new Error('no free ground beside the player');
        }
        await f.down(1, free[0], free[1]);
        await page.waitForTimeout(700);
        await f.move(1, free[0] + 2, free[1] + 2);
        await page.waitForTimeout(1300);
        await f.up(1);
        await page.waitForTimeout(500);
        const after = await selfCell(page);
        if (after[0] === before[0] && after[1] === before[1]) {
          throw new Error(`stayed at ${before}`);
        }
        return `a finger held at ${Math.round(free[0] - me[0])}, ${Math.round(free[1] - me[1])} px from the player walked it from ${before} to ${after}`;
      },
      alice,
    );

    await step(
      c,
      'joystick walk',
      async () => {
        await page.waitForTimeout(1000);
        const before = await selfCell(page);
        const [x, y] = await middle(page, '#joystick');
        const sent = frames.count;
        await f.down(7, x, y);
        // Push west, as the camera starts looking north.
        for (let i = 1; i <= 8; i++) {
          await f.move(7, x - 7 * Math.min(i, 6), y + (i % 2));
          await page.waitForTimeout(150);
        }
        const during = await selfCell(page);
        const frameCount = frames.count - sent;
        await f.up(7);
        const released = await selfCell(page);
        await page.waitForTimeout(900);
        const stopped = await selfCell(page);
        await page.waitForTimeout(800);
        const later = await selfCell(page);
        if (during[0] >= before[0]) {
          throw new Error(`from ${before} to ${during}`);
        }
        if (later[0] !== stopped[0] || later[1] !== stopped[1]) {
          throw new Error(`still walking after release: ${stopped} then ${later}`);
        }
        // The server ends the step it is on, then one more.
        if (Math.abs(stopped[0] - released[0]) > 3) {
          throw new Error(`walked from ${released} to ${stopped} after the release`);
        }
        // 1.2 seconds at one walk per 250 ms, and pings.
        if (frameCount > 8) {
          throw new Error(`${frameCount} messages in 1.2 s`);
        }
        // And back east.
        await f.down(7, x, y);
        for (let i = 1; i <= 8; i++) {
          await f.move(7, x + 7 * Math.min(i, 6), y);
          await page.waitForTimeout(150);
        }
        await f.up(7);
        await page.waitForTimeout(1000);
        const back = await selfCell(page);
        if (back[0] <= later[0]) {
          throw new Error(`east from ${later} to ${back}`);
        }
        return `walked west from ${before} to ${during}, released at ${released} and stopped at ${later}, sent ${frameCount} messages in 1.2 s, and walked east to ${back}`;
      },
      alice,
    );

    await step(
      c,
      'attack',
      async () => {
        await approach(page, f, (u) => u.name === 'Dummy');
        await markHits(page);
        const at = await unitScreen(page, (u) => u.name === 'Dummy');
        await f.tap(at[0], at[1]);
        await newHit(page, 15_000).catch(() => {
          throw new Error('no damage showed after a tap on a Dummy');
        });
        return 'tapped a Dummy, and damage showed';
      },
      alice,
    );

    await step(
      c,
      'NPC talk',
      async () => {
        await approach(page, f, (u) => u.name === 'Quartermaster');
        const at = await unitScreen(page, (u) => u.name === 'Quartermaster');
        await f.tap(at[0], at[1]);
        await page
          .locator('#dialog-window .dialog-menu button:has-text("Give")')
          .waitFor({ timeout: 15_000 });
        await tapOn(page, f, '#dialog-window .dialog-menu button:has-text("Give")');
        await page.locator('#dialog-window:has-text("Here you are")').waitFor({ timeout: 10_000 });
        const text = (await page.textContent('#dialog-window .dialog-body')) ?? '';
        for (let i = 0; i < 3 && (await page.locator('#dialog-window').isVisible()); i++) {
          await tapOn(page, f, '#dialog-window .dialog-controls button:visible');
          await page.waitForTimeout(400);
        }
        return `tapped the Quartermaster, chose Give, read "${text.trim()}"`;
      },
      alice,
    );

    await step(
      c,
      'skill to hotkey bar',
      async () => {
        await openWindow(page, f, '#skill-toggle');
        const row = '#skill-window li:has-text("Bash")';
        await page.locator(row).waitFor({ timeout: 10_000 });
        await tapOn(page, f, `${row} button.learn`);
        await page.locator(`${row}[draggable="true"]`).waitFor({ timeout: 10_000 });
        const from = await middle(page, `${row} .skill-text, ${row}`);
        const slot = await middle(page, '#hotkeys .hotkey >> nth=0');
        await f.longDrag(from, slot);
        const keys = await hook<({ skillId: number } | undefined)[]>(page, 'hotkeys');
        if (keys[0]?.skillId !== 5) {
          throw new Error(`slot F1 holds ${JSON.stringify(keys[0])}`);
        }
        await openWindow(page, f, '#skill-toggle');
        return 'learned Bash with "+", long-pressed and dragged it onto F1';
      },
      alice,
    );

    await step(
      c,
      'cast and cancel a skill',
      async () => {
        await tapOn(page, f, '#hotkeys .hotkey >> nth=0');
        await page.locator('#target-cancel').waitFor({ state: 'visible', timeout: 5000 });
        await tapOn(page, f, '#target-cancel');
        const cancelled = await hook<string | undefined>(page, 'targeting');
        if (cancelled !== undefined) {
          throw new Error('the cancel button did not cancel');
        }
        await approach(page, f, (u) => u.name === 'Dummy');
        await tapOn(page, f, '#hotkeys .hotkey >> nth=0');
        await page.locator('#target-cancel').waitFor({ state: 'visible', timeout: 5000 });
        await markHits(page);
        const at = await unitScreen(page, (u) => u.name === 'Dummy');
        await f.tap(at[0], at[1]);
        await newHit(page, 15_000).catch(() => {
          throw new Error('Bash showed no damage');
        });
        const left = await hook<string | undefined>(page, 'targeting');
        const hidden = await page.locator('#target-cancel').isHidden();
        if (left !== undefined || !hidden) {
          throw new Error(`target mode ${left}, cancel shown ${!hidden}`);
        }
        return 'F1 waited for a target with Cancel shown; Cancel ended it; F1 then a tap on a Dummy cast Bash';
      },
      alice,
    );

    await step(
      c,
      'inventory and equip',
      async () => {
        await openWindow(page, f, '#inventory-toggle');
        const box = await page.locator('#inventory-window').boundingBox();
        const width = aliceOptions.viewport.width;
        if (box === null || Math.abs(box.width - width) > 1) {
          throw new Error(`the inventory is ${box?.width} px wide on a ${width} px screen`);
        }
        const small = await page.evaluate(
          () =>
            [
              ...document.querySelectorAll<HTMLElement>(
                '#inventory-window button, #inventory-window li',
              ),
            ]
              .filter((e) => e.offsetParent !== null)
              .map((e) => e.getBoundingClientRect())
              .filter((r) => r.height < 44).length,
        );
        if (small > 0) {
          throw new Error(`${small} buttons or slots are under 44 px`);
        }
        const knife = '#inventory-window li:has-text("1201")';
        await page.locator(knife).waitFor({ timeout: 10_000 });
        const [x, y] = await middle(page, `${knife} .name`);
        await f.doubleTap(x, y);
        await page.locator(knife).waitFor({ state: 'detached', timeout: 10_000 });
        await openWindow(page, f, '#equipment-toggle');
        const worn = (await page.textContent('#equipment-window')) ?? '';
        await openWindow(page, f, '#equipment-toggle');
        if (!worn.includes('1201')) {
          throw new Error('the equipment window does not show the knife');
        }
        return `the inventory opened as a ${box.width} px panel with 44 px rows and buttons; a double tap equipped Item 1201`;
      },
      alice,
    );

    await step(
      c,
      'item drag',
      async () => {
        const jellopy = '#inventory-window li:has-text("909")';
        const from = await middle(page, `${jellopy} .name`);
        // Onto the game view, between the joystick and the hotkey bar: a
        // drop asks how many.
        const vw = aliceOptions.viewport.width;
        const vh = aliceOptions.viewport.height;
        await f.longDrag(from, [vw / 2, vh - 90]);
        await page
          .locator('#inventory-window .drop-form')
          .waitFor({ state: 'visible', timeout: 5000 });
        await tapOn(page, f, '#inventory-window .drop-form button:has-text("Cancel")');
        return 'a long press on Item 909 dragged it onto the game view, which asked how many to drop';
      },
      alice,
    );

    await step(
      c,
      'move a window',
      async () => {
        const before = await page.locator('#inventory-window').boundingBox();
        const [x, y] = await middle(page, '#inventory-window .window-bar');
        await f.down(1, x, y);
        for (let i = 1; i <= 5; i++) {
          await f.move(1, x + 10 * i, y + 20 * i);
        }
        await f.up(1);
        const after = await page.locator('#inventory-window').boundingBox();
        if (before === null || after === null || after.y < before.y + 50 || after.x !== before.x) {
          throw new Error(`from ${before?.x},${before?.y} to ${after?.x},${after?.y}`);
        }
        // Back up, for the trade.
        const [bx, by] = await middle(page, '#inventory-window .window-bar');
        await f.down(1, bx, by);
        await f.move(1, bx, by - 50);
        await f.move(1, bx, by - 100);
        await f.up(1);
        return `the title bar moved the panel down from y ${Math.round(before.y)} to ${Math.round(after.y)}, and only down`;
      },
      alice,
    );

    await step(
      c,
      'chat',
      async () => {
        await tapOn(page, f, '#chat-button');
        await page.waitForFunction(() => document.activeElement?.id === 'chat-input');
        await page.keyboard.type('hello from a phone');
        await tapOn(page, f, '#chat-send');
        await page.waitForFunction(
          () =>
            document.querySelector('#chat-log')?.textContent.includes('hello from a phone') ===
            true,
          null,
          { timeout: 10_000 },
        );
        await bob.page.waitForFunction(
          () =>
            document.querySelector('#chat-log')?.textContent.includes('hello from a phone') ===
            true,
          null,
          { timeout: 10_000 },
        );
        // Send on an empty line closes the chat, as Enter does.
        await tapOn(page, f, '#chat-send');
        if (await page.locator('#chat.open').count()) {
          throw new Error('the chat stayed open');
        }
        return 'the Chat button opened the input, Send sent the line, Bob read it, and Send on an empty line closed the chat';
      },
      alice,
    );

    const cb: Check = (name, ok, detail) => {
      check(`${other} touch ${name}`, ok, detail);
    };
    await step(
      cb,
      'joystick and panel',
      async () => {
        const p = bob.page;
        await tapOn(p, g, '#hp');
        const before = await selfCell(p);
        const [x, y] = await middle(p, '#joystick');
        await g.down(5, x, y);
        for (let i = 1; i <= 5; i++) {
          await g.move(5, x, y - 9 * i);
          await p.waitForTimeout(120);
        }
        await g.up(5);
        await p.waitForTimeout(800);
        const north = await selfCell(p);
        await g.down(5, x, y);
        for (let i = 1; i <= 5; i++) {
          await g.move(5, x, y + 9 * i);
          await p.waitForTimeout(120);
        }
        await g.up(5);
        await p.waitForTimeout(800);
        const back = await selfCell(p);
        if (north[1] <= before[1] || back[1] >= north[1]) {
          throw new Error(`from ${before} north to ${north} and back to ${back}`);
        }
        await openWindow(p, g, '#inventory-toggle');
        const box = await p.locator('#inventory-window').boundingBox();
        const width = bobOptions.viewport.width;
        await openWindow(p, g, '#inventory-toggle');
        if (box === null || Math.abs(box.width - width) > 1) {
          throw new Error(`the inventory is ${box?.width} px wide on a ${width} px screen`);
        }
        return `the joystick walked north from ${before} to ${north} and back to ${back}; the inventory opened as a ${box.width} px panel`;
      },
      bob,
    );

    await step(
      c,
      'trade',
      async () => {
        // Bob's page must have been touched to show touch controls too.
        await tapOn(bob.page, g, '#hp');
        await page.waitForTimeout(500);
        // The trade opens the inventory again, under the trade window.
        if (await page.locator('#inventory-window').isVisible()) {
          await openWindow(page, f, '#inventory-toggle');
        }
        await approach(page, f, (u) => u.name === 'Bob');
        // Trades need the players within a few cells: a tap beside Bob
        // walks there.
        const bob0 = (await units(page)).find((u) => u.name === 'Bob')?.cell;
        const me0 = await selfCell(page);
        if (bob0 !== undefined && Math.hypot(bob0[0] - me0[0], bob0[1] - me0[1]) > 2) {
          const side = await hook<[number, number] | undefined>(
            page,
            'screenOfCell',
            bob0[0] + Math.sign(me0[0] - bob0[0]),
            bob0[1] + Math.sign(me0[1] - bob0[1]),
          );
          if (side !== undefined) {
            await f.tap(side[0], side[1]);
            await walkedTo(
              page,
              (cell) => Math.hypot(bob0[0] - cell[0], bob0[1] - cell[1]) <= 2,
            ).catch(() => undefined);
          }
        }
        // A long press on Bob opens his menu.
        const at = await unitScreen(page, (u) => u.name === 'Bob');
        await f.press(at[0], at[1], 800);
        await page.locator('#player-menu button:has-text("Trade")').waitFor({ timeout: 10_000 });
        await tapOn(page, f, '#player-menu button:has-text("Trade")');
        await tapOn(bob.page, g, '.social-prompt .prompt-accept');
        await page.locator('#trade-window').waitFor({ state: 'visible', timeout: 10_000 });
        await bob.page.locator('#trade-window').waitFor({ state: 'visible', timeout: 10_000 });
        const guard = '#inventory-window li:has-text("2101")';
        const from = await middle(page, `${guard} .name`);
        const to = await middle(page, '#trade-window .trade-panes');
        await f.longDrag(from, to);
        await page
          .locator('#trade-window .trade-items li:has-text("2101")')
          .first()
          .waitFor({ timeout: 10_000 });
        await tapOn(page, f, '#trade-window .trade-lock');
        await tapOn(bob.page, g, '#trade-window .trade-lock');
        // Trade is enabled once the server says both offers are locked.
        await page.locator('#trade-window .trade-confirm:enabled').waitFor({ timeout: 10_000 });
        await bob.page.locator('#trade-window .trade-confirm:enabled').waitFor({ timeout: 10_000 });
        await tapOn(page, f, '#trade-window .trade-confirm');
        await tapOn(bob.page, g, '#trade-window .trade-confirm');
        await page.locator('#trade-window').waitFor({ state: 'hidden', timeout: 10_000 });
        // The trade opened Bob's inventory too. He had a Guard of his own.
        if (!(await bob.page.locator('#inventory-window').isVisible())) {
          await openWindow(bob.page, g, '#inventory-toggle');
        }
        await bob.page
          .locator('#inventory-window li:has-text("2101")')
          .nth(1)
          .waitFor({ timeout: 10_000 });
        return 'a long press on Bob opened his menu; Trade, Accept on the Pixel, a long-press drag of Item 2101 into the trade window, OK and Trade on both moved it to Bob';
      },
      alice,
    );
    await step(
      c,
      'pinch and twist',
      async () => {
        const d0 = await hook<number>(page, 'distance');
        const a0 = await hook<number>(page, 'azimuth');
        const w = aliceOptions.viewport.width / 2;
        const h = aliceOptions.viewport.height / 2;
        // Apart: zoom in.
        await f.down(1, w - 40, h);
        await f.down(2, w + 40, h);
        for (let i = 1; i <= 8; i++) {
          await f.move(1, w - 40 - i * 10, h);
          await f.move(2, w + 40 + i * 10, h);
        }
        await f.up(2);
        await f.up(1);
        await page.waitForTimeout(500);
        const d1 = await hook<number>(page, 'distance');
        // A twist: turn the camera.
        await f.down(1, w - 60, h);
        await f.down(2, w + 60, h);
        for (let i = 1; i <= 8; i++) {
          const t = (i / 8) * (Math.PI / 4);
          await f.move(1, w - 60 * Math.cos(t), h - 60 * Math.sin(t));
          await f.move(2, w + 60 * Math.cos(t), h + 60 * Math.sin(t));
        }
        await f.up(2);
        await f.up(1);
        await page.waitForTimeout(500);
        const a1 = await hook<number>(page, 'azimuth');
        if (!(d1 < d0 - 1) || Math.abs(a1 - a0) < 0.2) {
          throw new Error(
            `distance ${d0.toFixed(1)} to ${d1.toFixed(1)}, turn ${a0.toFixed(2)} to ${a1.toFixed(2)}`,
          );
        }
        return `distance ${d0.toFixed(1)} to ${d1.toFixed(1)}, turn ${a0.toFixed(2)} to ${a1.toFixed(2)} rad`;
      },
      alice,
    );

    const bobErrors = bob.errors.splice(0);
    c('second phone errors', bobErrors.length === 0, bobErrors.join('; ') || 'none');
  } finally {
    await alice.context.close();
    await bob.context.close();
  }
}

/** The tablet: its layout and walking. */
export async function checkTablet(
  browser: Browser,
  base: string,
  check: Check,
  device = 'iPad (gen 7)',
): Promise<void> {
  const d = devices[device];
  if (d === undefined) {
    throw new Error(`unknown device ${device}`);
  }
  const { defaultBrowserType: _t, ...options } = d;
  const tablet = await open(browser, options, gameURL(base, 'Cid'));
  const page = tablet.page;
  const f = await Fingers.on(page);
  const c: Check = (name, ok, detail) => {
    check(`${device} touch ${name}`, ok, detail);
  };
  try {
    await step(
      c,
      'layout',
      async () => {
        await tapOn(page, f, '#hp');
        await tapOn(page, f, '#inventory-toggle');
        const box = await page.locator('#inventory-window').boundingBox();
        const width = options.viewport.width;
        const menu = await page.locator('#menu-toggle').isVisible();
        if (box === null || box.width >= width - 24 || menu) {
          throw new Error(`inventory ${box?.width} px of ${width}, menu button shown ${menu}`);
        }
        await tapOn(page, f, '#inventory-toggle');
        return `the inventory is a ${box.width} px window on a ${width} px screen, the window buttons show without a menu`;
      },
      tablet,
    );
    await step(
      c,
      'tap and joystick',
      async () => {
        const before = await selfCell(page);
        const me = await unitScreen(page, (u) => u.self);
        await f.tap(me[0] - 120, me[1]);
        const tapped = await walkedTo(page, (cell) => cell[0] < before[0]);
        await settle(page);
        const [x, y] = await middle(page, '#joystick');
        await f.down(3, x, y);
        for (let i = 1; i <= 8; i++) {
          await f.move(3, x, y - 7 * i);
          await page.waitForTimeout(150);
        }
        await f.up(3);
        await page.waitForTimeout(800);
        const pushed = await selfCell(page);
        if (tapped[0] >= before[0] || pushed[1] <= tapped[1]) {
          throw new Error(`from ${before} to ${tapped} to ${pushed}`);
        }
        return `a tap walked west from ${before} to ${tapped}, the joystick north to ${pushed}`;
      },
      tablet,
    );
  } finally {
    await tablet.context.close();
  }
}

/** A phone held sideways: the controls fit, windows are not panels, and walking works. */
export async function checkLandscape(
  browser: Browser,
  base: string,
  check: Check,
  device = 'iPhone 13 landscape',
): Promise<void> {
  const d = devices[device];
  if (d === undefined) {
    throw new Error(`unknown device ${device}`);
  }
  const { defaultBrowserType: _l, ...options } = d;
  const phone = await open(browser, options, gameURL(base, 'Fay'));
  const page = phone.page;
  const f = await Fingers.on(page);
  try {
    await step(
      (name, ok, detail) => {
        check(`${device} touch ${name}`, ok, detail);
      },
      'layout and joystick',
      async () => {
        await tapOn(page, f, '#hp');
        const { width, height } = options.viewport;
        for (const sel of ['#joystick', '#hotkeys', '#chat-button']) {
          const box = await page.locator(sel).boundingBox();
          if (
            box === null ||
            box.x < 0 ||
            box.y < 0 ||
            box.x + box.width > width ||
            box.y + box.height > height
          ) {
            throw new Error(
              `${sel} is outside the ${width}×${height} screen: ${JSON.stringify(box)}`,
            );
          }
        }
        const joystick = await page.locator('#joystick').boundingBox();
        const hotkeys = await page.locator('#hotkeys').boundingBox();
        if (joystick !== null && hotkeys !== null && joystick.x + joystick.width > hotkeys.x) {
          throw new Error('the joystick and the hotkey bar overlap');
        }
        await tapOn(page, f, '#inventory-toggle');
        const inv = await page.locator('#inventory-window').boundingBox();
        await tapOn(page, f, '#inventory-toggle');
        if (inv === null || inv.width >= width - 24 || inv.y + inv.height > height) {
          throw new Error(`the inventory is ${JSON.stringify(inv)} on a ${width}×${height} screen`);
        }
        const before = await selfCell(page);
        const [x, y] = await middle(page, '#joystick');
        await f.down(4, x, y);
        for (let i = 1; i <= 6; i++) {
          await f.move(4, x + 8 * i, y);
          await page.waitForTimeout(120);
        }
        await f.up(4);
        await page.waitForTimeout(800);
        const after = await selfCell(page);
        if (after[0] <= before[0]) {
          throw new Error(`the joystick walked from ${before} to ${after}`);
        }
        return `${width}×${height}: the joystick, hotkey bar and chat button fit without overlap, the inventory is a ${inv.width} px window that fits the height, and the joystick walked east from ${before} to ${after}`;
      },
      phone,
    );
  } finally {
    await phone.context.close();
  }
}

/** A desktop page with a mouse and keyboard: what worked before still does. */
export async function checkMouse(browser: Browser, base: string, check: Check): Promise<void> {
  const desk = await open(
    browser,
    { viewport: { width: 1280, height: 800 } },
    gameURL(base, 'Dan'),
  );
  // Another player, for the right-click menu.
  const eve = await open(browser, { viewport: { width: 800, height: 600 } }, gameURL(base, 'Eve'));
  const page = desk.page;
  const c: Check = (name, ok, detail) => {
    check(`desktop mouse ${name}`, ok, detail);
  };
  try {
    await step(
      c,
      'no touch controls',
      async () => {
        await page.mouse.move(640, 400);
        const touch = await page.evaluate(() => document.body.classList.contains('touch'));
        const joystick = await page.locator('#joystick').isVisible();
        const menu = await page.locator('#menu-toggle').isVisible();
        if (touch || joystick || menu) {
          throw new Error(`touch ${touch}, joystick ${joystick}, menu ${menu}`);
        }
        return 'no touch class, joystick or menu button';
      },
      desk,
    );
    await step(
      c,
      'click to walk',
      async () => {
        const before = await selfCell(page);
        const me = await unitScreen(page, (u) => u.self);
        await page.mouse.click(me[0] + 120, me[1]);
        const after = await walkedTo(page, (cell) => cell[0] > before[0]);
        return `walked east from ${before} to ${after}`;
      },
      desk,
    );
    await step(
      c,
      'drag and wheel turn and zoom',
      async () => {
        const a0 = await hook<number>(page, 'azimuth');
        const d0 = await hook<number>(page, 'distance');
        await page.mouse.move(640, 300);
        await page.mouse.down();
        await page.mouse.move(760, 300, { steps: 8 });
        await page.mouse.up();
        await page.mouse.wheel(0, 300);
        await page.waitForTimeout(600);
        const a1 = await hook<number>(page, 'azimuth');
        const d1 = await hook<number>(page, 'distance');
        if (Math.abs(a1 - a0) < 0.1 || d1 <= d0) {
          throw new Error(`turn ${a0} to ${a1}, distance ${d0} to ${d1}`);
        }
        return `a left drag turned ${a0.toFixed(2)} to ${a1.toFixed(2)} rad, the wheel zoomed out ${d0.toFixed(1)} to ${d1.toFixed(1)}`;
      },
      desk,
    );
    await step(
      c,
      'skill drag and double click',
      async () => {
        await page.keyboard.press('Alt+KeyS');
        const row = '#skill-window li:has-text("Bash")';
        await page.click(`${row} button.learn`);
        await page.locator(`${row}[draggable="true"]`).waitFor({ timeout: 10_000 });
        await page.dragAndDrop(row, '#hotkeys .hotkey >> nth=2');
        const keys = await hook<({ skillId: number } | undefined)[]>(page, 'hotkeys');
        if (keys[2]?.skillId !== 5) {
          throw new Error(`F3 holds ${JSON.stringify(keys[2])}`);
        }
        await page.keyboard.press('Alt+KeyS');
        await page.keyboard.press('Alt+KeyE');
        const knife = '#inventory-window li:has-text("1201")';
        await page.dblclick(`${knife} .name`);
        await page.locator(knife).waitFor({ state: 'detached', timeout: 10_000 });
        return 'Alt+S, a mouse drag of Bash onto F3, Alt+E, and a double click equipped Item 1201';
      },
      desk,
    );
    await step(
      c,
      'right-click menu and window move',
      async () => {
        const at = await unitScreen(page, (u) => u.player && !u.self);
        await page.mouse.click(at[0], at[1], { button: 'right' });
        await page.locator('#player-menu button:has-text("Whisper")').waitFor({ timeout: 5000 });
        await page.keyboard.press('Escape');
        const before = await page.locator('#inventory-window').boundingBox();
        const [x, y] = await middle(page, '#inventory-window .window-bar');
        await page.mouse.move(x, y);
        await page.mouse.down();
        await page.mouse.move(x - 200, y - 100, { steps: 5 });
        await page.mouse.up();
        const after = await page.locator('#inventory-window').boundingBox();
        if (before === null || after === null || after.x > before.x - 150) {
          throw new Error(`from ${before?.x} to ${after?.x}`);
        }
        return 'a right click on another player opened the menu; the inventory moved by its title bar';
      },
      desk,
    );
  } finally {
    await desk.context.close();
    await eve.context.close();
  }
}

/**
 * Frame times at phone size with the CPU slowed down, as Chrome's
 * DevTools do, for the 30 FPS budget (strategy section 3.8).
 */
export async function frameTimes(
  browser: Browser,
  url: string,
  rate: number,
  ready: (page: Page) => Promise<void>,
): Promise<{ fps: number; p50: number; p95: number; frames: number }> {
  const d = devices['Pixel 7'];
  const { defaultBrowserType: _p, ...options } = d;
  const context = await browser.newContext(options);
  const page = await context.newPage();
  try {
    await page.goto(url);
    await ready(page);
    const cdp = await context.newCDPSession(page);
    await cdp.send('Emulation.setCPUThrottlingRate', { rate });
    await page.waitForTimeout(1000);
    const times = await page.evaluate(
      async () =>
        new Promise<number[]>((resolve) => {
          const out: number[] = [];
          let last = performance.now();
          const end = last + 10_000;
          const frame = (t: number): void => {
            out.push(t - last);
            last = t;
            if (t < end) {
              requestAnimationFrame(frame);
            } else {
              resolve(out);
            }
          };
          requestAnimationFrame(frame);
        }),
    );
    await cdp.send('Emulation.setCPUThrottlingRate', { rate: 1 });
    const sorted = times.slice(1).sort((a, b) => a - b);
    const at = (q: number): number =>
      sorted[Math.min(sorted.length - 1, Math.floor(q * sorted.length))] ?? 0;
    const total = sorted.reduce((a, b) => a + b, 0);
    return {
      fps: (sorted.length * 1000) / total,
      p50: at(0.5),
      p95: at(0.95),
      frames: sorted.length,
    };
  } finally {
    await context.close();
  }
}
