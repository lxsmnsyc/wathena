import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import { LogEntrySchema } from '../gen/wathena/admin/v1/admin_pb';
import { detailTabsFor, logAmount, rollsText, sanctionTargetOf, searchKindsFor } from './players';

describe('permission gating', () => {
  it('offers the IP search only with players.read_ip', () => {
    expect(searchKindsFor(['players.read'])).toEqual(['name', 'id', 'serial']);
    expect(searchKindsFor(['players.read', 'players.read_ip'])).toEqual([
      'name',
      'id',
      'serial',
      'ip',
    ]);
  });

  it('shows the Logs tab only with items.history', () => {
    expect(detailTabsFor(['players.read'])).not.toContain('logs');
    expect(detailTabsFor(['players.read', 'items.history'])).toEqual([
      'overview',
      'inventory',
      'equipment',
      'storage',
      'mail',
      'logs',
    ]);
  });

  it('shows the Sanctions tab with any moderation permission', () => {
    expect(detailTabsFor(['players.read'])).not.toContain('sanctions');
    expect(detailTabsFor(['players.read', 'moderation.reports'])).toContain('sanctions');
  });

  it('offers the sanctions of the role, with a ban of the account', () => {
    const c = { id: 150000n, name: 'Bob', accountId: 2000001n, account: 'bob' };
    expect(sanctionTargetOf(['moderation.mute', 'moderation.kick'], c).kinds).toEqual([
      'mute',
      'kick',
    ]);
    const all = sanctionTargetOf(
      ['moderation.mute', 'moderation.jail', 'moderation.kick', 'moderation.ban'],
      c,
    );
    expect(all).toEqual({
      characterId: 150000n,
      character: 'Bob',
      accountId: 2000001n,
      account: 'bob',
      kinds: ['mute', 'jail', 'kick', 'ban'],
    });
    expect(sanctionTargetOf(['moderation.reports'], c).kinds).toEqual([]);
  });
});

describe('item and log cells', () => {
  it('writes rolls sorted by name', () => {
    expect(rollsText({ matk: 3, atk: 5 })).toBe('atk=5 matk=3');
    expect(rollsText({})).toBe('');
  });

  it('writes a zeny line with its currency', () => {
    expect(logAmount(create(LogEntrySchema, { kind: 'item', amount: 3n }))).toBe('3');
    expect(logAmount(create(LogEntrySchema, { kind: 'zeny', amount: 500n }))).toBe('500 zeny');
    expect(
      logAmount(create(LogEntrySchema, { kind: 'zeny', amount: 2n, currencyName: 'Kafra Points' })),
    ).toBe('2 Kafra Points');
  });
});
