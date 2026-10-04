import { describe, expect, it } from 'vitest';
import { StoreSearchOutcome } from '../gen/wathena/v1/event_pb';
import { findItems } from './itemText';
import {
  parseRollFilter,
  placeText,
  resolveItems,
  storeSearchMessage,
  storeSearchProblem,
  usesText,
} from './storeSearch';

const names = new Map([
  [909, { name: 'Jellopy', unidentified: '', slots: 0 }],
  [501, { name: 'Red Potion', unidentified: '', slots: 0 }],
  [502, { name: 'Orange Potion', unidentified: '', slots: 0 }],
  [4001, { name: 'Poring Card', unidentified: '', slots: 0 }],
]);
const find = (q: string) => findItems(names, q);
const label = (id: number) => names.get(id)?.name ?? `Item ${String(id)}`;

describe('findItems', () => {
  it('finds by ID, exact name, or part of a name', () => {
    expect(find('909')).toEqual([909]);
    expect(find('jellopy')).toEqual([909]);
    expect(find('potion')).toEqual([501, 502]);
    expect(find('red potion')).toEqual([501]);
    expect(find('nothing')).toEqual([]);
  });
});

describe('resolveItems', () => {
  it('resolves names and IDs, and lists what did not resolve', () => {
    expect(resolveItems('Jellopy, 4001, red pot', find, label)).toEqual({
      ids: [909, 4001, 501],
      problems: [],
    });
    const r = resolveItems('potion, zzz', find, label);
    expect(r.ids).toEqual([]);
    expect(r.problems).toEqual([
      'potion matches Red Potion (501), Orange Potion (502). Type more of the name, or the ID.',
      'No item is named zzz.',
    ]);
  });
});

describe('parseRollFilter', () => {
  it('reads a name and a range', () => {
    expect(parseRollFilter({ name: 'VAR_MAXHPAMOUNT', min: '100', max: '' })).toEqual({
      name: 'VAR_MAXHPAMOUNT',
      min: 100,
      max: 2147483647,
    });
    expect(parseRollFilter({ name: '', min: '1', max: '2' })).toBeUndefined();
    expect(parseRollFilter({ name: 'atk', min: '5', max: '1' })).toBe(
      'The range of atk starts above its end.',
    );
    expect(parseRollFilter({ name: 'atk', min: 'x', max: '1' })).toBe(
      'The range of atk is whole numbers.',
    );
  });
});

describe('storeSearchProblem', () => {
  const q = {
    buying: false,
    itemIds: [909],
    cardIds: [],
    minPrice: 0,
    maxPrice: 0,
    rolls: [],
    options: [],
  };

  it('checks the items, the prices and the filters', () => {
    expect(storeSearchProblem(q)).toBeUndefined();
    expect(storeSearchProblem({ ...q, itemIds: [] })).toBe('Name at least one item to find.');
    expect(storeSearchProblem({ ...q, itemIds: Array.from({ length: 11 }, (_, i) => i) })).toBe(
      'A search finds at most 10 items.',
    );
    expect(storeSearchProblem({ ...q, minPrice: 10, maxPrice: 5 })).toBe(
      'The lowest price is above the highest.',
    );
    const roll = { name: 'atk', min: 1, max: 2 };
    expect(storeSearchProblem({ ...q, rolls: Array.from({ length: 9 }, () => roll) })).toBe(
      'A search has at most 8 roll filters and 8 option filters.',
    );
  });
});

describe('storeSearchMessage', () => {
  it('names every outcome, and a store place', () => {
    for (const o of Object.values(StoreSearchOutcome).filter((v) => typeof v === 'number')) {
      expect(storeSearchMessage(o)).not.toBe('');
    }
    expect(storeSearchMessage(StoreSearchOutcome.POSITION, 'map/prontera', 150, 180)).toBe(
      'The store is at prontera (150, 180).',
    );
    expect(placeText('map/payon', 1, 2)).toBe('payon (1, 2)');
    expect(usesText(1)).toBe('1 search left');
    expect(usesText(3)).toBe('3 searches left');
  });
});
