import { describe, expect, it } from 'vitest';
import {
  BUBBLE_MS,
  Bubbles,
  CHAT_BURST,
  CHAT_REFILL_MS,
  ChatRate,
  MAX_CHAT,
  chatLine,
  cleanChat,
  isTyping,
  pushLimited,
} from './chat';

describe('cleanChat', () => {
  it('trims and keeps text as it is', () => {
    expect(cleanChat('  hi  ')).toBe('hi');
    expect(cleanChat('<b>x</b>')).toBe('<b>x</b>');
    expect(cleanChat('포링 hi')).toBe('포링 hi');
  });

  it('turns control characters into spaces and drops bidi controls', () => {
    expect(cleanChat('a\tb\nc')).toBe('a b c');
    expect(cleanChat('abc\u202Egpj.exe')).toBe('abcgpj.exe');
    expect(cleanChat('\u2066x\u2069\u200E\u200F\u061C')).toBe('x');
  });

  it('sends nothing for an empty line', () => {
    expect(cleanChat('')).toBeUndefined();
    expect(cleanChat(' \n\t ')).toBeUndefined();
    expect(cleanChat('\u202E')).toBeUndefined();
  });

  it('cuts lines to MAX_CHAT code points', () => {
    expect(cleanChat('a'.repeat(MAX_CHAT + 10))).toBe('a'.repeat(MAX_CHAT));
    // An emoji is one code point but two UTF-16 units.
    const emoji = '\u{1F600}'.repeat(MAX_CHAT + 1);
    expect(cleanChat(emoji)).toBe('\u{1F600}'.repeat(MAX_CHAT));
  });
});

describe('chatLine', () => {
  it('shows the name and the text as RO does', () => {
    expect(chatLine('Alice', 'hello')).toBe('Alice : hello');
  });
});

describe('pushLimited', () => {
  it('keeps the newest items', () => {
    const list = [1, 2, 3];
    expect(pushLimited(list, 4, 3)).toEqual([2, 3, 4]);
    expect(pushLimited([], 1, 3)).toEqual([1]);
  });
});

describe('ChatRate', () => {
  it('allows a burst, then one line per refill', () => {
    const rate = new ChatRate(0);
    for (let i = 0; i < CHAT_BURST; i++) {
      expect(rate.take(0)).toBe(true);
    }
    expect(rate.take(0)).toBe(false);
    expect(rate.take(CHAT_REFILL_MS / 2)).toBe(false);
    expect(rate.take(CHAT_REFILL_MS)).toBe(true);
    expect(rate.take(CHAT_REFILL_MS)).toBe(false);
  });

  it('holds at most a burst', () => {
    const rate = new ChatRate(0);
    let sent = 0;
    while (rate.take(3_600_000)) {
      sent++;
    }
    expect(sent).toBe(CHAT_BURST);
  });
});

describe('Bubbles', () => {
  it('shows a line for BUBBLE_MS', () => {
    const b = new Bubbles();
    b.say(7, 'Alice : hi', 1000);
    expect(b.active(1000 + BUBBLE_MS - 1)).toEqual(new Map([[7, 'Alice : hi']]));
    expect(b.expire(1000 + BUBBLE_MS - 1)).toEqual([]);
    expect(b.active(1000 + BUBBLE_MS).size).toBe(0);
    expect(b.expire(1000 + BUBBLE_MS)).toEqual([7]);
    expect(b.expire(1000 + BUBBLE_MS)).toEqual([]);
  });

  it('replaces a speaker’s line and restarts its time', () => {
    const b = new Bubbles();
    b.say(7, 'first', 0);
    b.say(7, 'second', 3000);
    expect(b.active(BUBBLE_MS + 1)).toEqual(new Map([[7, 'second']]));
    b.remove(7);
    expect(b.active(BUBBLE_MS + 1).size).toBe(0);
  });
});

describe('isTyping', () => {
  it('knows text fields', () => {
    const target = (tagName: string, isContentEditable = false): EventTarget =>
      Object.assign(new EventTarget(), { tagName, isContentEditable });
    expect(isTyping(target('INPUT'))).toBe(true);
    expect(isTyping(target('TEXTAREA'))).toBe(true);
    expect(isTyping(target('DIV', true))).toBe(true);
    expect(isTyping(target('CANVAS'))).toBe(false);
    expect(isTyping(null)).toBe(false);
  });
});
