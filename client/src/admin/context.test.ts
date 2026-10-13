import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import { MeSchema } from '../gen/wathena/admin/v1/admin_pb';
import { isSelf, tabsFor } from './context';

describe('tabsFor', () => {
  it('shows only the tabs of the role', () => {
    expect(tabsFor(['players.read']).map((t) => t.kind)).toEqual(['players']);
    expect(tabsFor(['audit.read', 'items.history', 'items.restore']).map((t) => t.kind)).toEqual([
      'audit',
      'reviews',
      'item',
      'restore',
    ]);
    expect(tabsFor([])).toEqual([]);
  });
});

describe('isSelf', () => {
  it('compares account names in any case', () => {
    const me = create(MeSchema, { account: 'GameMaster' });
    expect(isSelf(me, 'gamemaster')).toBe(true);
    expect(isSelf(me, 'Other')).toBe(false);
  });
});
