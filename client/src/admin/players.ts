// The player search and the character detail screens. Every serial
// opens the item history with items.history.

import type {
  AccountSummary,
  CharacterDetail,
  Item,
  LogEntry,
} from '../gen/wathena/admin/v1/admin_pb';
import { LOG_PAGE, type LogRange, PLAYER_PAGE, SEARCH_KINDS, type SearchKind } from './api';
import type { Context } from './context';
import { button, el, field, input, row, table, when, wideRow } from './dom';
import {
  carried,
  hasMore,
  itemText,
  ownerText,
  pageCursor,
  positionText,
  sanctionText,
  storageTitle,
  worn,
} from './format';
import { holderCell, serialCell } from './history';

const KIND_LABELS: Record<SearchKind, string> = {
  name: 'Name',
  id: 'Account or character ID',
  serial: 'Item serial',
  ip: 'IP address or range',
};

/** The search kinds a role's permissions offer. */
export function searchKindsFor(permissions: readonly string[]): SearchKind[] {
  return SEARCH_KINDS.filter((k) => k !== 'ip' || permissions.includes('players.read_ip'));
}

/** Renders the player search into a section. */
export function renderPlayers(ctx: Context, section: HTMLElement): void {
  section.replaceChildren(el('h2', 'Players'));
  const form = el('form', '', 'admin-filter');
  const kind = el('select');
  kind.name = 'kind';
  for (const k of searchKindsFor(ctx.me.permissions)) {
    const o = el('option', KIND_LABELS[k]);
    o.value = k;
    kind.append(o);
  }
  const value = input('q');
  value.required = true;
  value.size = 30;
  const go = el('button', 'Search');
  go.type = 'submit';
  form.append(field('Search by', kind), field('Value', value), go);

  const holder = el('p', '', 'admin-holder');
  holder.hidden = true;
  const results = el('div', '', 'admin-results');
  let search: { kind: SearchKind; value: string } = { kind: 'name', value: '' };
  let next = '';
  const more = el('button', 'More');
  more.type = 'button';
  more.hidden = true;

  const load = async (fresh: boolean): Promise<void> => {
    const r = await ctx.api.players(
      search.kind,
      search.value,
      pageCursor(fresh, next),
      PLAYER_PAGE,
    );
    if (!r.ok) {
      ctx.fail(r);
      return;
    }
    if (fresh) {
      results.replaceChildren();
      holder.hidden = search.kind !== 'serial';
      holder.replaceChildren(el('strong', 'Holder: '));
      holder.append(holderCell(ctx, r.value.holder));
      if (r.value.accounts.length === 0) {
        results.append(el('p', 'No players found.'));
      }
    }
    for (const a of r.value.accounts) {
      results.append(accountBlock(ctx, a));
    }
    next = r.value.nextCursor;
    more.hidden = !hasMore(next);
  };
  more.addEventListener('click', () => {
    ctx.run(load(false));
  });

  form.addEventListener('submit', (ev) => {
    ev.preventDefault();
    const k = SEARCH_KINDS.find((s) => s === kind.value) ?? 'name';
    search = { kind: k, value: value.value.trim() };
    if (search.value !== '') {
      ctx.run(load(true));
    }
  });
  section.append(form, holder, results, more);
}

/** One account of the search results, with its characters. */
function accountBlock(ctx: Context, a: AccountSummary): HTMLElement {
  const block = el('article', '', 'admin-account');
  block.dataset.account = a.accountId.toString();
  block.append(el('h3', `${a.account} (account ${a.accountId.toString()})`));
  const lines = [`Created ${when(a.createdAt)}`];
  lines.push(a.lastLogin === undefined ? 'No login yet' : `Last login ${when(a.lastLogin)}`);
  block.append(el('p', lines.join('. '), 'admin-who'));
  const { table: t, body } = table(['Character', 'Job', 'Levels', 'Where', 'Sanctions']);
  for (const c of a.characters) {
    const open = button(
      c.name,
      () => {
        ctx.open({ kind: 'character', id: c.id });
      },
      'admin-link',
    );
    open.title = `Character ${c.id.toString()}`;
    const where = positionText(c.position);
    body.append(
      row([
        open,
        c.jobName,
        `${String(c.baseLevel)} / ${String(c.jobLevel)}`,
        c.online ? `Online at ${where}` : `Offline, saved at ${where}`,
        c.sanctions.map(sanctionText).join('; '),
      ]),
    );
  }
  if (a.characters.length === 0) {
    body.append(wideRow('No characters.', 5));
  }
  block.append(t);
  if (a.logins.length > 0) {
    const logins = table(['Login', 'Address', 'Browser']);
    for (const l of a.logins) {
      logins.body.append(row([when(l.at), l.ip, l.userAgent]));
    }
    block.append(el('h4', 'Matching logins'), logins.table);
  }
  return block;
}

/** The tabs of the character detail screen. */
export type DetailTab = 'overview' | 'inventory' | 'equipment' | 'storage' | 'mail' | 'logs';

const DETAIL_TABS: readonly { tab: DetailTab; label: string }[] = [
  { tab: 'overview', label: 'Overview' },
  { tab: 'inventory', label: 'Inventory' },
  { tab: 'equipment', label: 'Equipment' },
  { tab: 'storage', label: 'Storage' },
  { tab: 'mail', label: 'Mail' },
  { tab: 'logs', label: 'Logs' },
];

/** The tabs of the character detail screen that a role's permissions show. */
export function detailTabsFor(permissions: readonly string[]): DetailTab[] {
  return DETAIL_TABS.map((t) => t.tab).filter(
    (t) => t !== 'logs' || permissions.includes('items.history'),
  );
}

/** Renders a character's details into a section. */
export function renderCharacter(ctx: Context, section: HTMLElement, id: bigint): void {
  section.replaceChildren(el('h2', `Character ${id.toString()}`), el('p', 'Loading.'));
  ctx.run(loadCharacter(ctx, section, id));
}

async function loadCharacter(ctx: Context, section: HTMLElement, id: bigint): Promise<void> {
  const r = await ctx.api.character(id);
  if (!r.ok) {
    section.replaceChildren(el('h2', `Character ${id.toString()}`));
    ctx.fail(r);
    return;
  }
  showCharacter(ctx, section, r.value);
}

function showCharacter(ctx: Context, section: HTMLElement, c: CharacterDetail): void {
  const where = positionText(c.position);
  section.replaceChildren(el('h2', c.name));
  section.append(
    el(
      'p',
      [
        `Character ${c.id.toString()} of ${c.account} (account ${c.accountId.toString()})`,
        `${c.jobName}, base level ${String(c.baseLevel)}, job level ${String(c.jobLevel)}`,
        c.online ? `Online at ${where}` : `Offline, saved at ${where}`,
      ].join('. '),
      'admin-facts',
    ),
  );
  if (ctx.can('items.restore')) {
    section.append(
      button('Restore to this character', () => {
        ctx.open({ kind: 'restore', characterId: c.id, serials: [] });
      }),
    );
  }
  const nav = el('nav', '', 'admin-tabs');
  const panel = el('div', '', 'admin-panel');
  const buttons = new Map<DetailTab, HTMLButtonElement>();
  const show = (tab: DetailTab): void => {
    for (const [t, b] of buttons) {
      if (t === tab) {
        b.setAttribute('aria-current', 'page');
      } else {
        b.removeAttribute('aria-current');
      }
    }
    panel.replaceChildren(detailPanel(ctx, c, tab));
  };
  for (const tab of detailTabsFor(ctx.me.permissions)) {
    const label = DETAIL_TABS.find((t) => t.tab === tab)?.label ?? tab;
    const b = button(label, () => {
      show(tab);
    });
    buttons.set(tab, b);
    nav.append(b);
  }
  section.append(nav, panel);
  show('overview');
}

function detailPanel(ctx: Context, c: CharacterDetail, tab: DetailTab): HTMLElement {
  switch (tab) {
    case 'overview':
      return overview(c);
    case 'inventory':
      return itemTable(ctx, c.id, carried(c.inventory), 'No items.');
    case 'equipment':
      return itemTable(ctx, c.id, worn(c.inventory), 'Nothing worn.');
    case 'storage':
      return storages(ctx, c);
    case 'mail':
      return mails(ctx, c);
    default:
      return logs(ctx, c.id);
  }
}

/** Facts as a two-column table. */
function facts(pairs: readonly [string, string][]): HTMLTableElement {
  const t = el('table', '', 'admin-facts-table');
  const body = el('tbody');
  for (const [k, v] of pairs) {
    const tr = el('tr');
    tr.append(el('th', k), el('td', v));
    body.append(tr);
  }
  t.append(body);
  return t;
}

function overview(c: CharacterDetail): HTMLElement {
  const div = el('div');
  const s = c.stats;
  const stats =
    s === undefined
      ? ''
      : `Str ${String(s.str)}, Agi ${String(s.agi)}, Vit ${String(s.vit)}, Int ${String(s.int)}, Dex ${String(s.dex)}, Luk ${String(s.luk)}`;
  div.append(
    facts([
      ['Job', `${c.jobName} (${String(c.job)})`],
      ['Sex', c.male ? 'Male' : 'Female'],
      ['Base level', `${String(c.baseLevel)}, ${c.baseExp.toString()} exp`],
      ['Job level', `${String(c.jobLevel)}, ${c.jobExp.toString()} exp`],
      ['Stats', stats],
      ['Status points', String(c.statusPoints)],
      ['Skill points', String(c.skillPoints)],
      ['HP / SP', `${c.hp.toString()} / ${c.sp.toString()}`],
      ['Zeny', c.zeny.toString()],
      ['Position', positionText(c.position)],
      ['Save point', positionText(c.savePoint)],
      ['Party', c.party],
      ['Guild', c.guild],
      ['Created', when(c.createdAt)],
    ]),
  );
  div.append(el('h3', 'Currencies'));
  if (c.currencies.length === 0) {
    div.append(el('p', 'None.'));
  } else {
    const { table: t, body } = table(['Currency', 'Amount', 'Held by']);
    for (const cur of c.currencies) {
      body.append(
        row([
          cur.name === '' ? `currency ${String(cur.id)}` : cur.name,
          cur.amount.toString(),
          cur.account ? 'account' : 'character',
        ]),
      );
    }
    div.append(t);
  }
  div.append(el('h3', 'Sanctions'));
  if (c.sanctions.length === 0) {
    div.append(el('p', 'None in effect.'));
  } else {
    const { table: t, body } = table(['Kind', 'Reason', 'Started', 'Ends', 'By']);
    for (const s2 of c.sanctions) {
      body.append(row([s2.kind, s2.reason, when(s2.startedAt), when(s2.endsAt), s2.issuedBy]));
    }
    div.append(t);
  }
  return div;
}

/** Rolls as name=value pairs, sorted by name. */
export function rollsText(rolls: Readonly<Record<string, number>>): string {
  return Object.keys(rolls)
    .sort()
    .map((k) => `${k}=${String(rolls[k])}`)
    .join(' ');
}

function itemTable(
  ctx: Context,
  characterId: bigint,
  items: readonly Item[],
  none: string,
): HTMLElement {
  const { table: t, body } = table(['Item', 'Amount', 'Worn', 'Bound', 'Rolls', 'Serial']);
  for (const i of items) {
    body.append(
      row([
        itemText(i),
        String(i.amount),
        i.equip === 0 ? '' : `0x${i.equip.toString(16)}`,
        i.bound === 0 ? '' : String(i.bound),
        rollsText(i.rolls),
        serialCell(ctx, i.serial, characterId),
      ]),
    );
  }
  if (items.length === 0) {
    body.append(wideRow(none, 6));
  }
  return t;
}

function storages(ctx: Context, c: CharacterDetail): HTMLElement {
  const div = el('div');
  if (c.storages.length === 0) {
    div.append(el('p', 'No storages.'));
  }
  for (const s of c.storages) {
    div.append(el('h3', storageTitle(s)), itemTable(ctx, c.id, s.items, 'Empty.'));
  }
  return div;
}

function mails(ctx: Context, c: CharacterDetail): HTMLElement {
  const { table: t, body } = table(['Sent', 'From', 'Title', 'Zeny', 'State', 'Items']);
  for (const m of c.mails) {
    const items = el('ul', '', 'admin-mail-items');
    for (const i of m.items) {
      const li = el('li', `${itemText(i)} x${String(i.amount)} `);
      li.append(serialCell(ctx, i.serial, c.id));
      items.append(li);
    }
    const state = [m.read ? 'read' : 'unread'];
    if (m.returned) {
      state.push('returned');
    }
    const tr = row([when(m.sentAt), m.from, m.title, m.zeny.toString(), state.join(', '), items]);
    tr.dataset.id = m.id.toString();
    body.append(tr);
  }
  if (c.mails.length === 0) {
    body.append(wideRow('No mail.', 6));
  }
  return t;
}

/** The amount of a log line, with the currency for a zeny line. */
export function logAmount(e: LogEntry): string {
  if (e.kind !== 'zeny') {
    return e.amount.toString();
  }
  return `${e.amount.toString()} ${e.currencyName === '' ? 'zeny' : e.currencyName}`;
}

function logs(ctx: Context, characterId: bigint): HTMLElement {
  const div = el('div');
  const form = el('form', '', 'admin-filter');
  const since = input('since', 'date');
  const until = input('until', 'date');
  const go = el('button', 'Show');
  go.type = 'submit';
  form.append(field('From', since), field('Before', until), go);
  const { table: t, body } = table([
    'Time',
    'Kind',
    'Action',
    'From',
    'To',
    'Amount',
    'Item',
    'Serial',
    'Source',
  ]);
  let range: LogRange = { since: '', until: '' };
  let next = '';
  const more = el('button', 'Older rows');
  more.type = 'button';
  more.hidden = true;
  const load = async (fresh: boolean): Promise<void> => {
    const r = await ctx.api.logs(characterId, range, pageCursor(fresh, next), LOG_PAGE);
    if (!r.ok) {
      ctx.fail(r);
      return;
    }
    if (fresh) {
      body.replaceChildren();
      if (r.value.entries.length === 0) {
        body.append(wideRow('No rows.', 9));
      }
    }
    for (const e of r.value.entries) {
      const tr = row([
        when(e.at),
        e.kind,
        e.action,
        ownerText(e.from),
        ownerText(e.to),
        logAmount(e),
        e.kind === 'zeny'
          ? ''
          : itemText({ name: e.itemName, itemId: e.itemId, refine: e.refine, cards: e.cards }),
        serialCell(ctx, e.serial, characterId),
        serialCell(ctx, e.source, characterId),
      ]);
      tr.dataset.id = e.id.toString();
      body.append(tr);
    }
    next = r.value.nextCursor;
    more.hidden = !hasMore(next);
  };
  more.addEventListener('click', () => {
    ctx.run(load(false));
  });
  form.addEventListener('submit', (ev) => {
    ev.preventDefault();
    range = { since: since.value, until: until.value };
    ctx.run(load(true));
  });
  div.append(form, t, more);
  ctx.run(load(true));
  return div;
}
