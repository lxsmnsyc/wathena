// A session with the game server, as docs/proposals/0001-protocol.md
// describes: one WebSocket, binary frames of one message each, and Hello
// first.

import { create, fromBinary, toBinary } from '@bufbuild/protobuf';
import {
  type BaseStat,
  ChatChannel,
  CommandSchema,
  type NpcReply,
} from '../gen/wathena/v1/command_pb';
import {
  type ClientMessage,
  ClientMessageSchema,
  ServerMessageSchema,
} from '../gen/wathena/v1/envelope_pb';
import type { Event } from '../gen/wathena/v1/event_pb';
import type { DisconnectReason, Welcome } from '../gen/wathena/v1/session_pb';

/** What a skill targets: an entity, a cell, or nothing for a self skill. */
export type UseSkillTarget =
  | { case: 'entity'; value: number }
  | { case: 'cell'; value: { x: number; y: number } }
  | { case: undefined; value?: undefined };

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

  private constructor(
    private readonly socket: WebSocket,
    readonly welcome: Welcome,
  ) {}

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
