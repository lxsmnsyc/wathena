# 0055: Shared View Events

- Status: proposed. Not built.
- Date: 2026-10-20.
- Code: [server/zone](../../server/zone) (`zone.go`, `outbox.go`, `entities.go`, `session.go`), [server/gateway](../../server/gateway) (`gateway.go`).
- Follows the "Large slowdowns" section of decision [0050](0050-load-tests.md), which says this case needs a decision. Strategy section 3.7 targets 500 players in view of each other.
- Lua API: none. Protocol: none. The bytes on the wire stay valid `ServerMessage` frames. Settings: none.

## Context

- The zone builds view events for each viewer separately. In `viewEvents` (`server/zone/zone.go`), every entity that walks, stops, appears or changes its look gets a new `Event` for every player that sees it.
- The events differ only in the entity's ID. IDs are per connection, so clients cannot track players across sessions ([proposal 0001](../proposals/0001-protocol.md), "Entity IDs are per connection"). That rule stays.
- So the zone thread's work and allocations grow with viewers × movers. With 1,000 players in view, one tick can build about a million `Event` and `EntityMove` objects.
- `queueFrames` calls `proto.Size` on each player's whole frame every tick. That walks every event a second time.
- The gateway then marshals each frame in the connection's writer goroutine (`write` in `server/gateway/gateway.go`). That is off the zone thread, but it is the same work again for each viewer.
- Measured in decision 0050:
  - At 1,000 players in one town, the send phase took 5.1 to 5.8 ms per tick and the zone thread's CPU p99 was 14.7 ms. The budget is 10 ms.
  - A CPU profile put 30% of the process's samples in `viewEvents`. 40% of that was move events and 22% was appear events.
  - The Crowd scenario breaks the budgets from 250 players in view, and drops connections at 1,000.
- rAthena builds an area packet once and copies the same bytes to each viewer (`clif_send` with `AREA` in `src/map/clif.cpp`). Its packets carry the same ID for every viewer, so it has no per-viewer step at all.

## Decision

Encode each entity's view event once per tick. For each viewer, write only the parts that differ: the entity ID and the protobuf headers around it.

### How the bytes are built

Protobuf lets a message's fields appear in any order, and lets a repeated field's entries be written one after another. So a frame can be written as bytes without building `Event` objects:

```
ServerMessage.frame (field 3, length)
  Frame.tick (field 1)
  Frame.ack_seq (field 2)
  Frame.events (field 3, length)          one per event
    Event.move (field 3, length)
      EntityMove.id (field 1)             per viewer
      EntityMove fields 2 to 6            shared, encoded once per tick
```

- For each entity and each event kind (move, stop, appear, look, guild, shop, option), the zone encodes the fields other than `id` once per tick. It keeps the bytes on the entity, like `e.packed` keeps the packed steps today.
- For each viewer, the zone writes the event's tag, a length, the `id` field and then copies the shared bytes. The length is the shared length plus the size of the `id` field.
- Events that are already per viewer, such as quest icons, chat, damage and stats, are marshaled one by one as today and appended as bytes.
- The frame size is the sum of the parts. That replaces `proto.Size` in `queueFrames`, and the split into several frames under `schema.MaxMessageBytes` works on the same sums.

### What changes in the code

1. Add a frame writer in `server/zone`. It takes the tick, the ack sequence and a list of event parts, and returns the frame's bytes. An event part is either shared bytes with an ID, or one marshaled `Event`.
2. Change `viewEvents` to add event parts, not `*wathenav1.Event` values. The checks for view range, hiding and `netIDs` stay as they are.
3. Let the outbox carry encoded frames. `outMsg` and the player's `out` channel carry either a `*wathenav1.ServerMessage` or ready bytes. The gateway's `write` sends ready bytes as they are and keeps the size check.
4. Keep `*wathenav1.ServerMessage` for everything that is not a frame: welcome, pong and disconnect.
5. Add a test helper that decodes a player's frames back into `*wathenav1.ServerMessage`. Tests that read `GetFrame()` today then keep working with one changed line.

### Order of work

Each step must keep `scripts/check.sh` passing, and be measured before the next one.

1. Add the frame writer and a test that checks it against `proto.Marshal`. Every event kind must give byte-for-byte equal output to marshaling the same `Event` with `proto.MarshalOptions{Deterministic: true}`, and `proto.Unmarshal` must read it back as an equal message. Fuzz the writer with random events.
2. Move events only. They were the largest share in the profile. Measure Town at 1,000 with `wabots`.
3. Stop, appear and the change events.
4. Remove `proto.Size` from `queueFrames`.
5. If the gateway's marshal shows up in profiles, it is gone for frames after step 3.

## Breaks

- Nothing for clients, mods or worlds. The client decodes the same messages. Field order inside a message is not part of the protocol, and the frame writer writes fields in field-number order anyway.
- Tests that read frames from a player's channel use the new decode helper.
- Code that adds a new event kind must add it to the frame writer, or send it as a per-viewer `Event`. The parity test from step 1 fails if the two encodings differ.

## Alternatives

- **One ID for every viewer.** This is what rAthena does, and it removes the per-viewer step. It breaks the privacy rule of proposal 0001, so it is not chosen.
- **Pool the `Event` and `EntityMove` objects.** This removes the allocations but keeps building, sizing and marshaling the same content once per viewer. It is smaller work and can be a first step if the frame writer is delayed.
- **Longer ticks for crowded zones.** Decision 0050 named this. It lowers the cost per second but makes movement less smooth for everyone in the zone. Keep it as a fallback if the frame writer is not enough for 500 in view.
- **Send events only to some viewers in a crowd.** This changes what players see, so it needs its own decision.

## How to check it worked

Run the `wabots` scenarios from decision 0050 on the same machine, before and after.

- Town at 1,000: the zone thread's CPU p99 under 10 ms. It was 14.7 ms.
- Crowd at 500: the tick budget met, with no dropped connections.
- A CPU profile of the same run: `viewEvents` well under its 30% share.
- Live heap per player no higher than the 70 to 120 KB measured in decision 0050.
- Record the results in this file and change the status to accepted.

## Related cost, not in this decision

- Monster AI checks every monster in an awake zone on each AI tick (`server/zone/mobai.go`). rAthena runs hard AI only for monsters near players (`map_foreachinallrange` in `src/map/mob.cpp`). This costs less than the view events but grows with spawn counts. Profile it after this decision is built.
