import { describe, expect, it } from 'vitest';
import { StorageKind } from '../gen/wathena/v1/event_pb';
import { applyPlayerLimits, playerLimitsOf } from './limits';

describe('playerLimitsOf', () => {
  it('maps each limit by name and skips empty names', () => {
    const m = playerLimitsOf([
      { name: 'inventory_slots', value: 110 },
      { name: '', value: 5 },
      { name: 'cart_slots', value: 120 },
    ]);
    expect([...m]).toEqual([
      ['inventory_slots', 110],
      ['cart_slots', 120],
    ]);
  });
});

describe('applyPlayerLimits', () => {
  it('sizes the inventory, the open account storage and the cart', () => {
    const inventory = { size: 100 };
    const storage = { kind: StorageKind.PERSONAL, name: '', size: 600 };
    const cart = { size: 100 };
    const limits = new Map([
      ['inventory_slots', 130],
      ['storage_slots', 650],
      ['cart_slots', 110],
    ]);
    expect(applyPlayerLimits(limits, inventory, storage, cart)).toBe(true);
    expect([inventory.size, storage.size, cart.size]).toEqual([130, 650, 110]);
    // The same values change nothing a window draws.
    expect(applyPlayerLimits(limits, inventory, storage, cart)).toBe(false);
  });

  it('leaves an extra storage, a closed storage and no cart as they are', () => {
    const limits = new Map([
      ['storage_slots', 650],
      ['cart_slots', 110],
    ]);
    const extra = { kind: StorageKind.PERSONAL, name: 'example.bank:vault', size: 50 };
    const closed = { kind: StorageKind.UNSPECIFIED, name: '', size: 0 };
    const noCart = { size: 0 };
    const inventory = { size: 100 };
    expect(applyPlayerLimits(limits, inventory, extra, noCart)).toBe(false);
    expect(applyPlayerLimits(limits, inventory, closed, noCart)).toBe(false);
    expect([extra.size, closed.size, noCart.size, inventory.size]).toEqual([50, 0, 0, 100]);
  });
});
