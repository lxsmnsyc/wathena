// The restore form: gives serials, and zeny with items.create, back to
// a character by mail. A restore needs a recent login, so a reauth
// error shows the reauth link.

import { create } from '@bufbuild/protobuf';
import {
  type RestoreRequest,
  RestoreRequestSchema,
  type RestoreResult,
} from '../gen/wathena/admin/v1/admin_pb';
import type { Context } from './context';
import { button, el, field, input, row, table } from './dom';
import { itemText, ownerText } from './format';
import { serialCell } from './history';

/** The text of the restore form's fields. */
export interface RestoreFields {
  characterId: string;
  rows: readonly { serial: string; amount: string }[];
  /** Empty without items.create. */
  zeny: string;
  reason: string;
}

/** The longest reason the server takes. */
export const REASON_MAX = 255;

const INT32_MAX = 2_147_483_647;

/** A whole number of digits, or undefined. */
function whole(text: string): bigint | undefined {
  const t = text.trim();
  return /^\d+$/.test(t) ? BigInt(t) : undefined;
}

/**
 * Checks the form's fields and makes the request. Rows without a serial
 * are left out. An empty amount is 0, the whole stack.
 */
export function restoreRequest(
  f: RestoreFields,
  canCreate: boolean,
): { ok: true; value: RestoreRequest } | { ok: false; message: string } {
  const characterId = whole(f.characterId);
  if (characterId === undefined || characterId === 0n) {
    return { ok: false, message: 'Enter the ID of the character that gets the items.' };
  }
  const items: { serial: string; amount: number }[] = [];
  for (const r of f.rows) {
    const serial = r.serial.trim();
    if (serial === '') {
      continue;
    }
    const amount = r.amount.trim() === '' ? 0n : whole(r.amount);
    if (amount === undefined || amount > BigInt(INT32_MAX)) {
      return { ok: false, message: `The amount of ${serial} is not a number.` };
    }
    items.push({ serial, amount: Number(amount) });
  }
  const zeny = f.zeny.trim() === '' ? 0n : whole(f.zeny);
  if (zeny === undefined) {
    return { ok: false, message: 'The zeny is not a number.' };
  }
  if (zeny > 0n && !canCreate) {
    return { ok: false, message: 'Zeny needs the items.create permission.' };
  }
  if (items.length === 0 && zeny === 0n) {
    return { ok: false, message: 'Add a serial or zeny to restore.' };
  }
  const reason = f.reason.trim();
  if (reason === '') {
    return { ok: false, message: 'A restore needs a reason.' };
  }
  if (reason.length > REASON_MAX) {
    return { ok: false, message: 'The reason is longer than 255 characters.' };
  }
  return {
    ok: true,
    value: create(RestoreRequestSchema, { characterId, items, zeny, reason }),
  };
}

/** Renders the restore form into a section. */
export function renderRestore(
  ctx: Context,
  section: HTMLElement,
  characterId: bigint,
  serials: readonly string[],
): void {
  const canCreate = ctx.can('items.create');
  section.replaceChildren(el('h2', 'Restore'));
  section.append(
    el(
      'p',
      canCreate
        ? 'A serial that still exists moves back from its holder. A serial that is gone is made again as a copy. The items arrive by mail.'
        : 'A serial that still exists moves back from its holder. The items arrive by mail.',
      'admin-who',
    ),
  );
  const form = el('form', '', 'admin-restore');
  const character = input('character');
  character.inputMode = 'numeric';
  character.required = true;
  character.value = characterId === 0n ? '' : characterId.toString();

  const rows = el('div', '', 'admin-rows');
  const rowInputs: { serial: HTMLInputElement; amount: HTMLInputElement }[] = [];
  const addRow = (serial: string): void => {
    const line = el('div', '', 'admin-row');
    const s = input('serial');
    s.size = 40;
    s.value = serial;
    const a = input('amount');
    a.inputMode = 'numeric';
    a.size = 6;
    a.placeholder = 'all';
    const pair = { serial: s, amount: a };
    rowInputs.push(pair);
    const remove = button('Remove', () => {
      rowInputs.splice(rowInputs.indexOf(pair), 1);
      line.remove();
    });
    line.append(field('Serial', s), field('Amount', a), remove);
    rows.append(line);
  };
  for (const s of serials) {
    addRow(s);
  }
  if (serials.length === 0) {
    addRow('');
  }
  const add = button('Add a serial', () => {
    addRow('');
  });

  const zeny = input('zeny');
  zeny.inputMode = 'numeric';
  const reason = input('reason');
  reason.required = true;
  reason.maxLength = REASON_MAX;
  reason.size = 40;
  const submit = el('button', 'Restore');
  submit.type = 'submit';

  form.append(field('Character ID', character), rows, add);
  if (canCreate) {
    form.append(field('Zeny', zeny));
  }
  form.append(field('Reason', reason), submit);

  const result = el('div', '', 'admin-result');
  form.addEventListener('submit', (ev) => {
    ev.preventDefault();
    const req = restoreRequest(
      {
        characterId: character.value,
        rows: rowInputs.map((p) => ({ serial: p.serial.value, amount: p.amount.value })),
        zeny: canCreate ? zeny.value : '',
        reason: reason.value,
      },
      canCreate,
    );
    if (!req.ok) {
      result.replaceChildren(el('p', req.message, 'admin-error'));
      return;
    }
    submit.disabled = true;
    ctx.run(
      send(ctx, result, req.value).finally(() => {
        submit.disabled = false;
      }),
    );
  });
  section.append(form, result);
}

async function send(ctx: Context, out: HTMLElement, req: RestoreRequest): Promise<void> {
  out.replaceChildren(el('p', 'Restoring.'));
  const r = await ctx.api.restore(req);
  if (!r.ok) {
    out.replaceChildren();
    ctx.fail(r);
    return;
  }
  ctx.say(`Restored by mail ${r.value.mailId.toString()}.`);
  showResult(ctx, out, r.value, req.characterId);
}

function showResult(ctx: Context, out: HTMLElement, res: RestoreResult, characterId: bigint): void {
  out.replaceChildren(el('h3', `Mail ${res.mailId.toString()}`));
  if (res.zeny > 0n) {
    out.append(el('p', `Zeny: ${res.zeny.toString()}`));
  }
  if (res.items.length > 0) {
    const { table: t, body } = table(['Serial', 'Item', 'Amount', 'How', 'From', 'New serial']);
    for (const i of res.items) {
      body.append(
        row([
          serialCell(ctx, i.serial, characterId),
          itemText({ name: i.itemName, itemId: i.itemId, refine: 0, cards: [] }),
          String(i.amount),
          i.copy ? 'copy' : 'moved',
          ownerText(i.from),
          serialCell(ctx, i.newSerial, characterId),
        ]),
      );
    }
    out.append(t);
  }
}
