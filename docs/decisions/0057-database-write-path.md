# 0057: Database Write Path

- Status: proposed. Not built.
- Date: 2026-10-20.
- Code: [server/store](../../server/store) (`store.go`, `commit.go`, `statuses.go`, `market.go`, `retention.go`, `backup.go`, `migrations`, `queries`), [server/cmd/waserver](../../server/cmd/waserver) (`main.go`), [tools/wactl](../../tools/wactl) (`backup.go`).
- Builds on decisions [0003](0003-database-migrations-and-queries.md) and [0049](0049-retention.md).
- Lua API: none. Protocol: none. Settings: new pool settings in `waserver.yml` (step 3). Schema: migrations in steps 1, 4, 5 and 6.

## Context

The findings below come from reading the code on 2026-10-20. None was measured. Most of the save path is sound:

- An unchanged character is not saved (`server/zone/persist.go`).
- Inventory, storages, variables, quests and currencies write only the rows that changed.
- Item, zeny and chat logs are inserted in batches.
- Player search uses prefix indexes, and no query uses `LIKE '%x%'`.

These are the weak points, most severe first.

1. **Retention holds the item log's lock for its whole run.** The retention function in `migrations/00037_retention_floor.sql` loops over old month partitions. For each it runs `SELECT count(*)` and then `DROP TABLE`, all in one transaction. `DROP TABLE` takes an ACCESS EXCLUSIVE lock on the partition and the parent `item_log`, and keeps it until the function returns. That includes counting the next partitions, the `zeny_log` pass, and the `DELETE` on the default partition. Every commit writes logs (`writeLogs` in `commit.go`), so every transfer waits.
2. **One goroutine writes every commit.** `store.go` starts one `commitWork`. Every trade, drop, pickup, vending sale and mail is one transaction, written one at a time. Each takes `FOR UPDATE` on characters, guild rows and account storage advisory locks. A slow statement or lock wait stops all transfers in the process. A failed batch holds later commits for the retry delay.
3. **The pool size is the pgx default.** `main.go` does not set `MaxConns`, so it is the larger of 4 and the CPU count. The pool serves 4 save workers, the commit worker, the chat log writer, the social store, partition upkeep, admin queries and retention. Retention holds one connection for its whole run.
4. **Statuses and timed bonuses are rewritten on almost every save.** The saved values include time remaining (`Status.Tick` and `TimedBonus.Remaining` in `server/character/character.go`), which changes between saves. So any player with a buff deletes and inserts all of those rows (`statuses.go`). Timed bonuses are inserted one row per statement.
5. **Chat retention deletes 10,000 rows at a time from one large table.** `chat_log` has no partitions. Each batch scans the `at` index again from the oldest row, past entries vacuum has not cleaned yet, so later batches cost more.
6. **Partitions are made while the game writes.** `ensure_log_partitions` runs every 12 hours. Creating a partition while a default partition exists scans the default partition and locks the parent. No `lock_timeout` is set.
7. **Admin log queries for an account have no time range.** `AccountItemLog` and `AccountZenyLog` in `queries/accountpage.sql` match six OR branches over every partition, then sort all matches.
8. **The backup holds one snapshot for the whole dump.** `pg_dump` runs under REPEATABLE READ over the whole database, logs included. Vacuum cannot clean the `characters` and inventory rows that autosave updates while it runs.
9. **Mail and vending rewrite all their rows.** Marking a mail read deletes and inserts its items. A vending sale rewrites every line of the shop (`market.go`).
10. **Account variables can be overwritten.** Two characters of one account in different zones each write account variables from their own copy, with no version check (`store.go`). The last write wins. This is a correctness bug.

## Decision

Fix them in this order. Each step must keep `scripts/check.sh` passing.

### 1. Retention without a long lock

- Drop one partition per transaction.
- Detach it first with `ALTER TABLE ... DETACH PARTITION ... CONCURRENTLY`, then drop it. `CONCURRENTLY` cannot run inside a function's transaction, so the loop moves to Go in `retention.go`.
- Count the rows from `pg_class.reltuples` instead of `count(*)`. The count is only for the report.
- Set `lock_timeout` on each statement, and skip a partition that cannot be locked until the next run.
- Delete from the default partition in batches, as chat retention does.

### 2. Commit lanes

- Run several commit workers. Each commit goes to a lane chosen by the characters it touches, so commits of one character stay in order.
- A commit that touches characters of two lanes waits for both. Choose lanes by a hash of the lowest character ID first, then check that no commit touches a character that another lane has in flight. This keeps the order the current single lane gives.
- Lock rows in a fixed order (character IDs ascending, then guild IDs) to avoid deadlocks between lanes.
- Keep one lane as the default until the duplication and restore tests (`e2e/dup.go`, `e2e/restore.go`) pass with several lanes.

### 3. Pool size

- Set `MaxConns` for each pool from `waserver.yml`. The default is save workers + commit lanes + 4 for background jobs + 4 for admin.
- Log a warning at start when the setting is lower than the workers.

### 4. Statuses and timed bonuses store end times

- Store the time a status or bonus ends, not the time remaining. Saves then compare equal when nothing changed.
- Insert timed bonuses with one `unnest` statement.
- Migration: convert the remaining time of existing rows to an end time from the row's save time.

### 5. Chat log partitions

- Partition `chat_log` by month like `item_log`, and drop old partitions with the code from step 1.
- Rows that an open report holds move to a separate table before their partition is dropped. Today the delete skips them.

### 6. Partition upkeep ahead of time

- Create partitions 3 months ahead, so the default partition stays empty.
- Set `lock_timeout` on partition creation.

### 7. Smaller fixes

- Account log queries take a since and until range, and page by `(at, id)`, as `CharacterItemLog` does.
- The backup dumps log tables separately, or leaves them out when the owner sets so.
- Mail and vending write only the rows that changed, with the same diff as inventory (`diffItems`).
- Account variables get a version per row. A write with an old version is refused and retried from the new value.

## Breaks

- Step 1 moves part of retention from SQL to Go. The database role needs the right to detach partitions.
- Step 4 changes the status and timed bonus tables. The migration converts existing rows.
- Step 5 changes the chat log table. The migration copies existing rows into partitions, which can take long on a big table. Owners run it during a planned restart.
- Nothing changes for clients or mods.

## Alternatives

- **Keep one commit lane and make each commit faster.** It does not remove the stall when one statement waits on a lock.
- **Use TimescaleDB or pg_partman for partitions.** It adds an extension that hosted PostgreSQL may not offer. The partitions here are simple enough to manage in Go.
- **Delete logs by date without partitions.** It has the same vacuum problem as chat retention.

## How to check it worked

- A test runs retention on a log with several full months while commits run, and checks that no commit waits longer than the `lock_timeout`.
- The `wabots` scenarios from decision 0050 with many trades and drops, before and after step 2.
- `pg_stat_statements` after a 30 minute soak: the status and timed bonus statements drop to near zero for players whose buffs did not change.
- `e2e` duplication and restore tests pass with several lanes.
