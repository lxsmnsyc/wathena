// The quest check of decision 0043: a waserver with the Hunting Board
// sample mod, whose Hunt Master stands on the start map and whose quest
// counts the fixture's Dummies (game-check.ts moves them there).
//
// - Hunter, with the mouse, sees the "!" quest icon over the Hunt Master,
//   takes the quest, reads it in the quest window with its timer and its
//   objective, kills three Dummies, sees the count and the "?" icon, marks
//   the quest inactive and active again, and turns it in.
// - Tapper, on a phone with touch, sees the icon, takes the quest, opens
//   the quest window through the menu, and marks the quest inactive.
//   Tapper plays only when browser-check.ts passes touch, since touch
//   input is frozen as built in decision 0034.

import type { Browser } from 'playwright-core';
import { Player, choose, endDialog, kill, talk } from './exit-check.ts';
import type { FieldContext } from './field-check.ts';
import { gameURL, hook, sleep, startGame } from './game-check.ts';

type Check = (browser: string, name: string, ok: boolean, detail: string) => void;

/** A quest of the viewer's quests() hook. */
export interface QuestHook {
  id: number;
  title: string;
  active: boolean;
  timeLeftMs: number;
  objectives: { label: string; count: number; goal: number }[];
}

/** An icon of the viewer's questIcons() hook. */
interface IconHook {
  npc: number;
  name: string | undefined;
  icon: string;
}

/** Waits until a test on the viewer's hooks passes, and returns the last value. */
async function until<T>(
  read: () => Promise<T>,
  ok: (v: T) => boolean,
  timeout = 15_000,
): Promise<T> {
  const end = Date.now() + timeout;
  let v = await read();
  while (!ok(v) && Date.now() < end) {
    await sleep(200);
    v = await read();
  }
  if (!ok(v)) {
    throw new Error(`timed out: ${JSON.stringify(v).slice(0, 300)}`);
  }
  return v;
}

/** The icon over the Hunt Master that a player sees, or "". */
async function masterIcon(p: Player): Promise<string> {
  const icons = await hook<IconHook[]>(p.page, 'questIcons');
  return icons.find((i) => i.name === 'Hunt Master')?.icon ?? '';
}

/** The Poring Patrol in a player's quest log. */
async function patrol(p: Player): Promise<QuestHook | undefined> {
  const quests = await hook<QuestHook[]>(p.page, 'quests');
  return quests.find((q) => q.title === 'Poring Patrol');
}

/** Waits until the page draws a quest icon over the Hunt Master. */
async function iconShown(p: Player, icon: string): Promise<void> {
  await p.page
    .locator(`#quest-icons .quest-icon[data-icon="${icon}"]:not([hidden])`)
    .first()
    .waitFor({ state: 'visible', timeout: 15_000 });
}

/**
 * Runs the quest check. Chromium only, as the other game checks. With
 * touch, a phone page with touch plays after the desktop page.
 */
export async function checkQuests(
  name: string,
  browser: Browser,
  ctx: FieldContext,
  check: Check,
  touch = false,
): Promise<void> {
  const game = await startGame(ctx.repo, ctx.client, ctx.dir, ctx.wactl, ctx.build, {
    mods: ['mods/examples/hunting-board'],
    approve: '    example.hunting-board: [player.give_items, player.give_zeny]\n',
    name: 'quests',
  });
  const c = (label: string, ok: boolean, detail: string): void => {
    check(name, `quests ${label}`, ok, detail);
  };
  // run reports a step; a step that throws fails with its message.
  const run = async (label: string, p: Player, f: () => Promise<string>): Promise<boolean> => {
    try {
      const detail = await f();
      const errors = p.errors.splice(0);
      c(
        label,
        errors.length === 0,
        errors.length === 0 ? detail : `page errors: ${errors.join('; ')}`,
      );
      return errors.length === 0;
    } catch (e) {
      c(label, false, e instanceof Error ? (e.message.split('\n')[0] ?? '') : String(e));
      return false;
    }
  };
  const players: Player[] = [];
  try {
    const hunter = await Player.open(browser, 'Hunter', false, gameURL(game.base, 'Hunter'));
    players.push(hunter);
    await run('desktop icon', hunter, async () => {
      await until(
        async () => masterIcon(hunter),
        (i) => i === 'quest',
      );
      await iconShown(hunter, 'quest');
      return 'the Hunt Master shows "!"';
    });
    await run('desktop take', hunter, async () => {
      await talk(hunter, 'Hunt Master');
      await choose(hunter, "I'll do it.");
      await endDialog(hunter);
      const q = await until(
        async () => patrol(hunter),
        (v) => v !== undefined,
      );
      const o = q?.objectives[0];
      if (q === undefined || o?.label !== 'Dummy' || o.goal !== 3 || o.count !== 0 || !q.active) {
        throw new Error(`the quest log holds ${JSON.stringify(q)}`);
      }
      if (q.timeLeftMs <= 3_500_000 || q.timeLeftMs > 3_600_000) {
        throw new Error(`the time left is ${q.timeLeftMs} ms`);
      }
      return `quest ${q.id}, ${o.label} ${o.count}/${o.goal}, ${Math.round(q.timeLeftMs / 1000)} s left`;
    });
    const id = (await patrol(hunter))?.id ?? 0;
    const item = `#quest-window li.quest[data-quest-id="${id}"]`;
    await run('desktop window', hunter, async () => {
      await hunter.show('#quest-window', '#quest-toggle');
      const title = (await hunter.page.locator(`${item} .quest-title`).textContent()) ?? '';
      const timer = (await hunter.page.locator(`${item} .quest-timer`).textContent()) ?? '';
      const objective =
        (await hunter.page.locator(`${item} .quest-objective`).first().textContent()) ?? '';
      if (
        title !== 'Poring Patrol' ||
        !/^Time left (0:59|1:00):\d\d$/.test(timer) ||
        !objective.includes('Dummy 0/3')
      ) {
        throw new Error(`the window shows "${title}", "${timer}", "${objective}"`);
      }
      return `${title}: ${timer}, ${objective}`;
    });
    await run('desktop hunt', hunter, async () => {
      for (let i = 0; i < 3; i++) {
        await kill(hunter, 'Dummy');
      }
      await hunter.page
        .locator(`${item} .quest-objective.done:has-text("Dummy 3/3")`)
        .waitFor({ timeout: 15_000 });
      await until(
        async () => masterIcon(hunter),
        (i) => i === 'quest_question',
      );
      await iconShown(hunter, 'quest_question');
      return 'Dummy 3/3, and the Hunt Master shows "?"';
    });
    await run('desktop inactive', hunter, async () => {
      await hunter.press(`${item} .quest-state`);
      await hunter.page
        .locator(`#quest-window li.quest.inactive[data-quest-id="${id}"]`)
        .waitFor({ timeout: 15_000 });
      await until(
        async () => patrol(hunter),
        (q) => q?.active === false,
      );
      await hunter.press(`${item} .quest-state`);
      await hunter.page
        .locator(`#quest-window li.quest.active[data-quest-id="${id}"]`)
        .waitFor({ timeout: 15_000 });
      return 'inactive, then active again';
    });
    await run('desktop turn in', hunter, async () => {
      await hunter.hide('#quest-window', '#quest-toggle');
      await talk(hunter, 'Hunt Master');
      const said = (await hunter.page.textContent('#dialog-window .dialog-body')) ?? '';
      await endDialog(hunter);
      if (!said.includes('500 zeny and an Apple')) {
        throw new Error(`the Hunt Master said "${said}"`);
      }
      await until(
        async () => patrol(hunter),
        (q) => q === undefined,
      );
      await until(
        async () => masterIcon(hunter),
        (i) => i === '',
      );
      await hunter.show('#quest-window', '#quest-toggle');
      await hunter.page.locator('#quest-window .quest-empty').waitFor({ timeout: 15_000 });
      return 'paid, the quest left the window, and the icon went away';
    });

    if (touch) {
      const tapper = await Player.open(browser, 'Tapper', true, gameURL(game.base, 'Tapper'));
      players.push(tapper);
      await run('phone icon', tapper, async () => {
        await until(
          async () => masterIcon(tapper),
          (i) => i === 'quest',
        );
        await iconShown(tapper, 'quest');
        return 'the Hunt Master shows "!"';
      });
      await run('phone take', tapper, async () => {
        await talk(tapper, 'Hunt Master');
        await choose(tapper, "I'll do it.");
        await endDialog(tapper);
        await until(
          async () => patrol(tapper),
          (q) => q !== undefined,
        );
        return 'taken with taps';
      });
      await run('phone window', tapper, async () => {
        await tapper.show('#quest-window', '#quest-toggle');
        const phoneItem = `#quest-window li.quest[data-quest-id="${id}"]`;
        const objective =
          (await tapper.page.locator(`${phoneItem} .quest-objective`).first().textContent()) ?? '';
        if (!objective.includes('Dummy 0/3')) {
          throw new Error(`the window shows "${objective}"`);
        }
        await tapper.press(`${phoneItem} .quest-state`);
        await tapper.page
          .locator(`#quest-window li.quest.inactive[data-quest-id="${id}"]`)
          .waitFor({ timeout: 15_000 });
        return `${objective}, marked inactive with a tap`;
      });
    }
    c('server errors', game.errors.length === 0, game.errors.slice(0, 3).join('; ') || 'none');
  } catch (error) {
    c('run', false, String(error).split('\n')[0] ?? '');
  } finally {
    for (const p of players) {
      await p.context.close().catch(() => undefined);
    }
    await game.stop();
  }
}
