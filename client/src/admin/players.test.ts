import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import { LogEntrySchema } from '../gen/wathena/admin/v1/admin_pb';
import { detailTabsFor, logAmount, rollsText, searchKindsFor } from './players';

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
