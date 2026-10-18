# 0050: Load Tests

- Status: proposed.
- Date: 2026-10-18.
- Code: [tools/botclient](../../tools/botclient) (`travel.go`, `travel_test.go`, `behaviour.go`, `scenario.go`, `scenarios.go`), [tools/wabots](../../tools/wabots) (`main.go`, `place.go`, `main_test.go`), [server/zone](../../server/zone) (`zone.go`, `entities.go`, `warp.go`, `mobai.go`, `memory_test.go`), [scripts/release.sh](../../scripts/release.sh), [scripts/sqlcheck.sh](../../scripts/sqlcheck.sh).
- Docs: strategy sections 3.8 ("Load test results", "Phase 8") and 3.9, [deploy/README.md](../../deploy/README.md) ("Measure your own hardware"), [server/README.md](../../server/README.md) ("Load tests and metrics").
- Phase 8 step 4 (strategy section 15, "Phase 8"), strategy sections 3.7 to 3.9, and review decision 50 of [proposal 0002](../proposals/0002-lua-api.md).
- Lua API: none. Protocol: none. Settings: none. New `wabots` flags: `--database`, `--place`, `--fields` and `--travel`, and `wabots version`.

## Context

Phase 2 ran 500 bots in five scenarios on the kRO maps, without a database except in one run (strategy section 3.8). Phase 8 ships procedural worlds by default, with PostgreSQL. Section 3.9 names the player count that the phase 8 load test of the all-in-one release build measures, and its minimum hardware line waited for it. The project has no rented machine, so the tests ran on the development machine, and the docs may claim no more than they measured.

Review decision 50 set the setup, the scenarios, the steps, the rule for the documented capacity, the soak, and how to treat slowdowns.

## Decision

### Setup

- The release binaries of `scripts/release.sh darwin/arm64`, version `45241f7-dirty`: the commit of decision 50 with this step's changes.
- `waserver all --dev` on a loopback address, with `--debug-addr` for the measurements, and PostgreSQL 18 in Docker through Colima.
- A procedural world from `wactl world new` with the default layout: 30 fields of 300 × 300 cells, 5 towns, 6 dungeons with 17 floors, 52 maps and 8,236 monsters.
- Classic mode: one Town run on `prontera`, in a classic world from `wactl world new --mode classic`. The full kRO asset build needs about 2.4 GB, and the disk was too full: two attempts were stopped at the 5 GB floor of free space. The run used a build of the placeholder pack and Prontera's `.rsw`, `.gnd` and `.gat` files from the kRO client, which is all the server and the bots read of the map.
- `wabots` from the same release, at `nice 15` with `GOMAXPROCS=4`.
- The machine: an Apple M2 with 8 GB, shared with desktop apps and Colima's Docker VM of 5 GB. Swap use was 6.3 to 9.3 GB, and the kernel decompressed a median of 14,700 pages a second through the runs.

### Scenarios

The five scenarios of phase 2 run in the procedural world:

- Town, Crowd and Spread run in the start town, `gen:town:t1`. Their bots use new character names each step, so they start at the start cell, as in phase 2.
- Town with PostgreSQL is Town with the characters of an earlier run, which start where they were saved. Every procedural run loads and saves its characters in PostgreSQL.
- Fight runs on the lowest field, `gen:field:4:0`, of levels 1 to 10. The bots are the model player of level 5.

Two procedural scenarios are new in `botclient.Scenarios`:

- `fields`: 500 bots fight on 8 generated fields, picked from the lowest band to the highest, as `fight` does. Each bot is the economy simulator's model player of its field's band (strategy section 8.13): its job, levels, stats, gear and 50 potions. Its save point is its field, so a bot that dies comes back to it. 0.5 chat lines a second together.
- `graph`: 500 bots walk the generated map graph. Each walks a route to a portal of its map, other than the one it came in at, and goes on through the next map. They start spread over the towns as the model player of the highest band, so the fields they cross do not kill them, and they do not fight. 2 chat lines a second together. They warm up for 60 seconds, as `spread` does.

How the bots get there:

- `Scenario.Place` names where `wabots` puts the bots before they join: `fields` or `towns`. `wabots` does it through the database (`--database`, or `WATHENA_DATABASE_URL` as `waserver` reads it). A bot whose character does not exist logs in once first, so the server makes it. The update sets the character's job, levels, stats, items, map, cell and save point, and raises `save_version`, so a save of an older session cannot undo it. The game role has the rights it needs.
- A bot's cell is one that a path reaches from the map's first warp, so no bot starts in a closed pocket.
- `--fields N` picks N fields, spread from the lowest band to the highest. `--place fields --fields 1` puts every bot on the lowest field: the Fight runs.
- `Behaviour.Travel` walks routes: a breadth-first route over the map's cells to a portal more than 12 cells away, chosen at random, in legs of up to 24 cells that `path.Search` reaches. A bot that does not move for three walks, or reaches the portal without a map change, picks another portal. On a map without cells or portals, it walks as without `Travel`.
- Bots read a generated map's cells from `/gen/maps/<id>.wmap` and its places from `/gen/maps/<id>.json`, as the web client does. The portals are the places at the ends of the world's links, from `world_links`. `--assets` still gives the classic maps' cells.
- `wabots` reads the world's ruleset from the `world` table and builds the model players from the reference data built into it, so it runs outside the repository.
- Its queries bind every value, but are written in `tools/wabots`, not generated by sqlc. `scripts/sqlcheck.sh` skips `tools/wabots`, as it skips `e2e` and `tools/testdb`: it is a test tool, not game code.

### Steps

- 250, 500, 750 and 1,000 bots in every scenario, and 1,250 and 1,500 in Fields and Graph, with 3 minutes of measurement each.
- Steps stopped at 1,500. At 1,250 and 1,500 the machine was the limit: swap use passed 8.4 GB, and single bot commands waited up to 5.7 seconds for their acknowledging frame, against 55 ms at p99.
- Some steps at 500 ran again, as the results at 500 were close to the budget.
- Before each measurement ended, a forced garbage collection gave the live heap with every bot online, and after each scenario's steps, the live heap of the idle world.

### The documented capacity

The rule of review decision 50: the highest count at which every scenario but Crowd meets section 3.8. The tick work p99 is under 10 ms when fewer than 1% of a zone's ticks are over 10 ms. The histogram's p99 can read 10.5 ms then, since it is rounded up to its bucket.

- At 250, every run of every scenario but Crowd met the p99 budget. Town was the closest, with 0.97% of its ticks over 10 ms.
- At 500, every scenario met it in at least one run, but three runs did not: a Town run (1.17%), a Fight run (1.08%) and the classic Town run (1.06%). The other Town run and two other Fight runs at 500 met it.
- The documented capacity is 250 players on this machine. The docs claim no more than what every run met. 500 is a review item.
- The zone's own work fits at 500. A diagnostic build, not kept, read each zone thread's CPU time per tick, as in phase 2. At 500, the thread CPU time p99 was 4.7 ms in Town and 5.8 ms in Fight, with maxima of 7.5 and 13.4 ms, while the wall time of a tick reached 174 ms. The long ticks were waits for memory: outside Crowd the server process used at most 0.44 cores.
- At 1,000 in one town, the work itself is over the budget: a thread CPU time p99 of 14.7 ms. See "Large slowdowns".

The maximum of 25 ms is broken by rare ticks in most runs, as in phase 2. The decision allows rare spikes when reported. The tables in strategy section 3.8 list them.

### Soak

Fight at 250 bots for 30 minutes, in a server started for it. A sampler read the server's measurements every minute and started them again, and read the live heap after a forced garbage collection every 5 minutes. The table is in strategy section 3.8.

- Memory stayed flat: a live heap of 393 to 395 MB at every 5-minute mark, against 368 MB idle, Go memory at 868 MB from minute 12 on, and 894 goroutines.
- Tick times had no trend. 0.89% of all ticks were over 10 ms, so the run's p99 was under 10 ms. 12 of the 29 one-minute windows had 1% or more, in bursts that followed the swap in use.
- No bot lost its connection. Bots that walked into the field's warps played on the next maps, so the field held 242 players at the start and 200 at the end.

### Fix: players that left stayed in memory

The first pass of the steps, with the release build before this fix, showed the idle world's live heap growing from 365 MB to 635 MB after about 10,000 logins, with no player online. A heap profile held 101 MB of `netIDs` entries and 77 MB of inventories of players that had left. The retained players:

- A zone builds its block index of 8 × 8 cells only while it has players. When the last player leaves, the zone falls asleep with the index of its last tick, which holds every entity of that tick, players that left included.
- Each player keeps `netIDs`: its connection's IDs for what it sees, with the entities. When the player leaves, nothing clears them. So a player that the index kept, kept the players it saw, and they the players they saw. Most players that ever left a zone stayed in memory.
- The combat state's `hits` and `removed` were cut to length 0 at the end of each tick, which keeps the entities in the arrays until later appends overwrite them.
- A monster's `mob_attacked` timers, taken from the front of their slice, stayed in the array.

The fix, in `server/zone`:

- The block index clears its blocks when the zone falls asleep (`blockIndex.clear`).
- A player that leaves the zone, by logout or by a map change, resets its `netIDs`. The new zone of a map change makes a new player.
- `clear` empties `hits` and `removed` before they are cut.
- `mobHits` clears the timer it takes.

With the fix, the idle world's live heap went from 368 to 377 MB through the Town with PostgreSQL steps, about 2,500 logins. After five scenarios' steps, about 12,500 logins, it was 466 MB. The rest of that growth is the zones that woke up (Graph wakes 37 of them) and their state, which does not grow with logins. The live heap per online player, 70 to 120 KB, is the measurement of strategy section 3.8.

`TestLeftPlayersAreFreed` runs a zone with `Run`, lets two players see each other, makes both leave, and checks with weak pointers that the garbage collector frees both. `TestFighterThatLeftIsFreed` checks the same for a player that attacked a monster and left while another player keeps the zone awake. Both fail without the fix.

Two other parts of the fix were tried and dropped: clearing a monster's last skill target in `forgetMonster` broke `TestMobSkillUseParity`, since a monster that teleports forgets itself, and dropping a unit's pending hits in `forgetMonster` would change what happens after a teleport within a map.

### Large slowdowns

One, reported with a profile and not fixed:

- 1,000 players in one town near one cell. The send phase takes 5.1 to 5.8 ms per tick on average, and the zone thread's CPU time p99 is 14.7 ms. Each walk is a new `EntityMove` and `Event` for each viewer, since IDs are per connection, and appears grow with the players in view. A CPU profile of the first pass, taken during the join of 1,000 bots, put 30% of the process's samples in `viewEvents`: 40% of that in building move events, and 22% in appear events. A shared event per entity with a per-viewer ID, or larger ticks for crowds, would be the next step. It needs a decision: it is the Crowd case, and section 3.7's target of 500 players in view of each other.

No small slowdown in the zone's work was found. The rest of the long ticks were the machine.

### Release and docs

- `scripts/release.sh` builds `wabots-<os>-<arch>` without cgo, with the `procedural` tag and the version stamp, and prints its version with the others.
- `wabots version` prints the version.
- deploy/README.md, "Measure your own hardware": how an owner runs the scenarios against a test world, never a live one.
- Strategy section 3.8 has the new tables, with phase 2's kept. Section 3.9's capacity and minimum hardware lines name the measured count and the machine.

## Tests and checks

- `tools/botclient`: `TestRoute` (a route around a wall through a gap, without cutting corners, and none through a closed wall or to a wall cell) and `TestTravelPick` (a bot walks leg by leg to the far portal, each leg within `path.MaxWalkPath`, and without a grid it walks as without `Travel`).
- `tools/wabots`: `TestSpreadBands` and `TestHTTPBase`.
- `server/zone`: `TestLeftPlayersAreFreed` and `TestFighterThatLeftIsFreed`.
- The phase 8 step 1 to 3 checks: `scripts/check.sh quick`, `test security fuzz mods lua gen tinygo`, `e2e`, and the Compose smoke test without its HTTPS checks, which Colima's port forwarding cannot pass.

## Left

- The documented capacity is that of a shared 8 GB machine under memory pressure. A dedicated machine was not available (strategy section 15, "Phase 8").
- The classic run used Prontera's map files only, not a full kRO asset build.
- Crowd breaks the budgets from 250 players in view, and drops connections at 1,000.
- CI's nightly load run of section 12 does not exist (decision 0026).

## Review items

- Whether to document 500 players: the zone's work fits, and 3 of 10 runs at 500 had 1.06% to 1.17% of their ticks over 10 ms on this machine.
- The large slowdown of 1,000 players in one town, and section 3.7's 500 players in view.
- Whether the placement of bots through the database should also work through the account pages, for owners who do not want `wabots` to hold a database URL.
