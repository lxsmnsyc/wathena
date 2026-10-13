// The admin screens (decisions 0028, 0029 and 0030): the staff list, the
// GM audit log viewer, the review queue, the player search with character
// details and sanctions, item histories, restores, the report queue and
// the bans in effect. This bundle loads only after
// /me returned a staff role. User text is only ever set with
// textContent.

import './admin.css';
import { fromJson } from '@bufbuild/protobuf';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { type Me, MeSchema, type StaffList } from '../gen/wathena/admin/v1/admin_pb';
import { AdminAPI, type AuditFilter, NO_FILTER, type Result, isJson } from './api';
import { argumentsText, reviewText } from './audit';
import { type Context, type Tab, type View, tabsFor } from './context';
import { el, field, input, timeText } from './dom';
import { renderHistory } from './history';
import { renderCharacter, renderPlayers } from './players';
import { renderReport, renderReports } from './reports';
import { renderRestore } from './restore';
import { renderReviews } from './review';
import { renderBans } from './sanctions';

export { argumentsText, reviewText } from './audit';
export { timeText } from './dom';

/** Rows per page of the audit log. */
export const AUDIT_PAGE = 50;

/** Whether a role's permissions are all in the caller's. */
export function canGive(mine: readonly string[], role: readonly string[]): boolean {
  return role.every((p) => mine.includes(p));
}

/** The screen that a tab opens. */
function tabView(kind: Tab['kind']): View {
  switch (kind) {
    case 'item':
      return { kind, serial: '', characterId: 0n };
    case 'restore':
      return { kind, characterId: 0n, serials: [] };
    default:
      return { kind };
  }
}

/** The tab that a screen belongs to. */
export function tabOf(kind: View['kind']): string {
  switch (kind) {
    case 'character':
      return 'players';
    case 'report':
      return 'reports';
    default:
      return kind;
  }
}

/** The sections of the page, one per screen. */
type SectionKind = View['kind'];

class Screens implements Context {
  readonly api: AdminAPI;
  private readonly notice = el('p', '', 'admin-notice');
  private readonly nav = el('nav', '', 'admin-nav');
  private readonly tabButtons = new Map<Tab['kind'], HTMLButtonElement>();
  private readonly sections = new Map<SectionKind, HTMLElement>();
  private readonly staffSection: HTMLElement;
  private readonly auditSection: HTMLElement;
  private filter: AuditFilter = { ...NO_FILTER };
  private nextBefore = 0n;
  private readonly auditBody = el('tbody');
  private readonly more = el('button', 'Older rows');
  /** The reason that giving and removing a role send. */
  private readonly reason = input('reason');

  constructor(
    private readonly root: HTMLElement,
    readonly me: Me,
  ) {
    this.api = new AdminAPI(me.csrfToken);
    this.staffSection = this.section('staff');
    this.auditSection = this.section('audit');
  }

  can(permission: string): boolean {
    return this.me.permissions.includes(permission);
  }

  /** The section of a screen, made the first time. */
  private section(kind: SectionKind): HTMLElement {
    let s = this.sections.get(kind);
    if (s === undefined) {
      s = el('section', '', `admin-${kind}`);
      s.hidden = true;
      this.sections.set(kind, s);
      this.root.append(s);
    }
    return s;
  }

  render(): void {
    this.root.replaceChildren();
    const header = el('header');
    header.append(
      el('h1', 'wAthena admin'),
      el('p', `${this.me.account}, ${this.me.role}`, 'admin-who'),
    );
    this.notice.setAttribute('role', 'status');
    this.root.append(header, this.nav, this.notice);
    const tabs = tabsFor(this.me.permissions);
    for (const t of tabs) {
      const b = el('button', t.label);
      b.type = 'button';
      b.addEventListener('click', () => {
        this.open(tabView(t.kind));
      });
      this.tabButtons.set(t.kind, b);
      this.nav.append(b);
    }
    // The staff list loads at once, as in step 1. The audit log loads
    // each time its tab opens.
    this.root.append(this.staffSection, this.auditSection);
    if (this.can('staff.assign')) {
      this.run(this.loadStaff());
    }
    if (this.can('audit.read')) {
      this.renderAudit();
    }
    const first = tabs[0];
    if (first === undefined) {
      this.root.append(el('p', 'Your role has no tools on this page yet.'));
      return;
    }
    this.open(tabView(first.kind));
  }

  /** Shows one screen and hides the others. */
  open(view: View): void {
    this.say('');
    const s = this.section(view.kind);
    for (const [kind, section] of this.sections) {
      section.hidden = kind !== view.kind;
    }
    const tab = tabOf(view.kind);
    for (const [kind, b] of this.tabButtons) {
      if (kind === tab) {
        b.setAttribute('aria-current', 'page');
      } else {
        b.removeAttribute('aria-current');
      }
    }
    switch (view.kind) {
      case 'staff':
        return;
      case 'audit':
        // Rows that came since, such as reviews, show when the tab opens.
        if (this.can('audit.read')) {
          this.run(this.loadAudit(true));
        }
        return;
      case 'reviews':
        renderReviews(this, s);
        return;
      case 'players':
        // The search keeps its results when the screen comes back.
        if (s.childElementCount === 0) {
          renderPlayers(this, s);
        }
        return;
      case 'character':
        renderCharacter(this, s, view.id);
        return;
      case 'item':
        renderHistory(this, s, view.serial, view.characterId);
        return;
      case 'restore':
        renderRestore(this, s, view.characterId, view.serials);
        return;
      case 'reports':
        renderReports(this, s);
        return;
      case 'report':
        renderReport(this, s, view.id);
        return;
      case 'bans':
        renderBans(this, s);
    }
  }

  /** Runs a call from an event, and shows a failure that throws. */
  run(p: Promise<void>): void {
    p.catch(() => {
      this.notice.className = 'admin-notice admin-error';
      this.notice.textContent = 'Something went wrong. Reload the page.';
    });
  }

  /** Shows a call's error, with the reauth link when it needs one. */
  fail(r: Result<unknown>): void {
    if (r.ok) {
      return;
    }
    this.notice.className = 'admin-notice admin-error';
    this.notice.textContent = r.message;
    if (r.code === 'reauth' && r.reauthUrl !== '') {
      const a = el('a', 'Confirm it is you');
      a.href = r.reauthUrl;
      this.notice.append(' ', a);
    }
  }

  say(text: string): void {
    this.notice.className = 'admin-notice';
    this.notice.textContent = text;
  }

  private async loadStaff(): Promise<void> {
    const r = await this.api.staff();
    if (!r.ok) {
      this.fail(r);
      return;
    }
    this.renderStaff(r.value);
  }

  private renderStaff(list: StaffList): void {
    const s = this.staffSection;
    s.replaceChildren(el('h2', 'Staff'));
    const table = el('table');
    const head = el('tr');
    for (const h of ['Account', 'Role', 'Two-factor', 'Given by', 'Given at', '']) {
      head.append(el('th', h));
    }
    table.append(el('thead'), el('tbody'));
    table.tHead?.append(head);
    const body = table.tBodies[0];
    for (const m of list.members) {
      const tr = el('tr');
      tr.dataset.account = m.account;
      tr.append(
        el('td', m.account),
        el('td', m.role),
        el('td', m.twoFactor ? 'On' : 'Off'),
        el('td', m.fromConfig ? 'config' : m.givenBy),
        el('td', m.givenAt === undefined ? '' : timeText(timestampDate(m.givenAt))),
      );
      const cell = el('td');
      if (!m.fromConfig && m.account.toLowerCase() !== this.me.account.toLowerCase()) {
        const remove = el('button', 'Remove', 'admin-remove');
        remove.type = 'button';
        remove.addEventListener('click', () => {
          this.run(this.removeRole(m.account, this.reason.value));
        });
        cell.append(remove);
      }
      tr.append(cell);
      body?.append(tr);
    }
    s.append(table);

    const form = el('form', '', 'admin-give');
    const account = input('account');
    account.required = true;
    account.maxLength = 24;
    const role = el('select');
    role.name = 'role';
    for (const r of list.roles) {
      if (canGive(this.me.permissions, r.permissions)) {
        const o = el('option', r.name);
        o.value = r.name;
        role.append(o);
      }
    }
    const reason = this.reason;
    reason.maxLength = 255;
    const submit = el('button', 'Give role');
    submit.type = 'submit';
    form.append(
      el('h3', 'Give a role'),
      field('Account', account),
      field('Role', role),
      field('Reason, for giving and removing', reason),
      submit,
    );
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      this.run(this.giveRole(account.value.trim(), role.value, reason.value));
    });
    s.append(form);
  }

  private async giveRole(account: string, role: string, reason: string): Promise<void> {
    const r = await this.api.give(account, role, reason);
    if (!r.ok) {
      this.fail(r);
      return;
    }
    this.say(`${r.value.account} has the ${r.value.role} role.`);
    await this.refresh();
  }

  private async removeRole(account: string, reason: string): Promise<void> {
    const r = await this.api.remove(account, reason);
    if (!r.ok) {
      this.fail(r);
      return;
    }
    this.say(`${account} has no role now.`);
    await this.refresh();
  }

  private async refresh(): Promise<void> {
    await this.loadStaff();
    if (this.can('audit.read')) {
      await this.loadAudit(true);
    }
  }

  private renderAudit(): void {
    const s = this.auditSection;
    s.replaceChildren(el('h2', 'Audit log'));
    const form = el('form', '', 'admin-filter');
    const inputs = {
      account: input('account'),
      action: input('action'),
      target: input('target'),
      since: input('since', 'date'),
      until: input('until', 'date'),
    };
    const review = el('select');
    review.name = 'review';
    for (const [value, label] of [
      ['', 'Any'],
      ['pending', 'Needs review'],
      ['reviewed', 'Reviewed'],
    ] as const) {
      const o = el('option', label);
      o.value = value;
      review.append(o);
    }
    const apply = el('button', 'Search');
    apply.type = 'submit';
    form.append(
      field('Staff account', inputs.account),
      field('Action', inputs.action),
      field('Target', inputs.target),
      field('From', inputs.since),
      field('Before', inputs.until),
      field('Review', review),
      apply,
    );
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      const r = review.value;
      this.filter = {
        account: inputs.account.value,
        action: inputs.action.value,
        target: inputs.target.value,
        since: inputs.since.value,
        until: inputs.until.value,
        review: r === 'pending' || r === 'reviewed' ? r : '',
        needsReview: false,
      };
      this.run(this.loadAudit(true));
    });
    const table = el('table');
    const head = el('tr');
    for (const h of [
      'Time',
      'Staff',
      'Role',
      'Action',
      'Target',
      'Arguments',
      'Reason',
      'Review',
    ]) {
      head.append(el('th', h));
    }
    const thead = el('thead');
    thead.append(head);
    table.append(thead, this.auditBody);
    this.more.type = 'button';
    this.more.hidden = true;
    this.more.addEventListener('click', () => {
      this.run(this.loadAudit(false));
    });
    s.append(form, table, this.more);
  }

  private async loadAudit(fresh: boolean): Promise<void> {
    const r = await this.api.audit(this.filter, fresh ? 0n : this.nextBefore, AUDIT_PAGE);
    if (!r.ok) {
      this.fail(r);
      return;
    }
    if (fresh) {
      this.auditBody.replaceChildren();
    }
    for (const e of r.value.entries) {
      const tr = el('tr');
      tr.dataset.id = e.id.toString();
      tr.append(
        el('td', e.at === undefined ? '' : timeText(timestampDate(e.at))),
        el('td', e.account),
        el('td', e.role),
        el('td', e.action),
        el('td', e.target),
        el('td', argumentsText(e)),
        el('td', e.reason),
        el('td', reviewText(e)),
      );
      this.auditBody.append(tr);
    }
    if (fresh && r.value.entries.length === 0) {
      const tr = el('tr');
      const td = el('td', 'No rows.');
      td.colSpan = 8;
      tr.append(td);
      this.auditBody.append(tr);
    }
    this.nextBefore = r.value.nextBeforeId;
    this.more.hidden = this.nextBefore === 0n;
  }
}

/** Starts the admin screens with the /me answer that confirmed a role. */
export function start(root: HTMLElement, me: unknown): void {
  const m = fromJson(MeSchema, isJson(me) ? me : {}, { ignoreUnknownFields: true });
  new Screens(root, m).render();
}
