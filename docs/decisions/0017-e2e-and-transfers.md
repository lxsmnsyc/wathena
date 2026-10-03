# 0017: End-to-End Tests and Item Transfers

- Status: proposed.
- Date: 2026-10-03.
- Code: [e2e](../../e2e), [tools/botclient](../../tools/botclient) (`game.go`), [server/transfer](../../server/transfer), [server/zone](../../server/zone) (`transfers.go`), [server/store](../../server/store) (`commit.go`, migration 00015), [server/cmd/waserver](../../server/cmd/waserver) (`audit.go`, `--seed`), [scripts/check.sh](../../scripts/check.sh).
- Phase 5, step 1. Builds on strategy sections 3.5, 5.4, 7.10 and 12.1, and [decision 0014](0014-rolled-items.md). It adds no Lua API.

## Decision

### End-to-end tests

- The package `e2e` runs a real `waserver` in its own process for each test, as the load smoke test does, and plays it with `botclient` bots.
- The server runs with `--dev`, the placeholder map and a copy of it as a second map, a fixture mod in `e2e/testdata/mods`, and PostgreSQL when the test asks for it and `WATHENA_TEST_DATABASE_URL` is set.
- A new flag, `--seed`, fixes the random streams of the zones and world events. It needs `--dev`, since a known seed lets players predict drops.
- `Player` methods send a command and wait for the events that show the result, so each check is on what the server sent. Waits time out after 15 seconds and fail the test.
- `botclient` gained what the tests need: the inventory, stats, skill list, dialog and shop window from the events, a record of every event with `Wait`, and the NPC, shop, item and skill commands.
- With `-race`, the servers of the duplication tests are built with `-race`. The scenario servers are not, since the race detector makes a server start about ten times slower. Every server package already runs its own tests with `-race`.

### Transfers

- One package, `transfer`, moves items and zeny between owners. An owner is a holder (inventory, storage, cart, guild storage, mail, vending) or an end (ground, monster, shop, script, skill, used). Storage and the other holders exist as kinds and as a generic slot box, `ListBox`, for the steps that add them.
- A `Tx` lists moves. `Apply` runs every move on copies of the holders first: ownership by serial, amounts, weight and slot limits, and the stack rules of section 5.4. When one fails, nothing changes. Then it makes the moves and returns one log line for each.
- Serials follow section 5.4. A whole stack keeps its serial. A split gets a new one. An amount that joins a stack takes the stack's serial. Each line names the serial it came from when that differs.
- The zone runs every item and zeny change of a player through a `Tx`: drops, pick-ups, monster items given to the inventory, NPC shops, the script calls for items and zeny, item use, and skill products and costs. A purchase and a sale are one `Tx` each. The messages players get did not change, and every zone test passes unchanged in what it checks.

### One transaction per commit

- The zone owns its players' state in memory and saves copies to the store (strategy section 4.1). A transfer cannot lock database rows while the zone waits, since zones never wait on the database.
- After each event, the zone hands the characters its transfers changed, with the log lines, to the store as a commit. The store writes a commit in one transaction: it locks the characters' rows in ID order with `SELECT ... FOR UPDATE`, saves each character with its save version, and writes the lines.
- One worker writes commits in the order they came. The order covers items that pass between players through the ground: a drop's commit is written before the pick-up's, so after a crash the database never holds an item twice.
- A character's saves that came before its commit are dropped, since the commit's copy is newer. Its saves that came after wait for the commit.
- A commit that has not started takes in the commits after it, so a slow database gets fewer, larger transactions.
- Without a database, the lines go to the log at debug level, and saving works as before.

### Logs

- Migration 00015 adds `item_log` and `zeny_log`, partitioned by month, with a default partition. `ensure_log_partitions` makes the partitions of this month and the next. It runs with the owner's rights, so the game role can call it without the right to change the schema. The store calls it at start and every 12 hours.
- Rows have no foreign keys, so they outlive the characters they name.

### Duplication tests

- `DupTest` runs many bots that make moves at the same time, while the test reads every item the server holds from a `--dev`-only debug endpoint, `GET /debug/wathena/items`.
- After every audit and at the end, the totals of each item and of zeny are those of the start, no serial is held twice, and each bot's inventory matches the server's.
- With a database, it also checks the saved inventories, one log line for each move the bots saw, and a history for every serial.
- The first test uses the moves that exist: 16 bots drop parts of their stacks and pick up each other's for 6 seconds.

### Local checks

- GitHub Actions does not run for this repository. `scripts/check.sh` runs the jobs of `ci.yml` locally, one step per job, and the `test` step includes `./e2e`.
- It starts a PostgreSQL container when no test database is set, compares generated files before and after their generators, and reports a step whose tool is missing as skipped.
- The workflow files did not change.

## Why commits, and not transfers in the database

| | Commit the zone's copies in order | Run each transfer in the database |
|---|---|---|
| Zone waits on the database | Never. | On every drop, pick-up and purchase. |
| Atomic in the database | Yes, per commit. | Yes, per transfer. |
| Fits the save model of section 4.1 | Yes: saves and commits are copies of one state. | No: a second writer of the inventory rows would race the saves. |
| Throughput | One worker, with batching when the database is slow. | One transaction per transfer. |

A later step can replace the single worker with an order that only joins commits that share a character or a ground item, if one worker is too slow.

## Other choices

- A ground item is not saved, so a pick-up makes a new serial and logs the dropped stack's serial as its source, as before.
- Card insertion still changes slots directly. It moves a card into an item, which needs a log line kind of its own.
- When a newer session loaded a character whose commit is waiting, the commit is written without that character, and the error log names it. The item log then shows any copy that results.
- `Apply` panics when a move fails after its checks passed. That can only be a bug in a box, and the zone must not go on with half a transfer.

## Left

- Dropping log partitions after the 180 days of section 7.10, and a setting to shorten it.
- Card insertion and the other changes that do not move items between owners, such as refining.
- Holders other than the inventory, with their tables: storage, cart, guild storage, mail and vending (steps 4 to 6).
- A `wactl` command and a staff tool to trace a serial through the item log.
