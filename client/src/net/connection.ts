// A session with the game server, as docs/proposals/0001-protocol.md
// describes: one WebSocket, binary frames of one message each, and Hello
// first.

import { type MessageInitShape, create, fromBinary, toBinary } from '@bufbuild/protobuf';
import {
  type BaseStat,
  type ChannelAction,
  ChatChannel,
  CommandSchema,
  FriendCommandSchema,
  GuildCommandSchema,
  type NpcReply,
  PartyBookingCommandSchema,
  PartyCommandSchema,
  StorageCommandSchema,
} from '../gen/wathena/v1/command_pb';
import {
  type ClientMessage,
  ClientMessageSchema,
  ServerMessageSchema,
} from '../gen/wathena/v1/envelope_pb';
import type { Event } from '../gen/wathena/v1/event_pb';
import type { DisconnectReason, Welcome } from '../gen/wathena/v1/session_pb';

/** A party's sharing options, as PartyOptions. */
export interface PartyOptionValues {
  expEven: boolean;
  itemEven: boolean;
  pickupShared: boolean;
}

/** A party booking ad, as PartyBookingAd. */
export interface BookingAdValues {
  level: number;
  /** A map's asset name, such as map/prt_fild08, or empty. */
  map: string;
  /** Up to 6 job IDs. */
  jobs: number[];
  note: string;
}

/** A party booking search, as PartyBookingSearch. 0 and empty mean any. */
export interface BookingSearchValues {
  level: number;
  map: string;
  job: number;
}

/** The world's limits that size the client's windows, as Welcome.limits. */
export interface ClientLimits {
  partyMembers: number;
  friends: number;
  blockList: number;
  /** The most members of a guild before Guild Extension. */
  guildMembers: number;
  guildPositions: number;
  guildAllies: number;
  guildEnemies: number;
  /** The most slots of a guild storage. */
  guildStorage: number;
}

/**
 * rAthena's defaults, for a server that sends no limits: MAX_PARTY,
 * MAX_FRIENDS, MAX_IGNORE_LIST, the 16 members of guild_calcinfo,
 * MAX_GUILDPOSITION, max_guild_alliance, and 500 guild storage slots.
 */
export const DEFAULT_LIMITS: ClientLimits = {
  partyMembers: 12,
  friends: 40,
  blockList: 20,
  guildMembers: 16,
  guildPositions: 20,
  guildAllies: 3,
  guildEnemies: 3,
  guildStorage: 500,
};

/** The limits Welcome gives, with rAthena's defaults for the ones it leaves at 0. */
export function limitsOf(welcome: Pick<Welcome, 'limits'>): ClientLimits {
  const l = welcome.limits;
  const or = (v: number | undefined, fallback: number): number =>
    v === undefined || v === 0 ? fallback : v;
  return {
    partyMembers: or(l?.partyMembers, DEFAULT_LIMITS.partyMembers),
    friends: or(l?.friends, DEFAULT_LIMITS.friends),
    blockList: or(l?.blockList, DEFAULT_LIMITS.blockList),
    guildMembers: or(l?.guildMembers, DEFAULT_LIMITS.guildMembers),
    guildPositions: or(l?.guildPositions, DEFAULT_LIMITS.guildPositions),
    guildAllies: or(l?.guildAllies, DEFAULT_LIMITS.guildAllies),
    guildEnemies: or(l?.guildEnemies, DEFAULT_LIMITS.guildEnemies),
    guildStorage: or(l?.guildStorage, DEFAULT_LIMITS.guildStorage),
  };
}

/** What a skill targets: an entity, a cell, or nothing for a self skill. */
export type UseSkillTarget =
  | { case: 'entity'; value: number }
  | { case: 'cell'; value: { x: number; y: number } }
  | { case: undefined; value?: undefined };

type PartyAction = NonNullable<MessageInitShape<typeof PartyCommandSchema>['action']>;
type FriendAction = NonNullable<MessageInitShape<typeof FriendCommandSchema>['action']>;
type BookingAction = NonNullable<MessageInitShape<typeof PartyBookingCommandSchema>['action']>;
type GuildAction = NonNullable<MessageInitShape<typeof GuildCommandSchema>['action']>;
type StorageAction = NonNullable<MessageInitShape<typeof StorageCommandSchema>['action']>;

/** A guild position, as GuildPosition. */
export interface GuildPositionValues {
  index: number;
  name: string;
  canInvite: boolean;
  canExpel: boolean;
  canUseStorage: boolean;
  expTax: number;
}

/** An answer to an NPC dialog step. */
export type NpcAnswer = NpcReply['answer'];

/** The protocol version, schema.Version on the server. */
export const PROTOCOL_VERSION = 1;

/** The largest message the server may send, schema.MaxMessageBytes. */
const MAX_MESSAGE_BYTES = 64 * 1024;

export interface Handlers {
  /** Events from one tick. ackSeq is the last command the server processed. */
  events: (events: Event[], ackSeq: number) => void;
  /**
   * The session ended. message is shown to the player. reason is the
   * server's reason, when it sent one.
   */
  closed: (message: string, reason?: DisconnectReason) => void;
}

export class Connection {
  private seq = 0;

  /** The world's limits from Welcome, with defaults for the ones it left out. */
  readonly limits: ClientLimits;

  private constructor(
    private readonly socket: WebSocket,
    readonly welcome: Welcome,
  ) {
    this.limits = limitsOf(welcome);
  }

  /**
   * Connects and says hello. token is the session token: a one-time game
   * ticket from the account service, or the character name on a --dev
   * server. It goes in Hello, never in the URL.
   */
  static async open(url: string, token: string, handlers: Handlers): Promise<Connection> {
    const socket = new WebSocket(url);
    socket.binaryType = 'arraybuffer';
    return new Promise((resolve, reject) => {
      let connection: Connection | undefined;
      // The server's reason, when it gave one, stays shown after the
      // socket closes.
      let reason: string | undefined;
      socket.addEventListener('open', () => {
        const hello = create(ClientMessageSchema, {
          seq: 1,
          body: {
            case: 'hello',
            value: {
              protocolVersion: PROTOCOL_VERSION,
              sessionToken: new TextEncoder().encode(token),
              clientName: 'wathena-web/viewer',
              language: navigator.language,
            },
          },
        });
        socket.send(toBinary(ClientMessageSchema, hello));
      });
      socket.addEventListener('message', (e: MessageEvent<unknown>) => {
        if (!(e.data instanceof ArrayBuffer) || e.data.byteLength > MAX_MESSAGE_BYTES) {
          socket.close();
          return;
        }
        const m = fromBinary(ServerMessageSchema, new Uint8Array(e.data));
        switch (m.body.case) {
          case 'welcome':
            connection = new Connection(socket, m.body.value);
            connection.seq = 1;
            resolve(connection);
            break;
          case 'frame':
            handlers.events(m.body.value.events, m.body.value.ackSeq);
            break;
          case 'disconnect':
            reason = m.body.value.message;
            handlers.closed(reason, m.body.value.reason);
            if (connection === undefined) {
              reject(new Error(m.body.value.message));
            }
            break;
          default:
            // Messages this client does not know are skipped.
            break;
        }
      });
      socket.addEventListener('close', () => {
        if (connection === undefined) {
          reject(new Error('the server closed the connection'));
        }
        if (reason === undefined) {
          handlers.closed('Disconnected from the server.');
        }
      });
    });
  }

  /** Asks to walk to a cell. */
  walkTo(x: number, y: number): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'walkTo', value: { x, y } } }),
    });
  }

  /** Attacks an entity, once or until it dies or leaves. */
  attack(target: number, repeat: boolean): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'attack', value: { target, repeat } } }),
    });
  }

  /** Walks next to a ground item, by entity ID, and picks it up. */
  pickUp(item: number): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'pickUp', value: { item } } }),
    });
  }

  /** Uses the item in an inventory slot. */
  useItem(slot: number): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'useItem', value: { slot } } }),
    });
  }

  /** Drops an amount from an inventory slot near the player. */
  dropItem(slot: number, amount: number): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'dropItem', value: { slot, amount } } }),
    });
  }

  /**
   * Puts on the item in an inventory slot. position holds rAthena's EQP_
   * bits the player asks for, and 0 asks for every position the item takes.
   */
  equipItem(slot: number, position = 0): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'equipItem', value: { slot, position } } }),
    });
  }

  /** Takes off the item in an inventory slot. */
  unequipItem(slot: number): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'unequipItem', value: { slot } } }),
    });
  }

  /** Puts one card from a slot into the first free card slot of another slot's item. */
  insertCard(cardSlot: number, equipSlot: number): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, {
        body: { case: 'insertCard', value: { cardSlot, equipSlot } },
      }),
    });
  }

  /** Spends status points to raise a stat by up to amount, 1 to 255. */
  raiseStat(stat: BaseStat, amount = 1): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'raiseStat', value: { stat, amount } } }),
    });
  }

  /**
   * Casts a skill at a level. target is an entity or a cell, or empty for
   * a self skill.
   */
  useSkill(skillId: number, level: number, target: UseSkillTarget): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, {
        body: { case: 'useSkill', value: { skillId, level, target } },
      }),
    });
  }

  /** Spends a skill point on a skill. */
  learnSkill(skillId: number): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'learnSkill', value: { skillId } } }),
    });
  }

  /** Says a line in local chat. The caller cleans it with cleanChat. */
  chat(text: string): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, {
        body: { case: 'chat', value: { channel: ChatChannel.LOCAL, text } },
      }),
    });
  }

  /**
   * Whispers a line to a character by name, or to a chat channel for a
   * name that starts with #. The server answers with WhisperResult. The
   * caller cleans the line with cleanChat.
   */
  whisper(to: string, text: string): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, {
        body: { case: 'chat', value: { channel: ChatChannel.WHISPER, text, to } },
      }),
    });
  }

  /**
   * Says a line in a chat channel the player is in, such as #trade. The
   * caller cleans the line with cleanChat.
   */
  channelChat(name: string, text: string): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, {
        body: { case: 'chat', value: { channel: ChatChannel.NAMED, text, channelName: name } },
      }),
    });
  }

  /**
   * Joins or leaves a chat channel, or asks for the channel list or the
   * members of one. LIST does not use name.
   */
  channel(action: ChannelAction, name = ''): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'channel', value: { action, name } } }),
    });
  }

  /** Adds a character name to the block list, or removes it. The server answers with BlockList. */
  block(name: string, unblock = false): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'block', value: { name, unblock } } }),
    });
  }

  /**
   * Reports a chat line by its line ID, or a player when lineId is
   * undefined. reason is the player's own words. The server answers with
   * ReportResult.
   */
  report(name: string, lineId: Uint8Array | undefined, reason: string): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, {
        body: {
          case: 'report',
          value: { name, lineId: lineId ?? new Uint8Array(), reason },
        },
      }),
    });
  }

  /**
   * Reports a guild's name, or its emblem when emblem is set. The server
   * answers with ReportResult.
   */
  reportGuild(guildId: bigint, emblem: boolean): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, {
        body: { case: 'report', value: { name: '', guildId, emblem, reason: '' } },
      }),
    });
  }

  /**
   * Says a line to the player's party. The caller cleans it with
   * cleanChat.
   */
  partyChat(text: string): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, {
        body: { case: 'chat', value: { channel: ChatChannel.PARTY, text } },
      }),
    });
  }

  /**
   * Makes a party with the player as its leader. Exp share starts as
   * each, as in rAthena. The server answers with PartyResult and PartyInfo.
   */
  partyCreate(name: string, itemEven: boolean, pickupShared: boolean): void {
    this.party({
      case: 'create',
      value: { name, options: { expEven: false, itemEven, pickupShared } },
    });
  }

  /** Invites a player by name to the leader's party. */
  partyInvite(name: string): void {
    this.party({ case: 'invite', value: name });
  }

  /** Answers PartyInvite. */
  partyReply(partyId: bigint, accept: boolean): void {
    this.party({ case: 'reply', value: { partyId, accept } });
  }

  /** Leaves the party. When the leader leaves, the party breaks up. */
  partyLeave(): void {
    this.party({ case: 'leave', value: true });
  }

  /** Removes a member from the leader's party. */
  partyExpel(name: string): void {
    this.party({ case: 'expel', value: name });
  }

  /** Makes a member the party's leader. */
  partySetLeader(name: string): void {
    this.party({ case: 'setLeader', value: name });
  }

  /** Changes the party's sharing options. */
  partyOptions(options: PartyOptionValues): void {
    this.party({ case: 'options', value: options });
  }

  /** Asks an online player by name to be friends. */
  friendAdd(name: string): void {
    this.friend({ case: 'add', value: name });
  }

  /** Answers FriendRequest. */
  friendReply(name: string, accept: boolean): void {
    this.friend({ case: 'reply', value: { name, accept } });
  }

  /** Takes a friend off the list, on both sides. */
  friendRemove(name: string): void {
    this.friend({ case: 'remove', value: name });
  }

  /** Places the player's party booking ad. The server answers with PartyBookingList. */
  bookingRegister(ad: BookingAdValues): void {
    this.booking({ case: 'register', value: ad });
  }

  /** Searches the party booking ads. The server answers with PartyBookingList. */
  bookingSearch(search: BookingSearchValues): void {
    this.booking({ case: 'search', value: search });
  }

  /** Removes the player's own party booking ad. */
  bookingDelete(): void {
    this.booking({ case: 'delete', value: true });
  }

  /**
   * Says a line to the player's guild. The caller cleans it with
   * cleanChat.
   */
  guildChat(text: string): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, {
        body: { case: 'chat', value: { channel: ChatChannel.GUILD, text } },
      }),
    });
  }

  /** Makes a guild with the player as its master. The server answers with GuildResult. */
  guildCreate(name: string): void {
    this.guild({ case: 'create', value: name });
  }

  /** Invites a player by name to the player's guild. */
  guildInvite(name: string): void {
    this.guild({ case: 'invite', value: name });
  }

  /** Answers GuildInvite. */
  guildReply(guildId: bigint, accept: boolean): void {
    this.guild({ case: 'reply', value: { guildId, accept } });
  }

  /** Leaves the guild, with a reason of at most 39 characters. */
  guildLeave(reason: string): void {
    this.guild({ case: 'leave', value: reason });
  }

  /** Removes a member from the guild, with a reason of at most 39 characters. */
  guildExpel(name: string, reason: string): void {
    this.guild({ case: 'expel', value: { name, reason } });
  }

  /** Changes the guild's positions. Only the master may. */
  guildPositions(positions: GuildPositionValues[]): void {
    this.guild({ case: 'positions', value: { positions } });
  }

  /** Gives a member a position. Position 0 hands the guild to the member. */
  guildMemberPosition(name: string, position: number): void {
    this.guild({ case: 'memberPosition', value: { name, position } });
  }

  /** Changes the guild notice. */
  guildNotice(title: string, text: string): void {
    this.guild({ case: 'notice', value: { title, text } });
  }

  /** Raises a guild skill by its ID. */
  guildSkillUp(skillId: number): void {
    this.guild({ case: 'skillUp', value: skillId });
  }

  /** Asks the guild of an online player by name to be allies. */
  guildAlly(name: string): void {
    this.guild({ case: 'ally', value: name });
  }

  /** Answers GuildAllyRequest. */
  guildAllyReply(guildId: bigint, accept: boolean): void {
    this.guild({ case: 'allyReply', value: { guildId, accept } });
  }

  /** Makes the guild of an online player by name an enemy. */
  guildEnemy(name: string): void {
    this.guild({ case: 'enemy', value: name });
  }

  /** Ends an alliance, or an enemy relation when enemy is set. */
  guildEndRelation(guildId: bigint, enemy: boolean): void {
    this.guild({ case: 'endRelation', value: { guildId, enemy } });
  }

  /** Breaks the guild up. The master names the guild to confirm. */
  guildDisband(name: string): void {
    this.guild({ case: 'disband', value: name });
  }

  /** Moves an amount from an inventory slot into the open storage. */
  storagePut(slot: number, amount: number): void {
    this.storage({ case: 'put', value: { slot, amount } });
  }

  /** Moves an amount from a storage slot into the inventory. */
  storageTake(slot: number, amount: number): void {
    this.storage({ case: 'take', value: { slot, amount } });
  }

  /** Closes the open storage window. */
  storageClose(): void {
    this.storage({ case: 'close', value: true });
  }

  /** Asks for the guild storage log. The server answers with GuildStorageLog. */
  storageLog(): void {
    this.storage({ case: 'log', value: true });
  }

  /** Talks to an NPC, or opens its shop. */
  npcClick(npc: number): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'npcClick', value: { npc } } }),
    });
  }

  /** Answers the open NPC dialog step. */
  npcReply(dialog: number, answer: NpcAnswer): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'npcReply', value: { dialog, answer } } }),
    });
  }

  /** Buys items from the open shop. */
  shopBuy(npc: number, items: { itemId: number; amount: number }[]): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'shopBuy', value: { npc, items } } }),
    });
  }

  /** Sells inventory slots to the open shop. */
  shopSell(npc: number, items: { slot: number; amount: number }[]): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'shopSell', value: { npc, items } } }),
    });
  }

  /** Closes the open shop window. */
  shopClose(npc: number): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, { body: { case: 'shopClose', value: { npc } } }),
    });
  }

  close(): void {
    this.socket.close();
  }

  private party(action: PartyAction): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, {
        body: { case: 'party', value: create(PartyCommandSchema, { action }) },
      }),
    });
  }

  private friend(action: FriendAction): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, {
        body: { case: 'friend', value: create(FriendCommandSchema, { action }) },
      }),
    });
  }

  private booking(action: BookingAction): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, {
        body: { case: 'partyBooking', value: create(PartyBookingCommandSchema, { action }) },
      }),
    });
  }

  private guild(action: GuildAction): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, {
        body: { case: 'guild', value: create(GuildCommandSchema, { action }) },
      }),
    });
  }

  private storage(action: StorageAction): void {
    this.send({
      case: 'command',
      value: create(CommandSchema, {
        body: { case: 'storage', value: create(StorageCommandSchema, { action }) },
      }),
    });
  }

  private send(body: ClientMessage['body']): void {
    this.seq++;
    this.socket.send(
      toBinary(ClientMessageSchema, create(ClientMessageSchema, { seq: this.seq, body })),
    );
  }
}

/** Unpacks EntityMove steps: one facing per 4 bits, low bits first, 0xF for none. */
export function unpackSteps(steps: Uint8Array): number[] {
  const out: number[] = [];
  for (const b of steps) {
    for (const f of [b & 0x0f, b >> 4]) {
      if (f < 8) {
        out.push(f);
      }
    }
  }
  return out;
}
