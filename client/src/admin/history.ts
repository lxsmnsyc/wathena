// The item history screen: every move of a serial, of the stacks it
// came from and of the stacks it went into, oldest first, and where the
// serial is now.

import type { ItemHistory, Owner } from '../gen/wathena/admin/v1/admin_pb';
import type { Context } from './context';
import { button, el, field, input, row, table, when, wideRow } from './dom';
import { itemText, ownerCharacter, ownerText, relationOf, relationText } from './format';

/**
 * A serial in a table: a button that opens its history with
 * items.history, else plain text. characterId is the character the
 * screen shows, or 0.
 */
export function serialCell(ctx: Context, serial: string, characterId: bigint): Node {
  if (serial === '' || !ctx.can('items.history')) {
    return el('code', serial);
  }
  const b = button(
    serial,
    () => {
      ctx.open({ kind: 'item', serial, characterId });
    },
    'admin-link admin-serial',
  );
  b.title = 'Item history';
  return b;
}

/**
 * Where an item is: a button that opens the holder's character with
 * players.read, else text. An unset owner is "gone".
 */
export function holderCell(ctx: Context, holder: Owner | undefined): Node {
  if (holder === undefined) {
    return el('span', 'gone', 'admin-gone');
  }
  const id = ownerCharacter(holder);
  if (id === 0n || !ctx.can('players.read')) {
    return document.createTextNode(ownerText(holder));
  }
  return button(
    ownerText(holder),
    () => {
      ctx.open({ kind: 'character', id });
    },
    'admin-link',
  );
}

/** Renders the item history screen into a section. */
export function renderHistory(
  ctx: Context,
  section: HTMLElement,
  serial: string,
  characterId: bigint,
): void {
  section.replaceChildren(el('h2', 'Item history'));
  const form = el('form', '', 'admin-filter');
  const box = input('serial');
  box.required = true;
  box.placeholder = '00000000-0000-0000-0000-000000000000';
  box.size = 40;
  box.value = serial;
  const go = el('button', 'Show history');
  go.type = 'submit';
  form.append(field('Serial', box), go);
  const result = el('div');
  form.addEventListener('submit', (ev) => {
    ev.preventDefault();
    const s = box.value.trim();
    if (s !== '') {
      ctx.run(loadHistory(ctx, result, s, characterId));
    }
  });
  section.append(form, result);
  if (serial !== '') {
    ctx.run(loadHistory(ctx, result, serial, characterId));
  }
}

async function loadHistory(
  ctx: Context,
  out: HTMLElement,
  serial: string,
  characterId: bigint,
): Promise<void> {
  out.replaceChildren(el('p', 'Loading.'));
  const r = await ctx.api.history(serial);
  if (!r.ok) {
    out.replaceChildren();
    ctx.fail(r);
    return;
  }
  showHistory(ctx, out, r.value, characterId);
}

function showHistory(ctx: Context, out: HTMLElement, h: ItemHistory, characterId: bigint): void {
  out.replaceChildren();
  const now = el('p', '', 'admin-holder');
  now.append(el('strong', 'Now: '));
  now.append(holderCell(ctx, h.holder));
  out.append(now);
  if (ctx.can('items.restore')) {
    out.append(
      button('Restore this serial', () => {
        ctx.open({ kind: 'restore', characterId, serials: [h.serial] });
      }),
    );
  }
  if (h.serials.length > 1) {
    out.append(el('p', `Serials followed: ${h.serials.join(', ')}`, 'admin-who'));
  }
  const { table: t, body } = table([
    'Time',
    'Relation',
    'Action',
    'From',
    'To',
    'Amount',
    'Item',
    'Serial',
    'Source',
  ]);
  for (const m of h.moves) {
    const tr = row([
      when(m.at),
      relationText(m.relation),
      m.action,
      ownerText(m.from),
      ownerText(m.to),
      String(m.amount),
      itemText({ name: m.itemName, itemId: m.itemId, refine: m.refine, cards: m.cards }),
      m.serial === h.serial ? el('code', m.serial) : serialCell(ctx, m.serial, characterId),
      m.source === '' || m.source === h.serial
        ? el('code', m.source)
        : serialCell(ctx, m.source, characterId),
    ]);
    tr.className = `admin-move admin-${relationOf(m.relation)}`;
    tr.dataset.id = m.id.toString();
    body.append(tr);
  }
  if (h.moves.length === 0) {
    body.append(wideRow('No moves.', 9));
  }
  out.append(t);
  if (h.truncated) {
    out.append(el('p', 'The history stopped at its limits. Older or later moves are left out.'));
  }
}
