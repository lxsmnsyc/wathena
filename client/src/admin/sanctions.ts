// Sanctions (decision 0030): the form that gives a mute, a jail, a kick
// or a ban, the table of a target's sanctions with lift buttons, and the
// list of bans in effect. A ban and its lift need a recent login, so a
// reauth error shows the reauth link.

import { create } from '@bufbuild/protobuf';
import {
  type Sanction,
  type SanctionRequest,
  SanctionRequestSchema,
} from '../gen/wathena/admin/v1/admin_pb';
import { ACTIVE_BANS, SANCTION_PAGE, type SanctionKind } from './api';
import type { Context } from './context';
import { button, el, field, input, row, table, when, wideRow } from './dom';
import { REASON_MAX } from './restore';

/** The longest mute or jail, in minutes: a year. */
export const MAX_MUTE_MINUTES = 525_600;

/** The longest ban that ends, in minutes: ten years. */
export const MAX_BAN_MINUTES = 5_256_000;

/** The kinds of sanction that act on a character. */
const CHARACTER_KINDS: readonly SanctionKind[] = ['mute', 'jail', 'kick'];

/** The permission that gives and lifts a kind of sanction. */
export function sanctionPermission(kind: string): string {
  return `moderation.${kind}`;
}

/** The kinds of sanction on a character that a role's permissions give. */
export function characterKindsFor(permissions: readonly string[]): SanctionKind[] {
  return CHARACTER_KINDS.filter((k) => permissions.includes(sanctionPermission(k)));
}

/** Whether a role has any permission of the moderation group. */
export function canModerate(permissions: readonly string[]): boolean {
  return permissions.some((p) => p.startsWith('moderation.'));
}

/**
 * Whether the caller may lift a sanction: one in effect, not a kick. A
 * permanent ban also needs moderation.ban_permanent.
 */
export function canLift(permissions: readonly string[], s: Sanction): boolean {
  return (
    s.id !== 0n &&
    s.active &&
    s.kind !== 'kick' &&
    permissions.includes(sanctionPermission(s.kind)) &&
    (!(s.kind === 'ban' && s.permanent) || permissions.includes('moderation.ban_permanent'))
  );
}

/** The end of a sanction for a table cell. */
export function endText(s: Sanction): string {
  if (s.permanent) {
    return 'permanent';
  }
  return s.kind === 'kick' ? '' : when(s.endsAt);
}

/** The state of a sanction: in effect, ended, lifted or done. */
export function stateText(s: Sanction): string {
  if (s.liftedAt !== undefined) {
    const by = s.liftedBy === '' ? '' : ` by ${s.liftedBy}`;
    const why = s.liftReason === '' ? '' : `: ${s.liftReason}`;
    return `lifted${by}${why}`;
  }
  if (s.active) {
    return 'in effect';
  }
  return s.kind === 'kick' ? 'done' : 'ended';
}

/** Who a sanction acts on: the character, or the account of a ban. */
export function targetText(s: Sanction): string {
  if (s.kind === 'ban' || s.character === '') {
    const id = `account ${s.accountId.toString()}`;
    return s.account === '' ? id : `${s.account} (${id})`;
  }
  return s.character;
}

/** The text of a sanction form's fields. */
export interface SanctionFields {
  kind: SanctionKind;
  /** The character of a mute, a jail or a kick. */
  characterId: bigint;
  /** The account of a ban. */
  accountId: bigint;
  minutes: string;
  /** A ban without an end. */
  permanent: boolean;
  reason: string;
}

/**
 * Checks a sanction form's fields and makes the request. The server
 * checks the permissions again.
 */
export function sanctionRequest(
  f: SanctionFields,
  permissions: readonly string[],
): { ok: true; value: SanctionRequest } | { ok: false; message: string } {
  if (!permissions.includes(sanctionPermission(f.kind))) {
    return { ok: false, message: `Your role cannot give a ${f.kind}.` };
  }
  const ban = f.kind === 'ban';
  if (ban ? f.accountId <= 0n : f.characterId <= 0n) {
    return { ok: false, message: `A ${f.kind} needs a ${ban ? 'account' : 'character'}.` };
  }
  let minutes = 0;
  if (ban && f.permanent) {
    if (!permissions.includes('moderation.ban_permanent')) {
      return { ok: false, message: 'Your role cannot give a permanent ban.' };
    }
  } else if (f.kind !== 'kick') {
    const max = ban ? MAX_BAN_MINUTES : MAX_MUTE_MINUTES;
    const t = f.minutes.trim();
    minutes = /^\d+$/.test(t) ? Number(t) : 0;
    if (minutes < 1 || minutes > max) {
      return { ok: false, message: `A ${f.kind} lasts from 1 to ${String(max)} minutes.` };
    }
  }
  const reason = f.reason.trim();
  if (reason === '') {
    return { ok: false, message: `A ${f.kind} needs a reason.` };
  }
  if (reason.length > REASON_MAX) {
    return { ok: false, message: 'The reason is longer than 255 characters.' };
  }
  return {
    ok: true,
    value: create(SanctionRequestSchema, {
      kind: f.kind,
      characterId: ban ? 0n : f.characterId,
      accountId: ban ? f.accountId : 0n,
      minutes,
      reason,
    }),
  };
}

/** The words of a kind's button. */
function kindLabel(kind: SanctionKind): string {
  switch (kind) {
    case 'mute':
      return 'Mute';
    case 'jail':
      return 'Jail';
    case 'kick':
      return 'Kick';
    default:
      return 'Ban the account';
  }
}

/** The notice once a sanction is given, such as "Bob is muted.". */
export function givenText(kind: SanctionKind, name: string): string {
  switch (kind) {
    case 'mute':
      return `${name} is muted.`;
    case 'jail':
      return `${name} is jailed.`;
    case 'kick':
      return `${name} is kicked.`;
    default:
      return `The account ${name} is banned.`;
  }
}

/** The target of a sanction form, and the kinds it offers. */
export interface SanctionTarget {
  characterId: bigint;
  character: string;
  accountId: bigint;
  account: string;
  kinds: readonly SanctionKind[];
}

/**
 * The sanction form: minutes, a reason and a button per kind. given runs
 * with the sanction once the server gave it.
 */
export function sanctionForm(
  ctx: Context,
  target: SanctionTarget,
  given: (s: Sanction) => Promise<void>,
): HTMLElement {
  const form = el('form', '', 'admin-sanction');
  form.append(el('h3', 'Give a sanction'));
  const minutes = input('minutes');
  minutes.inputMode = 'numeric';
  minutes.size = 8;
  minutes.placeholder = '60';
  const reason = input('reason');
  reason.maxLength = REASON_MAX;
  reason.size = 40;
  reason.placeholder = 'The player sees it';
  form.append(field('Minutes, not for a kick', minutes), field('Reason', reason));
  const permanent = input('permanent', 'checkbox');
  if (target.kinds.includes('ban') && ctx.can('moderation.ban_permanent')) {
    form.append(field('Permanent ban', permanent));
  }
  const buttons: HTMLButtonElement[] = [];
  const give = (kind: SanctionKind): void => {
    const req = sanctionRequest(
      {
        kind,
        characterId: target.characterId,
        accountId: target.accountId,
        minutes: minutes.value,
        permanent: permanent.checked,
        reason: reason.value,
      },
      ctx.me.permissions,
    );
    if (!req.ok) {
      ctx.say(req.message);
      return;
    }
    for (const b of buttons) {
      b.disabled = true;
    }
    ctx.run(
      (async () => {
        const r = await ctx.api.sanction(req.value);
        if (!r.ok) {
          ctx.fail(r);
          return;
        }
        reason.value = '';
        ctx.say(givenText(kind, kind === 'ban' ? target.account : target.character));
        await given(r.value);
      })().finally(() => {
        for (const b of buttons) {
          b.disabled = false;
        }
      }),
    );
  };
  for (const kind of target.kinds) {
    const b = button(
      kindLabel(kind),
      () => {
        give(kind);
      },
      kind === 'ban' ? 'admin-flag' : '',
    );
    b.dataset.kind = kind;
    buttons.push(b);
    form.append(b);
  }
  form.addEventListener('submit', (ev) => {
    ev.preventDefault();
  });
  return form;
}

/** A lift button with the reason it asks for. */
function liftCell(ctx: Context, s: Sanction, lifted: () => Promise<void>): HTMLElement {
  const cell = el('div', '', 'admin-review');
  if (!canLift(ctx.me.permissions, s)) {
    return cell;
  }
  const reason = input('lift-reason');
  reason.maxLength = REASON_MAX;
  reason.placeholder = 'Reason to lift';
  const lift = button('Lift', () => {
    if (reason.value.trim() === '') {
      ctx.say('Lifting needs a reason.');
      reason.focus();
      return;
    }
    lift.disabled = true;
    ctx.run(
      (async () => {
        const r = await ctx.api.lift(s.id, reason.value);
        if (!r.ok) {
          ctx.fail(r);
          return;
        }
        ctx.say(`The ${s.kind} of ${targetText(s)} is lifted.`);
        await lifted();
      })().finally(() => {
        lift.disabled = false;
      }),
    );
  });
  cell.append(reason, lift);
  return cell;
}

/** A table of sanctions, newest first, with lift buttons. */
export function sanctionTable(
  ctx: Context,
  sanctions: readonly Sanction[],
  lifted: () => Promise<void>,
): { table: HTMLTableElement; body: HTMLTableSectionElement; add: (s: Sanction) => void } {
  const { table: t, body } = table([
    'Kind',
    'Target',
    'Reason',
    'Started',
    'Ends',
    'By',
    'State',
    '',
  ]);
  const add = (s: Sanction): void => {
    const tr = row([
      s.kind,
      targetText(s),
      s.reason,
      when(s.startedAt),
      endText(s),
      s.issuedBy,
      stateText(s),
      liftCell(ctx, s, lifted),
    ]);
    tr.dataset.id = s.id.toString();
    if (s.active) {
      tr.className = 'admin-active';
    }
    body.append(tr);
  };
  for (const s of sanctions) {
    add(s);
  }
  if (sanctions.length === 0) {
    body.append(wideRow('No sanctions.', 8));
  }
  return { table: t, body, add };
}

/** A character's sanctions and its account's bans, with older pages. */
export function sanctionHistory(ctx: Context, characterId: bigint): HTMLElement {
  const div = el('div', '', 'admin-sanctions');
  div.append(el('h3', 'Sanction history'));
  const list = el('div');
  let before = 0n;
  const more = el('button', 'Older sanctions');
  more.type = 'button';
  more.hidden = true;
  let add: ((s: Sanction) => void) | undefined;
  const load = async (fresh: boolean): Promise<void> => {
    const r = await ctx.api.sanctions(
      { character: characterId },
      fresh ? 0n : before,
      SANCTION_PAGE,
    );
    if (!r.ok) {
      ctx.fail(r);
      return;
    }
    if (fresh || add === undefined) {
      const t = sanctionTable(ctx, r.value.sanctions, async () => load(true));
      add = t.add;
      list.replaceChildren(t.table);
    } else {
      for (const s of r.value.sanctions) {
        add(s);
      }
    }
    before = r.value.nextBeforeId;
    more.hidden = before === 0n;
  };
  more.addEventListener('click', () => {
    ctx.run(load(false));
  });
  div.append(list, more);
  ctx.run(load(true));
  return div;
}

/** Renders the list of bans in effect into a section and loads it. */
export function renderBans(ctx: Context, section: HTMLElement): void {
  section.replaceChildren(el('h2', 'Bans in effect'));
  const list = el('div');
  let before = 0n;
  const more = el('button', 'Older bans');
  more.type = 'button';
  more.hidden = true;
  let add: ((s: Sanction) => void) | undefined;
  const load = async (fresh: boolean): Promise<void> => {
    const r = await ctx.api.sanctions(ACTIVE_BANS, fresh ? 0n : before, SANCTION_PAGE);
    if (!r.ok) {
      ctx.fail(r);
      return;
    }
    if (fresh || add === undefined) {
      const t = banTable(ctx, r.value.sanctions, async () => load(true));
      add = t.add;
      list.replaceChildren(t.table);
    } else {
      for (const s of r.value.sanctions) {
        add(s);
      }
    }
    before = r.value.nextBeforeId;
    more.hidden = before === 0n;
  };
  more.addEventListener('click', () => {
    ctx.run(load(false));
  });
  section.append(list, more);
  ctx.run(load(true));
}

/** The table of the bans in effect. */
function banTable(
  ctx: Context,
  bans: readonly Sanction[],
  lifted: () => Promise<void>,
): { table: HTMLTableElement; add: (s: Sanction) => void } {
  const { table: t, body } = table(['Account', 'Reason', 'Started', 'Ends', 'By', '']);
  const add = (s: Sanction): void => {
    const tr = row([
      targetText(s),
      s.reason,
      when(s.startedAt),
      endText(s),
      s.issuedBy,
      liftCell(ctx, s, lifted),
    ]);
    tr.dataset.id = s.id.toString();
    body.append(tr);
  };
  for (const s of bans) {
    add(s);
  }
  if (bans.length === 0) {
    body.append(wideRow('No bans in effect.', 6));
  }
  return { table: t, add };
}
