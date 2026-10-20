# 0058: Connection and Queue Limits

- Status: proposed. Not built.
- Date: 2026-10-20.
- Code: [server/gateway](../../server/gateway) (`gateway.go`, `connlimit.go`, `limits.go`), [server/zone](../../server/zone) (`zone.go`, `outbox.go`, `session.go`, `social.go`, `chat.go`), [server/chat](../../server/chat) (`hub.go`), [server/social](../../server/social) (`guild.go`, `guildview.go`), [server/account](../../server/account) (`service.go`), [internal/assets/serve](../../internal/assets/serve) (`serve.go`).
- Builds on decision [0050](0050-load-tests.md) and proposal [0001](../proposals/0001-protocol.md).
- Lua API: none. Protocol: none. Settings: new limits in `waserver.yml`.

## Context

The findings below come from reading the code on 2026-10-20. None was measured. They matter most with 500 to 1,000 players in one zone, or many players reconnecting at once.

Each connection runs 3 goroutines: the reader, the writer that sends the player's `out` channel, and the kick watcher. A slow client never blocks the zone, which is good.

These are the weak points, most severe first.

1. **A slow client is dropped after 64 messages.** `outBuffer` is 64 (`server/zone/zone.go`). The zone sends 20 frames a second, plus extra frames when a crowded frame is split, plus storage pages. A stall of about 3 seconds on mobile or Wi-Fi closes the channel, and the client sees "fell behind".
2. **One command queue of 1,024 per zone.** `commands` in `zone.go` is shared by every player in the zone, with a share of 64 each (`session.go`). With 1,000 players that is about 1 slot each. Each ping also queues an ack through the zone. A refused command counts as a strike (`gateway.go`), so on a busy tick normal players collect strikes and can be kicked for the rate limit.
3. **Channel chat is sent one entry per member under a world-wide lock.** `Hub.deliver` (`server/chat/hub.go`) loops over every member while holding `Hub.mu`. Each member gets a non-blocking send into their zone's `chatIn`, which holds 1,024. With 1,000 members of one channel in one zone, two lines fill it, and later lines are dropped without notice. The `map` channel is one channel for the whole world, filtered by map name for each member.
4. **Each message is a separate write.** `write` in `gateway.go` marshals, makes a context with a timeout, and writes. The websocket library flushes each write, which is one system call and one TLS record. At 1,000 connections that is about 20,000 timers and flushes a second.
5. **The pong is written from the reader goroutine.** It waits for the connection's write lock while the writer is busy. With a slow client, reading commands can wait up to the 10 second write timeout.
6. **Guild exp takes a world-wide lock on every exp gain.** `PayExp` (`server/social/guild.go`) holds the registry lock, builds a full guild view by copying all members and sorting skills, sends it to every member, and queues 2 database writes. This runs for every exp gain of every guild member with a tax.
7. **Logins have no limit on how many run at once.** Each hello loads the character, takes storage and mailbox, and then joins the zone. Joins run one at a time on the zone goroutine, between ticks. After a restart, 1,000 reconnects reach the database pool and the zone loop at once.
8. **Password hashing is limited to 4 at once.** `DefaultHashers` is 4 (`server/account/service.go`), at 64 MiB each. At about 53 ms per hash that is about 75 password logins a second. Waiters fail after 10 seconds. Reconnects with a ticket do not hash.
9. **Mod assets inside a zip are read whole on each request.** A file that cannot seek is read with `io.ReadAll`, up to 64 MiB, for every request, range requests included (`serve.go`).
10. **The asset manifest has no ETag.** Every client start downloads it in full, uncompressed.

## Decision

Fix them in this order. Each step must keep `scripts/check.sh` passing.

### 1. Limit the out queue by time and bytes

- Replace the count of 64 with two limits: the bytes waiting, and the age of the oldest message. Defaults: 4 MiB and 10 seconds. Both are settings.
- When frames wait, the writer sends them together in one write (step 4).
- Drop the connection only when a limit is passed.

### 2. Command queue per player

- Give each player a small queue of their own, such as 32 commands, and let the zone take commands from all players' queues in a fair order each tick.
- A full queue refuses the command. It counts as a strike only when the player passes the gateway's rate limit, not when the zone is busy.
- The gateway answers pings itself. The zone sends the last processed sequence in each frame already (`Frame.ack_seq`).

### 3. Chat delivery by zone

- `deliver` makes one entry per zone that has members, with the list of members in it. The zone sends the line to its own players.
- Copy the member list under the lock and send after it is released.
- Keep a map channel per map, so a line does not loop over the whole world.
- When a zone's `chatIn` is full, count the dropped lines in metrics and tell the sender.

### 4. Batched writes

- The writer takes everything waiting in the player's channel and writes it in one go.
- Use one deadline per batch, set on the connection, not one context per message.
- Pongs go through the writer's channel.

### 5. Guild exp in batches

- Add exp to a counter per guild, and apply it once a second.
- Send members a small exp event, not the whole guild view.
- Write to the database with the guild's other changes, at most once per save.

### 6. Login limits

- A limit on character loads in progress for the whole process. Default 64.
- A limit on joins per zone per tick. Default 20. The rest wait for the next tick.
- Hashers default to the CPU count, capped by memory (64 MiB each). Refuse a login at once when the queue is longer than the hashers can finish in 10 seconds, so the client can retry later.

### 7. Assets

- Read files from zip mods into memory once at load, or extract them to a cache folder on disk.
- Give the manifest an ETag from its hash, and store it compressed.

## Breaks

- Nothing for clients or mods. The client already handles extra frames in one tick.
- Players who were dropped after 3 seconds now stay for up to 10.
- Guild exp shows on the guild window once a second instead of at once.

## Alternatives

- **Raise `outBuffer` and the command queue size only.** It moves the limit but keeps the same failure for crowds.
- **Turn on websocket compression.** It costs CPU on every frame. Decision 0001 turned it off on purpose.
- **A separate chat server.** It is more moving parts than the zone fan-out needs now.

## How to check it worked

- The Crowd scenario of decision 0050 at 500 and 1,000: no "fell behind" drops and no rate limit kicks for bots that send at a normal rate.
- A test that throttles one bot's connection for 5 seconds and checks it stays connected.
- A chat test with 1,000 members of one channel in one zone and 10 lines a second: no lines dropped.
- A reconnect storm with `wabots`: 1,000 bots reconnect after a restart, and tick p99 stays in budget.
