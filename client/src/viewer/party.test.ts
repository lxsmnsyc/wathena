import { describe, expect, it } from 'vitest';
import { PartyOutcome } from '../gen/wathena/v1/event_pb';
import {
  NO_PARTY,
  type PartyMemberInput,
  hpPercent,
  leads,
  mapLabel,
  memberCount,
  memberDetail,
  mergeMember,
  partyAfterResult,
  partyFromInfo,
} from './party';

function member(name: string, values: Partial<PartyMemberInput> = {}): PartyMemberInput {
  return {
    name,
    leader: false,
    online: true,
    map: 'map/prontera',
    job: 7,
    baseLevel: 50,
    hp: 900n,
    maxHp: 1000n,
    ...values,
  };
}

const info = {
  id: 12n,
  name: 'Heroes',
  options: { expEven: true, itemEven: false, pickupShared: true },
  members: [member('Alice', { leader: true }), member('Bob', { job: 4, baseLevel: 30 })],
};

describe('partyFromInfo', () => {
  it('replaces the whole party, keeping join order', () => {
    const party = partyFromInfo(info);
    expect(party.id).toBe(12n);
    expect(party.name).toBe('Heroes');
    expect(party.options).toEqual({ expEven: true, itemEven: false, pickupShared: true });
    expect(party.members.map((m) => m.name)).toEqual(['Alice', 'Bob']);
    expect(party.members[0]).toMatchObject({ leader: true, hp: 900, maxHp: 1000 });
    const next = partyFromInfo({ ...info, members: [member('Bob', { leader: true })] });
    expect(next.members.map((m) => m.name)).toEqual(['Bob']);
  });

  it('gives no party for id 0', () => {
    expect(partyFromInfo({ id: 0n, name: '', members: [] })).toBe(NO_PARTY);
  });
});

describe('mergeMember', () => {
  it('changes one member and keeps the others', () => {
    const party = partyFromInfo(info);
    const next = mergeMember(party, member('Bob', { hp: 10n, maxHp: 500n, job: 4, baseLevel: 31 }));
    expect(next.members[1]).toMatchObject({ name: 'Bob', hp: 10, maxHp: 500, baseLevel: 31 });
    expect(next.members[0]).toBe(party.members[0]);
    // The old state is not changed.
    expect(party.members[1]?.hp).toBe(900);
  });

  it('keeps the leader mark and the values an update leaves empty', () => {
    const party = partyFromInfo(info);
    const next = mergeMember(
      party,
      member('Alice', { leader: false, map: '', job: 0, baseLevel: 0, hp: 0n, maxHp: 0n }),
    );
    expect(next.members[0]).toMatchObject({
      leader: true,
      map: 'map/prontera',
      job: 7,
      baseLevel: 50,
      hp: 900,
      maxHp: 1000,
    });
  });

  it('takes 0 HP when the update has max HP', () => {
    const next = mergeMember(partyFromInfo(info), member('Bob', { hp: 0n, maxHp: 1000n }));
    expect(next.members[1]?.hp).toBe(0);
  });

  it('takes the online state', () => {
    const next = mergeMember(partyFromInfo(info), member('Bob', { online: false }));
    expect(next.members[1]?.online).toBe(false);
  });

  it('ignores names the party does not have, and updates without a party', () => {
    const party = partyFromInfo(info);
    expect(mergeMember(party, member('Eve'))).toBe(party);
    expect(mergeMember(NO_PARTY, member('Alice'))).toBe(NO_PARTY);
  });
});

describe('partyAfterResult', () => {
  const party = partyFromInfo(info);

  it('resets when the player leaves, is expelled, or the party is disbanded', () => {
    expect(partyAfterResult(party, PartyOutcome.LEFT, 'Bob', 'Bob')).toBe(NO_PARTY);
    expect(partyAfterResult(party, PartyOutcome.EXPELLED, 'Bob', 'Bob')).toBe(NO_PARTY);
    expect(partyAfterResult(party, PartyOutcome.DISBANDED, 'Heroes', 'Bob')).toBe(NO_PARTY);
  });

  it('keeps the party when another member leaves', () => {
    expect(partyAfterResult(party, PartyOutcome.LEFT, 'Alice', 'Bob')).toBe(party);
    expect(partyAfterResult(party, PartyOutcome.JOINED, 'Bob', 'Bob')).toBe(party);
    expect(partyAfterResult(party, PartyOutcome.LEFT, 'Bob', undefined)).toBe(party);
  });
});

describe('party helpers', () => {
  it('knows the leader', () => {
    const party = partyFromInfo(info);
    expect(leads(party, 'Alice')).toBe(true);
    expect(leads(party, 'Bob')).toBe(false);
    expect(leads(party, undefined)).toBe(false);
    expect(leads(NO_PARTY, 'Alice')).toBe(false);
  });

  it('gives the HP percent', () => {
    expect(hpPercent(500, 1000)).toBe(50);
    expect(hpPercent(999, 1000)).toBe(99);
    expect(hpPercent(0, 1000)).toBe(0);
    expect(hpPercent(-5, 1000)).toBe(0);
    expect(hpPercent(10, 0)).toBe(0);
    expect(hpPercent(2000, 1000)).toBe(100);
  });

  it('formats members and counts', () => {
    expect(mapLabel('map/prt_fild08')).toBe('prt_fild08');
    expect(memberDetail({ job: 7, baseLevel: 50, map: 'map/prontera' })).toBe(
      'Lv 50 Knight, prontera',
    );
    expect(memberDetail({ job: 7, baseLevel: 50, map: '' }, new Map([[7, 'Rune Knight']]))).toBe(
      'Lv 50 Rune Knight',
    );
    expect(memberCount(partyFromInfo(info), 12)).toBe('(2 / 12)');
  });
});
