import type { AssetStore } from '../assets/store';
import type { NpcDialog } from '../gen/wathena/v1/event_pb';
import { ImagePosition } from '../gen/wathena/v1/event_pb';

// The NPC dialog window, as RO's: the NPC's lines, then a Next or Close
// button, a menu, or an input. Lines keep RO's color codes and item
// links, and are only ever set with textContent. The NPC illustration
// (cutin) shows at the bottom of the screen while the window is open.

/** A part of a dialog line: text in one color, or an item link. */
export interface DialogRun {
  text: string;
  color: string | undefined;
  /** The item a link names. */
  item?: number;
}

/** A color code, ^rrggbb, or an item link. */
const TOKENS = /\^([0-9a-fA-F]{6})|<ITEMLINK>(.*?)<INFO>(\d{1,10})<\/INFO><\/ITEMLINK>/g;

/** The longest line the window keeps, in UTF-16 units. */
const MAX_LINE = 1024;

/** The most lines the window keeps between two Next buttons. */
export const MAX_LINES = 200;

/**
 * Splits a dialog line into runs: text in one color, as itemText's
 * colorRuns, and item links, <ITEMLINK>name<INFO>id</INFO></ITEMLINK>,
 * as runs that name their item. Runs without text are left out, except
 * links.
 */
export function dialogRuns(line: string): DialogRun[] {
  const text = line.slice(0, MAX_LINE);
  const out: DialogRun[] = [];
  let color: string | undefined;
  let last = 0;
  for (const m of text.matchAll(TOKENS)) {
    if (m.index > last) {
      out.push({ text: text.slice(last, m.index), color });
    }
    if (m[1] === undefined) {
      out.push({ text: m[2] ?? '', color, item: Number(m[3]) });
    } else {
      color = `#${m[1].toLowerCase()}`;
    }
    last = m.index + m[0].length;
  }
  if (last < text.length) {
    out.push({ text: text.slice(last), color });
  }
  return out;
}

/** How an item link shows before names load. */
function itemFallback(id: number): string {
  return `Item ${id}`;
}

/** The class of an illustration's position. */
function positionClass(position: ImagePosition): string {
  switch (position) {
    case ImagePosition.LEFT:
      return 'left';
    case ImagePosition.CENTER:
      return 'center';
    default:
      return 'right';
  }
}

/** What the window sends back. */
export interface DialogActions {
  next: (dialog: number) => void;
  menu: (dialog: number, option: number) => void;
  number: (dialog: number, value: bigint) => void;
  text: (dialog: number, value: string) => void;
  close: (dialog: number) => void;
}

/** Checks a number input against the prompt's range. */
export function parseNumber(text: string, min: bigint, max: bigint): bigint | undefined {
  const t = text.trim();
  if (!/^-?\d{1,19}$/.test(t)) {
    return undefined;
  }
  const n = BigInt(t);
  return n < min || n > max ? undefined : n;
}

/** The dialog window on the page, with the NPC illustration. */
export class DialogWindow {
  private readonly title: HTMLDivElement;
  private readonly body: HTMLDivElement;
  private readonly controls: HTMLDivElement;
  private readonly image: HTMLImageElement;
  private imageName = '';
  /** The step that waits for an answer, and whether its lines should clear on the next lines. */
  private waiting: NpcDialog | undefined;
  private clearOnLines = false;
  /** Names an item ID, for links. */
  label: (id: number) => string = itemFallback;

  constructor(
    private readonly root: HTMLElement,
    imageRoot: HTMLElement,
    private readonly store: AssetStore,
    private readonly actions: DialogActions,
  ) {
    this.title = document.createElement('div');
    this.title.className = 'dialog-title';
    this.body = document.createElement('div');
    this.body.className = 'dialog-body';
    this.controls = document.createElement('div');
    this.controls.className = 'dialog-controls';
    root.append(this.title, this.body, this.controls);
    root.hidden = true;
    this.image = document.createElement('img');
    this.image.className = 'cutin';
    this.image.alt = '';
    this.image.hidden = true;
    imageRoot.append(this.image);
    root.addEventListener('keydown', (e) => {
      // Keys typed here are for the dialog, not game shortcuts.
      e.stopPropagation();
      if (e.key === 'Escape') {
        this.cancel();
      }
    });
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /** Shows one step. name is the NPC's name, for the window's title. */
  show(d: NpcDialog, name: string): void {
    if (d.prompt.case === 'end') {
      this.hide();
      return;
    }
    if (this.root.hidden) {
      this.root.hidden = false;
      this.body.replaceChildren();
    }
    this.title.textContent = name;
    if (d.lines.length > 0) {
      if (this.clearOnLines) {
        this.body.replaceChildren();
        this.clearOnLines = false;
      }
      for (const line of d.lines) {
        this.body.append(this.line(line));
      }
      while (this.body.childElementCount > MAX_LINES) {
        this.body.firstElementChild?.remove();
      }
      this.body.scrollTop = this.body.scrollHeight;
    }
    if (d.prompt.case === undefined) {
      return;
    }
    this.waiting = d;
    this.controls.replaceChildren();
    switch (d.prompt.case) {
      case 'next':
        this.button('Next', () => {
          // RO clears the lines when the next ones come.
          this.clearOnLines = true;
          this.answer((id) => {
            this.actions.next(id);
          });
        });
        break;
      case 'close':
        this.button('Close', () => {
          this.answer((id) => {
            this.actions.close(id);
          });
          this.hide();
        });
        break;
      case 'menu':
        this.menu(d.prompt.value.options);
        break;
      case 'inputNumber':
        this.input(true, d);
        break;
      case 'inputText':
        this.input(false, d);
        break;
      default:
        break;
    }
    const first = this.controls.querySelector<HTMLElement>('button, input');
    first?.focus();
  }

  /** Shows the NPC illustration, image/illust/<name>, or hides it for "". */
  showImage(name: string, position: ImagePosition): void {
    this.imageName = name;
    if (name === '') {
      this.image.hidden = true;
      return;
    }
    this.image.className = `cutin ${positionClass(position)}`;
    this.store
      .bytes(`image/illust/${name}`, 'texture')
      .then((data) => {
        if (this.imageName !== name) {
          return;
        }
        if (data === undefined) {
          this.image.hidden = true;
          return;
        }
        URL.revokeObjectURL(this.image.src);
        this.image.src = URL.createObjectURL(new Blob([data], { type: 'image/webp' }));
        this.image.hidden = false;
      })
      .catch((error: unknown) => {
        console.error(`image/illust/${name}:`, error);
      });
  }

  /** Closes the window and its illustration, as when the server ends the conversation. */
  hide(): void {
    this.root.hidden = true;
    this.waiting = undefined;
    this.clearOnLines = false;
    this.body.replaceChildren();
    this.controls.replaceChildren();
    this.showImage('', ImagePosition.UNSPECIFIED);
  }

  /** Cancels the waiting step, as Escape does in RO: it ends the conversation. */
  cancel(): void {
    if (this.waiting === undefined) {
      return;
    }
    this.answer((id) => {
      this.actions.close(id);
    });
    this.hide();
  }

  private answer(send: (dialog: number) => void): void {
    const d = this.waiting;
    if (d === undefined) {
      return;
    }
    this.waiting = undefined;
    this.controls.replaceChildren();
    send(d.dialog);
  }

  private line(text: string): HTMLDivElement {
    const div = document.createElement('div');
    for (const run of dialogRuns(text)) {
      const span = document.createElement('span');
      span.textContent = run.item === undefined ? run.text : this.label(run.item);
      if (run.item !== undefined) {
        span.className = 'item-link';
        span.title = run.text;
      }
      if (run.color !== undefined) {
        span.style.color = run.color;
      }
      div.append(span);
    }
    if (div.childElementCount === 0) {
      div.textContent = ' ';
    }
    return div;
  }

  private button(label: string, onClick: () => void): void {
    const b = document.createElement('button');
    b.type = 'button';
    b.textContent = label;
    b.addEventListener('click', onClick);
    this.controls.append(b);
  }

  private menu(options: readonly string[]): void {
    const list = document.createElement('div');
    list.className = 'dialog-menu';
    list.setAttribute('role', 'listbox');
    options.forEach((o, i) => {
      // Empty options are hidden and keep their number, as in RO.
      if (o === '') {
        return;
      }
      const b = document.createElement('button');
      b.type = 'button';
      b.setAttribute('role', 'option');
      for (const run of dialogRuns(o)) {
        const span = document.createElement('span');
        span.textContent = run.text;
        if (run.color !== undefined) {
          span.style.color = run.color;
        }
        b.append(span);
      }
      b.addEventListener('click', () => {
        this.answer((id) => {
          this.actions.menu(id, i + 1);
        });
      });
      list.append(b);
    });
    this.controls.append(list);
    this.button('Cancel', () => {
      this.cancel();
    });
  }

  private input(number: boolean, d: NpcDialog): void {
    const form = document.createElement('form');
    form.className = 'dialog-input';
    const input = document.createElement('input');
    if (number) {
      input.inputMode = 'numeric';
      input.value = String(d.numberMin > 0n ? d.numberMin : 0n);
      input.setAttribute('aria-label', `A number from ${d.numberMin} to ${d.numberMax}`);
    } else {
      input.maxLength = d.textMax;
      input.setAttribute('aria-label', `Text, at most ${d.textMax} characters`);
    }
    const ok = document.createElement('button');
    ok.type = 'submit';
    ok.textContent = 'OK';
    form.append(input, ok);
    form.addEventListener('submit', (e) => {
      e.preventDefault();
      if (number) {
        const n = parseNumber(input.value, d.numberMin, d.numberMax);
        if (n === undefined) {
          input.setCustomValidity(`Enter ${d.numberMin} to ${d.numberMax}.`);
          input.reportValidity();
          return;
        }
        this.answer((id) => {
          this.actions.number(id, n);
        });
        return;
      }
      // The server counts code points, as Array.from does.
      const text = Array.from(input.value).slice(0, d.textMax).join('');
      this.answer((id) => {
        this.actions.text(id, text);
      });
    });
    input.addEventListener('input', () => {
      input.setCustomValidity('');
    });
    this.controls.append(form);
  }
}
