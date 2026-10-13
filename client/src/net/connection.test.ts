import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import { DisconnectReason, WelcomeSchema } from '../gen/wathena/v1/session_pb';
import {
  DEFAULT_LIMITS,
  disconnectDelay,
  disconnectText,
  limitsOf,
  unpackSteps,
} from './connection';

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
        mailItems: 3,
        mailbox: 50,
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
      mailItems: 3,
      mailbox: 50,
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
    expect(DEFAULT_LIMITS.mailItems).toBe(5);
    expect(DEFAULT_LIMITS.mailbox).toBe(100);
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

describe('disconnectText', () => {
  it("shows the server's message, or a line for the reason", () => {
    expect(disconnectText('Bye.', DisconnectReason.AUTOTRADE)).toBe('Bye.');
    expect(disconnectText('', DisconnectReason.AUTOTRADE)).toBe(
      'Your shop stays open while you are away.',
    );
    expect(disconnectText('')).toBe('Disconnected from the server.');
  });

  it('shows the message of a kick and a ban', () => {
    expect(disconnectText('Kicked: spam', DisconnectReason.KICKED)).toBe('Kicked: spam');
    expect(
      disconnectText(
        'Your account is banned until 2026-10-20 15:04 UTC. Reason: bot',
        DisconnectReason.BANNED,
      ),
    ).toBe('Your account is banned until 2026-10-20 15:04 UTC. Reason: bot');
    expect(disconnectText('', DisconnectReason.KICKED)).toBe('A game master disconnected you.');
    expect(disconnectText('', DisconnectReason.BANNED)).toBe('Your account is banned.');
  });

  it('shows the reason of a planned restart', () => {
    expect(
      disconnectText(
        'The server is restarting: update. Come back in a few minutes.',
        DisconnectReason.RESTART,
      ),
    ).toBe('The server is restarting: update. Come back in a few minutes.');
    expect(disconnectText('', DisconnectReason.RESTART)).toBe(
      'The server is restarting. Come back in a few minutes.',
    );
  });
});

describe('disconnectDelay', () => {
  it('leaves at once for a bad ticket and waits longer for a kick or a ban', () => {
    expect(disconnectDelay(DisconnectReason.SESSION_INVALID)).toBe(0);
    expect(disconnectDelay(DisconnectReason.KICKED)).toBe(10_000);
    expect(disconnectDelay(DisconnectReason.BANNED)).toBe(10_000);
    expect(disconnectDelay(DisconnectReason.RESTART)).toBe(10_000);
    expect(disconnectDelay(DisconnectReason.LOGGED_OUT)).toBe(3000);
    expect(disconnectDelay()).toBe(3000);
  });
});
