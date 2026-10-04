import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import { WelcomeSchema } from '../gen/wathena/v1/session_pb';
import { DEFAULT_LIMITS, limitsOf, unpackSteps } from './connection';

describe('limitsOf', () => {
  it('takes the limits Welcome gives', () => {
    const welcome = create(WelcomeSchema, {
      limits: {
        partyMembers: 8,
        friends: 20,
        blockList: 30,
        guildMembers: 10,
        guildPositions: 8,
        guildAllies: 2,
        guildEnemies: 4,
        guildStorage: 600,
        inventorySlots: 120,
        storageSlots: 1000,
        cartSlots: 150,
      },
    });
    const limits = limitsOf(welcome);
    expect({ ...limits, ranges: undefined }).toEqual({
      partyMembers: 8,
      friends: 20,
      blockList: 30,
      guildMembers: 10,
      guildPositions: 8,
      guildAllies: 2,
      guildEnemies: 4,
      guildStorage: 600,
      inventorySlots: 120,
      storageSlots: 1000,
      cartSlots: 150,
      ranges: undefined,
    });
    // Without ranges, max is rAthena's, and never under the base.
    expect(limits.ranges.get('inventory_slots')).toEqual({ base: 120, max: 200 });
    expect(limits.ranges.get('storage_slots')).toEqual({ base: 1000, max: 1000 });
  });

  it('keeps the base and max of every range', () => {
    const limits = limitsOf(
      create(WelcomeSchema, {
        limits: {
          cartSlots: 100,
          ranges: [
            { name: 'cart_slots', base: 100, max: 300 },
            { name: 'storage_slots', base: 800, max: 10000 },
            { name: 'pet_slots', base: 1, max: 3 },
            { name: 'odd', base: 5, max: 2 },
          ],
        },
      }),
    );
    expect(limits.cartSlots).toBe(100);
    expect(limits.ranges.get('cart_slots')).toEqual({ base: 100, max: 300 });
    // A range gives the base when the named field is 0.
    expect(limits.storageSlots).toBe(800);
    expect(limits.ranges.get('storage_slots')).toEqual({ base: 800, max: 10000 });
    // Limits the client does not know are kept.
    expect(limits.ranges.get('pet_slots')).toEqual({ base: 1, max: 3 });
    expect(limits.ranges.get('odd')).toEqual({ base: 5, max: 5 });
  });

  it("uses rAthena's defaults for missing limits", () => {
    expect(limitsOf(create(WelcomeSchema))).toEqual(DEFAULT_LIMITS);
    expect(DEFAULT_LIMITS.cartSlots).toBe(100);
    expect(DEFAULT_LIMITS.ranges.get('guild_members')).toEqual({ base: 16, max: 76 });
    const limits = limitsOf(create(WelcomeSchema, { limits: { friends: 10 } }));
    expect({ ...limits, ranges: undefined }).toEqual({
      ...DEFAULT_LIMITS,
      friends: 10,
      ranges: undefined,
    });
    expect(limits.ranges.get('friends')).toEqual({ base: 10, max: 40 });
  });
});

describe('unpackSteps', () => {
  it('reads facings low bits first and skips 0xF', () => {
    expect(unpackSteps(new Uint8Array([0x21, 0xf3]))).toEqual([1, 2, 3]);
  });
});
