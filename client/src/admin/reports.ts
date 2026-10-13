// The report queue and the report screen (decision 0030). The screen
// shows the chat lines around the report, the target's earlier reports
// and sanctions, sanction buttons for the target character, and two ways
// to close the report:
//   - "Sanction and close" gives a sanction, then closes the report as
//     sanctioned with that sanction linked.
//   - The plain close form closes with no action, a warning, or a
//     sanction already given.

import type {
  ChatLine,
  ReportDetail,
  ReportSummary,
  Sanction,
} from '../gen/wathena/admin/v1/admin_pb';
import {
  OPEN_REPORTS,
  type Outcome,
  REPORT_KINDS,
  REPORT_PAGE,
  type ReportFilter,
  type SanctionKind,
} from './api';
import type { Context } from './context';
import { button, el, field, input, row, table, when, wideRow } from './dom';
import { REASON_MAX } from './restore';
import {
  characterKindsFor,
  sanctionForm,
  sanctionRequest,
  sanctionTable,
  stateText,
} from './sanctions';

/** The words for a kind of report. */
export function reportKindText(kind: string): string {
  switch (kind) {
    case 'player':
      return 'Player';
    case 'line':
      return 'Chat line';
    case 'guild_name':
      return 'Guild name';
    case 'guild_emblem':
      return 'Guild emblem';
    default:
      return kind;
  }
}

/** The words for an outcome. */
export function outcomeText(outcome: string): string {
  switch (outcome) {
    case 'no_action':
      return 'no action';
    case 'warned':
      return 'warned';
    case 'sanctioned':
      return 'sanctioned';
    default:
      return outcome;
  }
}

/** A report's status, such as "open" or "closed by Gm: warned". */
export function reportStatusText(r: ReportSummary): string {
  if (r.status !== 'closed') {
    return r.status;
  }
  const by = r.closedBy === '' ? '' : ` by ${r.closedBy}`;
  const outcome = r.outcome === '' ? '' : `: ${outcomeText(r.outcome)}`;
  return `closed${by}${outcome}`;
}

/** Where a chat line was said: its kind, its target and the map. */
export function lineWhere(l: ChatLine): string {
  let text = l.kind;
  if (l.target !== '') {
    text += ` to ${l.target}`;
  }
  if (l.map !== '') {
    text += ` on ${l.map}`;
  }
  return text;
}

/** Where a chat line comes from: what the reporter saw, or the target's log. */
export function lineSource(l: ChatLine): string {
  return l.seen ? 'reporter saw' : 'target log';
}

/** Whether a report's target is a character that sanctions can act on. */
export function hasCharacter(r: ReportSummary): boolean {
  return r.targetId !== 0n && (r.kind === 'player' || r.kind === 'line');
}

/** The sanctions a closed report can link: the saved ones. */
export function linkable(sanctions: readonly Sanction[]): Sanction[] {
  return sanctions.filter((s) => s.id !== 0n && s.kind !== '');
}

/**
 * Checks the plain close form. sanctioned needs a sanction to link. The
 * other outcomes link none.
 */
export function closeCheck(
  outcome: string,
  note: string,
  sanctionId: bigint,
):
  | { ok: true; value: { outcome: Outcome; note: string; sanctionId: bigint } }
  | { ok: false; message: string } {
  if (outcome !== 'no_action' && outcome !== 'warned' && outcome !== 'sanctioned') {
    return { ok: false, message: 'Choose an outcome.' };
  }
  const n = note.trim();
  if (n.length > REASON_MAX) {
    return { ok: false, message: 'The note is longer than 255 characters.' };
  }
  if (outcome === 'sanctioned' && sanctionId === 0n) {
    return { ok: false, message: 'Choose the sanction to link, or use Sanction and close.' };
  }
  return {
    ok: true,
    value: { outcome, note: n, sanctionId: outcome === 'sanctioned' ? sanctionId : 0n },
  };
}

/** The filter of the queue, kept while the page stays open. */
let lastFilter: ReportFilter = { ...OPEN_REPORTS };

/** Renders the report queue into a section and loads it. */
export function renderReports(ctx: Context, section: HTMLElement): void {
  section.replaceChildren(el('h2', 'Reports'));
  const form = el('form', '', 'admin-filter');
  const status = el('select');
  status.name = 'status';
  for (const [value, label] of [
    ['open', 'Open'],
    ['closed', 'Closed'],
    ['all', 'All'],
  ] as const) {
    const o = el('option', label);
    o.value = value;
    status.append(o);
  }
  status.value = lastFilter.status === '' ? 'open' : lastFilter.status;
  const kind = el('select');
  kind.name = 'kind';
  const any = el('option', 'Any');
  any.value = '';
  kind.append(any);
  for (const k of REPORT_KINDS) {
    const o = el('option', reportKindText(k));
    o.value = k;
    kind.append(o);
  }
  kind.value = lastFilter.kind;
  const target = input('target');
  target.maxLength = 24;
  target.value = lastFilter.target;
  const go = el('button', 'Search');
  go.type = 'submit';
  form.append(field('Status', status), field('Kind', kind), field('Target name', target), go);

  const { table: t, body } = table([
    'Time',
    'Kind',
    'Target',
    'Reporter',
    'Reason',
    'Map',
    'Status',
  ]);
  let before = 0n;
  const more = el('button', 'Older reports');
  more.type = 'button';
  more.hidden = true;
  const load = async (fresh: boolean): Promise<void> => {
    const r = await ctx.api.reports(lastFilter, fresh ? 0n : before, REPORT_PAGE);
    if (!r.ok) {
      ctx.fail(r);
      return;
    }
    if (fresh) {
      body.replaceChildren();
      if (r.value.reports.length === 0) {
        body.append(wideRow('No reports.', 7));
      }
    }
    for (const rep of r.value.reports) {
      body.append(reportRow(ctx, rep));
    }
    before = r.value.nextBeforeId;
    more.hidden = before === 0n;
  };
  more.addEventListener('click', () => {
    ctx.run(load(false));
  });
  form.addEventListener('submit', (ev) => {
    ev.preventDefault();
    const s = status.value;
    const k = REPORT_KINDS.find((x) => x === kind.value);
    lastFilter = {
      status: s === 'closed' || s === 'all' ? s : 'open',
      kind: k ?? '',
      target: target.value.trim(),
    };
    ctx.run(load(true));
  });
  section.append(form, t, more);
  ctx.run(load(true));
}

/** A row of a report table, whose time opens the report. */
function reportRow(ctx: Context, r: ReportSummary): HTMLTableRowElement {
  const open = button(
    when(r.createdAt) || `Report ${r.id.toString()}`,
    () => {
      ctx.open({ kind: 'report', id: r.id });
    },
    'admin-link',
  );
  open.title = `Report ${r.id.toString()}`;
  const tr = row([
    open,
    reportKindText(r.kind),
    r.target,
    r.reporter,
    r.reason,
    r.map,
    reportStatusText(r),
  ]);
  tr.dataset.id = r.id.toString();
  return tr;
}

/** Renders a report's screen into a section and loads it. */
export function renderReport(ctx: Context, section: HTMLElement, id: bigint): void {
  section.replaceChildren(el('h2', `Report ${id.toString()}`), el('p', 'Loading.'));
  ctx.run(loadReport(ctx, section, id));
}

async function loadReport(ctx: Context, section: HTMLElement, id: bigint): Promise<void> {
  const r = await ctx.api.report(id);
  if (!r.ok) {
    section.replaceChildren(el('h2', `Report ${id.toString()}`), backButton(ctx));
    ctx.fail(r);
    return;
  }
  showReport(ctx, section, r.value, async () => loadReport(ctx, section, id));
}

function backButton(ctx: Context): HTMLButtonElement {
  return button('Back to the reports', () => {
    ctx.open({ kind: 'reports' });
  });
}

/** Facts as a two-column table. */
function facts(pairs: readonly [string, string | Node][]): HTMLTableElement {
  const t = el('table', '', 'admin-facts-table');
  const body = el('tbody');
  for (const [k, v] of pairs) {
    const tr = el('tr');
    const td = el('td');
    td.append(v);
    tr.append(el('th', k), td);
    body.append(tr);
  }
  t.append(body);
  return t;
}

/** The image of a reported emblem. The page's policy allows blob: images. */
function emblemImage(png: Uint8Array): HTMLImageElement {
  const img = el('img', '', 'admin-emblem');
  img.alt = 'The reported emblem';
  const url = URL.createObjectURL(new Blob([new Uint8Array(png)], { type: 'image/png' }));
  const done = (): void => {
    URL.revokeObjectURL(url);
  };
  img.addEventListener('load', done, { once: true });
  img.addEventListener('error', done, { once: true });
  img.src = url;
  return img;
}

function showReport(
  ctx: Context,
  section: HTMLElement,
  d: ReportDetail,
  reload: () => Promise<void>,
): void {
  const r = d.report;
  if (r === undefined) {
    section.replaceChildren(el('h2', 'Report'), backButton(ctx), el('p', 'No such report.'));
    return;
  }
  section.replaceChildren(
    el('h2', `Report ${r.id.toString()}: ${reportKindText(r.kind)} ${r.target}`),
    backButton(ctx),
  );
  let target: string | Node = r.target;
  if (hasCharacter(r) && ctx.can('players.read')) {
    target = button(
      r.target,
      () => {
        ctx.open({ kind: 'character', id: r.targetId });
      },
      'admin-link',
    );
  }
  const rows: [string, string | Node][] = [
    ['Kind', reportKindText(r.kind)],
    ['Target', target],
    ['Reporter', r.reporter],
    ['Reason', r.reason],
    ['Map', r.map],
    ['Time', when(r.createdAt)],
    ['Status', reportStatusText(r)],
  ];
  if (r.status === 'closed') {
    rows.push(['Closed', when(r.closedAt)], ['Note', r.note]);
    if (r.sanctionId !== 0n) {
      rows.push(['Sanction', `#${r.sanctionId.toString()}`]);
    }
  }
  section.append(facts(rows));
  if (d.emblemPng.length > 0) {
    section.append(el('h3', 'Reported emblem'), emblemImage(d.emblemPng));
  }

  section.append(el('h3', 'Chat lines'));
  const lines = table(['', 'Time', 'Source', 'Who', 'Where', 'Text']);
  lines.table.classList.add('admin-chat');
  for (const l of d.lines) {
    const tr = row([
      l.reported ? 'Reported' : '',
      when(l.at),
      lineSource(l),
      l.name,
      lineWhere(l),
      l.text,
    ]);
    tr.dataset.id = l.id;
    tr.className = l.seen ? 'admin-line-seen' : 'admin-line-target';
    if (l.reported) {
      tr.classList.add('admin-reported');
    }
    lines.body.append(tr);
  }
  if (d.lines.length === 0) {
    lines.body.append(wideRow('No chat lines.', 6));
  }
  section.append(lines.table);

  section.append(el('h3', 'Earlier reports of the target'));
  const earlier = table(['Time', 'Kind', 'Target', 'Reporter', 'Reason', 'Map', 'Status']);
  for (const e of d.earlier) {
    earlier.body.append(reportRow(ctx, e));
  }
  if (d.earlier.length === 0) {
    earlier.body.append(wideRow('None.', 7));
  }
  section.append(earlier.table);

  section.append(el('h3', 'Sanctions of the target'));
  section.append(sanctionTable(ctx, d.sanctions, reload).table);

  const kinds = hasCharacter(r) ? characterKindsFor(ctx.me.permissions) : [];
  if (kinds.length > 0) {
    section.append(
      sanctionForm(
        ctx,
        {
          characterId: r.targetId,
          character: r.target,
          accountId: r.targetAccountId,
          account: '',
          kinds,
        },
        reload,
      ),
    );
  }
  if (r.status === 'closed') {
    return;
  }
  if (kinds.length > 0) {
    section.append(sanctionCloseForm(ctx, r, kinds, reload));
  }
  section.append(closeForm(ctx, r, linkable(d.sanctions), reload));
}

/** Gives a sanction and closes the report with it linked, as one action. */
function sanctionCloseForm(
  ctx: Context,
  r: ReportSummary,
  kinds: readonly SanctionKind[],
  reload: () => Promise<void>,
): HTMLElement {
  const form = el('form', '', 'admin-sanction-close');
  form.append(el('h3', 'Sanction and close'));
  const kind = el('select');
  kind.name = 'kind';
  for (const k of kinds) {
    const o = el('option', k);
    o.value = k;
    kind.append(o);
  }
  const minutes = input('minutes');
  minutes.inputMode = 'numeric';
  minutes.size = 8;
  minutes.placeholder = '60';
  const reason = input('reason');
  reason.maxLength = REASON_MAX;
  reason.size = 30;
  reason.placeholder = 'The player sees it';
  const note = input('note');
  note.maxLength = REASON_MAX;
  note.size = 30;
  const submit = el('button', 'Sanction and close');
  submit.type = 'submit';
  form.append(
    field('Sanction', kind),
    field('Minutes, not for a kick', minutes),
    field('Reason', reason),
    field('Note on the report', note),
    submit,
  );
  form.addEventListener('submit', (ev) => {
    ev.preventDefault();
    const k = kinds.find((x) => x === kind.value);
    if (k === undefined) {
      return;
    }
    const req = sanctionRequest(
      {
        kind: k,
        characterId: r.targetId,
        accountId: 0n,
        minutes: minutes.value,
        permanent: false,
        reason: reason.value,
      },
      ctx.me.permissions,
    );
    if (!req.ok) {
      ctx.say(req.message);
      return;
    }
    const n = note.value.trim();
    if (n.length > REASON_MAX) {
      ctx.say('The note is longer than 255 characters.');
      return;
    }
    submit.disabled = true;
    ctx.run(
      (async () => {
        const given = await ctx.api.sanction(req.value);
        if (!given.ok) {
          ctx.fail(given);
          return;
        }
        const s = given.value;
        const closed = await ctx.api.closeReport(r.id, 'sanctioned', n, s.id);
        if (!closed.ok) {
          await reload();
          ctx.fail({
            ...closed,
            message: `The ${k} is given as sanction ${s.id.toString()}, but the report is still open: ${closed.message} Close it with that sanction linked.`,
          });
          return;
        }
        await reload();
        ctx.say(`Report ${r.id.toString()} closed with a ${k}.`);
      })().finally(() => {
        submit.disabled = false;
      }),
    );
  });
  return form;
}

/** Closes the report with no action, a warning, or a sanction already given. */
function closeForm(
  ctx: Context,
  r: ReportSummary,
  sanctions: readonly Sanction[],
  reload: () => Promise<void>,
): HTMLElement {
  const form = el('form', '', 'admin-close');
  form.append(el('h3', 'Close the report'));
  const outcome = el('select');
  outcome.name = 'outcome';
  const outcomes: Outcome[] = ['no_action', 'warned'];
  if (sanctions.length > 0) {
    outcomes.push('sanctioned');
  }
  for (const o of outcomes) {
    const opt = el('option', outcomeText(o));
    opt.value = o;
    outcome.append(opt);
  }
  const link = el('select');
  link.name = 'sanction';
  const none = el('option', 'None');
  none.value = '';
  link.append(none);
  for (const s of sanctions) {
    const o = el('option', `#${s.id.toString()} ${s.kind}: ${s.reason} (${stateText(s)})`);
    o.value = s.id.toString();
    link.append(o);
  }
  const note = input('note');
  note.maxLength = REASON_MAX;
  note.size = 30;
  const submit = el('button', 'Close');
  submit.type = 'submit';
  form.append(field('Outcome', outcome), field('Note', note));
  if (sanctions.length > 0) {
    form.append(field('Sanction to link, for sanctioned', link));
  }
  form.append(submit);
  form.addEventListener('submit', (ev) => {
    ev.preventDefault();
    const id = /^\d+$/.test(link.value) ? BigInt(link.value) : 0n;
    const c = closeCheck(outcome.value, note.value, id);
    if (!c.ok) {
      ctx.say(c.message);
      return;
    }
    submit.disabled = true;
    ctx.run(
      (async () => {
        const res = await ctx.api.closeReport(
          r.id,
          c.value.outcome,
          c.value.note,
          c.value.sanctionId,
        );
        if (!res.ok) {
          ctx.fail(res);
          return;
        }
        await reload();
        ctx.say(`Report ${r.id.toString()} closed: ${outcomeText(c.value.outcome)}.`);
      })().finally(() => {
        submit.disabled = false;
      }),
    );
  });
  return form;
}
