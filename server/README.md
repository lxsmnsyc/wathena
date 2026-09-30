# server

Go services: gateway, account, world, and zone. See docs/strategy.md, section 4.

| Package | Content |
|---|---|
| `zone` | One map: its players and monsters, local chat to every player within 9 cells of the speaker, their walks through `shared/path`, and what each player sees within 14 cells, in 50 ms ticks. Monsters wander as rAthena's `mob_randomwalk`, from the zone's secret random stream. Units attack a target at their attack delay, walking into range first. A monster that can attack strikes back. Dead monsters respawn after their respawn time, and dead players return to the start after 3 seconds. A kill gives exp to the players who fought the monster and drops items around it. Items lie on the ground for a minute, and players pick them up with the looter rights rAthena gives the top attackers. Each player has rAthena's inventory: 100 slots, stacks up to 30000, and a weight limit that pick-ups respect. Players use healing items, and drop items at their feet with no looter rights. HP and SP regenerate as rAthena's natural regen, which stops from 70% weight in renewal (50% in pre-renewal). From 90% weight, attacks stop. |
| `item` | The inventory, ported from rAthena's `pc_additem`, `pc_delitem` and `pc_inventoryblank`, the overweight levels of `pc_updateweightstatus`, and the heal of `pc_itemheal` and `pc_percentheal`, with the item data from `reference/rathena/extracts/<mode>/items.json`. |
| `battle` | Attacks, ported from rAthena's `battle.cpp` in renewal mode and checked against the recorded reference. |
| `status` | Player stats from job, levels, raised stats and skills, ported from rAthena's `status_calc_pc` for a player with no equipment, in both modes, and checked against every recorded player. Raising stats with status points, ported from `pc_statusup`, `pc_need_status_point` and `pc_maxparameter`, with the costs of both modes and the job limits from `conf/battle/player.conf`. |
| `progression` | Exp tables, level ups, status and skill points, the renewal level penalty, exp shares, drop rates and rolls, where drops land, and pick-up rights, ported from rAthena's `pc.cpp`, `mob.cpp` and `map.cpp` with the default `conf/battle` settings. |
| `character` | A saved character as plain data: what the zone loads and hands to the store. |
| `store` | PostgreSQL persistence: goose migrations in `migrations`, sqlc queries in `queries` generated into `db`, and a pool of save workers. See [decision 0003](../docs/decisions/0003-database-migrations-and-queries.md). |
| `gateway` | The WebSocket protocol from docs/proposals/0001-protocol.md: Hello first within 10 seconds, version and size checks, and commands to the zone. Chat lines are cleaned and checked (valid UTF-8, no control or bidirectional formatting characters, 1 to 255 code points) and rate limited per connection: a burst of 5, then 1 per second. |
| `cmd/waserver` | A development server in one process: an asset build over HTTP and one zone over WebSocket, with characters in PostgreSQL or in memory. |

## Development server

There is no account service yet, so `--dev` takes any valid character name as the session token. Never expose a `--dev` server to the internet.

```text
go run ./server/cmd/waserver --assets /path/to/build --map prontera --dev
```

It spawns the map's monsters from the spawns that `reference/rathena` recorded (`--mode renewal` or `pre-renewal`, or `none`), until the core mod has its own.

Players are novices with no equipment. Their sex and hair come from a hash of their name. They start as a new rAthena character does: every stat at 1 and 48 status points to spend. `--build` picks other raised stats for testing, from the reference recorder's builds (STR, AGI, VIT, INT, DEX, LUK, before job bonuses), with the same 48 points:

| Build | Stats |
|---|---|
| `all_1` (default) | 1, 1, 1, 1, 1, 1 |
| `melee` | 90, 50, 30, 1, 40, 1 |
| `critical` | 70, 90, 1, 1, 40, 50 |
| `magic` | 1, 1, 30, 99, 80, 1 |
| `ranged` | 30, 40, 1, 1, 99, 30 |
| `tank` | 60, 1, 99, 1, 30, 1 |

The `all_1` novice can barely hurt a monster. `melee` is easier for testing drops and level-ups. Players start at base and job level 1, so there is no `--level`. Their stats come from the job tables in `reference/rathena/extracts/<mode>/job_stats.json` (renewal's with `--mode none`).

It serves the build at `http://127.0.0.1:8300/assets/` and the game at `ws://127.0.0.1:8300/ws`. With the web client's dev server:

```text
WATHENA_ASSETS=http://127.0.0.1:8300/assets pnpm dev
```

then open `http://localhost:5173/?server=ws://127.0.0.1:8300/ws&name=Alice` in two tabs with different names. Click the ground to walk, a monster to attack it, and an item to pick it up. Each tab sees the other walk and fight.

Players gain exp and levels from kills, with the exp tables, drops and level penalty from `reference/rathena/extracts/<mode>/progression.json`. Each base or job level up computes their stats again, and a base level up heals HP and SP fully. Players spend status points from the client's status window, and each raise computes their stats again. Local chat reaches every player within view, the speaker included; other channels are ignored for now. Without a database, characters and the inventory only live in memory. See "Running with PostgreSQL".

`--start-items` gives every new character items, for development only, such as `--start-items 501:5,512:10` for 5 Red Potions and 10 Apples. Without it, characters start with nothing and get items from drops. With a database, a character only gets them when it is made.

One character has one session. A second login with the same name, in any case, kicks the first, as rAthena does. The first page shows "Your character logged in somewhere else." The second continues once the first has saved.

## Running with PostgreSQL

`waserver` keeps characters in PostgreSQL 18 or later when it gets a database URL, from `--database` or the `WATHENA_DATABASE_URL` environment variable. Use the environment variable, so the password does not show in process lists.

With a database:

- It refuses to start while migrations are pending.
- A `--dev` token that is a new name makes an account and a character of that name, marked as a dev account. This is only for development, until the account service exists.
- Characters load when they log in and are saved every 30 seconds when they changed, when they log out, and when the server stops (Ctrl+C or SIGTERM).
- The level, exp, status and skill points, raised stats, HP, SP, position, look, zeny and inventory are saved. Items keep their serial numbers.

A local database in Docker:

```text
docker run -d --name wathena-pg -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:18
docker exec -i wathena-pg psql -U postgres < deploy/postgres/roles.sql
docker exec -it wathena-pg psql -U postgres -c '\password wathena_migrator' -c '\password wathena_game'
```

`deploy/postgres/roles.sql` makes the `wathena` database and two roles:

- `wathena_migrator` owns the schema. Only `wactl migrate` uses it.
- `wathena_game` is what `waserver` uses. It reads and writes rows, and cannot change the schema or delete accounts and characters.

Apply the migrations as the migrator, then start the server as the game role:

```text
export WATHENA_MIGRATE_DATABASE_URL='postgres://wathena_migrator:PASSWORD@127.0.0.1:5432/wathena?sslmode=disable'
go run ./tools/wactl migrate status
go run ./tools/wactl migrate up

export WATHENA_DATABASE_URL='postgres://wathena_game:PASSWORD@127.0.0.1:5432/wathena?sslmode=disable'
go run ./server/cmd/waserver --assets /path/to/build --map prontera --dev
```

Use `sslmode=disable` only for a database on the same machine.

### Changing the schema or queries

- Add a migration as the next numbered file in `server/store/migrations`, with `-- +goose Up` and `-- +goose Down` sections. Grant `wathena_game` only the rights it needs, as the first migration does.
- Add queries to `server/store/queries`. Game code only runs queries that sqlc generates. `.github/scripts/sqlcheck.sh` rejects `fmt.Sprintf` and raw query calls in database code.
- Generate the code, and commit it:

```text
cd server/store && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate
```

sqlc needs a C compiler for its PostgreSQL parser. CI generates the code again and fails when it differs.

### Tests

The store's integration tests need a PostgreSQL 18 server and a role that can create databases and roles. Each test makes its own database and drops it afterwards. Without `WATHENA_TEST_DATABASE_URL`, they are skipped.

```text
docker run -d --name wathena-test-pg -e POSTGRES_PASSWORD=postgres -p 55432:5432 postgres:18
WATHENA_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable' go test ./server/store/
```

CI's `test` job runs them against a `postgres:18` service container.
