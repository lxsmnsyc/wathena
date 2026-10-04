import { describe, expect, it } from 'vitest';
import { FriendOutcome, GuildOutcome, PartyOutcome } from '../gen/wathena/v1/event_pb';
import {
  friendMessage,
  friendRequestText,
  friendStatusMessage,
  guildAllyRequestText,
  guildInviteText,
  guildMessage,
  itemSharedMessage,
  partyInviteText,
  partyMessage,
} from './socialNotices';

describe('partyMessage', () => {
  it('says what happened to the party', () => {
    expect(partyMessage(PartyOutcome.JOINED, 'Bob')).toBe('Bob joined the party.');
    expect(partyMessage(PartyOutcome.JOINED, 'Bob', 'Bob')).toBe('You joined the party.');
    expect(partyMessage(PartyOutcome.LEFT, 'Bob')).toBe('Bob left the party.');
    expect(partyMessage(PartyOutcome.LEFT, 'Bob', 'Bob')).toBe('You left the party.');
    expect(partyMessage(PartyOutcome.EXPELLED, 'Bob')).toBe('Bob was expelled from the party.');
    expect(partyMessage(PartyOutcome.DISBANDED, 'Heroes')).toBe('The party was disbanded.');
    expect(partyMessage(PartyOutcome.LEADER_CHANGED, 'Bob')).toBe('Bob is the party leader now.');
    expect(partyMessage(PartyOutcome.CREATED, 'Heroes')).toBe('You made the party Heroes.');
  });

  it('says why an action failed', () => {
    expect(partyMessage(PartyOutcome.NOT_LEADER, '')).toBe('Only the party leader can do that.');
    expect(partyMessage(PartyOutcome.NOT_ONLINE, 'Eve')).toBe('Eve is not online.');
    expect(partyMessage(PartyOutcome.FULL, '')).toBe('The party is full.');
    expect(partyMessage(PartyOutcome.NAME_TAKEN, 'Heroes')).toBe(
      'A party named Heroes exists already.',
    );
    expect(partyMessage(PartyOutcome.EXP_SHARE_OFF, '')).toBe(
      "Even exp share was turned off: the members' levels are too far apart.",
    );
    expect(partyMessage(PartyOutcome.NO_PARTY, '')).toBe('You are not in a party.');
    expect(partyMessage(PartyOutcome.UNSPECIFIED, '')).toBe('The party action did nothing.');
  });

  it('has one sentence for every outcome', () => {
    for (const value of Object.values(PartyOutcome)) {
      if (typeof value === 'number') {
        expect(partyMessage(value, 'Bob')).toMatch(/^[A-Z].*\.$/);
      }
    }
  });
});

describe('friendMessage', () => {
  it('says how a friend action went', () => {
    expect(friendMessage(FriendOutcome.ADDED, 'Bob')).toBe('Bob is your friend now.');
    expect(friendMessage(FriendOutcome.REFUSED, 'Bob')).toBe('Bob does not want to be friends.');
    expect(friendMessage(FriendOutcome.LIST_FULL, 'Bob')).toBe('Your friend list is full.');
    expect(friendMessage(FriendOutcome.REQUEST_SENT, 'Bob')).toBe('You asked Bob to be friends.');
    expect(friendMessage(FriendOutcome.REMOVED, 'Bob')).toBe('Bob is no longer your friend.');
    expect(friendMessage(FriendOutcome.NOT_FOUND, 'Bob')).toBe('Bob is not on your friend list.');
  });

  it('has one sentence for every outcome', () => {
    for (const value of Object.values(FriendOutcome)) {
      if (typeof value === 'number') {
        expect(friendMessage(value, 'Bob')).toMatch(/^[A-Z].*\.$/);
      }
    }
  });
});

describe('other notices', () => {
  it('formats friend status, shared items and prompts', () => {
    expect(friendStatusMessage('Bob', true)).toBe('Bob is online.');
    expect(friendStatusMessage('Bob', false)).toBe('Bob logged out.');
    expect(itemSharedMessage('Bob', 'Jellopy', 1)).toBe('Bob got Jellopy.');
    expect(itemSharedMessage('Bob', 'Jellopy', 3)).toBe('Bob got Jellopy (3).');
    expect(partyInviteText('Alice', 'Heroes')).toBe('Alice invites you to the party Heroes.');
    expect(friendRequestText('Alice')).toBe('Alice wants to be friends.');
  });
});

describe('guildMessage', () => {
  it('says what happened to the guild', () => {
    expect(guildMessage(GuildOutcome.CREATED, 'Knights')).toBe('You made the guild Knights.');
    expect(guildMessage(GuildOutcome.JOINED, 'Bob')).toBe('Bob joined the guild.');
    expect(guildMessage(GuildOutcome.JOINED, 'Bob', 'Bob')).toBe('You joined the guild.');
    expect(guildMessage(GuildOutcome.LEFT, 'Bob', 'Bob')).toBe('You left the guild.');
    expect(guildMessage(GuildOutcome.EXPELLED, 'Bob')).toBe('Bob was expelled from the guild.');
    expect(guildMessage(GuildOutcome.DISBANDED, 'Knights')).toBe('The guild was disbanded.');
    expect(guildMessage(GuildOutcome.MASTER_CHANGED, 'Bob', 'Bob')).toBe(
      'You are the guild master now.',
    );
    expect(guildMessage(GuildOutcome.SKILL_RAISED, 'Guild Extension')).toBe(
      'The guild raised Guild Extension.',
    );
    expect(guildMessage(GuildOutcome.ALLIED, 'Lions')).toBe('The guild Lions is an ally now.');
    expect(guildMessage(GuildOutcome.RELATION_ENDED, 'Lions')).toBe(
      'The relation with the guild Lions ended.',
    );
  });

  it('says why an action failed', () => {
    expect(guildMessage(GuildOutcome.NO_EMPERIUM, '')).toBe('Making a guild takes an Emperium.');
    expect(guildMessage(GuildOutcome.NOT_MASTER, '')).toBe('Only the guild master can do that.');
    expect(guildMessage(GuildOutcome.NOT_ONLINE, '')).toBe('That player is not online.');
    expect(guildMessage(GuildOutcome.NOT_ONLINE, 'Eve')).toBe('Eve is not online.');
    expect(guildMessage(GuildOutcome.ALLY_REFUSED, '')).toBe('That guild refused the alliance.');
    expect(guildMessage(GuildOutcome.NOT_EMPTY, '')).toBe(
      'The guild can only be disbanded when the master is its only member.',
    );
    expect(guildMessage(GuildOutcome.UNSPECIFIED, '')).toBe('The guild action did nothing.');
  });

  it('has one sentence for every outcome', () => {
    for (const value of Object.values(GuildOutcome)) {
      if (typeof value === 'number') {
        expect(guildMessage(value, 'Bob')).toMatch(/^[A-Z].*\.$/);
        expect(guildMessage(value, '')).toMatch(/^[A-Z].*\.$/);
      }
    }
  });

  it('formats the prompts', () => {
    expect(guildInviteText('Alice', 'Knights')).toBe('Alice invites you to the guild Knights.');
    expect(guildAllyRequestText('Alice', 'Knights')).toBe(
      'Alice of the guild Knights asks your guild to be allies.',
    );
  });
});
