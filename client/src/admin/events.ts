// The events screen (decision 0031): every world event of core and the
// mods, with whether it is on, its trigger, its next run, the run that
// lasts with its time left, and the latest results. Staff start and stop
// runs, change an event's trigger, duration and whether it is on, and
// reset it to the config. Changes apply at once and are saved over the
// config. A procedural world adds its rotating dungeons and its regions'
// seasons, which staff force after a countdown (decision 0045).

import { type JsonObject, create } from '@bufbuild/protobuf';
import { type Timestamp, timestampDate } from '@bufbuild/protobuf/wkt';
import {
  type EventChange,
  EventChangeSchema,
  type EventInfo,
  type EventList,
  type EventResult,
  type WorldChange,
} from '../gen/wathena/admin/v1/admin_pb';
import type { Context } from './context';
import { button, el, field, input, row, table, when, wideRow } from './dom';
import { REASON_MAX } from './restore';
import { countdownText } from './world';

/** The kinds of trigger, as data/events.yml names them. */
export type TriggerKind = 'daily' | 'weekly' | 'monthly' | 'hourly' | 'random' | 'condition';

export const TRIGGER_KINDS: readonly TriggerKind[] = [
  'daily',
  'weekly',
  'monthly',
  'hourly',
  'random',
  'condition',
];

export const WEEKDAYS = [
  'sunday',
  'monday',
  'tuesday',
  'wednesday',
  'thursday',
  'friday',
  'saturday',
] as const;

/** The longest run, in minutes: a week. */
export const MAX_MINUTES = 10_080;

/** The text of the trigger form's fields. */
export interface TriggerFields {
  kind: TriggerKind;
  /** HH:MM, for daily, weekly and monthly. */
  time: string;
  /** A day name, for weekly. */
  weekday: string;
  /** 1 to 31, for monthly. */
  day: string;
  /** 0 to 59, for hourly. */
  minute: string;
  /** 1 to 10000 per 10,000, for random. */
  chance: string;
  /** The condition's module and function, and how long it must hold. */
  module: string;
  fn: string;
  lasting: string;
}

const EMPTY: TriggerFields = {
  kind: 'daily',
  time: '20:00',
  weekday: 'saturday',
  day: '1',
  minute: '0',
  chance: '100',
  module: '',
  fn: '',
  lasting: '0',
};

function str(v: unknown): string {
  if (typeof v === 'number') {
    return String(v);
  }
  return typeof v === 'string' ? v : '';
}

/** A field of a JSON object, or undefined. */
function get(v: unknown, key: string): unknown {
  if (typeof v !== 'object' || v === null || Array.isArray(v)) {
    return undefined;
  }
  return Reflect.get(v, key);
}

/** A trigger kind from a select's value. */
function kindOf(v: string): TriggerKind {
  return TRIGGER_KINDS.find((k) => k === v) ?? 'daily';
}

/** The form's fields of a trigger in the form of data/events.yml. */
export function fieldsOf(trigger: JsonObject | undefined): TriggerFields {
  const f = { ...EMPTY };
  if (trigger === undefined) {
    return f;
  }
  for (const kind of TRIGGER_KINDS) {
    if (!(kind in trigger)) {
      continue;
    }
    f.kind = kind;
    const v = trigger[kind];
    switch (kind) {
      case 'daily':
        f.time = str(v);
        break;
      case 'weekly':
        f.weekday = str(get(v, 'day'));
        f.time = str(get(v, 'time'));
        break;
      case 'monthly':
        f.day = str(get(v, 'day'));
        f.time = str(get(v, 'time'));
        break;
      case 'hourly':
        f.minute = str(get(v, 'minute'));
        break;
      case 'random':
        f.chance = str(get(v, 'chance_per_hour'));
        break;
      case 'condition':
        f.module = str(get(v, 'module'));
        f.fn = str(get(v, 'function'));
        f.lasting = str(get(v, 'lasting_minutes'));
    }
  }
  return f;
}

/** A whole number from min to max, or undefined. */
function whole(s: string, min: number, max: number): number | undefined {
  const t = s.trim();
  if (!/^\d+$/.test(t)) {
    return undefined;
  }
  const n = Number(t);
  return n >= min && n <= max ? n : undefined;
}

const CLOCK = /^([01]\d|2[0-3]):[0-5]\d$/;

/**
 * Checks the trigger form's fields and makes the trigger in the form of
 * data/events.yml. The server reads it again.
 */
export function triggerValue(
  f: TriggerFields,
): { ok: true; value: JsonObject } | { ok: false; message: string } {
  const time = f.time.trim();
  const needClock = (): string => (CLOCK.test(time) ? '' : 'The time is HH:MM, such as 20:00.');
  switch (f.kind) {
    case 'daily': {
      const err = needClock();
      return err === '' ? { ok: true, value: { daily: time } } : { ok: false, message: err };
    }
    case 'weekly': {
      const err = needClock();
      if (err !== '') {
        return { ok: false, message: err };
      }
      const day = f.weekday.trim().toLowerCase();
      if (!(WEEKDAYS as readonly string[]).includes(day)) {
        return { ok: false, message: 'The day is a day of the week, such as saturday.' };
      }
      return { ok: true, value: { weekly: { day, time } } };
    }
    case 'monthly': {
      const err = needClock();
      const day = whole(f.day, 1, 31);
      if (err !== '' || day === undefined) {
        return { ok: false, message: err === '' ? 'The day of the month is 1 to 31.' : err };
      }
      return { ok: true, value: { monthly: { day, time } } };
    }
    case 'hourly': {
      const minute = whole(f.minute, 0, 59);
      return minute === undefined
        ? { ok: false, message: 'The minute is 0 to 59.' }
        : { ok: true, value: { hourly: { minute } } };
    }
    case 'random': {
      const chance = whole(f.chance, 1, 10_000);
      return chance === undefined
        ? { ok: false, message: 'The chance per hour is 1 to 10000, per 10,000.' }
        : { ok: true, value: { random: { chance_per_hour: chance } } };
    }
    default: {
      // A condition.
      const lasting = whole(f.lasting, 0, MAX_MINUTES);
      if (f.module.trim() === '' || f.fn.trim() === '') {
        return { ok: false, message: 'A condition names its module and function.' };
      }
      if (lasting === undefined) {
        return { ok: false, message: 'The lasting minutes are 0 to 10080.' };
      }
      return {
        ok: true,
        value: {
          condition: { module: f.module.trim(), function: f.fn.trim(), lasting_minutes: lasting },
        },
      };
    }
  }
}

/** JSON with sorted keys, so two triggers compare by content. */
export function canonical(v: unknown): string {
  if (Array.isArray(v)) {
    return `[${v.map(canonical).join(',')}]`;
  }
  if (typeof v === 'object' && v !== null) {
    return `{${Object.keys(v)
      .toSorted()
      .map((k) => `${JSON.stringify(k)}:${canonical(get(v, k))}`)
      .join(',')}}`;
  }
  return JSON.stringify(v);
}

/** The text of the change form. */
export interface ChangeFields {
  on: boolean;
  trigger: TriggerFields;
  minutes: string;
  reason: string;
}

/**
 * Makes the change of an event from the form: what differs from the
 * event as it is. It fails when nothing changes.
 */
export function eventChange(
  e: EventInfo,
  f: ChangeFields,
): { ok: true; value: EventChange } | { ok: false; message: string } {
  const t = triggerValue(f.trigger);
  if (!t.ok) {
    return t;
  }
  const minutes = whole(f.minutes, 1, MAX_MINUTES);
  if (minutes === undefined) {
    return { ok: false, message: 'A run lasts 1 to 10080 minutes.' };
  }
  const reason = f.reason.trim();
  if (reason.length > REASON_MAX) {
    return { ok: false, message: 'The reason is at most 255 characters.' };
  }
  const change = create(EventChangeSchema, { reason });
  let changed = false;
  if (f.on !== e.on) {
    change.on = f.on;
    changed = true;
  }
  if (canonical(t.value) !== canonical(e.trigger ?? {})) {
    change.trigger = t.value;
    changed = true;
  }
  if (minutes !== e.durationMinutes) {
    change.durationMinutes = minutes;
    changed = true;
  }
  if (!changed) {
    return { ok: false, message: 'Nothing changed.' };
  }
  return { ok: true, value: change };
}

/** The time left of a run, against the viewer's clock. */
export function timeLeft(ends: Timestamp | undefined, now: Date): string {
  if (ends === undefined) {
    return '';
  }
  return `${countdownText(timestampDate(ends).getTime() - now.getTime())} left`;
}

/** A result for a table cell, such as "won 2026-10-13 21:05". */
export function resultText(r: EventResult): string {
  return `${r.result} ${when(r.ended)}`;
}

/** Who changed an event, or "config". */
export function changedText(e: EventInfo): string {
  const o = e.override;
  if (o === undefined) {
    return 'config';
  }
  return `${o.changedBy}, ${when(o.changedAt)}`;
}

/**
 * A change that waits, for a table cell, such as
 * "2026-10-23 on 2026-10-23 20:00, forced".
 */
export function worldChangeText(c: WorldChange | undefined): string {
  if (c === undefined) {
    return '';
  }
  const what = c.date === '' ? `season ${String(c.season)}` : c.date;
  return `${what} on ${when(c.at)}${c.forced ? ', forced' : ''}`;
}

/** The seasons' line above the regions, such as "Every 30 days since 2026-10-01: 2 season ends." */
export function seasonsText(list: EventList): string {
  const s = list.seasons;
  if (s === undefined) {
    return '';
  }
  if (s.days === 0) {
    return 'Seasons are off: procedural.seasons.days is not set. Staff can still change a region.';
  }
  const ends = s.ends === 1 ? '1 season end' : `${String(s.ends)} season ends`;
  const next =
    s.next === undefined ? '' : ` The next changes ${s.next.region}: ${worldChangeText(s.next)}.`;
  return `Every ${String(s.days)} days since ${s.started === '' ? 'today' : s.started}: ${ends}.${next}`;
}

/** Renders the rotating dungeons and the regions of a procedural world. */
function renderChanges(
  ctx: Context,
  into: HTMLElement,
  list: EventList,
  load: () => Promise<void>,
): void {
  into.replaceChildren();
  if (list.rotations.length === 0 && list.seasons === undefined) {
    return;
  }
  const force = (kind: 'rotation' | 'region', id: string, label: string): HTMLButtonElement => {
    const b = button(label, () => {
      b.disabled = true;
      ctx.run(
        (async () => {
          const r = await ctx.api.forceChange(kind, id, '');
          if (!r.ok) {
            ctx.fail(r);
            return;
          }
          ctx.say(
            `${id} changes in 10 minutes. Players there are warned, then go to the nearest town.`,
          );
          await load();
        })().finally(() => {
          b.disabled = false;
        }),
      );
    });
    b.dataset.action = kind;
    return b;
  };
  const rot = table(['Rotating dungeon', 'Rotation', 'Floors of', 'Next', '']);
  for (const r of list.rotations) {
    const tr = row([
      `${r.name} (${r.dungeon})`,
      r.rotation,
      r.date,
      worldChangeText(r.next),
      force('rotation', r.dungeon, 'Rotate in 10 minutes'),
    ]);
    tr.dataset.dungeon = r.dungeon;
    rot.body.append(tr);
  }
  if (list.rotations.length === 0) {
    rot.body.append(wideRow('No dungeon of this world rotates.', 5));
  }
  const reg = table(['Region', 'Fields', 'Season', 'Forced', '']);
  for (const r of list.seasons?.regions ?? []) {
    const tr = row([
      `${r.name} (${r.region})`,
      String(r.fields),
      String(r.season),
      worldChangeText(r.forced),
      force('region', r.region, 'Change in 10 minutes'),
    ]);
    tr.dataset.region = r.region;
    reg.body.append(tr);
  }
  into.append(
    el('h2', 'Rotations and seasons'),
    el(
      'p',
      'A forced change happens 10 minutes later, after the warnings, and writes an audit row. Times are the server time zone.',
      'admin-who',
    ),
    rot.table,
    el('p', seasonsText(list), 'admin-who'),
    reg.table,
  );
}

/** Renders the events screen into a section and loads it. */
export function renderEvents(ctx: Context, section: HTMLElement): void {
  section.replaceChildren(el('h2', 'World events'));
  const { table: t, body } = table([
    'Event',
    'Type',
    'On',
    'Trigger',
    'Minutes',
    'Next run',
    'Running',
    'Latest results',
    'Changed by',
    '',
  ]);
  const editor = el('div', '', 'admin-event-editor');
  const changes = el('div', '', 'admin-world-changes');
  section.append(
    el(
      'p',
      'Changes apply at once and are saved over the config. Reset goes back to the config. Results since the server started.',
      'admin-who',
    ),
    t,
    editor,
    changes,
  );
  const load = async (): Promise<void> => {
    const r = await ctx.api.events();
    if (!r.ok) {
      ctx.fail(r);
      return;
    }
    body.replaceChildren();
    const now = new Date();
    for (const e of r.value.events) {
      const actions = el('div', '', 'admin-review');
      const act = (label: string, action: 'start' | 'stop' | 'reset'): HTMLButtonElement => {
        const b = button(label, () => {
          b.disabled = true;
          ctx.run(
            (async () => {
              const res = await ctx.api.eventAction(e.name, action, '');
              if (!res.ok) {
                ctx.fail(res);
                return;
              }
              ctx.say(
                `${e.displayName}: ${action === 'reset' ? 'back to the config' : `${action} done`}.`,
              );
              await load();
            })().finally(() => {
              b.disabled = false;
            }),
          );
        });
        b.dataset.action = action;
        return b;
      };
      actions.append(e.runs.length > 0 ? act('Stop', 'stop') : act('Start', 'start'));
      const edit = button('Edit', () => {
        editForm(ctx, editor, e, load);
      });
      edit.dataset.action = 'edit';
      actions.append(edit);
      if (e.override !== undefined) {
        actions.append(act('Reset to config', 'reset'));
      }
      const results = el('div');
      for (const res of e.results) {
        results.append(el('div', resultText(res)));
      }
      if (e.failed !== '') {
        results.append(
          el('div', `failed to start ${when(e.failedAt)}: ${e.failed}`, 'admin-error'),
        );
      }
      const running = e.runs.map((run) => timeLeft(run.ends, now)).join(', ');
      const tr = row([
        e.name,
        e.type,
        e.on ? 'on' : 'off',
        e.triggerText,
        String(e.durationMinutes),
        when(e.nextRun),
        running,
        results,
        changedText(e),
        actions,
      ]);
      tr.dataset.name = e.name;
      if (e.runs.length > 0) {
        tr.className = 'admin-active';
      }
      body.append(tr);
    }
    if (r.value.events.length === 0) {
      body.append(wideRow('There are no world events.', 10));
    }
    renderChanges(ctx, changes, r.value, load);
  };
  ctx.run(load());
}

/** The form that changes one event. */
function editForm(ctx: Context, into: HTMLElement, e: EventInfo, saved: () => Promise<void>): void {
  const form = el('form', '', 'admin-event-form');
  form.dataset.name = e.name;
  const f = fieldsOf(e.trigger);
  const on = input('on', 'checkbox');
  on.checked = e.on;
  const kind = el('select');
  kind.name = 'kind';
  for (const k of TRIGGER_KINDS) {
    const o = el('option', k);
    o.value = k;
    kind.append(o);
  }
  kind.value = f.kind;
  const time = input('time');
  time.value = f.time;
  time.size = 6;
  const weekday = el('select');
  weekday.name = 'weekday';
  for (const d of WEEKDAYS) {
    const o = el('option', d);
    o.value = d;
    weekday.append(o);
  }
  weekday.value = f.weekday === '' ? 'saturday' : f.weekday;
  const day = input('day');
  day.value = f.day;
  day.size = 3;
  const minute = input('minute');
  minute.value = f.minute;
  minute.size = 3;
  const chance = input('chance');
  chance.value = f.chance;
  chance.size = 6;
  const module = input('module');
  module.value = f.module;
  const fn = input('function');
  fn.value = f.fn;
  const lasting = input('lasting');
  lasting.value = f.lasting;
  lasting.size = 5;
  const minutes = input('minutes');
  minutes.value = String(e.durationMinutes);
  minutes.size = 6;
  const reason = input('reason');
  reason.maxLength = REASON_MAX;
  // The labels are made each time a kind shows, since an input lives in
  // one label at a time and several kinds share the time input.
  const parts: Record<TriggerKind, () => HTMLElement[]> = {
    daily: () => [field('Time', time)],
    weekly: () => [field('Day', weekday), field('Time', time)],
    monthly: () => [field('Day of the month', day), field('Time', time)],
    hourly: () => [field('Minute', minute)],
    random: () => [field('Chance per hour, per 10,000', chance)],
    condition: () => [
      field('Module', module),
      field('Function', fn),
      field('Lasting minutes', lasting),
    ],
  };
  const triggerBox = el('div', '', 'admin-row');
  const showParts = (): void => {
    triggerBox.replaceChildren(...parts[kindOf(kind.value)]());
  };
  kind.addEventListener('change', showParts);
  showParts();
  const save = el('button', 'Save');
  save.type = 'submit';
  const close = button('Close', () => {
    into.replaceChildren();
  });
  form.append(
    el('h3', `Change ${e.displayName}`),
    el(
      'p',
      `The config: ${e.configOn ? 'on' : 'off'}, ${e.configTriggerText}, ${String(e.configDurationMinutes)} minutes.`,
      'admin-who',
    ),
    field('On', on),
    field('Trigger', kind),
    triggerBox,
    field('Minutes a run lasts', minutes),
    field('Reason', reason),
    save,
    close,
  );
  form.addEventListener('submit', (ev) => {
    ev.preventDefault();
    const change = eventChange(e, {
      on: on.checked,
      trigger: {
        kind: kindOf(kind.value),
        time: time.value,
        weekday: weekday.value,
        day: day.value,
        minute: minute.value,
        chance: chance.value,
        module: module.value,
        fn: fn.value,
        lasting: lasting.value,
      },
      minutes: minutes.value,
      reason: reason.value,
    });
    if (!change.ok) {
      ctx.say(change.message);
      return;
    }
    save.disabled = true;
    ctx.run(
      (async () => {
        const r = await ctx.api.changeEvent(e.name, change.value);
        if (!r.ok) {
          ctx.fail(r);
          return;
        }
        ctx.say(
          `${r.value.displayName} is ${r.value.on ? 'on' : 'off'}: ${r.value.triggerText}, ${String(r.value.durationMinutes)} minutes.`,
        );
        into.replaceChildren();
        await saved();
      })().finally(() => {
        save.disabled = false;
      }),
    );
  });
  into.replaceChildren(form);
}
