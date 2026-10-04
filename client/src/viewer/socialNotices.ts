// The text of party, friend and guild notices in the chat log: how a
// party, friend or guild action went, what happened to the party or the
// guild, friends who log in or out, and items that even item share gave.
// One plain sentence each.

import { FriendOutcome, GuildOutcome, PartyOutcome } from '../gen/wathena/v1/event_pb';

/**
 * The notice for a PartyResult. name is the player or party the outcome
 * is about. self is the player's own name, so "You left the party." can
 * read as such.
 */
export function partyMessage(outcome: PartyOutcome, name: string, self?: string): string {
  const you = name !== '' && name === self;
  switch (outcome) {
    case PartyOutcome.CREATED:
      return name === '' ? 'You made a party.' : `You made the party ${name}.`;
    case PartyOutcome.NAME_TAKEN:
      return name === ''
        ? 'A party of that name exists already.'
        : `A party named ${name} exists already.`;
    case PartyOutcome.ALREADY_IN_PARTY:
      return 'You are in a party already.';
    case PartyOutcome.BAD_NAME:
      return 'That party name is not allowed.';
    case PartyOutcome.NOT_LEADER:
      return 'Only the party leader can do that.';
    case PartyOutcome.NOT_ONLINE:
      return name === '' ? 'That player is not online.' : `${name} is not online.`;
    case PartyOutcome.TARGET_IN_PARTY:
      return name === '' ? 'That player is in a party already.' : `${name} is in a party already.`;
    case PartyOutcome.FULL:
      return 'The party is full.';
    case PartyOutcome.INVITE_SENT:
      return `You invited ${name} to the party.`;
    case PartyOutcome.REFUSED:
      return `${name} refused the party invite.`;
    case PartyOutcome.ACCEPTED:
      return `${name} accepted the party invite.`;
    case PartyOutcome.SAME_ACCOUNT:
      return 'A character of the same account is in the party already.';
    case PartyOutcome.JOINED:
      return you ? 'You joined the party.' : `${name} joined the party.`;
    case PartyOutcome.LEFT:
      return you ? 'You left the party.' : `${name} left the party.`;
    case PartyOutcome.EXPELLED:
      return you ? 'You were expelled from the party.' : `${name} was expelled from the party.`;
    case PartyOutcome.DISBANDED:
      return 'The party was disbanded.';
    case PartyOutcome.NOT_MEMBER:
      return `${name} is not in the party.`;
    case PartyOutcome.LEADER_CHANGED:
      return you ? 'You are the party leader now.' : `${name} is the party leader now.`;
    case PartyOutcome.LEADER_NOT_HERE:
      return 'The new leader must be on the same map as you.';
    case PartyOutcome.EXP_SHARE_REFUSED:
      return "Even exp share is not possible: the members' levels are too far apart.";
    case PartyOutcome.EXP_SHARE_OFF:
      return "Even exp share was turned off: the members' levels are too far apart.";
    case PartyOutcome.OPTIONS_CHANGED:
      return 'The party options changed.';
    case PartyOutcome.NO_PARTY:
      return 'You are not in a party.';
    default:
      return 'The party action did nothing.';
  }
}

/** The notice for a FriendResult. name is the other player. */
export function friendMessage(outcome: FriendOutcome, name: string): string {
  switch (outcome) {
    case FriendOutcome.ADDED:
      return `${name} is your friend now.`;
    case FriendOutcome.REFUSED:
      return `${name} does not want to be friends.`;
    case FriendOutcome.LIST_FULL:
      return 'Your friend list is full.';
    case FriendOutcome.THEIR_LIST_FULL:
      return `The friend list of ${name} is full.`;
    case FriendOutcome.NOT_ONLINE:
      return name === '' ? 'That player is not online.' : `${name} is not online.`;
    case FriendOutcome.ALREADY_FRIENDS:
      return `${name} is your friend already.`;
    case FriendOutcome.REQUEST_SENT:
      return `You asked ${name} to be friends.`;
    case FriendOutcome.REMOVED:
      return `${name} is no longer your friend.`;
    case FriendOutcome.NOT_FOUND:
      return `${name} is not on your friend list.`;
    default:
      return 'The friend action did nothing.';
  }
}

/** The notice for a friend who logged in or out. */
export function friendStatusMessage(name: string, online: boolean): string {
  return online ? `${name} is online.` : `${name} logged out.`;
}

/** The notice for an item that even item share gave a member, such as "Bob got Jellopy." */
export function itemSharedMessage(name: string, item: string, amount: number): string {
  return amount > 1 ? `${name} got ${item} (${String(amount)}).` : `${name} got ${item}.`;
}

/** The prompt text for a party invite. */
export function partyInviteText(from: string, party: string): string {
  return `${from} invites you to the party ${party}.`;
}

/** The prompt text for a friend request. */
export function friendRequestText(from: string): string {
  return `${from} wants to be friends.`;
}

/**
 * The notice for a GuildResult. name is the player, guild or skill the
 * outcome is about. self is the player's own name, so "You left the
 * guild." can read as such.
 */
export function guildMessage(outcome: GuildOutcome, name: string, self?: string): string {
  const you = name !== '' && name === self;
  const who = name === '' ? 'That player' : name;
  const guild = name === '' ? 'That guild' : `The guild ${name}`;
  switch (outcome) {
    case GuildOutcome.CREATED:
      return name === '' ? 'You made a guild.' : `You made the guild ${name}.`;
    case GuildOutcome.ALREADY_IN_GUILD:
      return 'You are in a guild already.';
    case GuildOutcome.NAME_TAKEN:
      return name === ''
        ? 'A guild of that name exists already.'
        : `A guild named ${name} exists already.`;
    case GuildOutcome.NO_EMPERIUM:
      return 'Making a guild takes an Emperium.';
    case GuildOutcome.BAD_NAME:
      return 'That guild name is not allowed.';
    case GuildOutcome.NO_GUILD:
      return 'You are not in a guild.';
    case GuildOutcome.NO_PERMISSION:
      return 'Your guild position does not allow that.';
    case GuildOutcome.NOT_MASTER:
      return 'Only the guild master can do that.';
    case GuildOutcome.NOT_ONLINE:
      return `${who} is not online.`;
    case GuildOutcome.TARGET_IN_GUILD:
      return `${who} is in a guild already or has an invite open.`;
    case GuildOutcome.FULL:
      return 'The guild is full.';
    case GuildOutcome.INVITE_SENT:
      return name === '' ? 'You sent the guild invite.' : `You invited ${name} to the guild.`;
    case GuildOutcome.REFUSED:
      return `${who} refused the guild invite.`;
    case GuildOutcome.ACCEPTED:
      return `${who} accepted the guild invite.`;
    case GuildOutcome.JOINED:
      return you ? 'You joined the guild.' : `${who} joined the guild.`;
    case GuildOutcome.LEFT:
      return you ? 'You left the guild.' : `${who} left the guild.`;
    case GuildOutcome.EXPELLED:
      return you ? 'You were expelled from the guild.' : `${who} was expelled from the guild.`;
    case GuildOutcome.DISBANDED:
      return 'The guild was disbanded.';
    case GuildOutcome.NOT_MEMBER:
      return `${who} is not in the guild.`;
    case GuildOutcome.MASTER_CANNOT_LEAVE:
      return 'The guild master cannot leave. Hand the guild over or disband it first.';
    case GuildOutcome.MASTER_CHANGED:
      return you ? 'You are the guild master now.' : `${who} is the guild master now.`;
    case GuildOutcome.MASTER_CHANGE_TOO_SOON:
      return 'The guild master changed too recently to change again.';
    case GuildOutcome.POSITIONS_CHANGED:
      return 'The guild positions changed.';
    case GuildOutcome.NOTICE_CHANGED:
      return 'The guild notice changed.';
    case GuildOutcome.SKILL_RAISED:
      return name === '' ? 'A guild skill was raised.' : `The guild raised ${name}.`;
    case GuildOutcome.SKILL_REFUSED:
      return 'That guild skill cannot be raised now.';
    case GuildOutcome.LEVEL_UP:
      return 'The guild reached a new level.';
    case GuildOutcome.ALLY_REQUEST_SENT:
      return name === ''
        ? 'You asked for an alliance.'
        : `You asked the guild of ${name} for an alliance.`;
    case GuildOutcome.ALLIED:
      return name === '' ? 'The guilds are allies now.' : `The guild ${name} is an ally now.`;
    case GuildOutcome.ALLY_REFUSED:
      return `${guild} refused the alliance.`;
    case GuildOutcome.ALLY_LIMIT:
      return 'Your guild has the most allies it can have.';
    case GuildOutcome.THEIR_ALLY_LIMIT:
      return `${guild} has the most allies it can have.`;
    case GuildOutcome.ALREADY_ALLIED:
      return `${guild} is an ally already.`;
    case GuildOutcome.TARGET_BUSY:
      return `${who} has another request open.`;
    case GuildOutcome.TARGET_NOT_MASTER:
      return `${who} is not a guild master.`;
    case GuildOutcome.ENEMY_ADDED:
      return name === '' ? 'The guild is an enemy now.' : `The guild ${name} is an enemy now.`;
    case GuildOutcome.ENEMY_LIMIT:
      return 'Your guild has the most enemies it can have.';
    case GuildOutcome.ALREADY_ENEMY:
      return `${guild} is an enemy already.`;
    case GuildOutcome.RELATION_ENDED:
      return name === ''
        ? 'The guild relation ended.'
        : `The relation with the guild ${name} ended.`;
    case GuildOutcome.SAME_GUILD:
      return `${who} is in your guild.`;
    case GuildOutcome.NOT_EMPTY:
      return 'The guild can only be disbanded when the master is its only member.';
    case GuildOutcome.EMBLEM_CHANGED:
      return 'The guild has a new emblem.';
    case GuildOutcome.POSITION_CHANGED:
      return you ? 'You have a new guild position.' : `${who} has a new guild position.`;
    default:
      return 'The guild action did nothing.';
  }
}

/** The prompt text for a guild invite. */
export function guildInviteText(from: string, guild: string): string {
  return `${from} invites you to the guild ${guild}.`;
}

/** The prompt text for an alliance request. */
export function guildAllyRequestText(from: string, guild: string): string {
  return `${from} of the guild ${guild} asks your guild to be allies.`;
}
