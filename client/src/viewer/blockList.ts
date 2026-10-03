// The block list panel: the names the player blocks, with an Unblock
// button each, and a field to block a name. A blocked player cannot
// whisper to the player, and the player does not see its chat. User text
// is only ever set with textContent.

import { MAX_NAME, cleanName } from './chatCommands';

/** The most names a block list holds, as the server's limit. */
export const MAX_BLOCKED = 20;

/** The notices for the names a new block list added and removed. */
export function blockChanges(before: readonly string[], after: readonly string[]): string[] {
  const out: string[] = [];
  for (const name of after) {
    if (!before.includes(name)) {
      out.push(`You blocked ${name}.`);
    }
  }
  for (const name of before) {
    if (!after.includes(name)) {
      out.push(`You unblocked ${name}.`);
    }
  }
  return out;
}

/** What the panel asks for. */
export interface BlockActions {
  block: (name: string) => void;
  unblock: (name: string) => void;
}

/**
 * The panel on the page. It holds a count, ul.block-names with one li
 * per name and its button.unblock, p.block-full, and a form with
 * input.block-name and its Block button.
 */
export class BlockPanel {
  private readonly count = document.createElement('span');
  private readonly names = document.createElement('ul');
  private readonly full = document.createElement('p');
  private readonly input = document.createElement('input');
  private readonly error = document.createElement('p');
  private current: string[] = [];

  constructor(
    private readonly root: HTMLElement,
    private readonly actions: BlockActions,
  ) {
    const title = document.createElement('h2');
    title.append('Blocked players ', this.count);
    this.names.className = 'block-names';
    this.full.className = 'block-full';
    this.full.textContent = 'The block list is full. Unblock a name first.';
    this.full.hidden = true;
    const form = document.createElement('form');
    form.className = 'block-form';
    this.input.className = 'block-name';
    this.input.autocomplete = 'off';
    this.input.spellcheck = false;
    this.input.maxLength = MAX_NAME;
    this.input.placeholder = 'Character name';
    this.input.setAttribute('aria-label', 'Name to block');
    const add = document.createElement('button');
    add.type = 'submit';
    add.className = 'block-add';
    add.textContent = 'Block';
    form.append(this.input, add);
    form.addEventListener('submit', (e) => {
      e.preventDefault();
      this.submit();
    });
    this.input.addEventListener('keydown', (e) => {
      // Keys typed here are text, not game shortcuts.
      e.stopPropagation();
      if (e.key === 'Escape') {
        this.input.blur();
      }
    });
    this.error.className = 'block-error';
    this.error.hidden = true;
    this.root.replaceChildren(title, this.names, this.full, form, this.error);
    this.show([], false);
  }

  /** Shows a block list as BlockList gives it. */
  show(names: string[], full: boolean): void {
    this.current = [...names];
    this.count.textContent = `(${String(names.length)} / ${String(MAX_BLOCKED)})`;
    const rows = names.map((name) => {
      const row = document.createElement('li');
      const label = document.createElement('bdi');
      label.textContent = name;
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'unblock';
      button.textContent = 'Unblock';
      button.setAttribute('aria-label', `Unblock ${name}`);
      button.addEventListener('click', () => {
        this.actions.unblock(name);
      });
      row.append(label, ' ', button);
      return row;
    });
    if (rows.length === 0) {
      const row = document.createElement('li');
      row.className = 'empty';
      row.textContent = 'Nobody is blocked.';
      rows.push(row);
    }
    this.names.replaceChildren(...rows);
    this.full.hidden = !full && names.length < MAX_BLOCKED;
  }

  /** Opens or closes the panel. */
  toggle(): void {
    this.root.hidden = !this.root.hidden;
  }

  private submit(): void {
    const name = cleanName(this.input.value);
    let problem: string | undefined;
    if (name === undefined) {
      problem = `Type a name of 1 to ${String(MAX_NAME)} characters.`;
    } else if (this.current.includes(name)) {
      problem = `${name} is already blocked.`;
    } else if (this.current.length >= MAX_BLOCKED) {
      problem = 'The block list is full. Unblock a name first.';
    }
    this.error.textContent = problem ?? '';
    this.error.hidden = problem === undefined;
    if (problem !== undefined || name === undefined) {
      return;
    }
    this.actions.block(name);
    this.input.value = '';
  }
}
