import { describe, expect, it } from 'vitest';
import { restoreRequest } from './restore';

const base = {
  characterId: '150000',
  rows: [{ serial: ' s1 ', amount: '' }],
  zeny: '',
  reason: 'lost in a crash',
};

describe('restoreRequest', () => {
  it('makes a request from the fields', () => {
    const r = restoreRequest({ ...base, rows: [...base.rows, { serial: '', amount: '3' }] }, false);
    expect(r.ok).toBe(true);
    if (r.ok) {
      expect(r.value.characterId).toBe(150000n);
      expect(r.value.items.map((i) => [i.serial, i.amount])).toEqual([['s1', 0]]);
      expect(r.value.zeny).toBe(0n);
      expect(r.value.reason).toBe('lost in a crash');
    }
  });

  it('takes zeny only with items.create', () => {
    expect(restoreRequest({ ...base, zeny: '500' }, false).ok).toBe(false);
    const r = restoreRequest({ ...base, rows: [], zeny: '500' }, true);
    expect(r.ok && r.value.zeny).toBe(500n);
  });

  it('refuses missing or bad fields', () => {
    const fails = (f: typeof base, canCreate = true) => !restoreRequest(f, canCreate).ok;
    expect(fails({ ...base, characterId: '' })).toBe(true);
    expect(fails({ ...base, characterId: 'Bob' })).toBe(true);
    expect(fails({ ...base, reason: '  ' })).toBe(true);
    expect(fails({ ...base, reason: 'x'.repeat(256) })).toBe(true);
    expect(fails({ ...base, rows: [{ serial: 's1', amount: '-1' }] })).toBe(true);
    expect(fails({ ...base, rows: [{ serial: 's1', amount: '3000000000' }] })).toBe(true);
    expect(fails({ ...base, rows: [] })).toBe(true);
  });
});
