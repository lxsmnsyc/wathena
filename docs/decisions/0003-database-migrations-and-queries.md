# 0003: goose for Migrations, sqlc Run by Version, and How Saves Work

- Status: accepted.
- Date: 2026-10-01.

## Decision

- Schema migrations use **goose** (`github.com/pressly/goose/v3`), as a library.
  - Migrations are plain SQL files in [server/store/migrations](../../server/store/migrations), embedded in the binaries.
  - `wactl migrate status` and `wactl migrate up` run them. `waserver` refuses to start while any are pending.
- Queries use **sqlc**, pinned to one version and run with `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate` in `server/store`.
  - The generated code in [server/store/db](../../server/store/db) is committed.
  - The `queries` CI job generates it again and fails on any difference.
- Saving follows the rules below ("Saving").

## Why goose and not Atlas

The strategy (section 3.4) allowed either.

| Need | goose | Atlas |
|---|---|---|
| Plain SQL migrations that sqlc reads as the schema | Yes. sqlc understands goose's `-- +goose Up` and `Down` markers. | Yes, with its versioned mode. |
| Runs inside our binaries, no extra tool to install | Yes. It is a Go library with a few small dependencies. | Mostly a separate CLI. Its Go SDK runs that CLI. |
| Each migration in one transaction, with a lock against two runs at once | Yes (`lock.NewPostgresSessionLocker`). | Yes. |
| License | MIT. | An open-source community build, plus paid features. |

goose is enough for hand-written, reviewed SQL, and it keeps `wactl migrate` a normal Go command. Atlas's strength, a declarative schema that it diffs, is not needed while one developer writes each migration by hand.

## How sqlc is run

sqlc is run by version with `go run`, the same way `scripts/check.sh` runs `buf`. It is not a `tool` directive in `go.mod`.

- sqlc requires about 45 modules, including a cgo PostgreSQL parser and MySQL and SQLite engines. As a `tool` directive, they would all enter the main `go.mod` and `go.sum`, next to the game server's own dependencies.
- Only people who change queries need sqlc. Everyone else uses the committed code.
- The version is written in one place, `sqlc_version` in `scripts/check.sh`, and in `server/README.md`.

sqlc settings, in [server/store/sqlc.yaml](../../server/store/sqlc.yaml):

- `sql_package: pgx/v5`, so queries run on `pgxpool` directly.
- `integer` and `smallint` columns map to Go `int`. pgx refuses a value that does not fit the column, so no conversion can overflow without an error, and no `#nosec G115` is needed.

## Keeping queries safe

- Game code only calls the functions sqlc generates. They always bind parameters.
- `scripts/sqlcheck.sh` runs in the `security` step of `scripts/check.sh`. In every Go file that imports pgx or `database/sql`, outside tests and the generated code, it rejects:
  - `fmt.Sprint`, `fmt.Sprintf` and `fmt.Sprintln`.
  - Raw query calls: `Exec`, `Query`, `QueryRow`, `SendBatch` and `CopyFrom`.
- It is a text check. It does not catch SQL built with `+`, or a string built in one file and run in another. Code review covers those. The raw call rule makes them unlikely, since only generated code runs queries.

## Saving

Strategy sections 3.8 and 4.1 (item 5) set the rules. The design:

- **Worker pool.** `server/store` runs a fixed number of workers (4 by default). The zone hands a complete copy of a character to its session's `Save`, which queues it and returns at once. The zone never waits on the database.
- **Order.** Each session writes one save at a time, in order. A newer copy replaces a copy that is still waiting, since each copy is complete.
- **When.**
  - Every 30 seconds, for characters that changed. Each player's interval starts when it enters, so saves spread out.
  - When the player leaves.
  - For every player when the server stops. `waserver` then waits up to 10 seconds for the queue to empty.
  - Map changes will save too, once they exist.
- **Changed data only.** The zone compares the new copy with the last one it handed over and skips the save when nothing changed. The store writes the character row and only the inventory slots that differ from what the database holds.
- **One transaction.** The character row and its inventory are written together.
- **Save version.** Loading a character raises `characters.save_version`, and so does each save. A save names the version its session holds, so a save from an older session changes nothing and is logged. That session then saves nothing more, and the zone disconnects its player with `DISCONNECT_REASON_LOGGED_IN_ELSEWHERE`, so no one plays on without being saved. The store reports it through `Session.OnStale` on its own goroutine, and the zone takes it through its inbox, so neither a save worker nor the zone waits.
- **Failures.** A failed save is tried again after 5 seconds, unless a newer copy is waiting.
- **Serials.** Every inventory row has a UUIDv7 serial, which is its primary key, so one item cannot be in two places. Items on the ground are not saved, as in rAthena. An item picked up from the ground gets a new serial. The item log, which does not exist yet, will link the two.

## One session per character

A login for a character that already has a session kicks the older session, as rAthena does.

1. The gateway finds the character's ID, then marks the new session as the character's live one.
2. The older connection gets `DISCONNECT_REASON_LOGGED_IN_ELSEWHERE` and closes.
3. The new login waits, up to 10 seconds, until the older session has left the zone and queued its last save.
4. Loading waits until the character's queued saves are written, then raises the save version.

rAthena refuses the second login and asks the player to try again. wAthena lets it continue once the older session has saved, so a reload of the page works at once. If the older session does not end in time, or runs on another server, its next save fails on the save version instead of overwriting the newer session, and its player is disconnected.

## Database roles

Strategy section 7.8 asks for one role per service with only the rights it needs. [deploy/postgres/roles.sql](../../deploy/postgres/roles.sql) makes three:

| Role | Rights |
|---|---|
| `wathena_migrator` | Owns the database and its schema. Only `wactl migrate`, restores and other manual `wactl` work use it. Compose backups connect as `wathena_backup`, which only reads (review 2026-10). |
| `wathena_game` | Connects, reads the applied migrations, reads and inserts accounts, reads, inserts and updates characters, reads and writes inventory rows, and reads and deletes game tickets. No DDL, it cannot delete accounts or characters, and it cannot read passwords or login sessions. |
| `wathena_account` | The account service. Reads and writes passwords, login sessions and game tickets, makes accounts and characters, and inserts new characters' start items. No DDL, and it cannot change or delete characters. See [decision 0004](0004-account-service-and-game-tickets.md). |

The migrations grant each role its table rights when the role exists.
