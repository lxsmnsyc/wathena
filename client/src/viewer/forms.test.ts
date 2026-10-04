import { describe, expect, it } from 'vitest';
import { bytesProblem, mapLabel, parseWhole, utf8Length, zenyLabel } from './forms';

describe('utf8Length', () => {
  it('counts UTF-8 bytes, as the server does', () => {
    expect(utf8Length('abc')).toBe(3);
    expect(utf8Length('é')).toBe(2);
    expect(utf8Length('안녕')).toBe(6);
  });
});

describe('bytesProblem', () => {
  it('checks the byte range', () => {
    expect(bytesProblem('The title', 'Hi', 1, 39)).toBeUndefined();
    expect(bytesProblem('The title', '', 1, 39)).toBe('The title is empty.');
    expect(bytesProblem('The title', 'a'.repeat(40), 1, 39)).toBe(
      'The title is too long: 40 of at most 39 bytes.',
    );
    // 20 Hangul letters are 60 bytes.
    expect(bytesProblem('The title', '가'.repeat(20), 1, 39)).toBe(
      'The title is too long: 60 of at most 39 bytes.',
    );
  });
});

describe('parseWhole', () => {
  it('reads whole numbers in range, with separators', () => {
    expect(parseWhole('1,000', 1, 5000)).toBe(1000);
    expect(parseWhole(' 7 ', 1, 10)).toBe(7);
    expect(parseWhole('0', 1, 10)).toBeUndefined();
    expect(parseWhole('11', 1, 10)).toBeUndefined();
    expect(parseWhole('1.5', 1, 10)).toBeUndefined();
    expect(parseWhole('', 0, 10)).toBeUndefined();
  });
});

describe('labels', () => {
  it('formats zeny and map names', () => {
    expect(zenyLabel(1500)).toBe('1,500 z');
    expect(zenyLabel(2n)).toBe('2 z');
    expect(mapLabel('map/prontera')).toBe('prontera');
  });
});
