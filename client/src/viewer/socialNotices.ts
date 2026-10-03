// The text of party and friend notices in the chat log: how a party or
// friend action went, what happened to the party, friends who log in or
// out, and items that even item share gave. One plain sentence each.

import { FriendOutcome, PartyOutcome } from '../gen/wathena/v1/event_pb';

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
