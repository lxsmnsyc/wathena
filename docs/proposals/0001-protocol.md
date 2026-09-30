# 0001: Web Client Protocol

- Status: accepted.
- Schema: [schema/proto/wathena/v1](../../schema/proto/wathena/v1).

## Problem

The web client and the server need one message format. It must be:

- Small, to fit the network budgets in strategy section 3.8.
- Generated for Go and TypeScript from one schema.
- Safe to decode from untrusted input, with fuzz tests.
- Able to change over time without breaking older clients by accident.

## Change

### Format

- Messages are Protocol Buffers (proto3), in the `wathena.v1` package.
- Go code is generated with `protoc-gen-go` and committed under `schema/gen/go`. It uses the opaque API, so fields are read and set through methods.
- TypeScript code is generated with `protobuf-es` and committed under `client/src/gen`.
- `buf lint` checks the schema. CI regenerates the Go code and fails if it differs from the committed code.

### Transport

- One WebSocket per session, over TLS, with binary frames.
- Each WebSocket message holds exactly one `ClientMessage` or `ServerMessage`.
- WebSocket compression (`permessage-deflate`) is off. Compressing chat that an attacker can send together with private data, such as whispers, lets a network observer guess that data from message sizes.

### Session start

1. The player logs in on the account service's page and receives a short-lived session token (strategy section 7.6).
2. The client opens the WebSocket and sends `Hello` as its first message. The token is in `Hello`, not in the URL, so it never appears in access logs.
3. The server checks the token and the protocol version, then sends `Welcome`, or `Disconnect` with a reason.
4. The server closes a connection that sends anything other than `Hello` first, or no `Hello` within 10 seconds.

### Versions

- `schema.Version` is the protocol version. It increases by 1 for each release that changes the protocol.
- The server accepts clients from `schema.MinVersion` to `schema.Version`. Older clients get `DISCONNECT_REASON_VERSION_UNSUPPORTED`.
- Within `wathena.v1`, changes are additive only: new fields, new messages, and new `oneof` cases. Field numbers are never reused.
- Clients skip `Event` cases they do not know. The server ignores `Command` cases it does not know and counts them toward the rate limit.
- A change that cannot be additive goes to a new package, `wathena.v2`, with its own proposal.
- `buf breaking` runs in CI from the first public release. Before then, `v1` can still change freely.

### Messages

Client to server, in `ClientMessage`:

| Case | Use |
|---|---|
| `hello` | Session start. |
| `ping` | Latency and keepalive. |
| `command` | A player action: walk, attack, skill, chat, NPC click or reply, use item. |

Server to client, in `ServerMessage`:

| Case | Use |
|---|---|
| `welcome` | Session accepted, with server time, tick, the player's own entity ID, and the map's asset name. |
| `pong` | Reply to `ping`, with server time. |
| `frame` | Everything visible that happened in one tick. |
| `disconnect` | Reason for closing, shown to the player. |

`ClientMessage.seq` increases by 1 per message. `Frame.ack_seq` returns the last one processed, so the client knows which commands the server has seen.

### Tick frames

- The server sends at most one `Frame` per player per tick (50 ms). Ticks with nothing visible send nothing.
- A `Frame` holds a list of `Event` values: entities appearing, vanishing, moving, and stopping, stat changes, damage, skill casts, chat, NPC dialog steps, and effects.
- Times inside a frame are milliseconds from the start of that tick, so events keep their exact timing within the 50 ms.

### Encoding rules

- **Entity IDs are per connection.** The server gives each visible entity a 32-bit `net_id` for that connection. Database IDs for accounts, characters, and items are never sent. This stops clients from tracking players across sessions by ID.
- **Movement sends a path, not positions.** `EntityMove` holds the start cell, the walk speed, and the steps as 4-bit directions, numbered as `EntityAppear.dir`: 0 south, turning clockwise. The client animates the walk. The server only sends a new message when a walk starts, changes, or stops. A unit in the middle of a step finishes it, so a changed walk starts with that step.
- **Other players' HP is a fraction.** `EntityAppear.hp_permille` is HP out of 1000, and `Damage.target_hp_permille` updates it after each hit. Exact HP goes only to the player, their party, and their homunculus.
- **Text is already translated.** NPC dialog and messages arrive in the player's language, chosen from `Hello.language` (strategy section 6.13).
- **Appearance uses table IDs.** `Look` holds sprite and palette IDs from the client data tables (strategy section 10.5).

### Limits

| Limit | Value |
|---|---|
| Largest server message | 64 KB (`schema.MaxMessageBytes`). Larger data, such as maps, uses HTTP. |
| Largest client message | 2 KB (`schema.MaxClientMessageBytes`). |
| Chat line | 255 characters. |
| Rates | Per connection and per command type, set in the gateway (strategy section 7.4). |

A message over its limit, or one that fails to decode, closes the connection with `DISCONNECT_REASON_BAD_MESSAGE`.

### Bandwidth check

`schema/protocol_test.go` builds sample events, measures their encoded size, and estimates bytes per second. It adds 26 bytes per message for the WebSocket and TLS headers.

| View | Assumptions | Estimate | Budget |
|---|---|---:|---:|
| Busy town | 100 players in view, each starting a walk every 3 s, 2 chat lines/s. | 1.9 KB/s | 8 KB/s |
| Town peak | 300 players in view, each starting a walk every 2 s, 10 chat lines/s. | 6.1 KB/s | 32 KB/s |
| Field | 15 monsters and a party of 3, 6 hits/s, 1 skill/s. | 0.7 KB/s | 2 KB/s |

Encoded sizes: a 10-step walk is 26 bytes, a player appearing is 68 bytes, and a hit is 21 bytes. The test fails if an estimate goes over its budget.

## Breaks

Nothing exists yet, so nothing breaks.

## Alternatives

| Option | Why not |
|---|---|
| FlatBuffers | Reads without copying, but messages are larger, and Go decoding of untrusted input needs a separate verify step. The copy it saves is small at these message sizes. |
| Cap'n Proto | TypeScript support is weak. |
| MessagePack or CBOR without a schema | No generated types, no checks for breaking changes, and field names cost bytes unless mapped by hand. |
| JSON | 3 to 5 times larger, and slower to parse in large frames. |
| A schema language of our own | Code generation, fuzzing, and breaking-change checks would all be ours to build and maintain. |
| gRPC-Web or Connect | Built for request and response. The game needs a server-driven stream every tick. Services still use gRPC between themselves (strategy section 4.1, item 6). |

## Open points for later proposals

- Messages for inventory, equipment, skills lists, party, guild, trade, vending, storage, and mail.
- Map changes and zone handoff during warps.
- Admin tool messages (strategy section 10.6), which may use a separate endpoint.
