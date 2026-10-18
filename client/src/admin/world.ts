// The world screen (decision 0031): the zones with their last minute of
// ticks, the process, the players online, the announcement box and the
// restart planner with its countdown. The dashboard loads again every 5
// seconds while the screen shows. The retention periods and the
// retention job's last run show below, loaded once (decision 0049).

import { create } from '@bufbuild/protobuf';
import { timestampDate, timestampFromDate } from '@bufbuild/protobuf/wkt';
import {
  type RestartPlan,
  type RestartRequest,
  RestartRequestSchema,
  type RetentionKind,
  type RetentionStatus,
  type WorldView,
  type ZoneView,
} from '../gen/wathena/admin/v1/admin_pb';
import type { Context } from './context';
import { button, el, field, input, row, table, when, wideRow } from './dom';
import { REASON_MAX } from './restore';

/** How often the dashboard loads again, in milliseconds. */
export const REFRESH_MS = 5000;

/** The longest announcement, in characters. */
export const ANNOUNCE_MAX = 255;

/** The least and most time from now to a planned restart. */
export const RESTART_MIN_MS = 60_000;
export const RESTART_MAX_MS = 7 * 24 * 60 * 60_000;

/** Microseconds as milliseconds for a table cell, such as "1.25". */
export function msText(us: bigint): string {
  return (Number(us) / 1000).toFixed(2);
}

/** Bytes as whole megabytes, such as "120 MB". */
export function bytesText(n: bigint): string {
  return `${String(Math.round(Number(n) / (1 << 20)))} MB`;
}

/** An uptime, such as "3 d 4 h", "2 h 5 min" or "45 s". */
export function uptimeText(seconds: bigint): string {
  const s = Number(seconds);
  if (s < 60) {
    return `${String(s)} s`;
  }
  const d = Math.floor(s / 86_400);
  const h = Math.floor((s % 86_400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) {
    return `${String(d)} d ${String(h)} h`;
  }
  return h > 0 ? `${String(h)} h ${String(m)} min` : `${String(m)} min`;
}

/** A countdown, such as "1:05:09", "12:05" or "0:30", and "now" at zero. */
export function countdownText(ms: number): string {
  if (ms <= 0) {
    return 'now';
  }
  const total = Math.ceil(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const ss = String(s).padStart(2, '0');
  if (h > 0) {
    return `${String(h)}:${String(m).padStart(2, '0')}:${ss}`;
  }
  return `${String(m)}:${ss}`;
}

/**
 * The zones to show: those with players first, most players first, then
 * by map. Sleeping zones show only when asked.
 */
export function shownZones(zones: readonly ZoneView[], sleeping: boolean): ZoneView[] {
  return zones
    .filter((z) => sleeping || !z.asleep)
    .toSorted((a, b) => b.players - a.players || a.map.localeCompare(b.map));
}

/** The text of the restart planner's fields. */
export interface RestartFields {
  /** A datetime-local value, in the viewer's time zone. */
  at: string;
  reason: string;
  message: string;
}

/**
 * Checks the restart planner's fields against the server clock and
 * makes the request. The server checks again.
 */
export function restartRequest(
  f: RestartFields,
  serverNow: Date,
): { ok: true; value: RestartRequest } | { ok: false; message: string } {
  const at = new Date(f.at);
  if (f.at.trim() === '' || Number.isNaN(at.getTime())) {
    return { ok: false, message: 'Pick the time of the restart.' };
  }
  const lead = at.getTime() - serverNow.getTime();
  if (lead < RESTART_MIN_MS || lead > RESTART_MAX_MS) {
    return { ok: false, message: 'A restart is planned from 1 minute to 7 days ahead.' };
  }
  const reason = f.reason.trim();
  const message = f.message.trim();
  if (reason === '') {
    return { ok: false, message: 'A restart needs a reason. Players see it.' };
  }
  if (reason.length > REASON_MAX || message.length > REASON_MAX) {
    return { ok: false, message: 'The reason and the message are at most 255 characters.' };
  }
  return {
    ok: true,
    value: create(RestartRequestSchema, { at: timestampFromDate(at), reason, message }),
  };
}

/** Checks an announcement's text. It returns an error, or "". */
export function announceError(text: string): string {
  const t = text.trim();
  if (t === '') {
    return 'Write the announcement first.';
  }
  if (t.length > ANNOUNCE_MAX || /[\r\n]/.test(t)) {
    return 'An announcement is one line of at most 255 characters.';
  }
  return '';
}

/** The planned restart for people. */
export function planText(p: RestartPlan): string {
  const at = when(p.at);
  const by = p.plannedBy === '' ? '' : `, planned by ${p.plannedBy}`;
  const closed = p.loginsClosed ? ' New logins are refused.' : '';
  return `Restart at ${at}${by}. Reason: ${p.reason}.${closed}`;
}

/** A datetime-local value for a date, in the viewer's time zone. */
export function localInput(d: Date): string {
  const pad = (n: number): string => String(n).padStart(2, '0');
  return `${String(d.getFullYear())}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** The data of each retention kind, for people. */
const RETENTION_NAMES: Record<string, string> = {
  login_ips: 'Login records with IP addresses',
  chat: 'Chat lines',
  item_log: 'Item log',
  zeny_log: 'Zeny log',
  audit: 'Staff audit log',
};

/** A retention kind's row: data, kept, default, last run, since start. */
export function retentionRow(k: RetentionKind): string[] {
  const days = (n: number): string => (n === 1 ? '1 day' : `${String(n)} days`);
  return [
    RETENTION_NAMES[k.kind] ?? k.kind,
    days(k.days),
    days(k.defaultDays),
    String(k.lastDeleted),
    String(k.deletedSinceStart),
  ];
}

/** The retention job's last run for people. */
export function retentionRunText(s: RetentionStatus): string {
  if (s.lastRun === undefined) {
    return 'The retention job has not run yet in this process.';
  }
  const next = s.nextRun === undefined ? '' : ` The next run is at ${when(s.nextRun)}.`;
  if (s.skipped) {
    return `The last run at ${when(s.lastRun)} found the job running in another process, and skipped.${next}`;
  }
  const dropped =
    s.partitionsDropped > 0
      ? ` It dropped ${String(s.partitionsDropped)} month partitions of the logs.`
      : '';
  if (s.lastError !== '') {
    return `The last run at ${when(s.lastRun)} failed: ${s.lastError}.${dropped}${next}`;
  }
  return `The last run at ${when(s.lastRun)} deleted old data.${dropped}${next}`;
}

/** The screen's state, kept while it shows. */
interface State {
  timer: number;
  tick: number;
  /** The server clock less the viewer's, in milliseconds. */
  skew: number;
  plan: RestartPlan | undefined;
}

const states = new WeakMap<HTMLElement, State>();

/** Renders the world screen into a section, and loads it every 5 seconds. */
export function renderWorld(ctx: Context, section: HTMLElement): void {
  const old = states.get(section);
  if (old !== undefined) {
    window.clearTimeout(old.timer);
    window.clearInterval(old.tick);
  }
  const state: State = { timer: 0, tick: 0, skew: 0, plan: undefined };
  states.set(section, state);
  section.replaceChildren(el('h2', 'World'));
  const status = el('p', '', 'admin-who');
  const process = el('p');
  const restartLine = el('p', '', 'admin-restart');
  const countdown = el('strong', '', 'admin-countdown');
  const sleeping = input('sleeping', 'checkbox');
  const zones = table([
    'Map',
    'Players',
    'Monsters',
    'Ticks',
    'p50 ms',
    'p99 ms',
    'Max ms',
    'Over 10 ms',
  ]);
  const players = table(['Name', 'Map', 'Cell', 'Level', 'Job']);
  const playerCount = el('h3', 'Players online');
  const tools = el('div');
  section.append(status, process, restartLine, tools);
  const maps = el('select');
  maps.name = 'map';
  section.append(
    el('h3', 'Zones'),
    field('Show sleeping zones', sleeping),
    zones.table,
    playerCount,
    players.table,
  );

  let view: WorldView | undefined;
  const showZones = (): void => {
    if (view === undefined) {
      return;
    }
    zones.body.replaceChildren();
    const list = shownZones(view.zones, sleeping.checked);
    for (const z of list) {
      const tr = row([
        z.map,
        String(z.players),
        String(z.monsters),
        String(z.ticks),
        msText(z.p50Us),
        msText(z.p99Us),
        msText(z.maxUs),
        String(z.overBudget),
      ]);
      tr.dataset.map = z.map;
      if (z.overBudget > 0) {
        tr.className = 'admin-over';
      }
      zones.body.append(tr);
    }
    if (list.length === 0) {
      zones.body.append(wideRow('Every zone sleeps: no one plays.', 8));
    }
  };
  sleeping.addEventListener('change', showZones);

  const showCountdown = (): void => {
    const p = state.plan;
    if (p?.at === undefined) {
      countdown.textContent = '';
      return;
    }
    const left = timestampDate(p.at).getTime() - (Date.now() + state.skew);
    countdown.textContent = `Restart in ${countdownText(left)}`;
  };

  const load = async (): Promise<void> => {
    const r = await ctx.api.world();
    if (!r.ok) {
      ctx.fail(r);
      return;
    }
    view = r.value;
    if (view.now !== undefined) {
      state.skew = timestampDate(view.now).getTime() - Date.now();
    }
    status.textContent = `Loaded ${when(view.now)}. It loads again every ${String(REFRESH_MS / 1000)} seconds.`;
    const p = view.process;
    process.textContent =
      p === undefined
        ? ''
        : `Version ${p.version}, up ${uptimeText(p.uptimeSeconds)}, CPU ${p.cpuCores.toFixed(2)} cores, heap ${bytesText(p.heapBytes)}, memory ${bytesText(p.totalBytes)}, ${String(p.goroutines)} goroutines.`;
    state.plan = view.restart;
    restartLine.replaceChildren();
    if (view.restart === undefined) {
      restartLine.textContent = 'No restart is planned.';
      restartLine.className = 'admin-restart';
    } else {
      restartLine.append(countdown, ' ', planText(view.restart));
      restartLine.className = 'admin-restart admin-error';
    }
    showCountdown();
    const chosen = maps.value;
    maps.replaceChildren();
    const all = el('option', 'Every map');
    all.value = '';
    maps.append(all);
    for (const z of view.zones) {
      const o = el('option', z.map);
      o.value = z.map;
      maps.append(o);
    }
    maps.value = chosen;
    showZones();
    playerCount.textContent = `Players online: ${String(view.playersOnline)}`;
    players.body.replaceChildren();
    for (const pl of view.players) {
      players.body.append(
        row([pl.name, pl.map, `${String(pl.x)},${String(pl.y)}`, String(pl.baseLevel), pl.jobName]),
      );
    }
    if (view.players.length === 0) {
      players.body.append(wideRow('No one plays.', 5));
    }
  };

  section.append(retentionPanel(ctx));
  if (ctx.can('world.announce')) {
    tools.append(announceForm(ctx, maps));
  }
  if (ctx.can('world.restart')) {
    tools.append(restartForm(ctx, state, load));
  }

  // The loop stops once the screen is hidden or gone. Opening the tab
  // again renders the screen anew.
  const visible = (): boolean => section.isConnected && !section.hidden;
  const loop = async (): Promise<void> => {
    if (!visible()) {
      return;
    }
    await load().catch(() => {
      ctx.say('The world view did not load. It tries again.');
    });
    if (visible() && states.get(section) === state) {
      state.timer = window.setTimeout(() => {
        ctx.run(loop());
      }, REFRESH_MS);
    }
  };
  state.tick = window.setInterval(() => {
    if (!visible() || states.get(section) !== state) {
      window.clearInterval(state.tick);
      return;
    }
    showCountdown();
  }, 1000);
  ctx.run(loop());
}

/** The retention periods and the job's last run, loaded once. */
function retentionPanel(ctx: Context): HTMLElement {
  const panel = el('div', '', 'admin-retention');
  const run = el('p');
  const warnings = el('ul', '', 'admin-error');
  const kinds = table([
    'Data',
    'Kept',
    'Default',
    'Deleted by the last run',
    'Deleted since the start',
  ]);
  panel.append(el('h3', 'Retention'), warnings, run, kinds.table);
  ctx.run(
    (async () => {
      const r = await ctx.api.retention();
      if (!r.ok) {
        run.textContent = r.message;
        return;
      }
      const s = r.value;
      warnings.replaceChildren(...s.warnings.map((w) => el('li', w)));
      run.textContent = retentionRunText(s);
      run.className = s.lastError === '' ? '' : 'admin-error';
      kinds.body.replaceChildren(...s.kinds.map((k) => row(retentionRow(k))));
    })(),
  );
  return panel;
}

/** The announcement box: a line, and a map or the world. */
function announceForm(ctx: Context, maps: HTMLSelectElement): HTMLFormElement {
  const form = el('form', '', 'admin-announce');
  const text = input('text');
  text.maxLength = ANNOUNCE_MAX;
  text.size = 60;
  text.placeholder = 'Every player of the map or the world sees it';
  const send = el('button', 'Announce');
  send.type = 'submit';
  form.append(el('h3', 'Announcement'), field('Text', text), field('To', maps), send);
  form.addEventListener('submit', (ev) => {
    ev.preventDefault();
    const err = announceError(text.value);
    if (err !== '') {
      ctx.say(err);
      return;
    }
    send.disabled = true;
    ctx.run(
      (async () => {
        const r = await ctx.api.announce(text.value.trim(), maps.value);
        if (!r.ok) {
          ctx.fail(r);
          return;
        }
        ctx.say(`Announced to ${r.value.map === '' ? 'the world' : r.value.map}: ${r.value.text}`);
        text.value = '';
      })().finally(() => {
        send.disabled = false;
      }),
    );
  });
  return form;
}

/** The restart planner: a time, a reason and a message, and a cancel. */
function restartForm(ctx: Context, state: State, changed: () => Promise<void>): HTMLFormElement {
  const form = el('form', '', 'admin-restart-form');
  const at = input('at', 'datetime-local');
  at.value = localInput(new Date(Date.now() + 30 * 60_000));
  const reason = input('reason');
  reason.maxLength = REASON_MAX;
  reason.placeholder = 'Players see it when they are disconnected';
  const message = input('message');
  message.maxLength = REASON_MAX;
  message.size = 40;
  message.placeholder = 'What the announcements say; the reason when empty';
  const plan = el('button', 'Plan the restart');
  plan.type = 'submit';
  const cancel = button('Cancel the planned restart', () => {
    cancel.disabled = true;
    ctx.run(
      (async () => {
        const r = await ctx.api.cancelRestart(reason.value);
        if (!r.ok) {
          ctx.fail(r);
          return;
        }
        ctx.say('The planned restart is cancelled. Players were told.');
        await changed();
      })().finally(() => {
        cancel.disabled = false;
      }),
    );
  });
  form.append(
    el('h3', 'Planned restart'),
    el(
      'p',
      'Players get announcements 30, 15, 10, 5 and 1 minute before, and 30 seconds before. New logins are refused in the last minute. A restart of the server before the time drops the plan.',
      'admin-who',
    ),
    field('Time', at),
    field('Reason', reason),
    field('Message', message),
    plan,
    cancel,
  );
  form.addEventListener('submit', (ev) => {
    ev.preventDefault();
    const req = restartRequest(
      { at: at.value, reason: reason.value, message: message.value },
      new Date(Date.now() + state.skew),
    );
    if (!req.ok) {
      ctx.say(req.message);
      return;
    }
    plan.disabled = true;
    ctx.run(
      (async () => {
        const r = await ctx.api.planRestart(req.value);
        if (!r.ok) {
          ctx.fail(r);
          return;
        }
        ctx.say(planText(r.value));
        await changed();
      })().finally(() => {
        plan.disabled = false;
      }),
    );
  });
  return form;
}
