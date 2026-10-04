import { describe, expect, it } from 'vitest';
import { StorageKind, StorageOutcome } from '../gen/wathena/v1/event_pb';
import { StorageModel, storageLogLine, storageMessage, storageTitle } from './storage';

describe('StorageModel', () => {
  it('opens with the stored items and the size', () => {
    const m = new StorageModel();
    expect(m.isOpen).toBe(false);
    m.open(
      StorageKind.GUILD,
      [
        { slot: 4, itemId: 909, amount: 10 },
        { slot: 1, itemId: 1201, amount: 1, refine: 7, cards: [4001] },
        { slot: 2, itemId: 501, amount: 0 },
      ],
      500,
    );
    expect(m.isOpen).toBe(true);
    expect(m.list().map((s) => s.slot)).toEqual([1, 4]);
    expect(m.get(1)).toMatchObject({ itemId: 1201, refine: 7, cards: [4001], equip: 0 });
    expect(m.countText).toBe('2 / 500');
  });

  it('takes the slot StorageAdd gives, and removes amounts', () => {
    const m = new StorageModel();
    m.open(StorageKind.GUILD, [{ slot: 0, itemId: 909, amount: 10 }], 100);
    // StorageAdd gives the slot after the change.
    m.add({ slot: 0, itemId: 909, amount: 13 });
    m.add({ slot: 3, itemId: 501, amount: 2 });
    expect(m.get(0)?.amount).toBe(13);
    expect(m.list().map((s) => s.slot)).toEqual([0, 3]);
    m.remove(0, 3);
    expect(m.get(0)?.amount).toBe(10);
    m.remove(3, 2);
    expect(m.get(3)).toBeUndefined();
    m.remove(9, 1);
    expect(m.countText).toBe('1 / 100');
  });

  it('forgets everything when it closes', () => {
    const m = new StorageModel();
    m.open(StorageKind.GUILD, [{ slot: 0, itemId: 909, amount: 1 }], 100);
    m.close();
    expect(m.isOpen).toBe(false);
    expect(m.list()).toEqual([]);
    expect(m.countText).toBe('0 / 0');
  });
});

describe('storage texts', () => {
  it('names the storage', () => {
    expect(storageTitle(StorageKind.GUILD)).toBe('Guild storage');
    expect(storageTitle(StorageKind.UNSPECIFIED)).toBe('Storage');
  });

  it('says why a storage action failed', () => {
    expect(storageMessage(StorageOutcome.IN_USE)).toBe(
      'Another guild member is using the guild storage.',
    );
    expect(storageMessage(StorageOutcome.FULL)).toBe('The storage is full.');
    expect(storageMessage(StorageOutcome.CANNOT_CARRY)).toBe('You cannot carry that item.');
    for (const value of Object.values(StorageOutcome)) {
      if (typeof value === 'number') {
        expect(storageMessage(value)).toMatch(/^[A-Z].*\.$/);
      }
    }
  });

  it('writes log lines for items put in and taken out', () => {
    const label = (id: number): string => (id === 909 ? 'Jellopy' : 'Knife');
    expect(
      storageLogLine({ atMs: 0n, name: 'Bob', itemId: 909, amount: 3, refine: 0 }, label),
    ).toBe('Bob put in Jellopy (3)');
    expect(
      storageLogLine({ atMs: 0n, name: 'Bob', itemId: 1201, amount: -1, refine: 7 }, label),
    ).toBe('Bob took out +7 Knife');
    expect(
      storageLogLine({ atMs: 0n, name: 'Bob', itemId: 909, amount: 1, refine: 0 }, label),
    ).toBe('Bob put in Jellopy');
  });
});
