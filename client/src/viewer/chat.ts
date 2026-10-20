// Chat: the log, the input rules, and the bubbles above speakers.
// The server checks every line again (server/gateway/chat.go). These
// rules only keep the client from sending lines the server drops.
// User text is only ever set with textContent.

import { ALL_TAB, GUILD_TAB, LOCAL_LINES, PARTY_TAB, showsLine } from './chatTabs';

/** The most characters a line may have, as Unicode code points. */
export const MAX_CHAT = 255;

/** How many lines the log keeps. */
export const LOG_LINES = 100;

/** How long a bubble stays above a speaker, in milliseconds. */
export const BUBBLE_MS = 5000;

/** The server's chat rate: a burst of 5 lines, then 1 per second. */
export const CHAT_BURST = 5;
export const CHAT_REFILL_MS = 1000;

/** Whether c is a bidirectional mark, embedding, override or isolate. */
function bidiControl(c: number): boolean {
  return (
    c === 0x061c ||
    c === 0x200e ||
    c === 0x200f ||
    (c >= 0x202a && c <= 0x202e) ||
    (c >= 0x2066 && c <= 0x2069)
  );
}

/** Whether c is a control character, as Go's unicode.IsControl. */
function control(c: number): boolean {
  return c <= 0x1f || (c >= 0x7f && c <= 0x9f);
}

/**
 * Cleans a line as the server does: control characters become spaces,
 * bidirectional formatting characters go, and spaces at both ends go.
 * Lines longer than MAX_CHAT code points are cut. Returns undefined for
 * a line with nothing left to send.
 */
export function cleanChat(text: string): string | undefined {
  let clean = '';
  // for...of walks code points, which is what the server counts.
  for (const ch of text) {
    const c = ch.codePointAt(0) ?? 0;
    if (!bidiControl(c)) {
      clean += control(c) ? ' ' : ch;
    }
  }
  let cut = '';
  let count = 0;
  for (const ch of clean.trim()) {
    if (count === MAX_CHAT) {
      break;
    }
    cut += ch;
    count++;
  }
  cut = cut.trim();
  return cut === '' ? undefined : cut;
}

/** A chat line as RO shows it, in the log and above the speaker. */
export function chatLine(name: string, text: string): string {
  return `${name} : ${text}`;
}

/** How long a report button waits for the second click, in milliseconds. */
export const REPORT_CONFIRM_MS = 3000;

/**
 * A line's color as CSS, from 0xrrggbb, or undefined for 0, which means
 * the line's own color.
 */
export function chatColor(color: number): string | undefined {
  if (!Number.isInteger(color) || color <= 0 || color > 0xffffff) {
    return undefined;
  }
  return `#${color.toString(16).padStart(6, '0')}`;
}

/** Adds an item to a list and drops the oldest past max. */
export function pushLimited<T>(list: T[], item: T, max: number): T[] {
  list.push(item);
  if (list.length > max) {
    list.splice(0, list.length - max);
  }
  return list;
}

/**
 * The server's chat rate limit, so the client can say a line was not
 * sent instead of the server dropping it without a word.
 */
export class ChatRate {
  private tokens = CHAT_BURST;
  private last: number;

  constructor(now: number) {
    this.last = now;
  }

  /** Takes a line at now, in milliseconds, if the rate allows it. */
  take(now: number): boolean {
    if (now > this.last) {
      this.tokens = Math.min(CHAT_BURST, this.tokens + (now - this.last) / CHAT_REFILL_MS);
      this.last = now;
    }
    if (this.tokens < 1) {
      return false;
    }
    this.tokens--;
    return true;
  }
}

/** The bubbles shown above speakers, by entity ID. */
export class Bubbles {
  private readonly shown = new Map<number, { text: string; until: number }>();

  /** Shows a line above a speaker, replacing the speaker's last one. */
  say(id: number, text: string, now: number): void {
    this.shown.set(id, { text, until: now + BUBBLE_MS });
  }

  /** The bubbles still shown at now. */
  active(now: number): Map<number, string> {
    const out = new Map<number, string>();
    for (const [id, b] of this.shown) {
      if (b.until > now) {
        out.set(id, b.text);
      }
    }
    return out;
  }

  /** Forgets the bubbles that ended by now, and returns their IDs. */
  expire(now: number): number[] {
    const gone: number[] = [];
    for (const [id, b] of this.shown) {
      if (b.until <= now) {
        gone.push(id);
        this.shown.delete(id);
      }
    }
    return gone;
  }

  /** Removes a speaker's bubble, such as when it leaves the view. */
  remove(id: number): void {
    this.shown.delete(id);
  }
}

/**
 * Whether a key press goes to a text field, so game shortcuts must not
 * act on it.
 */
export function isTyping(target: EventTarget | null): boolean {
  if (typeof target !== 'object' || target === null) {
    return false;
  }
  if ('isContentEditable' in target && target.isContentEditable === true) {
    return true;
  }
  return (
    'tagName' in target &&
    (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT')
  );
}

/**
 * The chat log on the page. It keeps the last LOG_LINES lines. Each line
 * belongs to a tab (chatTabs.ts) and shows when that tab is active, or on
 * All. A line a player said has a report button, button.report, that
 * asks for a second click and then calls onReport.
 */
export class ChatLog {
  /** The active tab's key. */
  private active = ALL_TAB;

  /** Called to report a line, with the speaker's name and the line's ID. */
  onReport: ((name: string, lineId: Uint8Array) => void) | undefined;

  constructor(private readonly list: HTMLElement) {}

  /** Shows the lines of a tab. */
  setTab(key: string): void {
    this.active = key;
    for (const row of this.list.children) {
      if (row instanceof HTMLElement) {
        row.hidden = !showsLine(key, row.dataset.tab ?? '');
      }
    }
    this.list.scrollTop = this.list.scrollHeight;
  }

  /**
   * Adds a local line said by a player, or by a monster in its color as
   * 0xrrggbb. A color of 0 keeps the log's color. lineId is the line's ID
   * for reports, empty for lines no player said.
   */
  add(name: string, text: string, mine: boolean, color = 0, lineId?: Uint8Array): void {
    const row = this.row(LOCAL_LINES);
    if (mine) {
      row.className = 'mine';
    }
    const css = chatColor(color);
    if (css !== undefined) {
      row.style.color = css;
    }
    // Names and text are isolated, so right-to-left text in one cannot
    // reorder the other.
    row.append(bdi(name), ' : ', bdi(text));
    if (!mine) {
      this.reportButton(row, name, lineId);
    }
    this.append(row);
  }

  /**
   * Adds a chat channel line under the channel's tab, as "[Alias] Name :
   * text", in the channel's color. An empty name is the channel's own line.
   */
  channel(
    tab: string,
    alias: string,
    name: string,
    text: string,
    color: number,
    mine: boolean,
    lineId?: Uint8Array,
  ): void {
    const row = this.row(tab);
    row.classList.add('channel');
    const css = chatColor(color);
    if (css !== undefined) {
      row.style.color = css;
    }
    if (name === '') {
      row.append(bdi(alias), ' ', bdi(text));
    } else {
      row.append(bdi(alias), ' ', bdi(name), ' : ', bdi(text));
      if (!mine) {
        this.reportButton(row, name, lineId);
      }
    }
    this.append(row);
  }

  /**
   * Adds a party line under the Party tab, as "Name : text" in the party
   * color. The All tab shows it too.
   */
  party(name: string, text: string, mine: boolean, lineId?: Uint8Array): void {
    const row = this.row(PARTY_TAB);
    row.classList.add('party');
    row.append(bdi(name), ' : ', bdi(text));
    if (!mine) {
      this.reportButton(row, name, lineId);
    }
    this.append(row);
  }

  /**
   * Adds a guild line under the Guild tab, as "Name : text" in the guild
   * color. The All tab shows it too.
   */
  guild(name: string, text: string, mine: boolean, lineId?: Uint8Array): void {
    const row = this.row(GUILD_TAB);
    row.classList.add('guild');
    row.append(bdi(name), ' : ', bdi(text));
    if (!mine) {
      this.reportButton(row, name, lineId);
    }
    this.append(row);
  }

  /** Adds a whisper under its tab, as "(From Name) : text" or "(To Name) : text". */
  whisper(tab: string, incoming: boolean, name: string, text: string, lineId?: Uint8Array): void {
    const row = this.row(tab);
    row.classList.add('whisper');
    row.append(`(${incoming ? 'From' : 'To'} `, bdi(name), ') : ', bdi(text));
    if (incoming) {
      this.reportButton(row, name, lineId);
    }
    this.append(row);
  }

  /**
   * Adds text the server sent to this player alone, such as a mod's
   * message, in its color as 0xrrggbb, or the server color for 0.
   */
  server(text: string, color = 0): void {
    const row = this.row('');
    row.className = 'server';
    row.textContent = text;
    const css = chatColor(color);
    if (css !== undefined) {
      row.style.color = css;
    }
    this.append(row);
  }

  /**
   * Adds an announcement to the map or the world, in its color as
   * 0xrrggbb, or the announcement color for 0.
   */
  announce(text: string, color: number): void {
    const row = this.row('');
    row.className = 'announcement';
    row.textContent = text;
    const css = chatColor(color);
    if (css !== undefined) {
      row.style.color = css;
    }
    this.append(row);
  }

  /** Adds a notice from the client itself, on every tab or on one. */
  notice(text: string, tab = ''): void {
    const row = this.row(tab);
    row.className = 'notice';
    row.textContent = text;
    this.append(row);
  }

  /** A new row for a tab, hidden unless the active tab shows it. */
  private row(tab: string): HTMLLIElement {
    const row = document.createElement('li');
    row.dataset.tab = tab;
    row.hidden = !showsLine(this.active, tab);
    return row;
  }

  /**
   * Adds a report button to a row with a line ID. The first click asks
   * for a second one, which sends the report.
   */
  private reportButton(row: HTMLLIElement, name: string, lineId: Uint8Array | undefined): void {
    if (lineId === undefined || lineId.length === 0 || name === '') {
      return;
    }
    const button = reportButton('!', 'Report this line', () => {
      button.remove();
      this.onReport?.(name, lineId);
    });
    row.append(' ', button);
  }

  private append(row: HTMLLIElement): void {
    // The log follows new lines, unless the player scrolled up.
    const atEnd = this.list.scrollTop + this.list.clientHeight >= this.list.scrollHeight - 4;
    this.list.append(row);
    while (this.list.childElementCount > LOG_LINES) {
      this.list.firstElementChild?.remove();
    }
    if (atEnd) {
      this.list.scrollTop = this.list.scrollHeight;
    }
  }
}

/**
 * A button.report that asks for a second click within REPORT_CONFIRM_MS,
 * showing "Report?", and then calls report. It is the chat log's report
 * button, also used to report a guild's name or emblem.
 */
export function reportButton(text: string, title: string, report: () => void): HTMLButtonElement {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'report';
  button.textContent = text;
  button.title = title;
  let timer: ReturnType<typeof setTimeout> | undefined;
  button.addEventListener('click', () => {
    if (button.classList.contains('armed')) {
      clearTimeout(timer);
      button.classList.remove('armed');
      button.textContent = text;
      report();
      return;
    }
    button.classList.add('armed');
    button.textContent = 'Report?';
    timer = setTimeout(() => {
      button.classList.remove('armed');
      button.textContent = text;
    }, REPORT_CONFIRM_MS);
  });
  return button;
}

/** Text in its own bidirectional isolate. */
function bdi(text: string): HTMLElement {
  const e = document.createElement('bdi');
  e.textContent = text;
  return e;
}
