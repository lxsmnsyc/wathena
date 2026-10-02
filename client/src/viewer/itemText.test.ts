import { describe, expect, it, vi } from 'vitest';
import {
  ITEM_DESCRIPTIONS,
  ITEM_NAMES,
  ItemTexts,
  colorRuns,
  itemLabel,
  parseItemDescriptions,
  parseItemNames,
  refinedLabel,
  stripColors,
} from './itemText';

describe('refinedLabel', () => {
  it('puts the refine level in front', () => {
    expect(refinedLabel('Knife [3]', 7)).toBe('+7 Knife [3]');
    expect(refinedLabel('Knife [3]', 0)).toBe('Knife [3]');
  });
});

describe('parseItemNames', () => {
  it('reads entries and skips what does not fit', () => {
    const names = parseItemNames({
      '501': { n: '빨간포션', u: '포션' },
      '1201': { n: '나이프', u: '단검', s: 3 },
      '1202': { n: 'Big', s: 99 },
      '0501': { n: 'leading zero' },
      x: { n: 'not an ID' },
      '7': 'not an object',
      '8': { n: 5 },
    });
    expect([...names.keys()].sort((a, b) => a - b)).toEqual([8, 501, 1201, 1202]);
    expect(names.get(501)).toEqual({ name: '빨간포션', unidentified: '포션', slots: 0 });
    expect(names.get(1201)?.slots).toBe(3);
    expect(names.get(1202)?.slots).toBe(4);
    expect(names.get(8)?.name).toBe('');
    expect(parseItemNames([1, 2]).size).toBe(0);
    expect(parseItemNames(undefined).size).toBe(0);
  });
});

describe('parseItemDescriptions', () => {
  it('reads lists of lines', () => {
    const d = parseItemDescriptions({ '501': ['a', 2, 'b'], '502': 'x' });
    expect(d.get(501)).toEqual(['a', '', 'b']);
    expect(d.has(502)).toBe(false);
  });
});

describe('itemLabel', () => {
  it('shows the name and slots, or the ID', () => {
    expect(itemLabel(501, { name: '빨간포션', unidentified: '', slots: 0 })).toBe('빨간포션');
    expect(itemLabel(1201, { name: '나이프', unidentified: '', slots: 3 })).toBe('나이프 [3]');
    expect(itemLabel(501, undefined)).toBe('Item 501');
    expect(itemLabel(8, { name: '', unidentified: 'x', slots: 1 })).toBe('Item 8');
  });
});

describe('colorRuns', () => {
  it('splits a line at color codes', () => {
    expect(colorRuns('무게 : ^7777777^000000')).toEqual([
      { text: '무게 : ', color: undefined },
      { text: '7', color: '#777777' },
    ]);
    expect(colorRuns('^FFFFFF_^000000 end')).toEqual([
      { text: '_', color: '#ffffff' },
      { text: ' end', color: '#000000' },
    ]);
    // A caret without six hex digits is text.
    expect(colorRuns('a ^12 b')).toEqual([{ text: 'a ^12 b', color: undefined }]);
    expect(colorRuns('')).toEqual([]);
  });

  it('strips color codes', () => {
    expect(stripColors('^000088HP^000000 회복')).toBe('HP 회복');
  });
});

describe('ItemTexts', () => {
  it('loads names once, on first need, and descriptions on demand', async () => {
    const table = vi.fn(async (name: string): Promise<unknown> => {
      await Promise.resolve();
      if (name === ITEM_NAMES) {
        return { '501': { n: '빨간포션' } };
      }
      return name === ITEM_DESCRIPTIONS ? { '501': ['line'] } : undefined;
    });
    const texts = new ItemTexts({ table });
    let loads = 0;
    texts.onLoad(() => {
      loads++;
    });
    expect(table).not.toHaveBeenCalled();
    expect(texts.label(501)).toBe('Item 501');
    expect(texts.label(502)).toBe('Item 502');
    await texts.loadNames();
    expect(table).toHaveBeenCalledTimes(1);
    expect(loads).toBe(1);
    expect(texts.label(501)).toBe('빨간포션');
    expect(texts.label(502)).toBe('Item 502');
    expect(await texts.description(501)).toEqual(['line']);
    expect(await texts.description(502)).toBeUndefined();
    expect(table).toHaveBeenCalledTimes(2);
  });

  it('keeps IDs when the build has no table', async () => {
    const texts = new ItemTexts({ table: async () => Promise.resolve(undefined) });
    await texts.loadNames();
    expect(texts.label(501)).toBe('Item 501');
  });
});
