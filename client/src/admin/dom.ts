// Small DOM helpers that the admin screens share. User text is only
// ever set with textContent.

import { type Timestamp, timestampDate } from '@bufbuild/protobuf/wkt';

/** Makes an element with text. */
export function el<K extends keyof HTMLElementTagNameMap>(
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
export function field(
  label: string,
  control: HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement,
): HTMLLabelElement {
  const l = el('label', label);
  l.append(control);
  return l;
}

/** A text input without autocomplete. */
export function input(name: string, type = 'text'): HTMLInputElement {
  const i = el('input');
  i.name = name;
  i.type = type;
  i.autocomplete = 'off';
  i.spellcheck = false;
  return i;
}

/** A button that runs a function when clicked. */
export function button(text: string, onClick: () => void, className = ''): HTMLButtonElement {
  const b = el('button', text, className);
  b.type = 'button';
  b.addEventListener('click', onClick);
  return b;
}

/** A table with a header row. The rows go in its tBodies[0]. */
export function table(headings: readonly string[]): {
  table: HTMLTableElement;
  body: HTMLTableSectionElement;
} {
  const t = el('table');
  const head = el('tr');
  for (const h of headings) {
    head.append(el('th', h));
  }
  const thead = el('thead');
  thead.append(head);
  const body = el('tbody');
  t.append(thead, body);
  return { table: t, body };
}

/** A table row of cells. A string becomes a text cell. */
export function row(cells: readonly (string | Node)[]): HTMLTableRowElement {
  const tr = el('tr');
  for (const c of cells) {
    if (typeof c === 'string') {
      tr.append(el('td', c));
    } else {
      const td = el('td');
      td.append(c);
      tr.append(td);
    }
  }
  return tr;
}

/** A row of one cell across a table, for "No rows." and the like. */
export function wideRow(text: string, span: number): HTMLTableRowElement {
  const tr = el('tr');
  const td = el('td', text);
  td.colSpan = span;
  tr.append(td);
  return tr;
}

/** A time for a table cell, in the viewer's local time. */
export function timeText(at: Date | undefined): string {
  return at === undefined ? '' : at.toLocaleString();
}

/** A protobuf time for a table cell, or "" when unset. */
export function when(at: Timestamp | undefined): string {
  return at === undefined ? '' : timeText(timestampDate(at));
}
