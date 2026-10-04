import { describe, expect, it } from 'vitest';
import { StorageKind, StorageOutcome } from '../gen/wathena/v1/event_pb';
import {
  STORAGE_PAGE_SIZE,
  StorageModel,
  cartWeightText,
  clampPage,
  movePrompt,
  moveWords,
  pageCount,
  storageLogLine,
  storageMessage,
  storageTitle,
} from './storage';

describe('StorageModel', () => {
  it('opens with the stored items and the size', () => {
    const m = new StorageModel();
    expect(m.isOpen).toBe(false);
    m.open({
      kind: StorageKind.GUILD,
      items: [
        { slot: 4, itemId: 909, amount: 10 },
        { slot: 1, itemId: 1201, amount: 1, refine: 7, cards: [4001] },
        { slot: 2, itemId: 501, amount: 0 },
      ],
      size: 500,
    });
    expect(m.isOpen).toBe(true);
    expect(m.list().map((s) => s.slot)).toEqual([1, 4]);
    expect(m.get(1)).toMatchObject({ itemId: 1201, refine: 7, cards: [4001], equip: 0 });
    expect(m.countText).toBe('2 / 500');
  });

  it('takes the slot StorageAdd gives, and removes amounts', () => {
    const m = new StorageModel();
    m.open({ kind: StorageKind.GUILD, items: [{ slot: 0, itemId: 909, amount: 10 }], size: 100 });
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
    m.open({ kind: StorageKind.GUILD, items: [{ slot: 0, itemId: 909, amount: 1 }], size: 100 });
    m.close();
    expect(m.isOpen).toBe(false);
    expect(m.list()).toEqual([]);
    expect(m.countText).toBe('0 / 0');
  });

  it('adds the pages after StorageOpen', () => {
    const m = new StorageModel();
    m.open({
      kind: StorageKind.PERSONAL,
      items: [{ slot: 0, itemId: 909, amount: 1 }],
      size: 10000,
      displayName: 'Storage',
      pages: 2,
    });
    expect(m.loading).toBe(true);
    // A page of another storage, or one not waited for, is ignored.
    expect(m.addPage(StorageKind.GUILD, 1, [{ slot: 5, itemId: 501, amount: 1 }])).toBe(false);
    expect(m.addPage(StorageKind.PERSONAL, 3, [{ slot: 5, itemId: 501, amount: 1 }])).toBe(false);
    // Pages may come in any order.
    expect(m.addPage(StorageKind.PERSONAL, 2, [{ slot: 9000, itemId: 502, amount: 4 }])).toBe(true);
    expect(m.loading).toBe(true);
    expect(m.addPage(StorageKind.PERSONAL, 1, [{ slot: 10, itemId: 501, amount: 2 }])).toBe(true);
    expect(m.loading).toBe(false);
    // A page that came already is ignored.
    expect(m.addPage(StorageKind.PERSONAL, 1, [{ slot: 11, itemId: 501, amount: 2 }])).toBe(false);
    expect(m.list().map((s) => s.slot)).toEqual([0, 10, 9000]);
    expect(m.countText).toBe('3 / 10000');
  });

  it('keeps changes that came before a later page', () => {
    const m = new StorageModel();
    m.open({ kind: StorageKind.PERSONAL, items: [], size: 600, pages: 1 });
    // StorageAdd gives the slot after the change, newer than the page.
    m.add({ slot: 7, itemId: 909, amount: 12 });
    m.addPage(StorageKind.PERSONAL, 1, [
      { slot: 7, itemId: 909, amount: 10 },
      { slot: 8, itemId: 501, amount: 1 },
    ]);
    expect(m.get(7)?.amount).toBe(12);
    expect(m.get(8)?.amount).toBe(1);
  });

  it('pages a large storage', () => {
    const m = new StorageModel();
    const items = Array.from({ length: 250 }, (_, i) => ({
      slot: 249 - i,
      itemId: 909,
      amount: 1,
    }));
    m.open({ kind: StorageKind.PERSONAL, items, size: 10000 });
    expect(pageCount(m.count)).toBe(3);
    expect(m.page(0)).toHaveLength(STORAGE_PAGE_SIZE);
    expect(m.page(0)[0]?.slot).toBe(0);
    expect(m.page(2).map((s) => s.slot)).toEqual(Array.from({ length: 50 }, (_, i) => 200 + i));
    expect(m.page(3)).toEqual([]);
    // The sorted list is kept until the next change.
    expect(m.list()).toBe(m.list());
    m.remove(0, 1);
    expect(m.page(0)[0]?.slot).toBe(1);
  });

  it('keeps the cart weight', () => {
    const m = new StorageModel();
    m.open({ kind: StorageKind.CART, items: [], size: 100, weight: 120, maxWeight: 80000 });
    m.add({ slot: 0, itemId: 909, amount: 10 }, 130);
    expect(m.weight).toBe(130);
    m.remove(0, 5, 125);
    expect(m.weight).toBe(125);
    // StorageRemove of a slot the client does not know still gives the weight.
    m.remove(4, 1, 100);
    expect(m.weight).toBe(100);
    expect(m.maxWeight).toBe(80000);
    // Storage has no weight.
    const s = new StorageModel();
    s.open({ kind: StorageKind.PERSONAL, items: [], size: 600 });
    s.add({ slot: 0, itemId: 909, amount: 1 }, 0);
    expect(s.weight).toBe(0);
  });
});

describe('paging', () => {
  it('counts pages and keeps the page inside them', () => {
    expect(pageCount(0)).toBe(1);
    expect(pageCount(100)).toBe(1);
    expect(pageCount(101)).toBe(2);
    expect(pageCount(10000)).toBe(100);
    expect(pageCount(5, 2)).toBe(3);
    expect(clampPage(-1, 3)).toBe(0);
    expect(clampPage(5, 3)).toBe(2);
    expect(clampPage(1, 3)).toBe(1);
    expect(clampPage(0, 0)).toBe(0);
  });
});

describe('storage texts', () => {
  it('names the storage', () => {
    expect(storageTitle(StorageKind.GUILD)).toBe('Guild storage');
    expect(storageTitle(StorageKind.UNSPECIFIED)).toBe('Storage');
    expect(storageTitle(StorageKind.PERSONAL)).toBe('Storage');
    expect(storageTitle(StorageKind.PERSONAL, 'Premium storage')).toBe('Premium storage');
    expect(storageTitle(StorageKind.CART)).toBe('Cart');
  });

  it('shows the cart weight in whole units', () => {
    expect(cartWeightText(1205, 80000)).toBe('Weight 120 / 8000');
    expect(cartWeightText(0, 0)).toBe('Weight 0 / 0');
  });

  it('asks how many to move', () => {
    expect(movePrompt(moveWords(false).put, 'Jellopy', 10)).toBe(
      'How many Jellopy to store? (1 to 10) ',
    );
    expect(movePrompt(moveWords(true).send, 'Jellopy', 3)).toBe(
      'How many Jellopy to move to the storage? (1 to 3) ',
    );
  });

  it('says why a storage action failed', () => {
    expect(storageMessage(StorageOutcome.IN_USE)).toBe(
      'Another guild member is using the guild storage.',
    );
    expect(storageMessage(StorageOutcome.FULL)).toBe('The storage is full.');
    expect(storageMessage(StorageOutcome.CANNOT_CARRY)).toBe('You cannot carry that item.');
    expect(storageMessage(StorageOutcome.RESTRICTED)).toBe('This item cannot be stored.');
    expect(storageMessage(StorageOutcome.BOUND)).toBe('This bounded item cannot be stored there.');
    expect(storageMessage(StorageOutcome.NO_CART)).toBe('You do not have a cart.');
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
