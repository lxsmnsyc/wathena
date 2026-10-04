// Small checks that the mail, vending, buying store and store search
// windows share: text lengths in bytes, as the server counts them, and
// whole numbers typed into inputs.

const encoder = new TextEncoder();

/** The length of a text in UTF-8 bytes, as the server counts it. */
export function utf8Length(text: string): number {
  return encoder.encode(text).length;
}

/**
 * Why a text does not fit min to max UTF-8 bytes, or undefined when it
 * fits. what names the field, such as "The title".
 */
export function bytesProblem(
  what: string,
  text: string,
  min: number,
  max: number,
): string | undefined {
  const n = utf8Length(text);
  if (n < min) {
    return min === 1 ? `${what} is empty.` : `${what} needs at least ${String(min)} bytes.`;
  }
  if (n > max) {
    return `${what} is too long: ${String(n)} of at most ${String(max)} bytes.`;
  }
  return undefined;
}

/**
 * Reads a whole number from min to max. Spaces and commas between digits
 * are skipped, so "1,000" reads as 1000. Empty text, and text that is not
 * a whole number in range, give undefined.
 */
export function parseWhole(text: string, min: number, max: number): number | undefined {
  const t = text.trim().replaceAll(/[\s,]/g, '');
  if (!/^\d{1,12}$/.test(t)) {
    return undefined;
  }
  const n = Number(t);
  return n >= min && n <= max ? n : undefined;
}

/** A zeny amount with thousands separators, such as "1,500 z". */
export function zenyLabel(zeny: number | bigint): string {
  return `${zeny.toLocaleString('en-US')} z`;
}

/** A map's asset name without "map/", such as prontera for map/prontera. */
export function mapLabel(map: string): string {
  return map.replace(/^map\//, '');
}

/** A time in the player's local time, or "-" for 0. */
export function dateText(ms: bigint | number): string {
  const n = Number(ms);
  return n === 0 ? '-' : new Date(n).toLocaleString();
}

/** Makes a button with a label and a click action. */
export function button(label: string, onClick: () => void, className = ''): HTMLButtonElement {
  const b = document.createElement('button');
  b.type = 'button';
  b.textContent = label;
  if (className !== '') {
    b.className = className;
  }
  b.addEventListener('click', onClick);
  return b;
}

/** Makes an element with a class and text. */
export function element<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  className = '',
  text = '',
): HTMLElementTagNameMap[K] {
  const el = document.createElement(tag);
  if (className !== '') {
    el.className = className;
  }
  if (text !== '') {
    el.textContent = text;
  }
  return el;
}

/** Makes a labelled input. */
export function field(
  label: string,
  input: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement,
): HTMLLabelElement {
  const l = document.createElement('label');
  l.append(`${label} `, input);
  return l;
}

/** Makes a text input. */
export function textInput(placeholder = '', size = 10): HTMLInputElement {
  const i = document.createElement('input');
  i.autocomplete = 'off';
  i.spellcheck = false;
  i.placeholder = placeholder;
  i.size = size;
  return i;
}

/** Makes an input for a whole number. */
export function numberInput(placeholder = '0', size = 7): HTMLInputElement {
  const i = textInput(placeholder, size);
  i.inputMode = 'numeric';
  return i;
}

/**
 * Makes a window's root keep its keys from the game shortcuts, and calls
 * onEscape on Escape.
 */
export function keepKeys(root: HTMLElement, onEscape: () => void): void {
  root.addEventListener('keydown', (e) => {
    e.stopPropagation();
    if (e.key === 'Escape') {
      onEscape();
    }
  });
}
