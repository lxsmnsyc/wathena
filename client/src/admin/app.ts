// The admin screens (decision 0028): the staff list, with roles to give
// and remove, and the GM audit log viewer with filters. This bundle
// loads only after /me returned a staff role. User text is only ever
// set with textContent.

import './admin.css';
import { fromJson } from '@bufbuild/protobuf';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import {
  type AuditEntry,
  type Me,
  MeSchema,
  type StaffList,
} from '../gen/wathena/admin/v1/admin_pb';
import { AdminAPI, type AuditFilter, NO_FILTER, type Result, isJson } from './api';

/** Rows per page of the audit log. */
export const AUDIT_PAGE = 50;

/** Makes an element with text. */
function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  text = '',
  className = '',
): HTMLElementTagNameMap[K] {
  const e = document.createElement(tag);
  if (text !== '') {
    e.textContent = text;
  }
  if (className !== '') {
    e.className = className;
  }
  return e;
}

/** A labelled input. */
function field(label: string, control: HTMLInputElement | HTMLSelectElement): HTMLLabelElement {
  const l = el('label', label);
  l.append(control);
  return l;
}

function input(name: string, type = 'text'): HTMLInputElement {
  const i = el('input');
  i.name = name;
  i.type = type;
  i.autocomplete = 'off';
  i.spellcheck = false;
  return i;
}

/** A time for a table cell, in the viewer's local time. */
export function timeText(at: Date | undefined): string {
  return at === undefined ? '' : at.toLocaleString();
}

/** An audit row's arguments as key=value pairs. */
export function argumentsText(entry: AuditEntry): string {
  const fields = entry.arguments ?? {};
  return Object.keys(fields)
    .sort()
    .map((k) => `${k}=${JSON.stringify(fields[k])}`)
    .join(' ');
}

/** The review cell of an audit row. */
export function reviewText(entry: AuditEntry): string {
  if (entry.reviewedAt !== undefined) {
    return `Reviewed by ${entry.reviewedBy}`;
  }
  return entry.needsReview ? 'Needs review' : '';
}

/** Whether a role's permissions are all in the caller's. */
export function canGive(mine: readonly string[], role: readonly string[]): boolean {
  return role.every((p) => mine.includes(p));
}

class Screens {
  private readonly api: AdminAPI;
  private readonly notice = el('p', '', 'admin-notice');
  private readonly staffSection = el('section');
  private readonly auditSection = el('section');
  private filter: AuditFilter = { ...NO_FILTER };
  private nextBefore = 0n;
  private readonly auditBody = el('tbody');
  private readonly more = el('button', 'Older rows');
  /** The reason that giving and removing a role send. */
  private readonly reason = input('reason');

  constructor(
    private readonly root: HTMLElement,
    private readonly me: Me,
  ) {
    this.api = new AdminAPI(me.csrfToken);
  }

  private can(permission: string): boolean {
    return this.me.permissions.includes(permission);
  }

  render(): void {
    this.root.replaceChildren();
    const header = el('header');
    header.append(
      el('h1', 'wAthena admin'),
      el('p', `${this.me.account}, ${this.me.role}`, 'admin-who'),
    );
    this.notice.setAttribute('role', 'status');
    this.root.append(header, this.notice);
    if (this.can('staff.assign')) {
      this.staffSection.className = 'admin-staff';
      this.root.append(this.staffSection);
      this.run(this.loadStaff());
    }
    if (this.can('audit.read')) {
      this.auditSection.className = 'admin-audit';
      this.root.append(this.auditSection);
      this.renderAudit();
      this.run(this.loadAudit(true));
    }
    if (!this.can('staff.assign') && !this.can('audit.read')) {
      this.root.append(el('p', 'Your role has no tools on this page yet.'));
    }
  }

  /** Runs a call from an event, and shows a failure that throws. */
  private run(p: Promise<void>): void {
    p.catch(() => {
      this.notice.className = 'admin-notice admin-error';
      this.notice.textContent = 'Something went wrong. Reload the page.';
    });
  }

  /** Shows a call's error, with the reauth link when it needs one. */
  private fail(r: Result<unknown>): void {
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

  private say(text: string): void {
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
