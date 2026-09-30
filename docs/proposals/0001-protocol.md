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
| `command` | A player action: walk, attack, skill, chat, NPC click or reply, use item, drop item, pick up, raise a stat. |

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
- **Ground items are entities.** They appear with `ENTITY_KIND_ITEM` and a `GroundItem` holding the item ID, amount, and place inside the cell. `PickUp` names the item's entity ID. Everyone in view gets `ItemTaken` and then a vanish with `VANISH_REASON_PICKED_UP`. The picker also gets `InventoryAdd`, or `PickUpFailed` when nothing happened. The reasons follow rAthena's `pc_takeitem` and `pc_additem`: the item is gone, another player has the right to it for now, it would pass the maximum weight (`OVERWEIGHT`), the inventory has no free slot (`INVENTORY_FULL`), or the stack would pass 30000 or the item's stack limit (`TOO_MANY`). An item that cannot be taken stays on the ground.
- **The inventory is slots.** The first frame carries `InventoryUpdate`: every slot that holds an item, with its index from 0, the item ID and the amount, and `size`, the slots that take new items (100, rAthena's `inventory_slots`). Slots are rAthena's inventory indexes, so a stack keeps its slot. `InventoryAdd` names the slot an amount entered, and `InventoryRemove` the slot an amount left, with the reason (used or dropped). A slot left with nothing is empty. Slots go up to 199, rAthena's `MAX_INVENTORY`.
- **Using and dropping items name a slot.** `UseItem` uses one item from a slot, as rAthena's `pc_useitem`. Everyone in view, the user included, gets `ItemUsed` with the user and the item ID, as `clif_useitemack` goes to the area. The user also gets `InventoryRemove`, the new HP and SP, and, for HP, a `Damage` with `DAMAGE_KIND_HEAL` whose source and target are the user and whose amount is the HP healed. Only the user gets the heal number. Only items whose script is a plain heal run for now; others fail with `USE_ITEM_FAILURE_NOT_SUPPORTED` and are not spent. `DropItem` drops an amount from a slot on a free cell 1 or 2 cells from the player, as rAthena's `pc_dropitem`. The item appears as a ground item that anyone may take at once. Failures come as `UseItemFailed` or `DropItemFailed` with a reason. The gateway drops a `UseItem` or `DropItem` whose slot is 200 or more, or whose amount is over 30000, without a reply.
- **Own stats are single numbers.** `StatUpdate` sends each changed stat once. HP and MAX_HP go together after damage, healing, or a new maximum. The others follow rAthena's status window (`clif_updatestatus`):
  - STR to LUK are totals, with job bonuses.
  - ATK, DEF and MDEF are the sum of the window's two numbers (`pc_leftside_*` plus `pc_rightside_*`). In renewal, ATK is status ATK plus weapon ATK.
  - MATK is the sum of both numbers in renewal. In pre-renewal the window shows a range, and MATK is its maximum.
  - HIT and FLEE are hit and flee. CRITICAL is critical rate in whole points (rAthena's value divided by 10).
  - ASPD is the displayed ASPD, `(2000 - amotion) / 10` rounded down: amotion 440 is ASPD 156.
  - WEIGHT and MAX_WEIGHT are in tenths of a weight unit, as rAthena's `weight` and `max_weight`. WEIGHT is sent on join and after every inventory change.
  - STR_COST to LUK_COST are the status points that raising each stat by 1 costs, as rAthena's `SP_USTR` to `SP_ULUK`. They are 0 at the stat's maximum.
- **Raising a stat spends status points.** `RaiseStat` names one of STR to LUK and an amount from 1 to 255. The server raises the stat as far as the points and the job's maximum allow, as rAthena's `pc_statusup`, and sends the changed stats, costs and STATUS_POINTS. A raise that fails changes nothing and sends nothing. The gateway drops a `RaiseStat` with another stat or amount.
- **Local chat reaches 9 cells.** `Chat` with `CHAT_CHANNEL_LOCAL` goes to every player within 9 cells of the speaker on both axes, as rAthena's `AREA_CHAT_WOC` does, the speaker included, as a `ChatMessage` with `from` set to that connection's ID for the speaker and `from_name` set to the speaker's name. Dead players can talk, as in rAthena. The other channels are accepted and ignored until they exist. The gateway checks the text before the zone sees it:
  - Text that is not valid UTF-8 is dropped.
  - Control characters (Unicode category Cc, such as tab and newline) become spaces.
  - Bidirectional formatting characters are removed: U+061C, U+200E, U+200F, U+202A to U+202E, and U+2066 to U+2069. They could reorder the line or the name next to it.
  - Spaces at both ends are trimmed. A line that is then empty, or longer than 255 characters, is dropped. A character is a Unicode code point.
  - Clients show chat with `textContent`, never as HTML (strategy section 7.5).
- **Appearance uses table IDs.** `Look` holds sprite and palette IDs from the client data tables (strategy section 10.5).

### Limits

| Limit | Value |
|---|---|
| Largest server message | 64 KB (`schema.MaxMessageBytes`). Larger data, such as maps, uses HTTP. |
| Largest client message | 2 KB (`schema.MaxClientMessageBytes`). |
| Chat line | 255 characters, as Unicode code points, after cleaning. |
| Chat rate | Per connection: a burst of 5 lines, then 1 per second. Lines over the rate are dropped without a reply, as rAthena's `min_chat_delay`. Over 60 dropped lines, less 1 per second, close the connection with `DISCONNECT_REASON_RATE_LIMITED`. |
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

Nothing is deployed yet, so nothing breaks. The inventory changed messages that no deployed client used: `InventoryAdd` gained `slot`, and `UseItem.slot` now refers to `InventoryUpdate`, which exists.

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

- Messages for equipment, item scripts other than heals, skills lists, party, guild, trade, vending, storage, and mail.
- Map changes and zone handoff during warps.
- Admin tool messages (strategy section 10.6), which may use a separate endpoint.
