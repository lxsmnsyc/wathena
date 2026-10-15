// The phase 6 exit check (review decision 40, decision 0035): three
// players on a desktop and a phone profile play every tier 1 system of
// strategy section 11.3 against a real waserver with PostgreSQL, a staff
// account with TOTP finds a player, reads its item history and mutes it,
// and a sample mod's item, monster, skill and job show their own names
// and art.
//
// It runs two servers on one database, one after the other:
//
// 1. A --dev server with the game role, where names are session tokens.
//    Ann plays on a desktop with the mouse, Bob on a Pixel 7 with touch,
//    and Cid on a smaller desktop window. They start as Merchants at base
//    level 50 and job level 40.
// 2. A server with accounts, which serves the account pages. Dora signs
//    up on a phone, makes a character, plays, makes a guild and uploads
//    its emblem on the account page. Admin, the owner, turns on TOTP,
//    logs in with a code, finds Dora on /admin, reads the history of one
//    of her items and mutes her, and Dora's chat shows the mute.
//
// The database comes from WATHENA_TEST_DATABASE_URL, a role that can make
// databases. tools/testdb makes one for the run and drops it at the end.

import { createHash, createHmac, randomBytes } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import { deflateSync } from 'node:zlib';
import { type Browser, type BrowserContext, type Page, devices } from 'playwright-core';
import {
  type Check,
  Fingers,
  type Game,
  type UnitView,
  hook,
  inGame,
  middle,
  selfCell,
  sleep,
  startGame,
  units,
} from './game-check.ts';

/** What the check needs from browser-check.ts. */
export interface ExitContext {
  repo: string;
  client: string;
  dir: string;
  wactl: string;
  build: string;
}

/** A player in a browser: a page with the mouse, or with fingers. */
class Player {
  readonly errors: string[] = [];
  /** URLs the page fetched, for the art checks. */
  readonly fetched: string[] = [];
  readonly name: string;
  readonly context: BrowserContext;
  readonly page: Page;
  readonly fingers: Fingers | undefined;

  private constructor(
    name: string,
    context: BrowserContext,
    page: Page,
    fingers: Fingers | undefined,
  ) {
    this.name = name;
    this.context = context;
    this.page = page;
    this.fingers = fingers;
    page.on('pageerror', (e) => this.errors.push(e.message));
    page.on('response', (r) => {
      if (r.ok()) {
        this.fetched.push(r.url());
      }
    });
  }

  static async open(
    browser: Browser,
    name: string,
    touch: boolean,
    url: string,
    wait = true,
  ): Promise<Player> {
    const { defaultBrowserType: _d, ...phone } = devices['Pixel 7'];
    let options: Parameters<Browser['newContext']>[0] = { viewport: { width: 1280, height: 800 } };
    if (touch) {
      // One device pixel per CSS pixel: SwiftShader draws on the CPU, and
      // three players at the Pixel 7's 2.625 would slow every step. The
      // touch check plays at the full ratio.
      options = { ...phone, deviceScaleFactor: 1 };
    } else if (name === 'Cid') {
      options = { viewport: { width: 1024, height: 700 } };
    }
    const context = await browser.newContext(options);
    const page = await context.newPage();
    const fingers = touch ? await Fingers.on(page) : undefined;
    const p = new Player(name, context, page, fingers);
    await page.goto(url);
    if (wait) {
      await inGame(page);
      await p.wake();
    }
    return p;
  }

  get touch(): boolean {
    return this.fingers !== undefined;
  }

  /** The first input picks touch or mouse mode, as on a real device. */
  async wake(): Promise<void> {
    if (this.fingers === undefined) {
      await this.page.mouse.move(10, 300);
      return;
    }
    const [x, y] = await middle(this.page, '#hp');
    await this.fingers.tap(x, y);
  }

  /** Taps or clicks an element. */
  async press(selector: string): Promise<void> {
    try {
      const el = this.page.locator(selector).first();
      await el.waitFor({ state: 'visible', timeout: 10_000 });
      if (this.fingers === undefined) {
        await el.click({ timeout: 10_000 });
        return;
      }
      // A panel's title bar stays over the top of its content, so the
      // element goes to the middle of its panel first.
      await el.evaluate((n) => {
        n.scrollIntoView({ block: 'center' });
      });
      const [x, y] = await middle(this.page, selector);
      await this.fingers.tap(x, y);
    } catch (e) {
      throw new Error(
        `${this.name} pressing ${selector}: ${e instanceof Error ? e.message.split('\n')[0] : String(e)}`,
        { cause: e },
      );
    }
  }

  /** Opens or closes a window by its button, through the Menu on a small screen. */
  async toggle(button: string): Promise<void> {
    for (let i = 0; i < 2 && !(await this.page.locator(button).isVisible()); i++) {
      await this.press('#menu-toggle');
      await this.page
        .locator(button)
        .waitFor({ state: 'visible', timeout: 2000 })
        .catch(() => undefined);
    }
    await this.press(button);
  }

  /** Makes sure a window is open. */
  async show(window: string, button: string): Promise<void> {
    if (!(await this.page.locator(window).isVisible())) {
      await this.toggle(button);
    }
    await this.page.locator(window).waitFor({ state: 'visible', timeout: 10_000 });
  }

  /** Makes sure a window is closed. */
  async hide(window: string, button: string): Promise<void> {
    if (await this.page.locator(window).isVisible()) {
      await this.toggle(button);
    }
  }

  /** Types a line into the chat and sends it. */
  async say(text: string): Promise<void> {
    if (this.fingers === undefined) {
      await this.page.fill('#chat-input', text);
      await this.page.press('#chat-input', 'Enter');
      return;
    }
    if (!(await this.page.locator('#chat.open').count())) {
      await this.press('#chat-button');
    }
    await this.page.fill('#chat-input', text);
    await this.press('#chat-send');
  }

  /** Waits for a line in the chat log. */
  async hears(text: string, timeout = 15_000): Promise<void> {
    await this.page.waitForFunction(
      (t) => document.querySelector('#chat-log')?.textContent.includes(t) === true,
      text,
      { timeout },
    );
  }

  /** The units the page shows. */
  async units(): Promise<UnitView[]> {
    return units(this.page);
  }

  /** The screen point of a unit, or undefined when it is not shown. */
  async screenOf(test: (u: UnitView) => boolean): Promise<[number, number] | undefined> {
    const u = (await this.units()).find(test);
    if (u === undefined) {
      return undefined;
    }
    return hook<[number, number] | undefined>(this.page, 'screenOf', u.id);
  }

  /** Whether a point of the page shows the game view, not a window over it. */
  async onView(at: [number, number]): Promise<boolean> {
    const size = this.page.viewportSize() ?? { width: 800, height: 600 };
    if (at[0] < 20 || at[1] < 20 || at[0] > size.width - 20 || at[1] > size.height - 20) {
      return false;
    }
    return this.page.evaluate(
      ([x, y]) => document.elementFromPoint(x, y)?.tagName === 'CANVAS',
      at,
    );
  }

  /** Whether a screen point is clear of every unit but the player. */
  async clearOfUnits(at: [number, number], radius = 40): Promise<boolean> {
    for (const u of await this.units()) {
      if (u.self) {
        continue;
      }
      const o = await hook<[number, number] | undefined>(this.page, 'screenOf', u.id);
      if (o !== undefined && Math.hypot(o[0] - at[0], o[1] - at[1]) < radius) {
        return false;
      }
    }
    return true;
  }

  /**
   * Walks to a cell: a click on the ground, or a tap. A cell off the
   * view, under a window or under a unit is reached in shorter walks, or
   * by a cell beside it.
   */
  async walkTo(x: number, y: number): Promise<void> {
    for (let leg = 0; leg < 5; leg++) {
      const me = await selfCell(this.page);
      if (Math.abs(me[0] - x) <= 1 && Math.abs(me[1] - y) <= 1) {
        return;
      }
      let target: { cell: [number, number]; at: [number, number] } | undefined;
      let tx = x;
      let ty = y;
      for (let i = 0; i < 5 && target === undefined; i++) {
        for (const [ox, oy] of [
          [0, 0],
          [1, 0],
          [0, 1],
          [-1, 0],
          [0, -1],
        ] as const) {
          const cell: [number, number] = [tx + ox, ty + oy];
          const at = await hook<[number, number] | undefined>(this.page, 'screenOfCell', ...cell);
          if (at !== undefined && (await this.onView(at)) && (await this.clearOfUnits(at))) {
            target = { cell, at };
            break;
          }
        }
        tx = Math.round((tx + me[0]) / 2);
        ty = Math.round((ty + me[1]) / 2);
      }
      if (target === undefined) {
        throw new Error(`no clear cell toward ${x},${y} is on the view`);
      }
      await this.click(target.at[0], target.at[1]);
      await this.page
        .waitForFunction(
          ([cx, cy]) => {
            const v: unknown = Reflect.get(globalThis, 'viewer');
            const f: unknown =
              typeof v === 'object' && v !== null ? Reflect.get(v, 'selfCell') : undefined;
            const c: unknown = typeof f === 'function' ? Reflect.apply(f, v, []) : undefined;
            return (
              Array.isArray(c) &&
              Math.abs(Number(c[0]) - cx) <= 1 &&
              Math.abs(Number(c[1]) - cy) <= 1
            );
          },
          target.cell,
          { timeout: 6000 },
        )
        .catch(() => undefined);
    }
    const me = await selfCell(this.page);
    if (Math.abs(me[0] - x) > 1 || Math.abs(me[1] - y) > 1) {
      throw new Error(`walked to ${me}, not ${x},${y}`);
    }
  }

  async click(x: number, y: number): Promise<void> {
    if (this.fingers === undefined) {
      await this.page.mouse.click(x, y);
    } else {
      await this.fingers.tap(x, y);
    }
  }

  /**
   * Clicks a unit: an NPC to talk, a monster to attack. A unit off the
   * view, under a window or under another unit is walked to first.
   */
  async clickUnit(test: (u: UnitView) => boolean, near = 2): Promise<void> {
    const at = await this.reach(test, near);
    await this.click(at[0], at[1]);
  }

  /** Walks until a unit can be clicked, and returns its screen point. */
  async reach(test: (u: UnitView) => boolean, near = 2): Promise<[number, number]> {
    const clear = async (p: [number, number] | undefined): Promise<boolean> => {
      if (p === undefined || !(await this.onView(p))) {
        return false;
      }
      for (const u of await this.units()) {
        if (test(u) || u.self) {
          continue;
        }
        const o = await hook<[number, number] | undefined>(this.page, 'screenOf', u.id);
        if (o !== undefined && Math.hypot(o[0] - p[0], o[1] - p[1]) < 45) {
          return false;
        }
      }
      return true;
    };
    let at = await this.screenOf(test);
    // Units walk, so the walk goes again while the unit is off the view
    // or under a window or another unit.
    // The first walk goes to the near side of the unit. On a phone the
    // controls cover the bottom of the view, so later walks try each
    // side, which puts the unit elsewhere on the screen.
    for (let i = 0; i < 5 && !(await clear(at)); i++) {
      const u = (await this.units()).find(test);
      if (u?.cell === undefined) {
        throw new Error('the unit is not in sight');
      }
      const me = await selfCell(this.page);
      const sides: [number, number][] = [
        [Math.sign(me[0] - u.cell[0]), Math.sign(me[1] - u.cell[1]) || 1],
        [0, -1],
        [0, 1],
        [1, 0],
        [-1, 0],
      ];
      const [dx, dy] = sides[i] ?? [0, 1];
      await this.walkTo(u.cell[0] + dx * near, u.cell[1] + dy * near).catch(() => undefined);
      await sleep(300);
      at = await this.screenOf(test);
    }
    if (at === undefined || !(await this.onView(at))) {
      throw new Error('the unit is not on the view');
    }
    return at;
  }

  /**
   * Stops an attack or a walk with a step onto a clear cell beside the
   * player, as a click on the ground does.
   */
  async stopAction(): Promise<void> {
    const me = await selfCell(this.page);
    for (const [dx, dy] of [
      [1, 0],
      [0, 1],
      [-1, 0],
      [0, -1],
    ] as const) {
      const at = await hook<[number, number] | undefined>(
        this.page,
        'screenOfCell',
        me[0] + dx,
        me[1] + dy,
      );
      if (at !== undefined && (await this.onView(at)) && (await this.clearOfUnits(at))) {
        await this.click(at[0], at[1]);
        await sleep(600);
        return;
      }
    }
  }

  /**
   * Opens the player menu of another player: a right click, or a long
   * press. A press that misses a player who moves goes again.
   */
  async menuOf(name: string): Promise<void> {
    for (let i = 0; ; i++) {
      const at = await this.screenOf((u) => u.name === name);
      if (at === undefined) {
        throw new Error(`${name} is not shown`);
      }
      if (this.fingers === undefined) {
        await this.page.mouse.click(at[0], at[1], { button: 'right' });
      } else {
        await this.fingers.press(at[0], at[1], 800);
      }
      try {
        await this.page.locator('#player-menu').waitFor({ state: 'visible', timeout: 4000 });
        return;
      } catch (e) {
        if (i >= 2) {
          throw e;
        }
      }
    }
  }

  /** Accepts the prompt whose key starts with a prefix, such as party. */
  async accept(prefix: string): Promise<void> {
    await this.press(`#social-prompts .social-prompt[data-prompt^="${prefix}"] .prompt-accept`);
  }

  /**
   * Closes the dialog, the shop and every window, so a step starts from
   * the game view whatever the step before left open.
   */
  async reset(): Promise<void> {
    const page = this.page;
    for (let i = 0; i < 6 && (await page.locator('#dialog-window').isVisible()); i++) {
      const b = page.locator('#dialog-window .dialog-controls button:visible');
      if ((await b.count()) > 0) {
        await this.press('#dialog-window .dialog-controls button:visible').catch(() => undefined);
      } else {
        await page.keyboard.press('Escape');
      }
      await sleep(200);
    }
    for (const [sel, close] of [
      ['#shop-window', '#shop-window button:has-text("Close")'],
      ['#storage-window', '#storage-window .storage-close'],
      ['#cart-window', '#cart-window .storage-close'],
      ['#store-search', '#store-search .store-head button:has-text("Close")'],
      ['#vending-browse', '#vending-browse .store-head button'],
    ] as const) {
      if (await page.locator(sel).isVisible()) {
        // A DOM click: other windows may cover the button.
        await page
          .locator(close)
          .first()
          .evaluate((b) => {
            if (b instanceof HTMLElement) {
              b.click();
            }
          });
      }
    }
    for (const [sel, toggle] of [
      ['#inventory-window', '#inventory-toggle'],
      ['#equipment-window', '#equipment-toggle'],
      ['#skill-window', '#skill-toggle'],
      ['#party-window', '#party-toggle'],
      ['#booking-window', '#booking-toggle'],
      ['#guild-window', '#guild-toggle'],
      ['#mail-window', '#mail-toggle'],
      ['#friend-panel', '#friend-toggle'],
    ] as const) {
      await this.hide(sel, toggle);
    }
  }

  async close(): Promise<void> {
    await this.context.close();
  }
}

/** A step of the check: its run returns the detail, and a throw fails it. */
type Run = () => Promise<string>;

/**
 * Runs the exit check. It reports each step as a check, and the time of
 * the whole run.
 */
export async function checkExit(
  name: string,
  browser: Browser,
  ctx: ExitContext,
  check: (browser: string, name: string, ok: boolean, detail: string) => void,
): Promise<void> {
  const started = Date.now();
  const url = process.env.WATHENA_TEST_DATABASE_URL;
  if (url === undefined || url === '') {
    check(name, 'exit database', false, 'set WATHENA_TEST_DATABASE_URL to a PostgreSQL 18 server');
    return;
  }
  const testdb = join(ctx.dir, 'testdb');
  execFileSync('go', ['build', '-o', testdb, './tools/testdb'], {
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
  const c: Check = (step, ok, detail) => {
    check(name, `exit ${step}`, ok, detail);
  };
  try {
    const password = `Exit-${randomBytes(9).toString('base64url')}`;
    execFileSync(ctx.wactl, ['world', 'new', '--admin', 'Admin', '--admin-password-file', '-'], {
      input: `${password}\n`,
      env: { ...process.env, WATHENA_MIGRATE_DATABASE_URL: dbURL() },
      stdio: ['pipe', 'ignore', 'inherit'],
    });
    await playPhaseOne(browser, ctx, dbURL('wathena_game'), c);
    await playPhaseTwo(browser, ctx, dbURL, password, c);
  } finally {
    execFileSync(testdb, ['drop', db], { stdio: 'inherit' });
    console.log(`info ${name} exit check took ${((Date.now() - started) / 1000).toFixed(0)} s`);
  }
}

/** Runs one step: a check named after it, and the page errors of the players it names. */
async function play(c: Check, label: string, players: Player[], run: Run): Promise<boolean> {
  // EXIT_STEPS, a regular expression, runs only the steps it matches,
  // for work on the check.
  const only = process.env.EXIT_STEPS;
  if (only !== undefined && !new RegExp(only).test(label)) {
    return true;
  }
  const began = Date.now();
  let ok = false;
  let detail: string;
  try {
    for (const p of players) {
      await p.reset();
    }
    detail = await run();
    const errs = players.flatMap((p) => p.errors.splice(0).map((e) => `${p.name}: ${e}`));
    ok = errs.length === 0;
    if (!ok) {
      detail = `page errors: ${errs.join('; ')}`;
    }
  } catch (e) {
    detail = e instanceof Error ? (e.message.split('\n')[0] ?? '') : String(e);
    // Where in this file it failed, for the person who reads the report.
    const at = (e instanceof Error ? (e.stack ?? '') : '')
      .split('\n')
      .find((l) => l.includes('exit-check.ts'))
      ?.match(/exit-check\.ts:(\d+)/)?.[1];
    if (at !== undefined) {
      detail += ` (exit-check.ts line ${at})`;
    }
  }
  c(label, ok, `${detail} (${((Date.now() - began) / 1000).toFixed(1)} s)`);
  const shots = process.env.SCREENSHOTS;
  if (!ok && shots !== undefined) {
    for (const p of players) {
      await p.page
        .screenshot({ path: join(shots, `exit-${label.replaceAll(/\W+/g, '-')}-${p.name}.png`) })
        .catch(() => undefined);
    }
  }
  return ok;
}

/** The game page of a --dev character. */
function devURL(base: string, who: string): string {
  const ws = `${base.replace(/^http/, 'ws')}/ws`;
  return `${base}/?server=${encodeURIComponent(ws)}&name=${who}&nosw`;
}

/** Phase 1: the --dev server with the database. */
async function playPhaseOne(
  browser: Browser,
  ctx: ExitContext,
  gameDB: string,
  c: Check,
): Promise<void> {
  const game = await startGame(ctx.repo, ctx.client, ctx.dir, ctx.wactl, ctx.build, {
    name: 'exit-dev',
    startJob: '5:50:40',
    startItems: '501:10,909:30,714:1,1201:1,2101:1,512:5',
    mods: ['mods/examples/lantern'],
    env: { WATHENA_DATABASE_URL: gameDB },
  });
  const players: Player[] = [];
  try {
    const ann = await Player.open(browser, 'Ann', false, devURL(game.base, 'Ann'));
    const bob = await Player.open(browser, 'Bob', true, devURL(game.base, 'Bob'));
    const cid = await Player.open(browser, 'Cid', false, devURL(game.base, 'Cid'));
    players.push(ann, bob, cid);
    await phaseOneSteps(c, game, ann, bob, cid);
  } finally {
    for (const p of players) {
      await p.close();
    }
    await game.stop();
    c(
      'dev server log',
      game.errors.length === 0,
      game.errors.slice(0, 3).join('; ') || 'no ERROR lines',
    );
  }
}

/** Phase 2: the server with accounts. */
async function playPhaseTwo(
  browser: Browser,
  ctx: ExitContext,
  dbURL: (role?: string) => string,
  password: string,
  c: Check,
): Promise<void> {
  const game = await startGame(ctx.repo, ctx.client, ctx.dir, ctx.wactl, ctx.build, {
    name: 'exit-accounts',
    dev: false,
    startItems: '909:5,714:1',
    mods: ['mods/examples/lantern'],
    config: 'staff:\n  owners: [Admin]\n',
    env: {
      WATHENA_DATABASE_URL: dbURL('wathena_game'),
      WATHENA_ACCOUNT_DATABASE_URL: dbURL('wathena_account'),
      WATHENA_ACCOUNT_SECRET_KEY: randomBytes(32).toString('base64'),
    },
  });
  const players: Player[] = [];
  try {
    await phaseTwoSteps(c, game, browser, password, players);
  } finally {
    for (const p of players) {
      await p.close();
    }
    await game.stop();
    c(
      'accounts server log',
      game.errors.length === 0,
      game.errors.slice(0, 3).join('; ') || 'no ERROR lines',
    );
  }
}

/** A ground item as the page shows it. */
interface Ground {
  id: number;
  itemId: number;
  name: string;
  screen: [number, number];
}

async function ground(p: Player): Promise<Ground[]> {
  return hook<Ground[]>(p.page, 'groundItems');
}

/** The text of an element, or ''. */
async function textOf(p: Player, selector: string): Promise<string> {
  return (await p.page.locator(selector).first().textContent()) ?? '';
}

/** Waits until an element holds a text. */
async function waitText(
  p: Player,
  selector: string,
  want: string,
  timeout = 15_000,
): Promise<void> {
  await p.page
    .locator(selector)
    .filter({ hasText: want })
    .first()
    .waitFor({ state: 'attached', timeout });
}

/** The HUD's level line, such as "Merchant: Base Lv 50 (0.0%), Job Lv 40 (0.0%)". */
async function levels(p: Player): Promise<string> {
  return textOf(p, '#exp');
}

/** Clicks an NPC and waits for its dialog or shop. */
async function talk(p: Player, npc: string, opens = '#dialog-window'): Promise<void> {
  // A Lantern Ghost that walks over the NPC may take the click, which
  // starts an attack: a step stops it, and the click goes again.
  for (let i = 0; ; i++) {
    await p.clickUnit((u) => u.npc && u.name === npc);
    try {
      await p.page.locator(opens).waitFor({ state: 'visible', timeout: 6000 });
      return;
    } catch (e) {
      if (i >= 2) {
        throw e;
      }
      await p.stopAction();
    }
  }
}

/** Picks a menu option of the dialog by its whole text. */
async function choose(p: Player, option: string): Promise<void> {
  const buttons = p.page.locator('#dialog-window .dialog-menu button');
  await buttons.first().waitFor({ timeout: 15_000 });
  const texts = (await buttons.allTextContents()).map((t) => t.trim());
  const i = texts.indexOf(option);
  if (i < 0) {
    throw new Error(`the dialog offers ${texts.join(', ')}, not ${option}`);
  }
  await p.press(`#dialog-window .dialog-menu button >> nth=${i}`);
}

/** Presses Next and Close until the dialog ends. */
async function endDialog(p: Player): Promise<void> {
  for (let i = 0; i < 8 && (await p.page.locator('#dialog-window').isVisible()); i++) {
    const b = p.page.locator('#dialog-window .dialog-controls button:visible');
    if ((await b.count()) > 0) {
      // The dialog may end while the button is pressed.
      await p.press('#dialog-window .dialog-controls button:visible').catch(() => undefined);
    }
    await sleep(300);
  }
}

/** Attacks a monster until it dies, and returns where it stood. */
async function kill(p: Player, monster: string): Promise<[number, number]> {
  // The nearest one that no other unit stands next to, so the click
  // finds it.
  let target: UnitView | undefined;
  for (let i = 0; i < 20 && target === undefined; i++) {
    const list = await p.units();
    const me = await selfCell(p.page);
    const d = (u: UnitView): number => Math.hypot(u.cell![0] - me[0], u.cell![1] - me[1]);
    target = list
      .filter((u) => u.name === monster && !u.npc && !u.player && u.cell !== undefined)
      .filter((u) =>
        list.every(
          (o) =>
            o.id === u.id ||
            o.self ||
            o.cell === undefined ||
            Math.hypot(o.cell[0] - u.cell![0], o.cell[1] - u.cell![1]) >= 1.5,
        ),
      )
      .sort((a, b) => d(a) - d(b))[0];
    if (target === undefined) {
      await sleep(500);
    }
  }
  if (target?.cell === undefined) {
    throw new Error(`no ${monster} stands clear in sight`);
  }
  const gone = async (ms: number): Promise<boolean> =>
    p.page
      .waitForFunction(
        (id) => {
          const v: unknown = Reflect.get(globalThis, 'viewer');
          const f: unknown =
            typeof v === 'object' && v !== null ? Reflect.get(v, 'units') : undefined;
          const l: unknown = typeof f === 'function' ? Reflect.apply(f, v, []) : [];
          return Array.isArray(l) && !l.some((u: unknown) => Reflect.get(Object(u), 'id') === id);
        },
        target.id,
        { timeout: ms },
      )
      .then(() => true)
      .catch(() => false);
  // Another unit may take the click: a step stops that attack, and the
  // click goes again.
  for (let i = 0; i < 3; i++) {
    if (await gone(10)) {
      break;
    }
    await p
      .clickUnit((u) => u.id === target.id)
      .catch(async (e: unknown) => {
        // It may have died meanwhile.
        if (!(await gone(10))) {
          throw e;
        }
      });
    if (await gone(8000)) {
      break;
    }
    if (i === 2) {
      throw new Error(`the ${monster} did not die`);
    }
    await p.stopAction();
  }
  return target.cell;
}

/**
 * Kills monsters until one leaves a drop that a test finds, up to three,
 * since a click can land on a Lantern Ghost that walks past.
 */
async function killForDrop(
  p: Player,
  monster: string,
  test: (g: Ground) => boolean,
): Promise<[number, number]> {
  for (let i = 0; ; i++) {
    const at = await kill(p, monster);
    for (let j = 0; j < 16; j++) {
      if ((await ground(p)).some(test)) {
        return at;
      }
      await sleep(250);
    }
    if (i >= 2) {
      throw new Error(`no ${monster} left a drop`);
    }
  }
}

/**
 * Casts a skill on a unit: reaches it, starts the skill's targeting, and
 * clicks or taps the unit where it is now, until damage shows. A unit
 * that walks may miss the click, which goes again from closer.
 */
async function castOn(
  p: Player,
  test: (u: UnitView) => boolean,
  target: () => Promise<void>,
): Promise<void> {
  for (let i = 0; ; i++) {
    await p.reach(test, 2);
    await target();
    // Earlier numbers fade and go, so new ones are told apart by a mark
    // on the old ones, not by a count.
    await p.page.evaluate(() => {
      for (const h of document.querySelectorAll<HTMLElement>('#hits .hit')) {
        h.dataset.seen = '1';
      }
    });
    const at = await p.screenOf(test);
    if (at !== undefined) {
      await p.click(at[0], at[1]);
    }
    const hit = await p.page
      .waitForFunction(() => document.querySelector('#hits .hit:not([data-seen])') !== null, null, {
        timeout: 6000,
      })
      .then(() => true)
      .catch(() => false);
    if (hit) {
      return;
    }
    if (i === 2) {
      throw new Error(
        `no damage showed; the chat ends: ${(await textOf(p, '#chat-log')).slice(-120)}`,
      );
    }
    if ((await hook<string | undefined>(p.page, 'targeting')) !== undefined) {
      await p.page.keyboard.press('Escape');
    }
    await p.stopAction();
  }
}

/** Picks up a ground item by a test, and waits for it to go. */
async function pickUp(p: Player, test: (g: Ground) => boolean): Promise<Ground> {
  let g: Ground | undefined;
  for (let i = 0; i < 40 && g === undefined; i++) {
    g = (await ground(p)).find(test);
    if (g === undefined) {
      await sleep(250);
    }
  }
  if (g === undefined) {
    const seen = (await ground(p)).map((x) => `${x.name}#${x.id}`).join(', ');
    throw new Error(`the item is not on the ground, which holds: ${seen || 'nothing'}`);
  }
  const id = g.id;
  // A Dummy comes back on its cell a second after it dies, and may take
  // the tap: a step stops that attack, and the tap goes again where the
  // item is now on the screen.
  for (let i = 0; ; i++) {
    const now = (await ground(p)).find((x) => x.id === id) ?? g;
    await p.click(now.screen[0], now.screen[1]);
    const taken = await p.page
      .waitForFunction(
        (gid) => {
          const v: unknown = Reflect.get(globalThis, 'viewer');
          const f: unknown =
            typeof v === 'object' && v !== null ? Reflect.get(v, 'groundItems') : undefined;
          const l: unknown = typeof f === 'function' ? Reflect.apply(f, v, []) : [];
          return Array.isArray(l) && !l.some((x: unknown) => Reflect.get(Object(x), 'id') === gid);
        },
        id,
        { timeout: 6000 },
      )
      .then(() => true)
      .catch(() => false);
    if (taken) {
      break;
    }
    if (i === 2) {
      throw new Error(`${g.name} stayed on the ground`);
    }
    await p.stopAction();
  }
  return g;
}

/** An inventory row by its text. */
function inv(item: string): string {
  return `#inventory-window li:has-text("${item}")`;
}

/** Buys amounts of items from the open shop window, by row text. */
async function buy(p: Player, items: Record<string, number>): Promise<void> {
  for (const [item, amount] of Object.entries(items)) {
    await p.page.fill(`#shop-window .shop-buy tr:has-text("${item}") input`, String(amount));
  }
  await p.press('#shop-window button:has-text("Buy")');
}

/**
 * The file of an asset in the lantern mod's build, as the client finds
 * it: the manifest names the shards, and the shard of a name, by the
 * first byte of its SHA-256, holds the file's hash.
 */
async function artFile(base: string, name: string): Promise<string> {
  const data: unknown = await (await fetch(`${base}/client-data.json`)).json();
  const mods: unknown = typeof data === 'object' && data !== null ? Reflect.get(data, 'mods') : [];
  const list: unknown[] = Array.isArray(mods) ? mods : [];
  const build: unknown = list.find((m) => Reflect.get(Object(m), 'id') === 'example.lantern');
  const root = `${base}${String(Reflect.get(Object(build), 'assets'))}`;
  const manifest: unknown = await (await fetch(`${root}manifest.json`)).json();
  const shards: unknown = Reflect.get(Object(manifest), 'shards');
  const index = createHash('sha256').update(name).digest()[0] ?? 0;
  const shard = Array.isArray(shards) ? String(shards[index]) : '';
  const entries: unknown = await (await fetch(`${root}${shard.slice(0, 2)}/${shard}.json`)).json();
  const hash: unknown = Reflect.get(Object(Reflect.get(Object(entries), name)), 'h');
  if (typeof hash !== 'string') {
    throw new Error(`the mod's build has no ${name}`);
  }
  return hash;
}

/** Waits until a player's page fetched an asset of the mod's build. */
async function fetchedArt(p: Player, base: string, name: string): Promise<void> {
  const hash = await artFile(base, name);
  for (let i = 0; i < 40; i++) {
    if (p.fetched.some((u) => u.includes('/assets/mods/example.lantern/') && u.includes(hash))) {
      return;
    }
    await sleep(250);
  }
  throw new Error(`the page did not load ${name}`);
}

/**
 * Counts the pixels near a unit that are close to a color, in a
 * screenshot of the page, for the art checks.
 */
async function pixelsNear(
  p: Player,
  at: [number, number],
  rgb: [number, number, number],
  box = 70,
): Promise<number> {
  // at is a cell above the feet: the clip holds the body, and what
  // stands on it or above the head.
  const png = await p.page.screenshot({
    clip: {
      x: Math.max(0, at[0] - box / 2),
      y: Math.max(0, at[1] - box * 1.6),
      width: box,
      height: box * 2.6,
    },
  });
  return p.page.evaluate(
    async ([data, color]) => {
      const img = new Image();
      img.src = `data:image/png;base64,${data}`;
      await img.decode();
      const c = document.createElement('canvas');
      c.width = img.width;
      c.height = img.height;
      const g = c.getContext('2d');
      if (g === null) {
        return 0;
      }
      g.drawImage(img, 0, 0);
      const d = g.getImageData(0, 0, c.width, c.height).data;
      let n = 0;
      for (let i = 0; i < d.length; i += 4) {
        if (
          Math.abs(d[i]! - color[0]) < 40 &&
          Math.abs(d[i + 1]! - color[1]) < 40 &&
          Math.abs(d[i + 2]! - color[2]) < 40
        ) {
          n++;
        }
      }
      return n;
    },
    [png.toString('base64'), rgb] as const,
  );
}

/** Phase 1: the basic game, every tier 1 system, and the mod run. */
async function phaseOneSteps(
  c: Check,
  game: Game,
  ann: Player,
  bob: Player,
  cid: Player,
): Promise<void> {
  const all = [ann, bob, cid];

  // The basic game, on the desktop with the mouse.
  await play(c, 'desktop walk', [ann], async () => {
    const before = await selfCell(ann.page);
    await ann.walkTo(before[0] + 3, before[1] - 2);
    return `Ann clicked the ground and walked from ${before} to ${await selfCell(ann.page)}`;
  });
  await play(c, 'desktop fight and loot', [ann], async () => {
    const jellopy = await count(ann, '909');
    await ann.hide('#inventory-window', '#inventory-toggle');
    const at = await killForDrop(ann, 'Dummy', (g) => g.name === 'Test Charm');
    // The two drops lie close, so a click may take the other one first.
    for (const name of ['Test Charm', 'Item 909']) {
      if ((await ground(ann)).some((g) => g.name === name)) {
        await pickUp(ann, (g) => g.name === name);
      }
    }
    await waitText(ann, '#chat-log', 'You got Test Charm');
    const now = await count(ann, '909');
    await waitText(ann, '#inventory-window li', 'Test Charm');
    if (now <= jellopy) {
      throw new Error(`Jellopy went from ${jellopy} to ${now}`);
    }
    return `Ann killed a Dummy at ${at} by clicks, and picked up its Jellopy (${jellopy} became ${now}) and its Test Charm, which the inventory lists by its mod name`;
  });
  await play(c, 'desktop level up', [ann], async () => {
    const before = await levels(ann);
    // The Training Dummy appears next to Ann, away from the Dummies.
    await ann.walkTo(15, 9);
    await ann.say('!trainer');
    await ann.hears('e2e trainer: true');
    await kill(ann, 'Training Dummy');
    await ann.page.waitForFunction(
      () => document.querySelector('#exp')?.textContent.includes('Base Lv 51') === true,
      null,
      { timeout: 15_000 },
    );
    return `"${before}" became "${await levels(ann)}" after a Training Dummy`;
  });
  await play(c, 'desktop NPC talk and zeny', [ann], async () => {
    await ann.hide('#inventory-window', '#inventory-toggle');
    await talk(ann, 'Quartermaster');
    await choose(ann, 'Give');
    await waitText(ann, '#dialog-window', 'Here you are');
    await endDialog(ann);
    return 'Ann chose Give at the Quartermaster and read "Here you are."';
  });
  await play(c, 'desktop equip', [ann], async () => {
    await ann.show('#inventory-window', '#inventory-toggle');
    const knife = inv('1201');
    await ann.page.dblclick(`${knife} .name`);
    await ann.page.locator(knife).waitFor({ state: 'detached', timeout: 10_000 });
    await ann.show('#equipment-window', '#equipment-toggle');
    const worn = await textOf(ann, '#equipment-window');
    await ann.hide('#equipment-window', '#equipment-toggle');
    if (!worn.includes('1201')) {
      throw new Error('the equipment window does not show the knife');
    }
    return 'a double click equipped Item 1201, which the equipment window shows';
  });
  await play(c, 'desktop skill', [ann], async () => {
    await ann.show('#skill-window', '#skill-toggle');
    const row = '#skill-window li:has(.name:text-is("Mammonite"))';
    await ann.press(`${row} button.learn`);
    await ann.page.locator(`${row}[draggable="true"]`).waitFor({ timeout: 10_000 });
    await ann.hide('#skill-window', '#skill-toggle');
    const dummy = (u: UnitView): boolean => u.name === 'Dummy' && !u.npc;
    await castOn(ann, dummy, async () => {
      await ann.show('#skill-window', '#skill-toggle');
      await ann.page.dblclick(`${row} .name`);
      await ann.hide('#skill-window', '#skill-toggle');
    });
    return 'Ann learned Mammonite, used it from the skill window, and its damage showed on a Dummy';
  });

  // The basic game on the phone, with touch.
  await play(c, 'phone walk, fight and loot', [bob], async () => {
    const before = await selfCell(bob.page);
    await bob.walkTo(before[0] - 2, before[1] + 2);
    await killForDrop(bob, 'Dummy', (g) => g.itemId === 909);
    await pickUp(bob, (g) => g.itemId === 909);
    return `Bob tapped the ground from ${before} to the Dummies, tapped one until it died, and tapped its Jellopy`;
  });
  await play(c, 'phone NPC, equip and skill', [bob], async () => {
    await talk(bob, 'Quartermaster');
    await choose(bob, 'Give');
    await waitText(bob, '#dialog-window', 'Here you are');
    await endDialog(bob);
    await bob.show('#inventory-window', '#inventory-toggle');
    const knife = inv('1201');
    // Two taps within 450 ms are a double tap. A busy machine may stretch
    // them, so the taps go again while the knife stays.
    for (let i = 0; i < 3 && (await bob.page.locator(knife).count()) > 0; i++) {
      const [x, y] = await middle(bob.page, `${knife} .name`);
      await bob.fingers!.quickDoubleTap(x, y);
      await bob.page
        .locator(knife)
        .waitFor({ state: 'detached', timeout: 4000 })
        .catch(() => undefined);
    }
    await bob.page.locator(knife).waitFor({ state: 'detached', timeout: 1000 });
    await bob.hide('#inventory-window', '#inventory-toggle');
    await bob.show('#skill-window', '#skill-toggle');
    const row = '#skill-window li:has(.name:text-is("Mammonite"))';
    await bob.press(`${row} button.learn`);
    await bob.page.locator(`${row}[draggable="true"]`).waitFor({ timeout: 10_000 });
    const from = await middle(bob.page, `${row} .skill-text`);
    const slot = await middle(bob.page, '#hotkeys .hotkey >> nth=0');
    await bob.fingers!.longDrag(from, slot);
    await bob.hide('#skill-window', '#skill-toggle');
    const dummy = (u: UnitView): boolean => u.name === 'Dummy' && !u.npc;
    await castOn(bob, dummy, async () => {
      await bob.press('#hotkeys .hotkey >> nth=0');
    });
    return 'Bob tapped Give at the Quartermaster, double-tapped the knife on, dragged Mammonite to F1, and cast it on a Dummy';
  });

  await socialSteps(c, ann, bob, cid);
  await economySteps(c, ann, bob, cid);
  await modSteps(c, game.base, ann);
  const left = all.flatMap((p) => p.errors.splice(0).map((e) => `${p.name}: ${e}`));
  c('phase 1 page errors', left.length === 0, left.join('; ') || 'none');
}

/** How many of an item a player's inventory lists, by row text. */
async function count(p: Player, item: string): Promise<number> {
  await p.show('#inventory-window', '#inventory-toggle');
  const rows = p.page.locator(`${inv(item)} .amount`);
  let n = 0;
  for (const t of await rows.allTextContents()) {
    n += Number(t.replace(/\D/g, '')) || 0;
  }
  return n;
}

/** Puts an item through an open storage or cart, by the inventory's right click. */
async function putAway(p: Player, window: string, item: string, amount: number): Promise<void> {
  await p.show('#inventory-window', '#inventory-toggle');
  const row = `${inv(item)} .name`;
  if (p.fingers === undefined) {
    await p.page.click(row, { button: 'right' });
  } else {
    const [x, y] = await middle(p.page, row);
    await p.fingers.press(x, y, 700);
  }
  const form = `${window} .storage-amount`;
  if (await p.page.locator(form).isVisible()) {
    await p.page.fill(`${form} input`, String(amount));
    await p.press(`${form} button[type="submit"]`);
  }
  await waitText(p, `${window} li`, item);
}

/** Takes an item out of an open storage or cart. */
async function takeOut(p: Player, window: string, item: string, amount: number): Promise<void> {
  await p.press(`${window} li:has-text("${item}") button:has-text("Take")`);
  const form = `${window} .storage-amount`;
  await p.page.locator(form).waitFor({ state: 'visible', timeout: 5000 });
  await p.page.fill(`${form} input`, String(amount));
  await p.press(`${form} button[type="submit"]`);
}

/** Chat, friends, party, booking and guild. */
async function socialSteps(c: Check, ann: Player, bob: Player, cid: Player): Promise<void> {
  const all = [ann, bob, cid];
  await play(c, 'chat public and whisper', all, async () => {
    await ann.say('Hello, lantern festival!');
    await bob.hears('Hello, lantern festival!');
    await cid.hears('Hello, lantern festival!');
    await ann.say('/w Bob a secret for Bob');
    await bob.hears('a secret for Bob');
    await bob.say('/w Ann got it');
    await ann.hears('got it');
    return 'Bob and Cid heard Ann in local chat; Ann and Bob whispered both ways, Bob from the phone';
  });
  await play(c, 'chat channel', all, async () => {
    await bob.say('/join #trade');
    await cid.say('/join #trade');
    await sleep(500);
    await cid.say('#trade buying lanterns');
    await bob.hears('buying lanterns');
    return "Bob and Cid joined #trade, and Bob read Cid's line there";
  });
  await play(c, 'friends', [ann, bob], async () => {
    await ann.press('#friend-toggle');
    await ann.page.fill('#friend-panel .friend-name', 'Bob');
    await ann.press('#friend-panel .friend-add');
    await bob.accept('friend');
    await waitText(ann, '#friend-panel', 'Bob');
    await ann.press('#friend-toggle');
    return "Ann added Bob in the friend panel, Bob accepted on the phone, and Ann's list shows Bob";
  });
  await play(c, 'party and party chat', [ann, bob], async () => {
    await bob.show('#party-window', '#party-toggle');
    await bob.page.fill('#party-window .party-create-name', 'Lanterns');
    await bob.press('#party-window .party-create button[type="submit"]');
    await bob.page.locator('#party-window .party-invite-name').waitFor({ timeout: 10_000 });
    await bob.page.fill('#party-window .party-invite-name', 'Ann');
    await bob.press('#party-window .party-invite button[type="submit"]');
    await ann.accept('party');
    await waitText(bob, '#party-window', 'Ann');
    await bob.hide('#party-window', '#party-toggle');
    await ann.say('%party line from Ann');
    await bob.hears('party line from Ann');
    return "Bob made the party Lanterns on the phone and invited Ann, who accepted; Bob read Ann's party line";
  });
  await play(c, 'party booking', [bob, cid], async () => {
    await cid.show('#booking-window', '#booking-toggle');
    await cid.page.fill('#booking-window .booking-ad-level', '50');
    await cid.page.fill('#booking-window .booking-ad-note', 'Merchant looking for a party');
    await cid.press('#booking-window .booking-register');
    await cid.page
      .locator('#booking-window .booking-own')
      .waitFor({ state: 'visible', timeout: 10_000 });
    await cid.hide('#booking-window', '#booking-toggle');
    await bob.show('#booking-window', '#booking-toggle');
    await bob.page.fill('#booking-window .booking-search-level', '50');
    await bob.press('#booking-window .booking-search-go');
    const row = '#booking-window li[data-owner="Cid"]';
    await bob.page.locator(row).waitFor({ timeout: 10_000 });
    await bob.press(`${row} .booking-invite`);
    await cid.accept('party');
    await bob.hide('#booking-window', '#booking-toggle');
    await bob.show('#party-window', '#party-toggle');
    await waitText(bob, '#party-window', 'Cid');
    await bob.hide('#party-window', '#party-toggle');
    return 'Cid placed an ad, Bob found it by level on the phone and invited Cid from it, and Cid joined the party';
  });
  await play(c, 'guild, guild chat and guild skills', [ann, cid], async () => {
    await ann.show('#guild-window', '#guild-toggle');
    await ann.page.fill('#guild-window .guild-create-name', 'Lamplight');
    await ann.press('#guild-window .guild-create button[type="submit"]');
    await ann.press('#guild-window .guild-tab[data-guild-tab="members"]');
    await ann.page.locator('#guild-window .guild-invite-name').waitFor({ timeout: 10_000 });
    await ann.page.fill('#guild-window .guild-invite-name', 'Cid');
    await ann.press('#guild-window .guild-invite button[type="submit"]');
    await cid.accept('guild');
    await waitText(ann, '#guild-window .guild-members', 'Cid');
    await ann.say('$guild line from Ann');
    await cid.hears('guild line from Ann');
    // Renewal's Guild Storage needs 20 skill points: guild level 21.
    await ann.say('!guild level 21');
    await ann.press('#guild-window .guild-tab[data-guild-tab="skills"]');
    await waitText(ann, '#guild-window .guild-skill-points', 'Skill points: 20');
    const raise = [10000, 10002, 10004, 10004, 10004, 10004, 10004, 10010, 10011, 10013];
    raise.push(10004, 10004, 10004, 10004, 10004, 10007, 10008, 10006, 10009, 10016);
    for (const [i, id] of raise.entries()) {
      await ann.press(`#guild-window li[data-skill="${id}"] .guild-skill-up:enabled`);
      await waitText(ann, '#guild-window .guild-skill-points', `Skill points: ${19 - i}`);
    }
    await ann.hide('#guild-window', '#guild-toggle');
    return 'Ann made the guild Lamplight with an Emperium and invited Cid; Cid read the guild line; at guild level 21 Ann raised 20 guild skills up to Guild Storage Expansion with "+"';
  });
  await play(c, 'guild storage', [ann], async () => {
    await talk(ann, 'Kafra');
    await choose(ann, 'Guild storage');
    await ann.page.locator('#storage-window').waitFor({ state: 'visible', timeout: 10_000 });
    await putAway(ann, '#storage-window', '909', 3);
    await ann.press('#storage-window .storage-close');
    return 'the Kafra opened the guild storage, and Ann stored 3 Item 909 with a right click';
  });
}

/** Trade, vending with store search, mail, NPC shops, storage and the cart. */
async function economySteps(c: Check, ann: Player, bob: Player, cid: Player): Promise<void> {
  await play(c, 'trade', [ann, bob], async () => {
    await ann.hide('#inventory-window', '#inventory-toggle');
    const at = (await bob.units()).find((u) => u.self)?.cell;
    if (at === undefined) {
      throw new Error('Bob has no cell');
    }
    await ann.walkTo(at[0] + 1, at[1]);
    await ann.menuOf('Bob');
    await ann.press('#player-menu button:has-text("Trade")');
    await bob.accept('trade');
    await ann.page.locator('#trade-window').waitFor({ state: 'visible', timeout: 10_000 });
    await ann.show('#inventory-window', '#inventory-toggle');
    await ann.page.click(`${inv('2101')} .name`, { button: 'right' });
    await waitText(ann, '#trade-window .trade-items', '2101');
    await ann.page.fill('#trade-window .trade-zeny-form input', '100');
    await ann.press('#trade-window .trade-zeny-form button');
    await ann.press('#trade-window .trade-lock');
    await bob.press('#trade-window .trade-lock');
    await sleep(500);
    await ann.press('#trade-window .trade-confirm');
    await bob.press('#trade-window .trade-confirm');
    await bob.page.waitForFunction(
      () => /e2e trade: .*1 items/.test(document.querySelector('#chat-log')?.textContent ?? ''),
      null,
      { timeout: 15_000 },
    );
    return 'Ann right-clicked Bob for Trade, Bob accepted on the phone, Ann offered Item 2101 and 100 zeny, both pressed OK and Trade, and the trade event confirmed it';
  });
  await play(c, 'NPC shops of every cost kind', [ann], async () => {
    await ann.hide('#inventory-window', '#inventory-toggle');
    const potions = await count(ann, '501');
    await ann.hide('#inventory-window', '#inventory-toggle');
    await talk(ann, 'Tool Dealer', '#shop-window');
    await buy(ann, { '501': 1 });
    await ann.page
      .locator('#shop-window')
      .waitFor({ state: 'hidden', timeout: 10_000 })
      .catch(() => undefined);
    await ann.say('!currency e2e.fixture:tokens 10');
    await ann.hears('e2e currency: 10');
    await talk(ann, 'Exchange', '#shop-window');
    await buy(ann, { '512': 1, '501': 1, '909': 1 });
    await ann.page
      .waitForFunction(() => !document.querySelector('#shop-window')?.checkVisibility(), null, {
        timeout: 10_000,
      })
      .catch(() => undefined);
    const after = await count(ann, '501');
    if (after !== potions + 2) {
      throw new Error(`Red Potions went from ${potions} to ${after}`);
    }
    return `Ann bought a Red Potion for zeny at the Tool Dealer, and at the Exchange an Apple for zeny, Jellopy and a token, a Red Potion for tokens and a Jellopy for a Jellopy; Red Potions went from ${potions} to ${after}`;
  });
  await play(c, 'storage', [ann], async () => {
    await ann.hide('#inventory-window', '#inventory-toggle');
    await talk(ann, 'Kafra');
    await choose(ann, 'Storage');
    await ann.page.locator('#storage-window').waitFor({ state: 'visible', timeout: 10_000 });
    const before = await count(ann, '909');
    await putAway(ann, '#storage-window', '909', 2);
    await takeOut(ann, '#storage-window', '909', 1);
    await ann.page.waitForFunction(
      (n) =>
        [...document.querySelectorAll('#inventory-window li')].some(
          (li) => li.textContent.includes('909') && li.textContent.includes(`× ${n}`),
        ),
      before - 1,
      { timeout: 10_000 },
    );
    await ann.press('#storage-window .storage-close');
    return `Ann stored 2 Item 909 at the Kafra and took 1 back: ${before} became ${before - 1}`;
  });
  await play(c, 'cart and vending setup', [cid], async () => {
    await talk(cid, 'Quartermaster');
    await choose(cid, 'Give');
    await endDialog(cid);
    await talk(cid, 'Lamplighter', '#shop-window');
    await buy(cid, { 'Paper Lantern': 2 });
    await cid.show('#skill-window', '#skill-toggle');
    // Pushcart needs Enlarge Weight Limit 5, and Vending Pushcart 3.
    const learn = ['Enlarge Weight Limit', 'Enlarge Weight Limit', 'Enlarge Weight Limit'];
    learn.push(
      'Enlarge Weight Limit',
      'Enlarge Weight Limit',
      'Pushcart',
      'Pushcart',
      'Pushcart',
      'Vending',
    );
    for (const skill of learn) {
      await cid.press(`#skill-window li:has(.name:text-is("${skill}")) button.learn:enabled`);
      await sleep(400);
    }
    await cid.say('!cart');
    await cid.hears('e2e cart: true');
    await cid.hide('#skill-window', '#skill-toggle');
    // Vending needs a cell more than 3 cells from every NPC.
    await cid.walkTo(9, 12);
    await cid.show('#cart-window', '#cart-toggle');
    await putAway(cid, '#cart-window', 'Paper Lantern', 2);
    await takeOut(cid, '#cart-window', 'Paper Lantern', 1);
    await waitText(cid, '#inventory-window li', 'Paper Lantern');
    await cid.show('#skill-window', '#skill-toggle');
    await cid.page.dblclick('#skill-window li:has(.name:text-is("Vending")) .name');
    await cid.page.locator('#vending-setup').waitFor({ state: 'visible', timeout: 10_000 });
    await cid.hide('#skill-window', '#skill-toggle');
    await cid.hide('#cart-window', '#cart-toggle');
    await cid.hide('#inventory-window', '#inventory-toggle');
    await cid.page.fill('#vending-setup input[placeholder="Shop title"]', 'Lantern sale');
    const row = '#vending-setup li:has-text("Paper Lantern")';
    await cid.page.fill(`${row} input[placeholder="0"]`, '1');
    await cid.page.fill(`${row} input[placeholder="Price"]`, '300');
    await cid.press('#vending-setup button:has-text("Open")');
    await cid.hears('Your shop is open.');
    await cid.hide('#skill-window', '#skill-toggle');
    return 'Cid bought 2 Paper Lanterns at the Lamplighter, learned Pushcart and Vending, got a cart, moved both lanterns in and one back, and opened the shop "Lantern sale" with the other';
  });
  await play(c, 'store search and vending', [bob, cid], async () => {
    await bob.say('!zeny 1000');
    await bob.hears('e2e zeny: true');
    await bob.say('!search 1 open');
    await bob.page.locator('#store-search').waitFor({ state: 'visible', timeout: 10_000 });
    await bob.page.fill('#store-search input[placeholder^="Item names"]', 'Paper Lantern');
    await bob.press('#store-search .search-form button[type="submit"]');
    const hit = '#store-search .search-results li:has-text("Lantern sale")';
    await bob.page.locator(hit).waitFor({ timeout: 10_000 });
    await bob.press(hit);
    await bob.page.locator('#vending-browse').waitFor({ state: 'visible', timeout: 10_000 });
    await bob.page.fill('#vending-browse li:has-text("Paper Lantern") input', '1');
    await bob.press('#vending-browse button:has-text("Buy")');
    await bob
      .hears('e2e vend: Bob bought 1 items from Cid for 300 zeny')
      .catch(async (e: unknown) => {
        const said = `${await textOf(bob, '#message')} | ${await textOf(bob, '#vending-browse')}`;
        throw new Error(`no sale: ${said}`, { cause: e });
      });
    await cid.hears('e2e vend: Bob bought 1 items from Cid');
    await bob.press('#store-search .store-head button:has-text("Close")');
    await bob.page
      .locator('#store-search')
      .isVisible()
      .then(async (v) => {
        if (v) {
          await bob.press('#store-search .store-head button:has-text("Close")');
        }
      });
    return 'Bob searched the stores for "Paper Lantern" on the phone, opened Cid\'s shop from the result and bought the lantern for 300 zeny';
  });
  await play(c, 'mail', [ann, bob], async () => {
    await ann.hide('#inventory-window', '#inventory-toggle');
    // A mail with items costs a fee.
    await ann.say('!zeny 10000');
    await ann.hears('e2e zeny: true');
    await ann.show('#mail-window', '#mail-toggle');
    await ann.press('#mail-window .mail-tabs button:has-text("Write")');
    await ann.page.fill('#mail-window input[placeholder="Character name"]', 'Bob');
    await ann.page.fill('#mail-window input[placeholder="Title"]', 'Lantern oil money');
    await ann.page.fill('#mail-window textarea', 'For the festival.');
    await ann.page.fill('#mail-window .mail-write input[placeholder="0"]', '50');
    const value = await ann.page.$eval('#mail-window select', (sel) => {
      if (!(sel instanceof HTMLSelectElement)) {
        return '';
      }
      return [...sel.options].find((x) => x.text.includes('909'))?.value ?? '';
    });
    await ann.page.selectOption('#mail-window select', value);
    await ann.page.fill('#mail-window .mail-attach input', '2');
    await ann.press('#mail-window .mail-attach button:has-text("Attach")');
    await waitText(ann, '#mail-window .mail-attachments', '909');
    await ann.press('#mail-window .mail-write button[type="submit"]');
    await ann.hears('The mail was sent.');
    await ann.hide('#mail-window', '#mail-toggle');
    await bob.hide('#inventory-window', '#inventory-toggle');
    const before = await count(bob, '909');
    await bob.hide('#inventory-window', '#inventory-toggle');
    await bob.show('#mail-window', '#mail-toggle');
    const row = '#mail-window .mail-row:has-text("Lantern oil money")';
    await bob.page.locator(row).waitFor({ timeout: 15_000 });
    await bob.press(row);
    await bob.press('#mail-window .mail-buttons button:has-text("Take all")');
    await sleep(800);
    await bob.hide('#mail-window', '#mail-toggle');
    const after = await count(bob, '909');
    if (after !== before + 2) {
      throw new Error(`Bob's Jellopy went from ${before} to ${after}`);
    }
    return `Ann wrote Bob a mail with 50 zeny and 2 Item 909, and Bob took on the phone all from the inbox: ${before} became ${after}`;
  });
}

/**
 * Waits until a unit is in sight. Ghosts that other players killed by a
 * stray tap come back after a while, and the player waits in the middle
 * of the map, where it sees all of it.
 */
async function inSight(p: Player, test: (u: UnitView) => boolean): Promise<void> {
  for (let i = 0; i < 60; i++) {
    if ((await p.units()).some(test)) {
      return;
    }
    if (i === 4) {
      await p.walkTo(12, 12).catch(() => undefined);
    }
    await sleep(500);
  }
  throw new Error('the unit did not come in sight in 30 s');
}

/** Where the player stands on the screen. */
async function selfScreen(p: Player): Promise<[number, number]> {
  const at = await p.screenOf((u) => u.self);
  if (at === undefined) {
    throw new Error('the player is not shown');
  }
  return at;
}

/** The mod run: the lantern mod's item, monster, skill and job (review decision 40). */
async function modSteps(c: Check, base: string, ann: Player): Promise<void> {
  const ghost = (u: UnitView): boolean => u.name === 'Lantern Ghost' && !u.npc && !u.player;
  await play(c, 'mod item names and icons', [ann], async () => {
    await ann.hide('#inventory-window', '#inventory-toggle');
    await ann.hide('#mail-window', '#mail-toggle');
    await talk(ann, 'Lamplighter', '#shop-window');
    const shop = await textOf(ann, '#shop-window .shop-buy');
    for (const name of ['Paper Lantern', 'Lantern Oil', 'Unlit Lantern', 'Lantern Hat']) {
      if (!shop.includes(name)) {
        throw new Error(`the shop does not list ${name}: ${shop}`);
      }
    }
    await ann.page.waitForFunction(
      () =>
        [...document.querySelectorAll<HTMLTableRowElement>('#shop-window .shop-buy tr')].some(
          (tr) =>
            tr.textContent.includes('Paper Lantern') &&
            (tr.querySelector('img')?.naturalWidth ?? 0) > 0,
        ),
      null,
      { timeout: 10_000 },
    );
    await buy(ann, { 'Paper Lantern': 1, 'Lantern Hat': 1 });
    await ann.show('#inventory-window', '#inventory-toggle');
    await waitText(ann, '#inventory-window li', 'Lantern Hat');
    await ann.page.click(`${inv('Paper Lantern')} .name`);
    await waitText(ann, '#inventory-window', 'A lantern of thin paper that Lantern Ghosts carry.');
    await ann.page.waitForFunction(
      () =>
        [...document.querySelectorAll<HTMLLIElement>('#inventory-window li')].some(
          (li) =>
            li.textContent.includes('Lantern Hat') &&
            (li.querySelector('img')?.naturalWidth ?? 0) > 0,
        ),
      null,
      { timeout: 10_000 },
    );
    await fetchedArt(ann, base, 'mod/example.lantern/icon/item/Paper_Lantern');
    await fetchedArt(ann, base, 'mod/example.lantern/icon/item/Lantern_Hat');
    return "the Lamplighter's shop lists Paper Lantern, Lantern Oil, Unlit Lantern and Lantern Hat; the inventory names the bought lantern and hat and shows the lantern's description; both icons loaded from the mod's build";
  });
  await play(c, 'mod worn sprite', [ann], async () => {
    await ann.show('#inventory-window', '#inventory-toggle');
    await ann.page.dblclick(`${inv('Lantern Hat')} .name`);
    await ann.page.locator(inv('Lantern Hat')).waitFor({ state: 'detached', timeout: 10_000 });
    await ann.hide('#inventory-window', '#inventory-toggle');
    await fetchedArt(ann, base, 'mod/example.lantern/sprite/headgear/Lantern_Hat');
    let red = 0;
    for (let i = 0; i < 10 && red < 20; i++) {
      await sleep(500);
      red = await pixelsNear(ann, await selfScreen(ann), [170, 30, 40]);
    }
    if (red < 20) {
      throw new Error(`${red} hat pixels show`);
    }
    return `the Lantern Hat went on with a double click; its worn sprite came from the mod's build, and ${red} pixels of its red show above Ann`;
  });
  await play(c, 'mod monster and drop', [ann], async () => {
    await inSight(ann, ghost);
    // The ghost walks, so the mouse follows it until its name shows.
    let named = false;
    for (let i = 0; i < 10 && !named; i++) {
      await ann.reach(ghost, 3);
      const at = await ann.screenOf(ghost);
      if (at !== undefined) {
        await ann.page.mouse.move(at[0], at[1] - 10);
      }
      named = await waitText(ann, '#item-label:not([hidden])', 'Lantern Ghost', 1500)
        .then(() => true)
        .catch(() => false);
    }
    if (!named) {
      throw new Error('hovering a Lantern Ghost showed no name');
    }
    await kill(ann, 'Lantern Ghost');
    await fetchedArt(ann, base, 'mod/example.lantern/sprite/mob/Lantern_Ghost');
    // Renewal's level penalty makes the low ghost's drops rare, so Ann
    // drops a lantern of her own.
    await ann.show('#inventory-window', '#inventory-toggle');
    await ann.press(`${inv('Paper Lantern')} button:has-text("Drop")`);
    const form = '#inventory-window .drop-form';
    if (await ann.page.locator(form).isVisible()) {
      await ann.page.fill(`${form} input`, '1');
      await ann.press(`${form} button[type="submit"]`);
    }
    await ann.hide('#inventory-window', '#inventory-toggle');
    let drop: Ground | undefined;
    for (let i = 0; i < 20 && drop === undefined; i++) {
      drop = (await ground(ann)).find((g) => g.name === 'Paper Lantern');
      await sleep(250);
    }
    if (drop === undefined) {
      throw new Error('no Paper Lantern lies on the ground');
    }
    await ann.page.mouse.move(drop.screen[0], drop.screen[1]);
    await waitText(ann, '#item-label', 'Paper Lantern');
    await fetchedArt(ann, base, 'mod/example.lantern/sprite/item/Paper_Lantern');
    await pickUp(ann, (g) => g.name === 'Paper Lantern');
    return 'hovering a Lantern Ghost, whose sprite loaded from the mod\'s build, showed "Lantern Ghost", and Ann killed it; a Paper Lantern dropped on the ground showed its name on hover, its dropped sprite loaded from the build, and it went back into the inventory';
  });
  await play(c, 'mod job', [ann], async () => {
    await talk(ann, 'Lantern Master');
    await ann.press('#dialog-window .dialog-controls button:has-text("Next")');
    await choose(ann, 'Yes');
    await waitText(ann, '#dialog-window', 'You are now a Lantern Keeper.');
    await endDialog(ann);
    await waitText(ann, '#exp', 'Lantern Keeper');
    const sex = (await artFile(base, 'mod/example.lantern/sprite/body/Lantern_Keeper/m').then((h) =>
      ann.fetched.some((u) => u.includes(h)),
    ))
      ? 'm'
      : 'f';
    await fetchedArt(ann, base, `mod/example.lantern/sprite/body/Lantern_Keeper/${sex}`);
    let robe = 0;
    for (let i = 0; i < 10 && robe < 50; i++) {
      await sleep(500);
      const at = await selfScreen(ann);
      robe =
        (await pixelsNear(ann, at, [20, 110, 70])) + (await pixelsNear(ann, at, [40, 150, 100]));
    }
    if (robe < 50) {
      throw new Error(`${robe} robe pixels show`);
    }
    return `the Lantern Master changed Ann into a "${(await levels(ann)).split(':')[0]}"; the body sprite came from the mod's build, and ${robe} pixels of the green robe show`;
  });
  await play(c, 'mod skill', [ann], async () => {
    await ann.show('#skill-window', '#skill-toggle');
    // rAthena's player_skillup_limit: a second job learns its own skills
    // once the first job's points are spent, and a new job has none at
    // job level 1. Ann spends the Merchant's points, then a Training
    // Dummy takes her to job level 2.
    // The "+" buttons are clicked in the page, a few a second, since the
    // window draws its rows again after each answer. A click past a
    // skill's last level is refused, and a later one learns another.
    await ann.page.evaluate(async () => {
      const left = (): number =>
        Number(
          /Skill Points: (\d+)/.exec(
            document.querySelector('#skill-window')?.textContent ?? '',
          )?.[1] ?? '0',
        );
      for (let i = 0; i < 200 && left() > 0; i++) {
        document.querySelector<HTMLButtonElement>('#skill-window button.learn:enabled')?.click();
        await new Promise((resolve) => {
          setTimeout(resolve, 150);
        });
      }
    });
    await ann.page.waitForFunction(
      () =>
        (document.querySelector('#skill-window')?.textContent ?? '').includes('Skill Points: 0'),
      null,
      { timeout: 10_000 },
    );
    await ann.hide('#skill-window', '#skill-toggle');
    await ann.walkTo(15, 9);
    await ann.say('!trainer');
    await ann.hears('e2e trainer: true');
    await kill(ann, 'Training Dummy');
    await waitText(ann, '#exp', 'Job Lv 2');
    await ann.show('#skill-window', '#skill-toggle');
    const row = '#skill-window li:has(.name:text-is("Lantern Flare"))';
    await ann.page
      .locator(row)
      .waitFor({ timeout: 10_000 })
      .catch(async (e: unknown) => {
        const names = await ann.page.locator('#skill-window li .name').allTextContents();
        throw new Error(`the skill window lists ${names.join(', ')}`, { cause: e });
      });
    await ann.page.locator(`${row} img.skill-icon`).waitFor({ timeout: 10_000 });
    await ann.page.waitForFunction(
      () =>
        (document.querySelector<HTMLImageElement>('#skill-window img.skill-icon')?.naturalWidth ??
          0) > 0,
      null,
      { timeout: 10_000 },
    );
    await fetchedArt(ann, base, 'mod/example.lantern/icon/skill/LANTERN_FLARE');
    await ann.press(`${row} button.learn`);
    await ann.page.locator(`${row}[draggable="true"]`).waitFor({ timeout: 10_000 });
    await ann.hide('#skill-window', '#skill-toggle');
    await inSight(ann, ghost);
    await castOn(ann, ghost, async () => {
      await ann.show('#skill-window', '#skill-toggle');
      await ann.page.dblclick(`${row} .name`);
      await ann.hide('#skill-window', '#skill-toggle');
    });
    await fetchedArt(ann, base, 'mod/example.lantern/effect/flare');
    return 'the skill window lists "Lantern Flare" with its icon from the mod; Ann learned it and cast it on a Lantern Ghost, and its hit effect came from the mod\'s build';
  });
}

/** The game page of a player with an account, which logs in on the account pages. */
function accountURL(base: string): string {
  const ws = `${base.replace(/^http/, 'ws')}/ws`;
  return `${base}/?server=${encodeURIComponent(ws)}&nosw`;
}

/** The TOTP time step of now. */
function nowStep(): number {
  return Math.floor(Date.now() / 30_000);
}

/** Phase 2: accounts, the guild emblem, and the admin run. */
async function phaseTwoSteps(
  c: Check,
  game: Game,
  browser: Browser,
  password: string,
  players: Player[],
): Promise<void> {
  const client = accountURL(game.base);
  let dora: Player | undefined;
  let admin: Player | undefined;
  const ok = await play(c, 'phone sign up and play', [], async () => {
    dora = await Player.open(
      browser,
      'Dora',
      true,
      `${game.base}/account/signup?return=${encodeURIComponent(client)}`,
      false,
    );
    players.push(dora);
    const page = dora.page;
    await page.fill('input[name="name"]', 'DoraLamp');
    await page.fill('input[name="password"]', 'lantern-festival-2026');
    await page.fill('input[name="confirm"]', 'lantern-festival-2026');
    await page.click('button[type="submit"]');
    await page.locator('form.create').waitFor({ timeout: 15_000 });
    await page.fill('form.create input[name="name"]', 'Dora');
    await page.click('form.create button[type="submit"]');
    await page.locator('.characters button:has-text("Play")').waitFor({ timeout: 15_000 });
    await page.click('.characters button:has-text("Play")');
    await inGame(page);
    await dora.wake();
    return `Dora signed up on the phone's account pages, made the character Dora and pressed Play, which took her into the game: ${await levels(dora)}`;
  });
  if (!ok || dora === undefined) {
    return;
  }
  const d = dora;
  await play(c, 'phone guild and emblem upload', [d], async () => {
    await talk(d, 'Quartermaster');
    await choose(d, 'Give');
    await waitText(d, '#dialog-window', 'Here you are');
    await endDialog(d);
    await d.show('#guild-window', '#guild-toggle');
    await d.page.fill('#guild-window .guild-create-name', 'Lantern Hall');
    await d.press('#guild-window .guild-create button[type="submit"]');
    await d.page
      .locator('#guild-window .guild-emblem')
      .waitFor({ state: 'attached', timeout: 10_000 });
    const page = await d.context.newPage();
    await page.goto(`${game.base}/account/guild/emblem`);
    await page.locator('form.emblem:has-text("Lantern Hall") input[type="file"]').setInputFiles({
      name: 'lantern.png',
      mimeType: 'image/png',
      buffer: emblemPNG(),
    });
    await page.click('form.emblem:has-text("Lantern Hall") button[type="submit"]');
    await page.waitForLoadState();
    const notice = (await page.textContent('main, body')) ?? '';
    await page.close();
    await d.page.waitForFunction(
      () =>
        (document.querySelector<HTMLImageElement>('#guild-window img.guild-emblem')?.naturalWidth ??
          0) === 24,
      null,
      { timeout: 20_000 },
    );
    return `Dora made the guild Lantern Hall on the phone, uploaded a 24 by 24 PNG on the account page ("${notice.replaceAll(/\s+/g, ' ').trim().slice(0, 80)}"), and the guild window shows the emblem`;
  });
  await play(c, 'admin TOTP login', [], async () => {
    admin = await Player.open(browser, 'Admin', false, `${game.base}/account/login`, false);
    players.push(admin);
    const page = admin.page;
    await page.fill('input[name="name"]', 'Admin');
    await page.fill('input[name="password"]', password);
    await page.click('button[type="submit"]');
    await page.goto(`${game.base}/account/settings`);
    await page.click('form[action="/account/settings/totp/start"] button');
    const key = ((await page.textContent('.key code')) ?? '').trim();
    const first = nowStep();
    await page.fill('input[name="code"]', totp(key, first));
    await page.click('form[action="/account/settings/totp"] button');
    await page.waitForLoadState();
    await page.goto(`${game.base}/account/settings`);
    await page.click('form.logout button');
    await page.goto(`${game.base}/account/login`);
    await page.fill('input[name="name"]', 'Admin');
    await page.fill('input[name="password"]', password);
    await page.click('button[type="submit"]');
    await page.locator('form[action="/account/login/code"]').waitFor({ timeout: 10_000 });
    // A code works once, so the login takes the next step's code, which
    // the server's skew of one step accepts.
    await page.fill('input[name="code"]', totp(key, Math.max(first, nowStep()) + 1));
    await page.click('form[action="/account/login/code"] button');
    await page.waitForURL(/\/account\/characters/, { timeout: 10_000 });
    await page.goto(`${game.base}/admin.html`);
    await page.locator('.admin-nav button:has-text("Players")').waitFor({ timeout: 30_000 });
    return 'the owner Admin logged in with a password, turned on TOTP with a code from its key, logged out, logged in again with a password and a code, and /admin showed the staff screens';
  });
  if (admin === undefined) {
    return;
  }
  const a = admin;
  await play(c, 'admin item history and mute', [d], async () => {
    const page = a.page;
    await page.click('.admin-nav button:has-text("Players")');
    await page.fill('.admin-players input[name="q"]', 'Dora');
    await page.click('.admin-players button[type="submit"]');
    await page.click('.admin-players button.admin-link:has-text("Dora")');
    await page.click('.admin-character .admin-tabs button:has-text("Inventory")');
    await page.locator('.admin-character tr:has-text("Jellopy") button').first().click();
    await page.locator('.admin-item .admin-holder').waitFor({ timeout: 10_000 });
    const holder = (await page.textContent('.admin-item .admin-holder')) ?? '';
    const history = (await page.textContent('.admin-item')) ?? '';
    if (!holder.includes('Dora')) {
      throw new Error(`the history shows ${holder}`);
    }
    await page.click('.admin-nav button:has-text("Players")');
    await page.fill('.admin-players input[name="q"]', 'Dora');
    await page.click('.admin-players button[type="submit"]');
    await page.click('.admin-players button.admin-link:has-text("Dora")');
    await page.click('.admin-character .admin-tabs button:has-text("Sanctions")');
    await page.fill('.admin-sanction input[name="minutes"]', '30');
    await page.fill('.admin-sanction input[name="reason"]', 'insults in chat');
    await page.click('.admin-sanction button[data-kind="mute"]');
    await page.locator('text=Dora is muted.').waitFor({ timeout: 10_000 });
    await d.hears('You are muted until');
    await d.hears('insults in chat');
    await d.say('can anyone hear me');
    await d.page.waitForFunction(
      () => (document.querySelector('#chat-log')?.textContent.match(/muted/g) ?? []).length >= 2,
      null,
      { timeout: 10_000 },
    );
    return `Admin found Dora, followed her Jellopy to its history (${history.includes('Quartermaster') || history.includes('npc') ? 'from the NPC, ' : ''}"${holder.trim()}"), and muted her for 30 minutes; Dora's chat showed the mute with its reason and refused her next line`;
  });
}

/** A TOTP code of a base32 key at a time step, as RFC 6238 with SHA-1. */
export function totp(key: string, at: number): string {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
  let bits = '';
  for (const ch of key.replaceAll(/[\s=]/g, '').toUpperCase()) {
    const v = alphabet.indexOf(ch);
    if (v < 0) {
      throw new Error('the key is not base32');
    }
    bits += v.toString(2).padStart(5, '0');
  }
  const bytes = Buffer.alloc(Math.floor(bits.length / 8));
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = Number.parseInt(bits.slice(i * 8, i * 8 + 8), 2);
  }
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(at));
  const mac = createHmac('sha1', bytes).update(counter).digest();
  const offset = mac[mac.length - 1]! & 0xf;
  const n = (mac.readUInt32BE(offset) & 0x7fffffff) % 1_000_000;
  return String(n).padStart(6, '0');
}

/** A 24 by 24 PNG emblem: a gold lantern on a red field, as the account page takes it. */
export function emblemPNG(): Buffer {
  const size = 24;
  const raw = Buffer.alloc(size * (1 + size * 3));
  for (let y = 0; y < size; y++) {
    raw[y * (1 + size * 3)] = 0;
    for (let x = 0; x < size; x++) {
      const i = y * (1 + size * 3) + 1 + x * 3;
      const lamp = Math.hypot(x - 11.5, (y - 12) * 0.8) < 7;
      raw[i] = lamp ? 240 : 160;
      raw[i + 1] = lamp ? 190 : 20;
      raw[i + 2] = lamp ? 60 : 30;
    }
  }
  const crcTable = Array.from({ length: 256 }, (_, n) => {
    let v = n;
    for (let k = 0; k < 8; k++) {
      v = v & 1 ? 0xedb88320 ^ (v >>> 1) : v >>> 1;
    }
    return v >>> 0;
  });
  const crc = (b: Buffer): number => {
    let v = 0xffffffff;
    for (const x of b) {
      v = crcTable[(v ^ x) & 0xff]! ^ (v >>> 8);
    }
    return (v ^ 0xffffffff) >>> 0;
  };
  const chunk = (type: string, data: Buffer): Buffer => {
    const len = Buffer.alloc(4);
    len.writeUInt32BE(data.length);
    const td = Buffer.concat([Buffer.from(type, 'ascii'), data]);
    const sum = Buffer.alloc(4);
    sum.writeUInt32BE(crc(td));
    return Buffer.concat([len, td, sum]);
  };
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(size, 0);
  ihdr.writeUInt32BE(size, 4);
  ihdr[8] = 8;
  ihdr[9] = 2;
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', ihdr),
    chunk('IDAT', deflateSync(raw)),
    chunk('IEND', Buffer.alloc(0)),
  ]);
}
