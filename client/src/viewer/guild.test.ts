import { describe, expect, it } from 'vitest';
import { GuildOutcome } from '../gen/wathena/v1/event_pb';
import {
  type GuildInfoInput,
  type GuildMemberInput,
  MAX_EXP_TAX,
  MAX_NOTICE_TEXT,
  MAX_NOTICE_TITLE,
  MAX_REASON,
  NO_GUILD,
  badgeLine,
  canExpel,
  checkNotice,
  checkPositions,
  cleanReason,
  expPercent,
  guildAfterResult,
  guildFromInfo,
  guildMemberCount,
  guildReportTitle,
  guildSkillName,
  isMaster,
  mergeGuildMember,
  parseExpTax,
  permissions,
  positionName,
  positionSlots,
  relationsOf,
} from './guild';

function member(name: string, values: Partial<GuildMemberInput> = {}): GuildMemberInput {
  return {
    name,
    position: 1,
    online: true,
    map: 'map/prontera',
    job: 7,
    baseLevel: 50,
    exp: 100n,
    master: false,
    ...values,
  };
}

const position = (index: number, name: string, rights = false, expTax = 0) => ({
  index,
  name,
  canInvite: rights,
  canExpel: rights,
  canUseStorage: rights,
  expTax,
});

const info: GuildInfoInput = {
  id: 5n,
  name: 'Knights',
  level: 3,
  exp: 250n,
  nextExp: 1000n,
  skillPoints: 2,
  maxMembers: 22,
  master: 'Alice',
  notice: { title: 'Hello', text: 'War at 8' },
  emblemVersion: 2,
  members: [
    member('Alice', { position: 0, master: true }),
    member('Bob', { position: 1 }),
    member('Carol', { position: 2, online: false }),
  ],
  // Out of order on purpose: the window sorts positions by index.
  positions: [position(1, 'Officer', true, 10), position(0, 'Master', true), position(2, 'Member')],
  relations: [
    { guildId: 8n, name: 'Lions', enemy: false },
    { guildId: 9n, name: 'Wolves', enemy: true },
  ],
  skills: [
    { skillId: 10000, level: 1, maxLevel: 1, canRaise: false },
    { skillId: 10004, level: 1, maxLevel: 10, canRaise: true },
  ],
  position: 1,
  averageLevel: 50,
};

describe('guildFromInfo', () => {
  it('replaces the whole guild, with positions in index order', () => {
    const g = guildFromInfo(info);
    expect(g.id).toBe(5n);
    expect(g.name).toBe('Knights');
    expect(g.notice).toEqual({ title: 'Hello', text: 'War at 8' });
    expect(g.members.map((m) => m.name)).toEqual(['Alice', 'Bob', 'Carol']);
    expect(g.positions.map((p) => p.index)).toEqual([0, 1, 2]);
    expect(g.relations).toHaveLength(2);
    expect(g.skills[1]).toEqual({ skillId: 10004, level: 1, maxLevel: 10, canRaise: true });
  });

  it('gives no guild for id 0', () => {
    expect(guildFromInfo({ ...info, id: 0n })).toBe(NO_GUILD);
  });

  it('reads a missing notice as empty', () => {
    expect(guildFromInfo({ ...info, notice: undefined }).notice).toEqual({ title: '', text: '' });
  });
});

describe('mergeGuildMember', () => {
  it('changes one member and keeps its position and master mark', () => {
    const g = guildFromInfo(info);
    const next = mergeGuildMember(
      g,
      member('Alice', { position: 3, master: false, online: false, baseLevel: 51, exp: 300n }),
    );
    expect(next.members[0]).toMatchObject({
      name: 'Alice',
      position: 0,
      master: true,
      online: false,
      baseLevel: 51,
      exp: 300n,
    });
    expect(next.members[1]).toBe(g.members[1]);
    expect(g.members[0]?.online).toBe(true);
  });

  it('keeps values an update leaves empty', () => {
    const next = mergeGuildMember(
      guildFromInfo(info),
      member('Bob', { map: '', job: 0, baseLevel: 0, exp: 5n }),
    );
    expect(next.members[1]).toMatchObject({ map: 'map/prontera', job: 7, baseLevel: 50 });
  });

  it('gives the same state for an update that changes nothing shown', () => {
    const g = guildFromInfo(info);
    // Only the cell changes every 5 seconds, which the window does not show.
    expect(mergeGuildMember(g, member('Bob'))).toBe(g);
    expect(mergeGuildMember(g, member('Eve', { online: false }))).toBe(g);
    expect(mergeGuildMember(NO_GUILD, member('Bob'))).toBe(NO_GUILD);
  });
});

describe('guildAfterResult', () => {
  const g = guildFromInfo(info);

  it('resets when the player leaves, is expelled, or the guild is disbanded', () => {
    expect(guildAfterResult(g, GuildOutcome.LEFT, 'Bob', 'Bob')).toBe(NO_GUILD);
    expect(guildAfterResult(g, GuildOutcome.EXPELLED, 'Bob', 'Bob')).toBe(NO_GUILD);
    expect(guildAfterResult(g, GuildOutcome.DISBANDED, 'Knights', 'Bob')).toBe(NO_GUILD);
  });

  it('keeps the guild for other members and other outcomes', () => {
    expect(guildAfterResult(g, GuildOutcome.LEFT, 'Carol', 'Bob')).toBe(g);
    expect(guildAfterResult(g, GuildOutcome.EXPELLED, 'Bob', undefined)).toBe(g);
    expect(guildAfterResult(g, GuildOutcome.JOINED, 'Bob', 'Bob')).toBe(g);
  });
});

describe('rights', () => {
  const g = guildFromInfo(info);

  it('knows the master', () => {
    expect(isMaster(g, 'Alice')).toBe(true);
    expect(isMaster(g, 'Bob')).toBe(false);
    expect(isMaster(g, undefined)).toBe(false);
    expect(isMaster(NO_GUILD, '')).toBe(false);
  });

  it("follows the player's position, and the master may do everything", () => {
    expect(permissions(g, 'Alice')).toEqual({ invite: true, expel: true, storage: true });
    expect(permissions(g, 'Bob')).toEqual({ invite: true, expel: true, storage: true });
    const plain = { ...g, position: 2 };
    expect(permissions(plain, 'Carol')).toEqual({ invite: false, expel: false, storage: false });
    expect(permissions({ ...g, position: 7 }, 'Carol').invite).toBe(false);
    expect(permissions(NO_GUILD, 'Bob').storage).toBe(false);
  });

  it('never expels the master or the player itself', () => {
    const [alice, bob, carol] = g.members;
    expect(canExpel(g, 'Bob', alice!)).toBe(false);
    expect(canExpel(g, 'Bob', bob!)).toBe(false);
    expect(canExpel(g, 'Bob', carol!)).toBe(true);
    expect(canExpel({ ...g, position: 2 }, 'Carol', bob!)).toBe(false);
  });
});

describe('positions', () => {
  const g = guildFromInfo(info);

  it('names positions, with a default for unknown ones', () => {
    expect(positionName(g, 1)).toBe('Officer');
    expect(positionName(g, 6)).toBe('Position 7');
  });

  it('fills the slots up to the limit', () => {
    const slots = positionSlots(g, 5);
    expect(slots.map((p) => p.name)).toEqual([
      'Master',
      'Officer',
      'Member',
      'Position 4',
      'Position 5',
    ]);
    expect(positionSlots(g, 2)).toHaveLength(2);
    // The slots are copies.
    slots[0]!.name = 'Boss';
    expect(g.positions[0]?.name).toBe('Master');
  });

  it('reads exp taxes', () => {
    expect(parseExpTax('10')).toBe(10);
    expect(parseExpTax(' 0 ')).toBe(0);
    expect(parseExpTax(String(MAX_EXP_TAX))).toBe(MAX_EXP_TAX);
    expect(parseExpTax(String(MAX_EXP_TAX + 1))).toBeUndefined();
    expect(parseExpTax('-1')).toBeUndefined();
    expect(parseExpTax('1.5')).toBeUndefined();
    expect(parseExpTax('')).toBeUndefined();
  });

  it('checks names and taxes, and cleans names', () => {
    const ok = checkPositions([position(0, '  Boss '), position(1, 'Officer', true, 50)]);
    expect(ok).toEqual({ positions: [position(0, 'Boss'), position(1, 'Officer', true, 50)] });
    expect(checkPositions([position(3, ' ')])).toEqual({
      error: 'Position 4 needs a name of 1 to 24 characters.',
    });
    expect(checkPositions([position(1, 'Officer', false, -1)])).toEqual({
      error: `The exp tax of Officer must be 0 to ${String(MAX_EXP_TAX)}.`,
    });
  });
});

describe('notice and reasons', () => {
  it('checks the notice lengths', () => {
    expect(checkNotice(' Hi ', 'War at 8')).toEqual({ title: 'Hi', text: 'War at 8' });
    expect(checkNotice('', '')).toEqual({ title: '', text: '' });
    expect(checkNotice('a'.repeat(MAX_NOTICE_TITLE + 1), '')).toHaveProperty('error');
    expect(checkNotice('', 'a'.repeat(MAX_NOTICE_TEXT + 1))).toHaveProperty('error');
    expect(
      checkNotice('a'.repeat(MAX_NOTICE_TITLE), 'b'.repeat(MAX_NOTICE_TEXT)),
    ).not.toHaveProperty('error');
  });

  it('cuts reasons', () => {
    expect(cleanReason('  bye ')).toBe('bye');
    expect(cleanReason('')).toBe('');
    expect(cleanReason('포'.repeat(MAX_REASON + 5))).toBe('포'.repeat(MAX_REASON));
  });
});

describe('guild helpers', () => {
  const g = guildFromInfo(info);

  it('gives the exp percent', () => {
    expect(expPercent(250n, 1000n)).toBe(25);
    expect(expPercent(999n, 1000n)).toBe(99);
    expect(expPercent(0n, 1000n)).toBe(0);
    expect(expPercent(5n, 0n)).toBe(100);
    expect(expPercent(2000n, 1000n)).toBe(100);
  });

  it('counts members against the max', () => {
    expect(guildMemberCount(g, 16)).toBe('(3 / 22)');
    expect(guildMemberCount({ ...g, maxMembers: 0 }, 16)).toBe('(3 / 16)');
  });

  it('splits allies and enemies', () => {
    expect(relationsOf(g, false).map((r) => r.name)).toEqual(['Lions']);
    expect(relationsOf(g, true).map((r) => r.name)).toEqual(['Wolves']);
  });

  it('names guild skills', () => {
    expect(guildSkillName(10004)).toBe('Guild Extension');
    expect(guildSkillName(10050)).toBe('Skill 10050');
  });

  it('writes the guild line under a name', () => {
    expect(badgeLine({ name: 'Knights', position: 'Officer' })).toBe('Knights [Officer]');
    expect(badgeLine({ name: 'Knights', position: '' })).toBe('Knights');
  });
});

describe('guildReportTitle', () => {
  it('names the guild of a name report and an emblem report', () => {
    expect(guildReportTitle('Wolves', false)).toBe('Report the guild name Wolves');
    expect(guildReportTitle('Wolves', true)).toBe('Report the emblem of the guild Wolves');
  });
});
