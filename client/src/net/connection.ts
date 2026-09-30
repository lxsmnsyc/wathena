// A session with the game server, as docs/proposals/0001-protocol.md
// describes: one WebSocket, binary frames of one message each, and Hello
// first.

import { create, fromBinary, toBinary } from '@bufbuild/protobuf';
import { CommandSchema } from '../gen/wathena/v1/command_pb';
import {
  type ClientMessage,
  ClientMessageSchema,
  ServerMessageSchema,
} from '../gen/wathena/v1/envelope_pb';
import type { Event } from '../gen/wathena/v1/event_pb';
import type { Welcome } from '../gen/wathena/v1/session_pb';

/** The protocol version, schema.Version on the server. */
export const PROTOCOL_VERSION = 1;

/** The largest message the server may send, schema.MaxMessageBytes. */
const MAX_MESSAGE_BYTES = 64 * 1024;

export interface Handlers {
  /** Events from one tick. ackSeq is the last command the server processed. */
  events: (events: Event[], ackSeq: number) => void;
  /** The session ended. reason is shown to the player. */
  closed: (reason: string) => void;
}

export class Connection {
  private seq = 0;

  private constructor(
    private readonly socket: WebSocket,
    readonly welcome: Welcome,
  ) {}

  /**
   * Connects and says hello. token is the session token, which during
   * development is the character name.
   */
  static async open(url: string, token: string, handlers: Handlers): Promise<Connection> {
    const socket = new WebSocket(url);
    socket.binaryType = 'arraybuffer';
    return new Promise((resolve, reject) => {
      let connection: Connection | undefined;
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
            handlers.closed(m.body.value.message);
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
        handlers.closed('Disconnected from the server.');
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
