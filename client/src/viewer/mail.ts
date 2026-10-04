// The mail window, as RO's RODEX window. The Mail button or Alt+M opens
// the mailbox, and the server answers with the list of mails, newest
// first. A click on a mail reads it: its text, its zeny and its
// attachments, with buttons to take them and to delete the mail once it
// is empty. The Write tab writes a mail to a character by name, with
// zeny and up to Limits.mail_items items from the inventory. It shows an
// estimate of the fee, from rAthena's default ruleset. A new mail shows a
// line in the chat log and a count on the Mail button. User text is only
// ever set with textContent.

import {
  ItemBound,
  ItemRestriction,
  MailOutcome,
  type MailSummary,
} from '../gen/wathena/v1/event_pb';
import {
  button,
  bytesProblem,
  dateText,
  element,
  field,
  keepKeys,
  numberInput,
  parseWhole,
  textInput,
  utf8Length,
  zenyLabel,
} from './forms';
import { type ItemIcons, type Stack, itemRuleLines, stackDetails, stackRow } from './inventory';
import type { MailView } from './online';
import { stackLabel } from './equipment';
import type { ItemTexts } from './itemText';

/** The longest title, in bytes, as rAthena's RODEX_TITLE_LENGTH - 1. */
export const MAIL_TITLE_MAX = 39;
/** The longest text, in bytes, as rAthena's RODEX_BODY_LENGTH - 1. */
export const MAIL_BODY_MAX = 499;
/** rAthena's mail_zeny_fee: the percent of the zeny sent that the fee takes. */
export const MAIL_ZENY_FEE = 2;
/** rAthena's mail_attachment_price: the fee for each attached item. */
export const MAIL_ATTACHMENT_PRICE = 2500;
/** The most zeny a mail holds, as rAthena's MAX_ZENY. */
export const MAIL_MAX_ZENY = 1_000_000_000;

/**
 * The fee of a mail by rAthena's defaults: 2% of the zeny, rounded down,
 * and 2500 zeny for each attached item. The server's ruleset may differ.
 */
export function mailFee(zeny: number, items: number): number {
  return Math.floor((zeny * MAIL_ZENY_FEE) / 100) + MAIL_ATTACHMENT_PRICE * items;
}

/** The fee line of the Write tab. */
export function mailFeeText(zeny: number, items: number): string {
  const fee = mailFee(zeny, items);
  return `Fee about ${zenyLabel(fee)}, total about ${zenyLabel(zeny + fee)} (estimate, the server's rules may differ)`;
}

/** An attachment of a mail being written. */
export interface Attachment {
  slot: number;
  amount: number;
}

/** A mail being written. */
export interface MailDraft {
  to: string;
  title: string;
  body: string;
  zeny: number;
  items: readonly Attachment[];
}

/** Why an inventory stack cannot go into a mail, or undefined when it can. */
export function attachProblem(s: Stack): string | undefined {
  if (s.equip !== 0) {
    return 'An equipped item cannot be mailed.';
  }
  if ((s.restrictions & ItemRestriction.NO_MAIL) !== 0) {
    return 'This item cannot be mailed.';
  }
  if (s.bound !== ItemBound.UNSPECIFIED) {
    return 'A bound item cannot be mailed.';
  }
  return undefined;
}

/** What the Write tab checks before it sends. */
export interface MailSendContext {
  /** The most items one mail holds. */
  maxItems: number;
  /** The zeny the player has. */
  zeny: number;
  inventory: (slot: number) => Stack | undefined;
}

/** Why a mail cannot be sent, or undefined when it can. */
export function mailDraftProblem(d: MailDraft, c: MailSendContext): string | undefined {
  if (d.to.trim() === '') {
    return "Enter the recipient's name.";
  }
  const title = bytesProblem('The title', d.title, 1, MAIL_TITLE_MAX);
  if (title !== undefined) {
    return title;
  }
  const body = bytesProblem('The text', d.body, 0, MAIL_BODY_MAX);
  if (body !== undefined) {
    return body;
  }
  if (d.items.length > c.maxItems) {
    return `A mail holds at most ${String(c.maxItems)} items.`;
  }
  const seen = new Set<number>();
  for (const a of d.items) {
    const s = c.inventory(a.slot);
    if (s === undefined || seen.has(a.slot)) {
      return 'An attachment is no longer in your inventory.';
    }
    seen.add(a.slot);
    const problem = attachProblem(s);
    if (problem !== undefined) {
      return problem;
    }
    if (a.amount < 1 || a.amount > s.amount) {
      return `You have ${String(s.amount)} of that item.`;
    }
  }
  if (d.zeny < 0 || d.zeny > MAIL_MAX_ZENY) {
    return 'That amount of zeny cannot be mailed.';
  }
  if (d.zeny + mailFee(d.zeny, d.items.length) > c.zeny) {
    return 'You do not have enough zeny for the zeny and the fee.';
  }
  return undefined;
}

/** What to tell the player about a mail action. message is a mod's reason. */
export function mailMessage(o: MailOutcome, message = ''): string {
  switch (o) {
    case MailOutcome.SENT:
      return 'The mail was sent.';
    case MailOutcome.FAILED:
      return 'The mail could not be sent.';
    case MailOutcome.DAILY_LIMIT:
      return 'You cannot send more mails today.';
    case MailOutcome.NO_RECIPIENT:
      return 'No character has that name.';
    case MailOutcome.MAILBOX_FULL:
      return "The recipient's mailbox is full.";
    case MailOutcome.TOO_FAST:
      return 'You are sending mails too fast. Wait a moment.';
    case MailOutcome.ZENY:
      return 'You do not have enough zeny for the zeny and the fee.';
    case MailOutcome.RESTRICTED:
      return 'An item cannot be mailed.';
    case MailOutcome.TOO_MANY:
      return 'The mail holds too many items.';
    case MailOutcome.TOO_HEAVY:
      return 'The attachments are too heavy.';
    case MailOutcome.HOOK:
      return message === '' ? 'The mail is not allowed.' : message;
    case MailOutcome.TAKEN:
      return 'You took the attachments.';
    case MailOutcome.CANNOT_CARRY:
      return 'You cannot carry the attachments.';
    case MailOutcome.DELETED:
      return 'The mail was deleted.';
    case MailOutcome.NOT_EMPTY:
      return 'Take the attachments before you delete the mail.';
    case MailOutcome.BUSY:
      return 'You cannot use mail while trading, vending or using a storage.';
    case MailOutcome.NOT_FOUND:
      return 'That mail is gone.';
    default:
      return 'The mail action did nothing.';
  }
}

/** The chat log line for a new mail. */
export function mailNewText(from: string, title: string): string {
  return `You have new mail from ${from}: ${title}`;
}

/** The Mail button's label, with the unread count when there is one. */
export function mailButtonText(unread: number): string {
  return unread > 0 ? `Mail (${String(unread)})` : 'Mail';
}

/** How many mails are not read. */
export function unreadCount(mails: readonly Pick<MailSummary, 'read'>[]): number {
  return mails.filter((m) => !m.read).length;
}

/** Mails newest first. */
export function newestFirst<T extends Pick<MailSummary, 'sentMs'>>(mails: readonly T[]): T[] {
  return [...mails].sort((a, b) => {
    if (a.sentMs === b.sentMs) {
      return 0;
    }
    return a.sentMs > b.sentMs ? -1 : 1;
  });
}

/** The mailbox line, such as "12 / 30 mails · Sent today 3 / 10". */
export function mailboxText(count: number, size: number, sentToday: number, daily: number): string {
  const sent =
    daily === 0
      ? `Sent today ${String(sentToday)}`
      : `Sent today ${String(sentToday)} / ${String(daily)}`;
  return `${String(count)} / ${String(size)} mails · ${sent}`;
}

/** Whether a mail can be deleted: it holds no zeny and no items. */
export function canDelete(m: Pick<MailSummary, 'zeny' | 'items'>): boolean {
  return m.zeny === 0 && m.items === 0;
}

/** What a mail holds, such as "1,000 z, 2 items", or empty. */
export function mailContentsText(m: Pick<MailSummary, 'zeny' | 'items'>): string {
  const parts: string[] = [];
  if (m.zeny > 0) {
    parts.push(zenyLabel(m.zeny));
  }
  if (m.items > 0) {
    parts.push(m.items === 1 ? '1 item' : `${String(m.items)} items`);
  }
  return parts.join(', ');
}

/** A byte counter, such as "12 / 39 bytes". */
export function bytesText(text: string, max: number): string {
  return `${String(utf8Length(text))} / ${String(max)} bytes`;
}

/** What the mail window asks the server to do. */
export interface MailActions {
  read: (id: bigint) => void;
  send: (draft: MailDraft) => void;
  take: (id: bigint, zeny: boolean, items: boolean) => void;
  remove: (id: bigint) => void;
  close: () => void;
}

/** What the mail window reads. */
export interface MailSource {
  inventory: (slot: number) => Stack | undefined;
  /** The inventory's stacks, in slot order. */
  stacks: () => readonly Stack[];
  zeny: () => number;
  /** The most items one mail holds. */
  maxItems: () => number;
}

/**
 * The window on the page, section#mail-window. It holds h2, the Inbox
 * and Write tabs, div.mail-inbox with output.mail-info and ul.mail-list,
 * div.mail-read for the mail being read, and form.mail-write.
 */
export class MailWindow {
  private readonly inboxTab = button('Inbox', () => {
    this.showTab('inbox');
  });
  private readonly writeTab = button('Write', () => {
    this.showTab('write');
  });
  private readonly inbox = element('div', 'mail-inbox');
  private readonly info = element('output', 'mail-info');
  private readonly list = element('ul', 'mail-list');
  private readonly reader = element('div', 'mail-read');
  private readonly readTitle = element('h3');
  private readonly readMeta = element('div', 'mail-meta');
  private readonly readBody = element('div', 'mail-body');
  private readonly readZeny = element('div', 'mail-zeny');
  private readonly readItems = element('ul', 'mail-items');
  private readonly takeZeny = button('Take zeny', () => {
    this.take(true, false);
  });
  private readonly takeItems = button('Take items', () => {
    this.take(false, true);
  });
  private readonly takeAll = button('Take all', () => {
    this.take(true, true);
  });
  private readonly deleteButton = button('Delete', () => {
    if (this.reading !== undefined) {
      this.actions.remove(this.reading.mail.id);
    }
  });
  private readonly form = element('form', 'mail-write');
  private readonly to = textInput('Character name', 16);
  private readonly title = textInput('Title', 24);
  private readonly titleCount = element('output', 'mail-count');
  private readonly body = document.createElement('textarea');
  private readonly bodyCount = element('output', 'mail-count');
  private readonly zeny = numberInput('0', 10);
  private readonly attachList = element('ul', 'mail-attachments');
  private readonly picker = document.createElement('select');
  private readonly pickAmount = numberInput('1', 5);
  private readonly fee = element('output', 'mail-fee');
  private readonly problem = element('output', 'mail-problem');
  private mails: MailSummary[] = [];
  private size = 0;
  private reading: MailView | undefined;
  private attachments: Attachment[] = [];

  constructor(
    private readonly root: HTMLElement,
    private readonly source: MailSource,
    private readonly actions: MailActions,
    private readonly texts: ItemTexts,
    private readonly icons: ItemIcons | undefined,
  ) {
    const head = element('div', 'mail-head');
    const tabs = element('div', 'mail-tabs');
    this.inboxTab.setAttribute('aria-pressed', 'true');
    this.writeTab.setAttribute('aria-pressed', 'false');
    tabs.append(this.inboxTab, this.writeTab);
    head.append(
      element('h2', '', 'Mail'),
      tabs,
      button('Close', () => {
        this.requestClose();
      }),
    );

    this.list.setAttribute('aria-label', 'Mails');
    const back = button('Back', () => {
      this.reading = undefined;
      this.render();
    });
    const readButtons = element('div', 'mail-buttons');
    readButtons.append(this.takeZeny, this.takeItems, this.takeAll, this.deleteButton, back);
    this.readItems.setAttribute('aria-label', 'Attachments');
    this.reader.append(
      this.readTitle,
      this.readMeta,
      this.readBody,
      this.readZeny,
      this.readItems,
      readButtons,
    );
    this.reader.hidden = true;
    this.inbox.append(this.info, this.list, this.reader);

    this.title.maxLength = MAIL_TITLE_MAX;
    this.body.rows = 5;
    this.body.maxLength = MAIL_BODY_MAX;
    this.body.placeholder = 'Text';
    this.picker.setAttribute('aria-label', 'Item to attach');
    const attachRow = element('div', 'mail-attach');
    attachRow.append(
      this.picker,
      field('Amount', this.pickAmount),
      button('Attach', () => {
        this.attachPicked();
      }),
    );
    const send = document.createElement('button');
    send.type = 'submit';
    send.textContent = 'Send';
    this.attachList.setAttribute('aria-label', 'Attached items');
    this.form.append(
      field('To', this.to),
      field('Title', this.title),
      this.titleCount,
      field('Text', this.body),
      this.bodyCount,
      field('Zeny', this.zeny),
      element('div', 'mail-attach-head', 'Attachments'),
      this.attachList,
      attachRow,
      this.fee,
      this.problem,
      send,
    );
    this.form.hidden = true;
    this.form.addEventListener('input', () => {
      this.renderWrite();
    });
    this.form.addEventListener('submit', (e) => {
      e.preventDefault();
      this.send();
    });
    keepKeys(root, () => {
      this.requestClose();
    });
    root.replaceChildren(head, this.inbox, this.form);
    root.hidden = true;
    texts.onLoad(() => {
      if (this.open) {
        this.render();
      }
    });
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /** Whether the Write tab shows, which takes items from the inventory. */
  get writing(): boolean {
    return this.open && !this.form.hidden;
  }

  /** Shows the mailbox, as MailBox. It comes again after each change. */
  showBox(mails: readonly MailSummary[], size: number, sentToday: number, daily: number): void {
    this.mails = newestFirst(mails);
    this.size = size;
    this.info.textContent = mailboxText(mails.length, size, sentToday, daily);
    const r = this.reading;
    if (r !== undefined) {
      // The mail being read keeps up with the list: taken attachments
      // go, and a deleted mail closes.
      const now = this.mails.find((m) => m.id === r.mail.id);
      if (now === undefined) {
        this.reading = undefined;
      } else {
        this.reading = { ...r, mail: now, items: now.items === 0 ? [] : r.items };
      }
    }
    if (this.root.hidden) {
      this.root.hidden = false;
      this.showTab('inbox');
      return;
    }
    this.render();
  }

  /** Shows a mail, as MailRead. */
  showMail(m: MailView): void {
    this.reading = m;
    const i = this.mails.findIndex((x) => x.id === m.mail.id);
    if (i >= 0) {
      this.mails[i] = { ...m.mail, read: true };
    }
    this.root.hidden = false;
    this.showTab('inbox');
  }

  /** The mails as the window last showed them. */
  get unread(): number {
    return unreadCount(this.mails);
  }

  /** Clears the Write tab after the server sent the mail. */
  sent(): void {
    this.to.value = '';
    this.title.value = '';
    this.body.value = '';
    this.zeny.value = '';
    this.attachments = [];
    this.renderWrite();
  }

  /** Hides the window without telling the server. */
  hide(): void {
    this.root.hidden = true;
    this.reading = undefined;
  }

  /** Closes the window and tells the server. */
  requestClose(): void {
    if (this.root.hidden) {
      return;
    }
    this.hide();
    this.actions.close();
  }

  /**
   * Attaches an inventory slot in the Write tab, such as on a right click
   * on an inventory row: the whole stack, which the amount can change.
   */
  requestAttach(slot: number): void {
    const s = this.source.inventory(slot);
    if (s === undefined || !this.writing) {
      return;
    }
    this.attach(s, s.amount);
  }

  private showTab(tab: 'inbox' | 'write'): void {
    this.inbox.hidden = tab !== 'inbox';
    this.form.hidden = tab !== 'write';
    this.inboxTab.setAttribute('aria-pressed', String(tab === 'inbox'));
    this.writeTab.setAttribute('aria-pressed', String(tab === 'write'));
    this.render();
  }

  private readonly label = (id: number): string => this.texts.label(id);

  /** Draws the shown tab again. */
  render(): void {
    if (!this.inbox.hidden) {
      this.renderInbox();
    }
    if (!this.form.hidden) {
      this.renderWrite();
    }
  }

  private renderInbox(): void {
    const r = this.reading;
    this.list.hidden = r !== undefined;
    this.reader.hidden = r === undefined;
    if (r === undefined) {
      const rows = this.mails.map((m) => this.mailRow(m));
      if (rows.length === 0) {
        rows.push(element('li', 'empty', 'No mails.'));
      }
      this.list.replaceChildren(...rows);
      return;
    }
    const m = r.mail;
    this.readTitle.textContent = m.title;
    const meta = [`From ${m.from}`, dateText(m.sentMs)];
    if (m.returned) {
      meta.push('Returned');
    }
    if (m.expiresMs !== 0n) {
      meta.push(`Expires ${dateText(m.expiresMs)}`);
    }
    this.readMeta.textContent = meta.join(' · ');
    this.readBody.textContent = r.body;
    this.readZeny.textContent = m.zeny > 0 ? `Zeny ${zenyLabel(m.zeny)}` : '';
    this.readZeny.hidden = m.zeny === 0;
    this.readItems.replaceChildren(...r.items.map((s) => stackRow(s, this.label, this.icons)));
    this.readItems.hidden = r.items.length === 0;
    this.takeZeny.disabled = m.zeny === 0;
    this.takeItems.disabled = m.items === 0 && r.items.length === 0;
    this.takeAll.disabled = this.takeZeny.disabled && this.takeItems.disabled;
    this.deleteButton.disabled = !canDelete(m) || r.items.length > 0;
    this.deleteButton.title = this.deleteButton.disabled
      ? 'Take the attachments first'
      : 'Delete the mail';
  }

  private mailRow(m: MailSummary): HTMLLIElement {
    const li = document.createElement('li');
    li.classList.toggle('unread', !m.read);
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'mail-row';
    const mark = element('span', 'mail-mark', m.read ? '' : '●');
    mark.title = m.read ? '' : 'Not read';
    const from = element('bdi', 'mail-from');
    from.textContent = m.from;
    const title = element('bdi', 'mail-title');
    title.textContent = m.title;
    const date = element('span', 'mail-date', dateText(m.sentMs));
    const parts: string[] = [];
    const contents = mailContentsText(m);
    if (contents !== '') {
      parts.push(contents);
    }
    if (m.returned) {
      parts.push('Returned');
    }
    if (m.expiresMs !== 0n) {
      parts.push(`Expires ${dateText(m.expiresMs)}`);
    }
    const extra = element('span', 'mail-extra', parts.join(' · '));
    b.append(mark, from, title, date, extra);
    b.addEventListener('click', () => {
      this.actions.read(m.id);
    });
    li.append(b);
    return li;
  }

  private take(zeny: boolean, items: boolean): void {
    if (this.reading !== undefined) {
      this.actions.take(this.reading.mail.id, zeny, items);
    }
  }

  private draft(): MailDraft | undefined {
    const zenyText = this.zeny.value.trim();
    const zeny = zenyText === '' ? 0 : parseWhole(zenyText, 0, MAIL_MAX_ZENY);
    if (zeny === undefined) {
      return undefined;
    }
    return {
      to: this.to.value.trim(),
      title: this.title.value,
      body: this.body.value,
      zeny,
      items: this.attachments,
    };
  }

  private context(): MailSendContext {
    return {
      maxItems: this.source.maxItems(),
      zeny: this.source.zeny(),
      inventory: this.source.inventory,
    };
  }

  private renderWrite(): void {
    // Attachments that left the inventory, or shrank, follow it.
    this.attachments = this.attachments.flatMap((a) => {
      const s = this.source.inventory(a.slot);
      return s === undefined ? [] : [{ slot: a.slot, amount: Math.min(a.amount, s.amount) }];
    });
    this.titleCount.textContent = bytesText(this.title.value, MAIL_TITLE_MAX);
    this.bodyCount.textContent = bytesText(this.body.value, MAIL_BODY_MAX);
    this.attachList.replaceChildren(...this.attachments.map((a) => this.attachmentRow(a)));
    const used = new Set(this.attachments.map((a) => a.slot));
    const keep = this.picker.value;
    const options = this.source
      .stacks()
      .filter((s) => s.equip === 0 && !used.has(s.slot))
      .map((s) => {
        const o = document.createElement('option');
        o.value = String(s.slot);
        const rules = itemRuleLines(s);
        o.textContent = `${stackLabel(s, this.label)} × ${String(s.amount)}${rules.length > 0 ? ` (${rules.join(', ')})` : ''}`;
        o.disabled = attachProblem(s) !== undefined;
        return o;
      });
    this.picker.replaceChildren(...options);
    if (options.some((o) => o.value === keep)) {
      this.picker.value = keep;
    }
    const full = this.attachments.length >= this.source.maxItems();
    this.picker.disabled = full || options.length === 0;
    const d = this.draft();
    if (d === undefined) {
      this.fee.textContent = '';
      this.problem.textContent = 'Zeny is a whole number.';
      return;
    }
    this.fee.textContent = mailFeeText(d.zeny, d.items.length);
    this.problem.textContent = `${String(d.items.length)} / ${String(this.source.maxItems())} items`;
  }

  private attachmentRow(a: Attachment): HTMLLIElement {
    const s = this.source.inventory(a.slot);
    if (s === undefined) {
      return element('li');
    }
    const amount = numberInput('1', 5);
    amount.value = String(a.amount);
    amount.setAttribute(
      'aria-label',
      `Amount of ${stackLabel(s, this.label)}, 1 to ${String(s.amount)}`,
    );
    amount.addEventListener('change', () => {
      const n = parseWhole(amount.value, 1, s.amount);
      if (n !== undefined) {
        a.amount = n;
      }
      this.renderWrite();
    });
    const shown = { ...s, amount: a.amount };
    const li = stackRow(
      shown,
      this.label,
      this.icons,
      amount,
      button('Remove', () => {
        this.attachments = this.attachments.filter((x) => x !== a);
        this.renderWrite();
      }),
    );
    li.title = stackDetails(s, this.label);
    return li;
  }

  private attachPicked(): void {
    const slot = Number(this.picker.value);
    const s = this.picker.value === '' ? undefined : this.source.inventory(slot);
    if (s === undefined) {
      return;
    }
    const n =
      this.pickAmount.value.trim() === ''
        ? s.amount
        : parseWhole(this.pickAmount.value, 1, s.amount);
    if (n === undefined) {
      this.problem.textContent = `Enter 1 to ${String(s.amount)}.`;
      return;
    }
    this.attach(s, n);
    this.pickAmount.value = '';
  }

  private attach(s: Stack, amount: number): void {
    const problem = attachProblem(s);
    if (problem !== undefined) {
      this.problem.textContent = problem;
      return;
    }
    if (this.attachments.some((a) => a.slot === s.slot)) {
      return;
    }
    if (this.attachments.length >= this.source.maxItems()) {
      this.problem.textContent = `A mail holds at most ${String(this.source.maxItems())} items.`;
      return;
    }
    this.attachments.push({ slot: s.slot, amount });
    this.renderWrite();
  }

  private send(): void {
    const d = this.draft();
    if (d === undefined) {
      this.problem.textContent = 'Zeny is a whole number.';
      return;
    }
    const problem = mailDraftProblem(d, this.context());
    if (problem !== undefined) {
      this.problem.textContent = problem;
      return;
    }
    this.actions.send({ ...d, items: d.items.map((a) => ({ ...a })) });
  }

  /** The mailbox size, as the last MailBox gave it. */
  get mailboxSize(): number {
    return this.size;
  }
}
