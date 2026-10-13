// The review queue: audit rows that need a review and have none yet.
// With audit.review each row has ok and flag buttons and a note. No one
// reviews their own row, so those rows have no buttons.

import type { AuditEntry, Me } from '../gen/wathena/admin/v1/admin_pb';
import { REVIEW_QUEUE } from './api';
import { argumentsText } from './audit';
import { type Context, isSelf } from './context';
import { button, el, input, row, table, when, wideRow } from './dom';

/** Rows per page of the review queue. */
export const REVIEW_PAGE = 50;

/** Whether the caller may review an audit row. */
export function canReview(me: Me, entry: AuditEntry): boolean {
  return (
    me.permissions.includes('audit.review') &&
    entry.needsReview &&
    entry.reviewedAt === undefined &&
    !isSelf(me, entry.account)
  );
}

/** Renders the review queue into a section and loads it. */
export function renderReviews(ctx: Context, section: HTMLElement): void {
  section.replaceChildren(el('h2', 'Review queue'));
  if (!ctx.can('audit.review')) {
    section.append(el('p', 'Your role can see the queue but not review it.', 'admin-who'));
  }
  const { table: t, body } = table([
    'Time',
    'Staff',
    'Action',
    'Target',
    'Arguments',
    'Reason',
    'Review',
  ]);
  let before = 0n;
  const more = el('button', 'Older rows');
  more.type = 'button';
  more.hidden = true;
  const load = async (fresh: boolean): Promise<void> => {
    const r = await ctx.api.audit(REVIEW_QUEUE, fresh ? 0n : before, REVIEW_PAGE);
    if (!r.ok) {
      ctx.fail(r);
      return;
    }
    if (fresh) {
      body.replaceChildren();
      if (r.value.entries.length === 0) {
        body.append(wideRow('Nothing waits for a review.', 7));
      }
    }
    for (const e of r.value.entries) {
      body.append(reviewRow(ctx, e, async () => load(true)));
    }
    before = r.value.nextBeforeId;
    more.hidden = before === 0n;
  };
  more.addEventListener('click', () => {
    ctx.run(load(false));
  });
  section.append(t, more);
  ctx.run(load(true));
}

function reviewRow(ctx: Context, e: AuditEntry, reload: () => Promise<void>): HTMLElement {
  const cell = el('div', '', 'admin-review');
  if (canReview(ctx.me, e)) {
    const note = input('note');
    note.maxLength = 255;
    note.placeholder = 'Note, needed for a flag';
    const send = (verdict: 'ok' | 'flag'): void => {
      if (verdict === 'flag' && note.value.trim() === '') {
        ctx.say('A flag needs a note.');
        note.focus();
        return;
      }
      ctx.run(
        (async () => {
          const r = await ctx.api.review(e.id, verdict, note.value.trim());
          if (!r.ok) {
            ctx.fail(r);
            return;
          }
          ctx.say(`Row ${e.id.toString()} reviewed: ${verdict}.`);
          await reload();
        })(),
      );
    };
    cell.append(
      note,
      button('OK', () => {
        send('ok');
      }),
      button(
        'Flag',
        () => {
          send('flag');
        },
        'admin-flag',
      ),
    );
  } else if (isSelf(ctx.me, e.account)) {
    cell.append('Your own action');
  }
  const tr = row([
    when(e.at),
    `${e.account} (${e.role})`,
    e.action,
    e.target,
    argumentsText(e),
    e.reason,
    cell,
  ]);
  tr.dataset.id = e.id.toString();
  return tr;
}
