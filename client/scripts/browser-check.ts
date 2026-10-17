// Runs the client in real browsers and checks what unit tests cannot:
//
// - audio: each browser decodes the encoder's Opus test sounds to their
//   exact length (src/audio/testdata).
// - render: the map viewer draws a map with sprites and no page errors.
// - cache: on a second visit, the Service Worker serves every asset.
// - admin: the admin page loads the admin screens for a staff account,
//   and a player's page never loads the admin bundle. The admin API is
//   faked with page.route, so no server or database is needed. The world
//   screen shows its zones and the restart countdown, the events screen
//   opens an event's form, and the mods screen shows errors and switches.
//
// - phone: the admin screens and the account pages at 375 pixels wide do
//   not scroll sideways (decision 0034).
// - touch: game-check.ts plays on a waserver with --dev sessions: phones
//   and a tablet with touch, a desktop page with the mouse, and frame
//   times at phone size with the CPU slowed down. Chromium only.
//
// - exit: the phase 6 exit check (exit-check.ts): three players on a
//   desktop and a phone play every tier 1 system against waserver with
//   PostgreSQL, a staff account with TOTP mutes a player, and a sample
//   mod's records show their names and art. It needs
//   WATHENA_TEST_DATABASE_URL, which scripts/check.sh browsers sets.
//   Chromium only.
//
// - field: field-check.ts runs a --dev procedural world with two
//   generated fields on a waserver built with -tags procedural. A page
//   generates its field in a worker with the server's checksum, walks it,
//   and a page with ?nogen downloads the same map (decision 0039).
//   Chromium only.
//
// - places: places-check.ts runs a --dev procedural world with a field,
//   a town and a crypt, and the Market Town sample mod. A player walks
//   from the field into the town, talks to the Town Guide, which stands
//   at the square by its place, and walks into the crypt's first floor
//   (decision 0040). Chromium only.
//
// - layout: layout-check.ts runs a --dev world layout with the Home
//   Village sample mod. A player starts on the home map, which the page
//   downloads, walks through its portal into the first town, sees the
//   town's display name, and opens the world map, which shows both maps
//   (decision 0041). Chromium only.
//
// - quests: quests-check.ts runs a --dev world with the Hunting Board
//   sample mod, whose Hunt Master stands on the start map. A desktop page
//   with the mouse sees her quest icon, takes her quest, reads it in the
//   quest window, hunts Dummies for it, marks it inactive and active, and
//   turns it in. A phone page with touch takes it and marks it inactive
//   (decision 0043). Chromium only.
//
// - content: content-check.ts makes a small procedural world with a
//   wactl built with -tags procedural, in a database of
//   WATHENA_TEST_DATABASE_URL, and runs it on a waserver built with the
//   tag. A player in the first town walks to the Kafra Employee at the
//   storage place and to the quest board at its quest_board place, and
//   reads the town's quests (decision 0044). Chromium only.
//
// - rotation: rotation-check.ts runs a --dev world layout whose dungeon
//   rotates. A player inside its first floor sees the warning when the
//   server clock nears the rotation, is moved to the town at the time,
//   and warps into the new first floor, which the page generates
//   (decision 0045). Chromium only.
//
// - gen: the map generator module (src/world/gen.wasm) gives the golden
//   checksums of shared/gen/testdata in Chromium, Firefox and WebKit,
//   whatever BROWSERS says (decision 0038): fields, dungeon floors and
//   towns, with their WMAP and places hashes (decision 0040).
//   GEN_BROWSERS picks others. A browser that is not installed is
//   skipped with a message.
//
// CHECKS picks checks by name, such as CHECKS=touch,phone. All run by
// default.
//
// By default it builds the placeholder pack with wactl and serves it, so
// it needs no game files. Set WATHENA_ASSETS to an asset server, such as
// `wactl assets serve`, and MAP to a map name to check other builds.
//
//   pnpm check:browsers
//   BROWSERS=chromium,firefox,webkit pnpm check:browsers
//
// Browsers come from `playwright-core install`, or CHROME_PATH for an
// installed Chrome. SCREENSHOTS names a folder to save each browser's
// render in.

import { type ChildProcess, execFileSync, spawn } from 'node:child_process';
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { type Browser, type Page, chromium, firefox, webkit } from 'playwright-core';
import { createServer } from 'vite';
import {
  checkLandscape,
  checkMouse,
  checkPhones,
  checkTablet,
  frameTimes,
  inGame,
  startGame,
} from './game-check.ts';
import { checkExit } from './exit-check.ts';
import { checkFields } from './field-check.ts';
import checkContent from './content-check.ts';
import { checkLayout } from './layout-check.ts';
import { checkQuests } from './quests-check.ts';
import { checkPlaces } from './places-check.ts';
import { checkRotation } from './rotation-check.ts';
import type * as GeneratorModule from '../src/world/generator.ts';

const repo = join(import.meta.dirname, '..', '..');
const client = join(repo, 'client');
const map = process.env.MAP ?? 'placeholder/map';
const browsers = (process.env.BROWSERS ?? 'chromium').split(',');
const only = process.env.CHECKS?.split(',');

function wanted(name: string): boolean {
  return only === undefined || only.includes(name);
}

const failures: string[] = [];

function check(browser: string, name: string, ok: boolean, detail: string): void {
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${browser} ${name}: ${detail}`);
  if (!ok) {
    failures.push(`${browser} ${name}`);
  }
}

async function waitFor(url: string, seconds: number): Promise<void> {
  for (let i = 0; i < seconds * 10; i++) {
    try {
      if ((await fetch(url)).ok) {
        return;
      }
    } catch {
      // Not listening yet.
    }
    await new Promise((resolve) => {
      setTimeout(resolve, 100);
    });
  }
  throw new Error(`${url} did not answer`);
}

/**
 * Builds wactl and the placeholder pack in a new folder, and serves the
 * pack unless WATHENA_ASSETS names an asset server.
 */
async function assetServer(): Promise<{
  url: string;
  dir: string;
  wactl: string;
  build: string;
  stop: () => void;
}> {
  const dir = mkdtempSync(join(tmpdir(), 'wathena-check-'));
  const wactl = join(dir, 'wactl');
  const build = join(dir, 'build');
  execFileSync('go', ['build', '-o', wactl, './tools/wactl'], { cwd: repo, stdio: 'inherit' });
  execFileSync(wactl, ['assets', 'build', '--source', 'placeholder', '--out', build], {
    stdio: 'inherit',
  });
  const remove = (): void => {
    rmSync(dir, { recursive: true, force: true });
  };
  const given = process.env.WATHENA_ASSETS;
  if (given !== undefined) {
    return { url: given, dir, wactl, build, stop: remove };
  }
  const addr = '127.0.0.1:8299';
  const server: ChildProcess = spawn(wactl, ['assets', 'serve', '--build', build, '--addr', addr], {
    stdio: 'ignore',
  });
  process.once('exit', () => {
    server.kill();
  });
  const url = `http://${addr}`;
  await waitFor(`${url}/manifest.json`, 10);
  return {
    url,
    dir,
    wactl,
    build,
    stop: () => {
      server.kill();
      remove();
    },
  };
}

async function launch(name: string): Promise<Browser> {
  switch (name) {
    case 'chromium':
      return chromium.launch({
        ...(process.env.CHROME_PATH === undefined
          ? {}
          : { executablePath: process.env.CHROME_PATH }),
        // Headless Linux has no GPU, so WebGL runs on SwiftShader.
        args: ['--enable-unsafe-swiftshader', '--ignore-gpu-blocklist'],
      });
    case 'firefox':
      return firefox.launch();
    case 'webkit':
      return webkit.launch();
    default:
      throw new Error(`unknown browser ${name}`);
  }
}

async function loaded(page: Page): Promise<string> {
  await page.waitForFunction(
    () => /done|Error/.test(document.querySelector<HTMLOutputElement>('#status')?.value ?? ''),
    null,
    { timeout: 90_000 },
  );
  return page.evaluate(() => document.querySelector<HTMLOutputElement>('#status')?.value ?? '');
}

interface TestSound {
  file: string;
  channels: number;
  frames: number;
}

function isTestSound(v: unknown): v is TestSound {
  return (
    typeof v === 'object' &&
    v !== null &&
    'file' in v &&
    typeof v.file === 'string' &&
    'channels' in v &&
    typeof v.channels === 'number' &&
    'frames' in v &&
    typeof v.frames === 'number'
  );
}

/** The test sounds and their lengths, which the Go encoder's tests write. */
function sounds(): TestSound[] {
  const list: unknown = JSON.parse(
    readFileSync(join(client, 'src/audio/testdata/sounds.json'), 'utf8'),
  );
  if (!Array.isArray(list) || !list.every(isTestSound)) {
    throw new Error('sounds.json: invalid');
  }
  return list;
}

async function checkAudio(name: string, browser: Browser, base: string): Promise<void> {
  const page = await browser.newPage();
  await page.goto(`${base}/src/audio/testdata/sounds.json`);
  const results = await page.evaluate(async (list) => {
    const out: { file: string; want: string; got: string }[] = [];
    for (const s of list) {
      const data = await (await fetch(`/src/audio/testdata/${s.file}`)).arrayBuffer();
      const b = await new OfflineAudioContext(1, 1, 48000).decodeAudioData(data);
      out.push({
        file: s.file,
        want: `${s.frames} frames, ${s.channels} ch`,
        got: `${b.length} frames, ${b.numberOfChannels} ch`,
      });
    }
    return out;
  }, sounds());
  for (const r of results) {
    check(name, `audio ${r.file}`, r.got === r.want, r.got);
  }
  await page.close();
}

async function checkRender(name: string, browser: Browser, base: string): Promise<void> {
  const page = await browser.newPage({ viewport: { width: 800, height: 600 } });
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto(`${base}/?map=${encodeURIComponent(map)}&characters=5&drops=&nosw`);
  const status = await loaded(page);
  check(name, 'render status', status.startsWith('done'), status);
  check(name, 'render errors', errors.length === 0, errors.join('; ') || 'none');
  // A drawn scene has many colors. A blank or failed canvas has few.
  const png = await page.screenshot();
  if (process.env.SCREENSHOTS !== undefined) {
    mkdirSync(process.env.SCREENSHOTS, { recursive: true });
    writeFileSync(join(process.env.SCREENSHOTS, `${name}-render.png`), png);
  }
  const shot = png.toString('base64');
  const colors = await page.evaluate(async (data) => {
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
    const seen = new Set<number>();
    for (let i = 0; i < d.length; i += 4 * 7) {
      seen.add((d[i]! << 16) | (d[i + 1]! << 8) | d[i + 2]!);
    }
    return seen.size;
  }, shot);
  check(name, 'render pixels', colors > 50, `${colors} colors`);
  await page.close();
}

async function checkCache(name: string, browser: Browser, base: string): Promise<void> {
  if (name === 'firefox') {
    // Playwright's Firefox does not use service workers for navigations
    // after the first, even with a minimal worker.
    console.log(`skip ${name} cache: not testable in Playwright's Firefox`);
    return;
  }
  const context = await browser.newContext({ serviceWorkers: 'allow' });
  const visit = async (): Promise<{ worker: number; network: number }> => {
    const page = await context.newPage();
    const counts = { worker: 0, network: 0 };
    page.on('response', (r) => {
      // Hashed asset files, not the manifest.
      if (/\/assets\/[0-9a-f]{2}\/[0-9a-f]{64}\./.test(r.url())) {
        counts[r.fromServiceWorker() ? 'worker' : 'network']++;
      }
    });
    await page.goto(`${base}/?map=${encodeURIComponent(map)}&characters=0&drops=`);
    await loaded(page);
    // The worker may still be installing after a fast first load.
    await page.evaluate(async () => {
      await navigator.serviceWorker.ready;
    });
    await page.close();
    return counts;
  };
  await visit();
  const second = await visit();
  check(
    name,
    'cache second visit',
    second.worker > 0 && second.network === 0,
    `${second.worker} from the worker, ${second.network} from the network`,
  );
  await context.close();
}

/** The item serial of the faked admin API. */
const SERIAL = '0198a3c4-0000-7000-8000-000000000001';

/** A 1x1 PNG, the emblem of the faked emblem report. */
const EMBLEM_PNG =
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=';

/** The faked admin API's ban of Bob's account. */
const BAN = {
  id: '3',
  kind: 'ban',
  reason: 'botting',
  issuedBy: 'Admin',
  accountId: '2000001',
  account: 'bobby',
  permanent: true,
  active: true,
};

/** A report of a chat line of Bob's. */
const LINE_REPORT = {
  id: '9',
  kind: 'line',
  reporter: 'Ann',
  target: 'Bob',
  targetId: '150000',
  targetAccountId: '2000001',
  reason: 'insults',
  map: 'prontera',
  status: 'open',
};

/** A report of a guild's emblem. */
const EMBLEM_REPORT = {
  id: '10',
  kind: 'guild_emblem',
  reporter: 'Ann',
  target: 'Wolves',
  guildId: '7',
  status: 'closed',
  outcome: 'no_action',
  closedBy: 'Gm',
};

/** Fakes the admin API: /me answers with me, and the lists are small. */
async function fakeAdminAPI(page: Page, me: object): Promise<void> {
  await page.route('**/admin/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    const bodies: Record<string, object> = {
      '/admin/api/v1/me': me,
      '/admin/api/v1/staff': {
        members: [
          { accountId: '1', account: 'Owner', role: 'owner', fromConfig: true, twoFactor: true },
        ],
        roles: [{ name: 'moderator', permissions: ['moderation.mute'] }],
      },
      '/admin/api/v1/audit': {
        entries: [
          {
            id: '1',
            account: 'Owner',
            role: 'owner',
            action: 'staff.give',
            target: 'Bobby',
            needsReview: true,
            reason: 'helps',
          },
        ],
      },
      '/admin/api/v1/players': {
        accounts: [
          {
            accountId: '2000001',
            account: 'bobby',
            characters: [
              {
                id: '150000',
                name: 'Bob',
                jobName: 'Swordman',
                baseLevel: 50,
                jobLevel: 30,
                online: true,
                position: { map: 'prontera', x: 150, y: 180 },
              },
            ],
          },
        ],
      },
      '/admin/api/v1/characters/150000': {
        id: '150000',
        name: 'Bob',
        accountId: '2000001',
        account: 'bobby',
        jobName: 'Swordman',
        baseLevel: 50,
        jobLevel: 30,
        zeny: '1000',
        inventory: [{ serial: SERIAL, itemId: 1201, name: 'Knife', amount: 1 }],
        ban: BAN,
      },
      '/admin/api/v1/characters/150000/logs': { entries: [] },
      '/admin/api/v1/reports': { reports: [LINE_REPORT, EMBLEM_REPORT] },
      '/admin/api/v1/reports/9': {
        report: LINE_REPORT,
        lines: [
          { id: 'a', kind: 'local', name: 'Ann', map: 'prontera', text: 'hello', seen: true },
          {
            id: 'b',
            kind: 'local',
            name: 'Bob',
            characterId: '150000',
            map: 'prontera',
            text: 'go away',
            seen: true,
            reported: true,
          },
          { id: 'c', kind: 'whisper', name: 'Bob', target: 'Cid', text: 'spam spam' },
        ],
        earlier: [EMBLEM_REPORT],
        sanctions: [BAN],
      },
      '/admin/api/v1/reports/10': { report: EMBLEM_REPORT, emblemPng: EMBLEM_PNG },
      '/admin/api/v1/sanctions': { sanctions: [BAN] },
      '/admin/api/v1/world': {
        zones: [
          { map: 'prontera', players: 1, monsters: 20, ticks: 1200, p50Us: '800', p99Us: '2500' },
          { map: 'geffen', monsters: 30, asleep: true },
        ],
        process: {
          version: 'check',
          uptimeSeconds: '3600',
          goroutines: '40',
          heapBytes: '1048576',
        },
        players: [
          { name: 'Bob', map: 'prontera', x: 150, y: 180, baseLevel: 50, jobName: 'Swordman' },
        ],
        playersOnline: 1,
        restart: {
          at: new Date(Date.now() + 10 * 60_000).toISOString(),
          reason: 'update',
          plannedBy: 'Admin',
        },
        now: new Date().toISOString(),
      },
      '/admin/api/v1/events': {
        events: [
          {
            name: 'core:Field_Boss',
            displayName: 'Field Boss',
            type: 'boss',
            on: true,
            trigger: { daily: '20:00' },
            triggerText: 'daily at 20:00',
            durationMinutes: 60,
            results: [{ result: 'won', ended: new Date().toISOString() }],
            configTriggerText: 'daily at 20:00',
            configDurationMinutes: 60,
          },
        ],
      },
      '/admin/api/v1/mods': {
        mods: [
          { id: 'core', name: 'Core', version: '0.1.0', builtin: true, signature: 'builtin' },
          {
            id: 'example.a',
            name: 'Example',
            version: '1.0.0',
            signature: 'folder',
            granted: ['game.read'],
            errors: '2',
            zones: [{ map: 'prontera', errors: '2', lastError: 'timer: boom' }],
          },
        ],
        maps: ['prontera', 'geffen'],
      },
      [`/admin/api/v1/items/${SERIAL}/history`]: {
        serial: SERIAL,
        serials: [SERIAL],
        moves: [
          {
            id: '1',
            action: 'drop',
            from: { kind: 'monster', id: '1002' },
            to: { kind: 'inventory', id: '150000', name: 'Bob', characterId: '150000' },
            serial: SERIAL,
            itemId: 1201,
            itemName: 'Knife',
            amount: 1,
            relation: 'self',
          },
        ],
        holder: { kind: 'inventory', id: '150000', name: 'Bob', characterId: '150000' },
      },
    };
    await route.fulfill({
      contentType: 'application/json',
      json: bodies[path] ?? { code: 'not_found' },
    });
  });
}

async function checkAdmin(name: string, browser: Browser, base: string): Promise<void> {
  const visit = async (
    me: object,
    steps?: (page: Page) => Promise<void>,
  ): Promise<{ text: string; loaded: boolean; errors: string[] }> => {
    const page = await browser.newPage();
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));
    let bundle = false;
    page.on('request', (r) => {
      if (/\/src\/admin\/app\.ts|\/app\/app-[\w-]+\.js/.test(r.url())) {
        bundle = true;
      }
    });
    await fakeAdminAPI(page, me);
    await page.goto(`${base}/admin.html`);
    await page.waitForFunction(
      () => !/Checking|Loading/.test(document.querySelector('#admin')?.textContent ?? 'Checking'),
      null,
      { timeout: 30_000 },
    );
    await page.waitForTimeout(500);
    if (steps !== undefined) {
      try {
        await steps(page);
      } catch (e) {
        errors.push(e instanceof Error ? e.message : String(e));
      }
    }
    const text = (await page.textContent('#admin')) ?? '';
    await page.close();
    return { text, loaded: bundle, errors };
  };
  // The audit log loads when its tab opens.
  const staff = await visit(
    {
      account: 'Owner',
      role: 'owner',
      permissions: ['staff.assign', 'audit.read'],
      twoFactor: true,
      csrfToken: 't',
    },
    async (page) => {
      await page.click('.admin-nav button:has-text("Audit log")');
      await page.waitForSelector('text=staff.give', { timeout: 10_000 });
    },
  );
  check(
    name,
    'admin staff',
    staff.loaded &&
      staff.text.includes('Staff') &&
      staff.text.includes('staff.give') &&
      staff.errors.length === 0,
    staff.loaded
      ? `admin bundle loaded; errors: ${staff.errors.join('; ') || 'none'}`
      : 'admin bundle not loaded',
  );
  // A game master searches a player, opens the character, follows a
  // serial to its history and opens the review queue.
  const gm = await visit(
    {
      account: 'Gm',
      role: 'game_master',
      permissions: ['audit.read', 'audit.review', 'players.read', 'items.history', 'items.restore'],
      twoFactor: true,
      csrfToken: 't',
    },
    async (page) => {
      await page.click('.admin-nav button:has-text("Players")');
      await page.fill('.admin-players input[name="q"]', 'bob');
      await page.click('.admin-players button[type="submit"]');
      await page.click('.admin-players button.admin-link:has-text("Bob")');
      await page.click('.admin-character .admin-tabs button:has-text("Inventory")');
      await page.click(`.admin-character button:has-text("${SERIAL}")`);
      await page.waitForSelector('.admin-item .admin-holder', { timeout: 10_000 });
      await page.click('.admin-nav button:has-text("Review queue")');
      await page.waitForSelector('.admin-reviews button:has-text("Flag")', { timeout: 10_000 });
    },
  );
  check(
    name,
    'admin players',
    gm.loaded &&
      gm.text.includes('Review queue') &&
      gm.text.includes('Swordman') &&
      gm.text.includes('Now: Bob (inventory)') &&
      gm.errors.length === 0,
    `errors: ${gm.errors.join('; ') || 'none'}`,
  );
  // A moderator reads a report and its emblem report, the bans, and a
  // player's sanctions.
  const mod = await visit(
    {
      account: 'Mod',
      role: 'moderator',
      permissions: [
        'players.read',
        'moderation.reports',
        'moderation.mute',
        'moderation.jail',
        'moderation.kick',
        'moderation.ban',
      ],
      twoFactor: true,
      csrfToken: 't',
    },
    async (page) => {
      await page.click('.admin-nav button:has-text("Reports")');
      await page.click('.admin-reports tr[data-id="9"] button.admin-link');
      await page.waitForSelector('.admin-report .admin-reported:has-text("go away")', {
        timeout: 10_000,
      });
      await page.waitForSelector('.admin-report button:has-text("Sanction and close")', {
        timeout: 10_000,
      });
      await page.click('.admin-report tr[data-id="10"] button.admin-link');
      await page.waitForFunction(
        () => (document.querySelector<HTMLImageElement>('.admin-emblem')?.naturalWidth ?? 0) > 0,
        null,
        { timeout: 10_000 },
      );
      await page.click('.admin-nav button:has-text("Bans")');
      await page.waitForSelector('.admin-bans tr[data-id="3"]:has-text("permanent")', {
        timeout: 10_000,
      });
      await page.click('.admin-nav button:has-text("Players")');
      await page.fill('.admin-players input[name="q"]', 'bob');
      await page.click('.admin-players button[type="submit"]');
      await page.click('.admin-players button.admin-link:has-text("Bob")');
      await page.click('.admin-character .admin-tabs button:has-text("Sanctions")');
      await page.waitForSelector('.admin-character button[data-kind="ban"]', { timeout: 10_000 });
      await page.waitForSelector('.admin-character .admin-sanctions tr[data-id="3"]', {
        timeout: 10_000,
      });
    },
  );
  check(
    name,
    'admin moderator',
    mod.loaded &&
      mod.text.includes('The account is banned') &&
      mod.text.includes('Sanction history') &&
      mod.errors.length === 0,
    `errors: ${mod.errors.join('; ') || 'none'}`,
  );
  // An admin opens the world view with its zones and countdown, an
  // event's form, and the mods with their errors and switches.
  const tools = await visit(
    {
      account: 'Admin',
      role: 'admin',
      permissions: [
        'world.read',
        'world.announce',
        'world.restart',
        'events.control',
        'mods.read',
        'mods.disable',
      ],
      twoFactor: true,
      csrfToken: 't',
    },
    async (page) => {
      await page.click('.admin-nav button:has-text("World")');
      await page.waitForSelector('.admin-world tr[data-map="prontera"]', { timeout: 10_000 });
      await page.waitForSelector('.admin-countdown:has-text("Restart in")', { timeout: 10_000 });
      await page.waitForSelector('.admin-announce button:has-text("Announce")', {
        timeout: 10_000,
      });
      await page.click('.admin-nav button:has-text("Events")');
      await page.click('.admin-events tr[data-name="core:Field_Boss"] button[data-action="edit"]');
      await page.waitForSelector('.admin-event-form select[name="kind"]', { timeout: 10_000 });
      // Each kind shows its fields, and the kinds share the time field.
      await page.selectOption('.admin-event-form select[name="kind"]', 'weekly');
      await page.waitForSelector('.admin-event-form select[name="weekday"]', { timeout: 10_000 });
      await page.fill('.admin-event-form input[name="time"]', '21:30');
      await page.selectOption('.admin-event-form select[name="kind"]', 'hourly');
      await page.waitForSelector('.admin-event-form input[name="minute"]', { timeout: 10_000 });
      await page.click('.admin-nav button:has-text("Mods")');
      await page.waitForSelector(
        '.admin-mods tr[data-mod="example.a"] button[data-action="disable"]',
        {
          timeout: 10_000,
        },
      );
    },
  );
  check(
    name,
    'admin tools',
    tools.loaded &&
      tools.text.includes('Players online: 1') &&
      tools.text.includes('timer: boom') &&
      tools.text.includes('won') &&
      tools.errors.length === 0,
    `errors: ${tools.errors.join('; ') || 'none'}`,
  );
  const player = await visit({ account: 'Player', csrfToken: 't' });
  check(
    name,
    'admin player',
    !player.loaded && player.text.includes('This page is for staff'),
    player.loaded ? 'the admin bundle loaded for a player' : 'no admin bundle',
  );
}

/** The width of a small phone, such as an iPhone SE or mini. */
const PHONE = { width: 375, height: 667 };

/** Whether a page scrolls sideways. */
async function sideways(page: Page): Promise<string | undefined> {
  return page.evaluate(() => {
    const w = document.documentElement.clientWidth;
    const sw = document.documentElement.scrollWidth;
    if (sw <= w) {
      return undefined;
    }
    const wide = [...document.querySelectorAll<HTMLElement>('body *')]
      .filter((e) => e.getBoundingClientRect().right > w + 1)
      .map((e) => `${e.tagName.toLowerCase()}${e.className === '' ? '' : `.${e.className}`}`)
      .slice(0, 3);
    return `${sw} px wide: ${wide.join(', ')}`;
  });
}

/**
 * The admin screens and the account pages at phone width. The account
 * pages are rendered with sample data by a Go test.
 */
async function checkPhoneWidth(name: string, browser: Browser, base: string): Promise<void> {
  const context = await browser.newContext({ viewport: PHONE, isMobile: true, hasTouch: true });
  const page = await context.newPage();
  await fakeAdminAPI(page, {
    account: 'Admin',
    role: 'admin',
    permissions: [
      'staff.assign',
      'audit.read',
      'audit.review',
      'players.read',
      'items.history',
      'moderation.reports',
      'moderation.ban',
      'world.read',
      'world.announce',
      'events.control',
      'mods.read',
      'mods.disable',
    ],
    twoFactor: true,
    csrfToken: 't',
  });
  await page.goto(`${base}/admin.html`);
  await page.waitForSelector('.admin-nav button', { timeout: 30_000 });
  const tabs = await page.locator('.admin-nav button').allTextContents();
  const wide: string[] = [];
  for (const tab of tabs) {
    await page.click(`.admin-nav button:has-text("${tab}")`);
    await page.waitForTimeout(400);
    const w = await sideways(page);
    if (w !== undefined) {
      wide.push(`${tab}: ${w}`);
    }
  }
  check(
    name,
    'phone admin',
    wide.length === 0 && tabs.length > 3,
    wide.length === 0 ? `${tabs.length} screens fit ${PHONE.width} px` : wide.join('; '),
  );
  const samples = mkdtempSync(join(tmpdir(), 'wathena-pages-'));
  try {
    execFileSync('go', ['test', './server/account', '-run', 'TestPageSamples', '-count=1'], {
      cwd: repo,
      env: { ...process.env, WATHENA_PAGE_SAMPLES: samples },
      stdio: 'ignore',
    });
    await page.unroute('**/admin/api/v1/**');
    await page.route('http://account.test/account/**', async (route) => {
      const file = new URL(route.request().url()).pathname.replace('/account/', '');
      if (!existsSync(join(samples, file))) {
        // Such as passkey.js, which the layout checks need not run.
        await route.fulfill({ status: 404, body: '' });
        return;
      }
      await route.fulfill({
        body: readFileSync(join(samples, file)),
        contentType: file.endsWith('.css') ? 'text/css' : 'text/html',
      });
    });
    const bad: string[] = [];
    let count = 0;
    for (const file of readdirSync(samples)) {
      if (!file.endsWith('.html') || file === 'layout.html') {
        continue;
      }
      count++;
      await page.goto(`http://account.test/account/${file}`);
      const w = await sideways(page);
      if (w !== undefined) {
        bad.push(`${file}: ${w}`);
      }
    }
    check(
      name,
      'phone account pages',
      bad.length === 0 && count > 10,
      bad.length === 0 ? `${count} pages fit ${PHONE.width} px` : bad.join('; '),
    );
  } finally {
    rmSync(samples, { recursive: true, force: true });
    await context.close();
  }
}

/** A golden map of shared/gen/testdata. */
interface GoldenMap {
  key: string;
  width: number;
  height: number;
  biome?: number | undefined;
  checksum: string;
  wmap?: string | undefined;
  /** A dungeon floor or a town, with its params as hex (version 3). */
  kind?: 'dungeon' | 'town' | undefined;
  params?: string | undefined;
  name?: string | undefined;
  /** The SHA-256 of the places' encoding (version 3). */
  places?: string | undefined;
}

interface GoldenFile {
  version: number;
  rng: string;
  maps: GoldenMap[];
}

function isGoldenFile(v: unknown): v is GoldenFile {
  return (
    typeof v === 'object' &&
    v !== null &&
    'version' in v &&
    typeof v.version === 'number' &&
    'rng' in v &&
    typeof v.rng === 'string' &&
    'maps' in v &&
    Array.isArray(v.maps) &&
    v.maps.every(
      (m: unknown) =>
        typeof m === 'object' &&
        m !== null &&
        'key' in m &&
        typeof m.key === 'string' &&
        'width' in m &&
        typeof m.width === 'number' &&
        'height' in m &&
        typeof m.height === 'number' &&
        'checksum' in m &&
        typeof m.checksum === 'string',
    )
  );
}

/** Every generator version's golden maps, from shared/gen/testdata. */
function goldenMaps(): GoldenFile[] {
  const dir = join(repo, 'shared/gen/testdata');
  return readdirSync(dir)
    .filter((f) => /^golden-v\d+\.json$/.test(f))
    .map((f) => {
      const g: unknown = JSON.parse(readFileSync(join(dir, f), 'utf8'));
      if (!isGoldenFile(g)) {
        throw new Error(`${f}: invalid`);
      }
      return {
        version: g.version,
        rng: g.rng,
        maps: g.maps.map((m) => ({
          key: m.key,
          width: m.width,
          height: m.height,
          biome: m.biome,
          checksum: m.checksum,
          wmap: m.wmap,
          kind: m.kind,
          params: m.params,
          name: m.name,
          places: m.places,
        })),
      };
    });
}

/**
 * Generates the golden maps with the client's generator module in the
 * browser, through the client's own loader, and compares the checksums.
 */
async function checkGen(name: string, browser: Browser, base: string): Promise<void> {
  const page = await browser.newPage();
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto(`${base}/src/audio/testdata/sounds.json`);
  const goldens = goldenMaps();
  const results = await page.evaluate(async (versions) => {
    const path = '/src/world/generator.ts';
    // The page imports the client's own module, which vite serves.
    // oxlint-disable-next-line typescript/no-unsafe-type-assertion
    const m = (await import(path)) as typeof GeneratorModule;
    const g = await m.Generator.load();
    const out: { label: string; ok: boolean; detail: string }[] = [];
    for (const v of versions) {
      out.push({
        label: `gen v${v.version} supported`,
        ok: g.supports(v.version, v.rng),
        detail: `${v.rng}, newest ${g.latest}`,
      });
      for (const want of v.maps) {
        const began = performance.now();
        const grid =
          want.kind === undefined
            ? g.generate(
                v.version,
                m.keyFromHex(want.key),
                want.width,
                want.height,
                want.biome ?? 0,
              )
            : g.generateMap(
                v.version,
                m.keyFromHex(want.key),
                want.kind,
                m.bytesFromHex(want.params ?? ''),
              );
        const took = performance.now() - began;
        // A map's WMAP and places must be the same bytes as the server's.
        const wmap =
          grid.wmap.length === 0
            ? undefined
            : m.hex(new Uint8Array(await crypto.subtle.digest('SHA-256', grid.wmap)));
        const places =
          v.version < 3
            ? undefined
            : m.hex(new Uint8Array(await crypto.subtle.digest('SHA-256', grid.places)));
        out.push({
          label: `gen v${v.version} ${want.kind ?? 'field'} ${want.name ?? want.key.slice(0, 8)} ${want.width}x${want.height}`,
          ok: grid.checksum === want.checksum && wmap === want.wmap && places === want.places,
          detail: `${grid.checksum.slice(0, 16)}${wmap === undefined ? '' : ` wmap ${wmap.slice(0, 16)}`}${places === undefined ? '' : ` places ${places.slice(0, 16)}`} in ${took.toFixed(1)} ms`,
        });
      }
    }
    return out;
  }, goldens);
  for (const r of results) {
    check(name, r.label, r.ok, r.detail);
  }
  check(name, 'gen errors', errors.length === 0, errors.join('; ') || 'none');
  await page.close();
}

/** The touch, tablet and mouse runs on a game server, and frame times. */
async function checkGame(
  name: string,
  browser: Browser,
  assets: { dir: string; wactl: string; build: string },
  viewerBase: string,
): Promise<void> {
  if (name !== 'chromium') {
    console.log(`skip ${name} touch: touch runs through the Chrome DevTools Protocol`);
    return;
  }
  const game = await startGame(repo, client, assets.dir, assets.wactl, assets.build);
  const c = (label: string, ok: boolean, detail: string): void => {
    check(name, label, ok, detail);
  };
  try {
    await checkPhones(browser, game.base, c);
    await checkTablet(browser, game.base, c);
    await checkLandscape(browser, game.base, c);
    await checkMouse(browser, game.base, c);
    for (const rate of [1, 4]) {
      const online = await frameTimes(
        browser,
        `${game.base}/?server=${encodeURIComponent(`${game.base.replace(/^http/, 'ws')}/ws`)}&name=Perf${rate}&nosw`,
        rate,
        inGame,
      );
      const viewer = await frameTimes(
        browser,
        `${viewerBase}/?map=${encodeURIComponent(map)}&characters=40&nosw`,
        rate,
        async (page) => {
          await loaded(page);
        },
      );
      for (const [label, t] of [
        ['online', online],
        ['viewer 40 characters', viewer],
      ] as const) {
        console.log(
          `info ${name} frames ${label}, Pixel 7 size, CPU ${rate}x slower: ${t.fps.toFixed(1)} FPS, p50 ${t.p50.toFixed(1)} ms, p95 ${t.p95.toFixed(1)} ms over ${t.frames} frames`,
        );
      }
    }
  } finally {
    await game.stop();
  }
}

const assets = await assetServer();
process.env.WATHENA_ASSETS = assets.url;
const vite = await createServer({
  root: client,
  server: { port: 5190, strictPort: false },
  logLevel: 'error',
});
await vite.listen();
const base = (vite.resolvedUrls?.local[0] ?? 'http://localhost:5190/').replace(/\/$/, '');
try {
  for (const name of browsers) {
    const browser = await launch(name);
    try {
      if (wanted('audio')) {
        await checkAudio(name, browser, base);
      }
      if (wanted('render')) {
        await checkRender(name, browser, base);
      }
      if (wanted('cache')) {
        await checkCache(name, browser, base);
      }
      if (wanted('admin')) {
        await checkAdmin(name, browser, base);
      }
      if (wanted('phone')) {
        await checkPhoneWidth(name, browser, base);
      }
      if (wanted('touch')) {
        await checkGame(name, browser, assets, base);
      }
      if (wanted('field')) {
        if (name === 'chromium') {
          await checkFields(name, browser, { repo, client, ...assets }, check);
        } else {
          console.log(`skip ${name} field: the game checks run in Chromium`);
        }
      }
      if (wanted('places')) {
        if (name === 'chromium') {
          await checkPlaces(name, browser, { repo, client, ...assets }, check);
        } else {
          console.log(`skip ${name} places: the game checks run in Chromium`);
        }
      }
      if (wanted('layout')) {
        if (name === 'chromium') {
          await checkLayout(name, browser, { repo, client, ...assets }, check);
        } else {
          console.log(`skip ${name} layout: the game checks run in Chromium`);
        }
      }
      if (wanted('quests')) {
        if (name === 'chromium') {
          await checkQuests(name, browser, { repo, client, ...assets }, check);
        } else {
          console.log(`skip ${name} quests: the game checks run in Chromium`);
        }
      }
      if (wanted('content')) {
        if (name === 'chromium') {
          await checkContent(name, browser, { repo, client, ...assets }, check);
        } else {
          console.log(`skip ${name} content: the game checks run in Chromium`);
        }
      }
      if (wanted('rotation')) {
        if (name === 'chromium') {
          await checkRotation(name, browser, { repo, client, ...assets }, check);
        } else {
          console.log(`skip ${name} rotation: the game checks run in Chromium`);
        }
      }
      if (wanted('exit')) {
        if (name === 'chromium') {
          await checkExit(name, browser, { repo, client, ...assets }, check);
        } else {
          console.log(`skip ${name} exit: its phone runs through the Chrome DevTools Protocol`);
        }
      }
    } finally {
      await browser.close();
    }
  }
  if (wanted('gen')) {
    for (const name of (process.env.GEN_BROWSERS ?? 'chromium,firefox,webkit').split(',')) {
      let browser: Browser;
      try {
        browser = await launch(name);
      } catch (error) {
        console.log(`skip ${name} gen: the browser did not start: ${String(error).split('\n')[0]}`);
        continue;
      }
      try {
        await checkGen(name, browser, base);
      } finally {
        await browser.close();
      }
    }
  }
} finally {
  await vite.close();
  assets.stop();
}
if (failures.length > 0) {
  console.error(`${failures.length} checks failed`);
  process.exit(1);
}
