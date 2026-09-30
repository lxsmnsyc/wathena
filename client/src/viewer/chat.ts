// Local chat: the log, the input rules, and the bubbles above speakers.
// The server checks every line again (server/gateway/chat.go). These
// rules only keep the client from sending lines the server drops.
// User text is only ever set with textContent.

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

/** The chat log on the page. It keeps the last LOG_LINES lines. */
export class ChatLog {
  constructor(private readonly list: HTMLElement) {}

  /** Adds a line said by a player. */
  add(name: string, text: string, mine: boolean): void {
    const row = document.createElement('li');
    if (mine) {
      row.className = 'mine';
    }
    // Names and text are isolated, so right-to-left text in one cannot
    // reorder the other.
    const who = document.createElement('bdi');
    who.textContent = name;
    const what = document.createElement('bdi');
    what.textContent = text;
    row.append(who, ' : ', what);
    this.append(row);
  }

  /** Adds a notice from the client itself. */
  notice(text: string): void {
    const row = document.createElement('li');
    row.className = 'notice';
    row.textContent = text;
    this.append(row);
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
