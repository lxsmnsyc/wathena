import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { describe, expect, it } from 'vitest';
import { SanctionSchema } from '../gen/wathena/admin/v1/admin_pb';
import {
  MAX_BAN_MINUTES,
  MAX_MUTE_MINUTES,
  type SanctionFields,
  canLift,
  canModerate,
  characterKindsFor,
  endText,
  givenText,
  sanctionPermission,
  sanctionRequest,
  stateText,
  targetText,
} from './sanctions';

const ALL = [
  'moderation.mute',
  'moderation.jail',
  'moderation.kick',
  'moderation.ban',
  'moderation.ban_permanent',
];

const mute: SanctionFields = {
  kind: 'mute',
  characterId: 150000n,
  accountId: 2000001n,
  minutes: ' 30 ',
  permanent: false,
  reason: ' spam ',
};

describe('permission gating', () => {
  it('offers the character sanctions of the role', () => {
    expect(characterKindsFor(['moderation.jail', 'moderation.ban'])).toEqual(['jail']);
    expect(characterKindsFor(ALL)).toEqual(['mute', 'jail', 'kick']);
    expect(characterKindsFor(['players.read'])).toEqual([]);
    expect(sanctionPermission('ban')).toBe('moderation.ban');
  });

  it('counts any moderation permission', () => {
    expect(canModerate(['moderation.reports'])).toBe(true);
    expect(canModerate(['players.read', 'commands.hide'])).toBe(false);
  });

  it('lifts a sanction in effect with the permission of its kind', () => {
    const active = create(SanctionSchema, { id: 4n, kind: 'mute', active: true });
    expect(canLift(['moderation.mute'], active)).toBe(true);
    expect(canLift(['moderation.jail'], active)).toBe(false);
    expect(canLift(['moderation.mute'], create(SanctionSchema, { ...active, active: false }))).toBe(
      false,
    );
    expect(canLift(['moderation.mute'], create(SanctionSchema, { ...active, id: 0n }))).toBe(false);
    const kick = create(SanctionSchema, { id: 5n, kind: 'kick', active: true });
    expect(canLift(['moderation.kick'], kick)).toBe(false);
    const ban = create(SanctionSchema, { id: 6n, kind: 'ban', active: true, permanent: true });
    expect(canLift(['moderation.ban'], ban)).toBe(false);
    expect(canLift(['moderation.ban', 'moderation.ban_permanent'], ban)).toBe(true);
    expect(canLift(['moderation.ban'], create(SanctionSchema, { ...ban, permanent: false }))).toBe(
      true,
    );
  });
});

describe('sanctionRequest', () => {
  it('makes a mute of a character', () => {
    const r = sanctionRequest(mute, ALL);
    expect(r.ok && r.value.kind).toBe('mute');
    expect(r.ok && r.value.characterId).toBe(150000n);
    expect(r.ok && r.value.accountId).toBe(0n);
    expect(r.ok && r.value.minutes).toBe(30);
    expect(r.ok && r.value.reason).toBe('spam');
  });

  it('needs the permission, the minutes in range and a reason', () => {
    expect(sanctionRequest(mute, ['moderation.jail']).ok).toBe(false);
    expect(sanctionRequest({ ...mute, minutes: '' }, ALL).ok).toBe(false);
    expect(sanctionRequest({ ...mute, minutes: '0' }, ALL).ok).toBe(false);
    expect(sanctionRequest({ ...mute, minutes: '1.5' }, ALL).ok).toBe(false);
    expect(sanctionRequest({ ...mute, minutes: String(MAX_MUTE_MINUTES) }, ALL).ok).toBe(true);
    expect(sanctionRequest({ ...mute, minutes: String(MAX_MUTE_MINUTES + 1) }, ALL).ok).toBe(false);
    expect(sanctionRequest({ ...mute, reason: '  ' }, ALL).ok).toBe(false);
    expect(sanctionRequest({ ...mute, reason: 'x'.repeat(256) }, ALL).ok).toBe(false);
    expect(sanctionRequest({ ...mute, characterId: 0n }, ALL).ok).toBe(false);
  });

  it('sends a kick with no minutes', () => {
    const r = sanctionRequest({ ...mute, kind: 'kick', minutes: 'ignored' }, ALL);
    expect(r.ok && r.value.minutes).toBe(0);
  });

  it('bans the account, for a time or forever', () => {
    const ban = { ...mute, kind: 'ban' as const, minutes: String(MAX_BAN_MINUTES) };
    const r = sanctionRequest(ban, ALL);
    expect(r.ok && r.value.accountId).toBe(2000001n);
    expect(r.ok && r.value.characterId).toBe(0n);
    expect(r.ok && r.value.minutes).toBe(MAX_BAN_MINUTES);
    expect(sanctionRequest({ ...ban, minutes: String(MAX_BAN_MINUTES + 1) }, ALL).ok).toBe(false);
    const forever = sanctionRequest({ ...ban, minutes: '', permanent: true }, ALL);
    expect(forever.ok && forever.value.minutes).toBe(0);
    const noPermanent = ALL.filter((p) => p !== 'moderation.ban_permanent');
    expect(sanctionRequest({ ...ban, permanent: true }, noPermanent).ok).toBe(false);
    expect(sanctionRequest({ ...ban, accountId: 0n }, ALL).ok).toBe(false);
  });
});

describe('sanction cells', () => {
  const at = timestampFromDate(new Date(Date.UTC(2026, 9, 20, 15, 4)));

  it('writes the end, permanent, or nothing for a kick', () => {
    expect(endText(create(SanctionSchema, { kind: 'ban', permanent: true }))).toBe('permanent');
    expect(endText(create(SanctionSchema, { kind: 'kick', endsAt: at }))).toBe('');
    expect(endText(create(SanctionSchema, { kind: 'mute', endsAt: at }))).not.toBe('');
  });

  it('writes the state', () => {
    expect(stateText(create(SanctionSchema, { kind: 'mute', active: true }))).toBe('in effect');
    expect(stateText(create(SanctionSchema, { kind: 'mute' }))).toBe('ended');
    expect(stateText(create(SanctionSchema, { kind: 'kick' }))).toBe('done');
    expect(
      stateText(
        create(SanctionSchema, {
          kind: 'jail',
          liftedAt: at,
          liftedBy: 'Gm',
          liftReason: 'mistake',
        }),
      ),
    ).toBe('lifted by Gm: mistake');
  });

  it('names the character, or the account of a ban', () => {
    expect(targetText(create(SanctionSchema, { kind: 'mute', character: 'Bob' }))).toBe('Bob');
    expect(
      targetText(create(SanctionSchema, { kind: 'ban', account: 'bob', accountId: 2000001n })),
    ).toBe('bob (account 2000001)');
    expect(targetText(create(SanctionSchema, { kind: 'ban', accountId: 7n }))).toBe('account 7');
  });

  it('says what was given', () => {
    expect(givenText('mute', 'Bob')).toBe('Bob is muted.');
    expect(givenText('kick', 'Bob')).toBe('Bob is kicked.');
    expect(givenText('ban', 'bob')).toBe('The account bob is banned.');
  });
});
