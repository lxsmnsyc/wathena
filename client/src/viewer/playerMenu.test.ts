import { describe, expect, it } from 'vitest';
import { type PlayerMenuInfo, playerMenuItems } from './playerMenu';

const base: PlayerMenuInfo = {
  name: 'Bob',
  badge: undefined,
  partyInvite: false,
  guildInvite: false,
  ownGuild: 0n,
};

const badge = { guildId: 9n, name: 'Wolves', emblemVersion: 2, position: 'Member' };

describe('playerMenuItems', () => {
  it('always offers a whisper', () => {
    expect(playerMenuItems(base)).toEqual(['whisper']);
  });

  it('offers invites the player may send', () => {
    expect(playerMenuItems({ ...base, partyInvite: true, guildInvite: true })).toEqual([
      'whisper',
      'partyInvite',
      'guildInvite',
    ]);
    // A player in a guild cannot join another.
    expect(playerMenuItems({ ...base, guildInvite: true, badge })).not.toContain('guildInvite');
  });

  it("reports another guild's name, and its emblem when it has one", () => {
    expect(playerMenuItems({ ...base, badge })).toEqual([
      'whisper',
      'reportGuildName',
      'reportEmblem',
    ]);
    expect(playerMenuItems({ ...base, badge: { ...badge, emblemVersion: 0 } })).toEqual([
      'whisper',
      'reportGuildName',
    ]);
    // Not the player's own guild.
    expect(playerMenuItems({ ...base, badge, ownGuild: 9n })).toEqual(['whisper']);
  });
});
