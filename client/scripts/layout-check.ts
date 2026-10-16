// The world layout check of decision 0041: a waserver built with -tags
// procedural runs a --dev world layout of two fields and a town, with
// the Home Village sample mod, whose home map is where players start.
//
// - Settler starts on the home map, which the page downloads, since a
//   client cannot generate a mod's map. The map's name shows its display
//   name.
// - Settler walks to the home map's portal, which leads to the first
//   town, and the page generates the town. The map's name shows the
//   town's display name.
// - The world map opens with its button and shows the home map and the
//   town as visited, with the field the town leads to as a map not
//   visited yet.

import type { Browser, Page } from 'playwright-core';
import { gameURL, hook, sleep, startGame } from './game-check.ts';
import type { FieldContext } from './field-check.ts';
import { loads, nthLoad, placesOf, walk } from './places-check.ts';

type Check = (browser: string, name: string, ok: boolean, detail: string) => void;

/** A node of the viewer's worldMap() hook. */
export interface WorldMapNode {
  id: string;
  kind: string;
  name: string;
  visited: boolean;
}

/** The display name of a generated map, from the server's map info. */
async function displayName(game: string, id: string): Promise<string> {
  const res = await fetch(`${game}/gen/maps/${encodeURIComponent(id)}.json`);
  if (!res.ok) {
    throw new Error(`${id}: HTTP ${res.status}`);
  }
  const info: unknown = await res.json();
  const name: unknown =
    typeof info === 'object' && info !== null ? Reflect.get(info, 'displayName') : undefined;
  return typeof name === 'string' ? name : '';
}

/** Waits until the viewer shows a map name. */
async function shownName(page: Page, want: string, timeout = 15_000): Promise<string> {
  const end = Date.now() + timeout;
  let got = '';
  while (Date.now() < end) {
    got = await hook<string>(page, 'displayName').catch(() => '');
    if (got === want) {
      return got;
    }
    await sleep(200);
  }
  return got;
}

/** Runs the world layout check. Chromium only, as the other game checks. */
export async function checkLayout(
  name: string,
  browser: Browser,
  ctx: FieldContext,
  check: Check,
): Promise<void> {
  const game = await startGame(ctx.repo, ctx.client, ctx.dir, ctx.wactl, ctx.build, {
    layout: true,
    fieldSize: 64,
    config: 'procedural:\n  layout: {grid: [2, 1], towns: 1, dungeons: 0}\n',
    mods: ['mods/examples/home-village'],
    name: 'layout',
  });
  const c = (label: string, ok: boolean, detail: string): void => {
    check(name, `layout ${label}`, ok, detail);
  };
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  try {
    const page = await context.newPage();
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.goto(gameURL(game.base, 'Settler'));
    const home = await nthLoad(page, 1);
    c(
      'home map',
      home.map === 'gen:home:village' && home.kind === 'home' && home.source === 'downloaded',
      `${home.map} ${home.kind}: ${home.source}, ${home.ms.toFixed(0)} ms`,
    );
    const homeName = await shownName(page, 'Willow Village');
    c('home name', homeName === 'Willow Village', homeName);

    // The home map's portal leads to the first town.
    const homePlaces = await placesOf(game.game, 'gen:home:village');
    const portal = homePlaces.find((p) => p.tags.includes('portal'));
    if (portal === undefined) {
      throw new Error('the home map has no portal');
    }
    const inTown = async (): Promise<boolean> => (await loads(page)).length >= 2;
    const legs = await walk(page, [portal.x, portal.y], 0, inTown);
    const town = await nthLoad(page, 2);
    c(
      'town',
      town.map === 'gen:town:t1' && town.kind === 'town' && town.source === 'generated',
      `${town.map} ${town.kind}: ${town.source}${town.reason === undefined ? '' : ` (${town.reason})`}, ${town.ms.toFixed(0)} ms, after ${legs.length} legs`,
    );
    const want = await displayName(game.game, 'gen:town:t1');
    const townName = await shownName(page, want);
    c('town name', want !== '' && townName === want, `${townName}, the server says ${want}`);

    // The world map shows what Settler found.
    await page.locator('#world-map-toggle').click();
    const window = page.locator('#world-map-window');
    await window.waitFor({ state: 'visible', timeout: 10_000 });
    const visited = await page.locator('#world-map-window g.world-map-node.visited').count();
    const nodes = (await hook<{ maps: WorldMapNode[] } | undefined>(page, 'worldMap'))?.maps ?? [];
    const seen = nodes.filter((n) => n.visited).map((n) => n.id);
    const label =
      (await page.locator('#world-map-window g.world-map-node.current title').textContent()) ?? '';
    c(
      'world map',
      visited === 2 &&
        seen.includes('gen:home:village') &&
        seen.includes('gen:town:t1') &&
        nodes.some((n) => !n.visited && n.kind === 'field') &&
        label.startsWith(want),
      `${visited} visited nodes drawn, visited ${seen.join(', ')}, ${nodes.length} nodes, the current map "${label}"`,
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
