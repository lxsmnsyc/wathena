# wAthena Strategy: Go Server, Lua Scripting, Web Client, and Community Modding

This document is the plan for wAthena, a rebuild of rAthena on a new stack. The plan has these parts:

- A new server core in Go.
- PostgreSQL as the only database.
- Lua 5.4 instead of the custom NPC and item script language.
- A browser-based game client written in TypeScript. It is the only supported client.
- Renewal and pre-Renewal rules as feature flags that server owners can mix.
- A modding system that lets the community build and share content safely.
- Fixes for the security problems in the current design.
- A procedural world mode where fields, dungeons, and towns are generated from a seed.

Each phase has exit criteria that can be tested. Section 2.3 lists the decisions already made.

## 1. Current state

rAthena runs as four C++ processes.

| Process | Role |
|---|---|
| `login` | Account authentication. |
| `char` | Character select. Also the database proxy for guilds, parties, storage, mail, and similar (`src/char/int_*.cpp`). |
| `map` | All gameplay. |
| `web` | Small HTTP service for emblems, party booking, and user config. |

The map server holds most of the code.

| File | Lines | Content |
|---|---|---|
| `src/map/script.cpp` | 28.6k | Custom script language parser and interpreter. 676 built-in commands. |
| `src/map/clif.cpp` | 25.9k | Client packet encoding and decoding, mixed with game logic. |
| `src/map/status.cpp` | 16.5k | Stat and status effect calculation. |
| `src/map/skill.cpp` | 16.3k | Skill logic. |
| `src/map/pc.cpp` | 16.2k | Player logic. |
| `src/map/battle.cpp` | 9.2k | Damage formulas. |

Game content is large:

- 1138 NPC script files with about 808k lines, in `npc/`.
- Item, mob, skill, and other data in YAML files, in `db/`.

Main structural problems:

- The map server runs one thread with one event loop. Most state is global and mutable.
- `map_session_data` is a single class that holds almost all player state.
- Client version (`PACKETVER`) and ruleset are chosen at compile time. There are 682 preprocessor checks for `RENEWAL` or its sub-flags in 132 files under `src/map`. Each ruleset needs its own build.
- MySQL queries block the main thread.
- Servers talk to each other with hand-written binary packets (`chrif`, `intif`, `char_mapif`).
- Timers are function pointers with an untyped `intptr` argument. Nothing ties a timer to the lifetime of the entity it uses.
- Every status recalculation reruns the script of every equipped item and card, even when nothing the script reads has changed.
- Clock and random number generation are global. Formulas cannot be tested in isolation.
- There are almost no automated tests.
- Customization works by editing core files or copying scripts from forums. There is no mod format, no versioned API, and no isolation between custom scripts.

Security problems in the current design:

- Passwords are stored in plain text by default. `use_MD5_passwords: no` in `conf/login_athena.conf`, and `login.user_pass` is `varchar(32)`. The MD5 option is unsalted and fast to crack.
- The char and map servers log in to the login server with a default account, `s1` / `p1` (`conf/char_athena.conf`, `conf/map_athena.conf`, and the seed row in `sql-files/main.sql`).
- Many queries are built with `printf`-style formatting and manual escaping instead of bound parameters. One missed escape is an SQL injection.
- The `query_sql` script command lets NPC scripts run any SQL. A script that puts player `input` into a query string is an SQL injection.
- The game protocol has no encryption. Packet obfuscation (`clif_obfuscation.hpp`) hides packet IDs but does not protect data.
- C++ packet parsing has no automatic bounds checks. A missed length check can read or write outside a buffer.

One refactor is already going the right way. Skills are moving from large `switch` blocks to `SkillImpl` classes in `src/map/skills/` (1244 files).

## 2. Goals, non-goals, and decisions

### 2.1 Goals

- Reproduce rAthena gameplay results for each `core` ruleset mod. Same inputs give the same damage, status, drop, and exp results as rAthena in that mode.
- Let server owners mix Renewal and pre-Renewal features through feature flags (section 3.6).
- Replace the custom script language with Lua and convert existing NPC and item scripts automatically.
- Use more than one CPU core by running maps in parallel.
- Support 5,000 concurrent players per world (section 3.7).
- Make game logic testable without a network or database.
- Let the community build, test, share, and install mods without editing the server source.
- Ship the base game content as a mod that uses the same public API as community mods.
- Fix every security problem listed in section 1.
- Offer a procedural world mode where every part of the world is generated (section 8).

### 2.2 Non-goals

- Support for the official Ragnarok Online client.
- Hosting any Gravity assets.
- Mobile-native apps. The web client should work on mobile browsers later.
- Importing accounts, characters, or items from existing rAthena servers.

### 2.3 Decisions

| Topic | Decision |
|---|---|
| Server language | Go. |
| Scripting | Lua 5.4. |
| Database | PostgreSQL 18 or later (section 3.5). |
| Ruleset | Feature flags with a Renewal or pre-Renewal base. Rulesets are mods, so modders define their own mixes. `core` ships one ruleset mod per mode (section 3.6). |
| Default ruleset | `core.ruleset.renewal` is the default choice when creating a world. |
| Clients | Only the web client. The official client is not supported. |
| Assets | Supplied by each player from their own game files. Mods can add their own original assets. |
| Capacity target | 5,000 concurrent players per world. |
| Login | Username and password at signup. Players can turn on TOTP and passkeys later. |
| Client customization | The web client is open source. The community changes it by forking or with plugins. |
| Procedural mode | Generates every part of the world: fields, dungeons, towns, spawns, and quests. Included in a build with the `procedural` build tag (section 8.1). |
| Default world mode | `procedural` in builds that include it. `classic` otherwise. |
| Launch order | Set by the build tag. The recommended first release is procedural mode only (section 14.2). |
| Mod index | Run by the community. The core team defines the index format and the `wactl` support. |
| Data import | No import from existing rAthena servers. New worlds start empty. |
| Rotating content | Depends on the automatic event system (section 8.8). |
| Mod content | Mods can add NPCs, monsters, items, skills, jobs, and pre-made maps. They change `core` content only through overlays and hooks. |
| Mod IDs | Fixed numeric ranges above all IDs used by `core` (section 6.5). |
| Repository | A new repository, separate from rAthena. |
| License | GPL-3.0-or-later with a mod exception (section 2.4). |
| Project name | wAthena ("web Athena"), following the jAthena, eAthena, and rAthena naming pattern (section 2.4). |
| Client data tables | The project ships minimum tables for sprites and effects, plus a guide and template for replacing them (section 10.5). |
| Map format | One project map format for generated maps, pre-made mod maps, and classic maps converted from player files (section 8.12). |
| Admin tools | Built into the web client, shown only to staff accounts (section 10.6). |
| Time zone | Events use the server's configured time zone (section 8.8). |
| Team | One developer. The roadmap is sized and ordered for that (section 14). |

### 2.4 License and naming

License: **GPL-3.0-or-later** for the server, the shared WebAssembly package, the web client, and `wactl`.

- The project converts rAthena's NPC scripts and game data and ports its formulas. rAthena is GPL v3 (`LICENSE`), so this work is a derived work and must use a GPL v3 compatible license.
- The web client includes the shared WebAssembly package, which contains ported rAthena logic such as pathfinding. The client therefore uses the same license. Forks that are published on the web must offer their source, which keeps community improvements available to everyone.
- roBrowser and roBrowserLegacy code can be reused only if its license is GPL v3 compatible. Check each file's header before copying.
- AGPL v3 was considered. It would force server owners to publish their server changes. Many server owners keep custom changes private, so AGPL would slow adoption. GPL v3 matches what the rAthena community already accepts.

Mod exception:

- A written exception states that mods which use only the public Lua API, YAML data, and assets are not derived works of the server. Mod authors choose any license for their mods.
- Mods that include converted rAthena scripts or data stay under GPL v3, because that content is GPL v3.
- The mod template repository (section 6.10) uses the 0BSD license, so mods can start from it with no conditions.

Other content:

- Documentation uses CC BY-SA 4.0.
- Data converted from rAthena's `db/` stays GPL v3. Each converted file keeps a header naming its source.

Naming:

- The project is named **wAthena**, for "web Athena". It follows the naming pattern of jAthena, eAthena, and rAthena. "Athena" is a Greek goddess, not a Gravity name.
- Tools and namespaces: `waserver` for the server binary, `wactl` for the command-line tool, and `wa` for the Lua API table.
- The name must not suggest that the rAthena team endorses the project. The project tells the rAthena team about wAthena as a courtesy, since it converts their GPL code and data.
- The name avoids `goAthena` and `pAthena`, which other projects already use.
- The project name, domains, logo, and client branding do not use "Ragnarok", "Ragnarok Online", "RO", "Gravity", or names of Gravity characters and places.
- Documentation may say the project is compatible with game files from Ragnarok Online. It must not suggest any link with Gravity.
- Converted content keeps rAthena's internal names, such as `AegisName` values, because the player's game files need them. Player-facing names in `core` locale files can be changed later if needed.

## 3. Technology decisions

### 3.1 Server language: Go

Go was chosen over C#. The deciding points:

- **Builds.** `GOOS` and `GOARCH` cross-compile static binaries for Linux, Windows, macOS, FreeBSD, and ARM from any machine. No runtime install is needed on the server.
- **Scripting without cgo.** A pure-Go Lua 5.4 runtime exists (section 3.2). Avoiding cgo keeps cross-compiling simple.
- **Concurrency.** A goroutine per zone with a channel inbox fits the zone model in section 4.
- **Shared code with the browser.** TinyGo compiles Go packages to small WebAssembly modules, so the map generator runs in the web client too (section 3.3).
- **Operations.** One binary per service, built-in profiling (`pprof`), and a low-pause garbage collector.
- **Community.** Go is easy to learn, which lowers the barrier for contributors.

Go weaknesses and how to handle them:

| Weakness | Mitigation |
|---|---|
| No enums or sum types. | Typed integer constants with `stringer` codegen. The `exhaustive` linter fails the build on `switch` statements that miss a case. |
| Verbose error handling. | Game logic returns errors only at system boundaries (network, database, scripts). Rule code works on validated data and does not return errors. |
| Garbage collector pauses in hot paths. | Set `GOMEMLIMIT`. Reuse message buffers with `sync.Pool`. Preallocate per-zone slices. Track tick time p99 in load tests. |

### 3.2 Lua runtime

Primary: **arnodel/golua**, a pure-Go implementation of Lua 5.4.

- Native 64-bit integers, which zeny, exp, and IDs need.
- Coroutines, which NPC dialogs need.
- CPU and memory limits per execution, which the sandbox needs.
- No cgo.

It is less mature and slower than the C implementation of Lua. The phase 0b benchmark confirmed the choice ([docs/decisions/0001-lua-runtime.md](decisions/0001-lua-runtime.md)):

- A script call costs under 1 µs with limits on.
- The CPU limit works across coroutine resumes.
- The memory limit panics when a coroutine is resumed in a later call. It is applied only to calls that do not resume a coroutine until this is fixed upstream.

Fallback: **C Lua 5.4 through our own small cgo binding.** It is fastest for pure Lua work and loading. Host calls cross cgo, and cross-compiling needs a C toolchain per target, built in CI with Docker.

yuin/gopher-lua is not a fallback. It implements Lua 5.1 with float numbers, has no memory limit, and its time limit does not follow coroutines across calls.

All server code talks to Lua through one binding layer (`internal/script`). Game systems never call the Lua library directly. This keeps the runtime replaceable.

### 3.3 Sharing logic with the web client

The client and server must agree on:

- Message formats.
- Pathfinding and cell walk timing.
- Map cell data (walkable, water, cliff).
- Distance and area checks.
- Generated map layouts in procedural mode.

Damage, status, drops, AI, and scripts stay on the server only. The client displays values the server sends.

Approach:

- Messages and ID tables are defined in schema files. Codegen produces Go and TypeScript code.
- The procedural map generator is written once in Go and compiled to WebAssembly with TinyGo. Generated maps must match on both sides bit for bit, and a second implementation would drift.
- Pathfinding, cell checks, and distance checks live in the same TinyGo package. The web client calls them instead of keeping its own copy.
- Shared test vectors, stored as JSON files, check the Go build and the WebAssembly build against each other.

### 3.4 Stack summary

| Area | Choice |
|---|---|
| Server language | Go, current stable release. |
| Networking | `coder/websocket` behind TLS. There is no raw TCP game port. |
| Database | PostgreSQL 18 or later (section 3.5). `jackc/pgx` driver and `pgxpool`. `sqlc` generates typed query code. |
| Schema migrations | `goose` or Atlas, with migration files in the repository. |
| Inter-service messages | gRPC (`grpc-go`) or ConnectRPC. |
| Scripting | Lua 5.4 through `arnodel/golua` (section 3.2). |
| Game data | YAML files read with `goccy/go-yaml`, validated with JSON Schema. Same layout as `db/*.yml` where possible. |
| Protocol definition | One schema file set. Codegen produces Go and TypeScript encoders and decoders. |
| Shared client logic | Go package compiled to WebAssembly with TinyGo. |
| Metrics and profiling | Prometheus client and `pprof`. |
| Linting | `golangci-lint` with the `exhaustive` linter enabled. |
| Release builds | `goreleaser`, static binaries for every target. |
| Web client | TypeScript, WebGL2 (WebGPU later), DOM-based UI. |
| Mod tooling | `wactl` command-line tool written in Go (section 6.9). |
| Password hashing | Argon2id (`golang.org/x/crypto/argon2`). |
| Two-factor login | TOTP (`pquerna/otp`) and passkeys (`go-webauthn/webauthn`). |
| Security checks in CI | `govulncheck`, `gosec`, the race detector, and fuzz tests. |

### 3.5 Database: PostgreSQL

PostgreSQL replaces MySQL. The new server supports only PostgreSQL. Supporting two databases doubles the query and test work.

Reasons:

- **Constraints that protect the economy.** `CHECK` constraints such as `zeny BETWEEN 0 AND 2147483647` and `amount > 0`, plus foreign keys, stop overflow and duplication bugs at the database even if the server has a bug.
- **Transactional schema changes.** A failed migration rolls back completely. MySQL commits most schema changes immediately, which can leave a half-migrated database.
- **JSONB.** Mod storage (section 6.7) keeps structured data per mod without a new table for each mod. GIN indexes make it searchable.
- **Row locking for queues.** `SELECT ... FOR UPDATE SKIP LOCKED` gives the persistence workers a simple job queue in the database.
- **`LISTEN` and `NOTIFY`.** Services can react to changes, such as a ban from the web panel reaching the account service at once.
- **Partitioning.** Logs such as item, chat, and zeny logs are partitioned by month. Old months are dropped or archived without slow deletes.
- **Go support.** `pgx` is fast and well maintained. `sqlc` supports PostgreSQL best.
- **Row-level security.** Web panels and reporting tools can get database roles that only see allowed rows and columns.

Minimum version: **PostgreSQL 18.**

- PostgreSQL supports each major version for 5 years. Version 18 was released in September 2025, so it is supported until late 2030. Version 14 reaches end of life in November 2026, and version 19 is in beta.
- Version 18 adds a built-in `uuidv7()` function. Time-ordered IDs for sessions, item serials, and log rows then need no extension.
- Version 18 adds asynchronous I/O, which speeds up the large log scans used by reports.
- CI tests against the minimum version and the newest release. The minimum moves up when a version has less than 2 years of support left.

Schema rules:

- Use `bigint` for IDs, zeny, and exp. Use `timestamptz` for times.
- Every table has a primary key. References use foreign keys.
- Every item instance or stack has a unique serial number. Logs record the serial on every move and every stack split or merge, so duplicated items can be traced.
- Trades, storage moves, mail attachments, and vending run in one transaction. They use `SELECT ... FOR UPDATE` on both sides, locked in a fixed order to avoid deadlocks.
- Game code uses only queries generated by `sqlc`, which always use bound parameters.

Costs:

- Tools built for rAthena's MySQL schema, such as FluxCP, will not work. The web service offers a documented HTTP API for control panels instead.
- Server owners who know MySQL must learn PostgreSQL basics. The admin guide covers backups (`pg_dump` and point-in-time recovery), connection limits, and `pgbouncer`.

### 3.6 Ruleset feature flags

Renewal and pre-Renewal differ in 682 code locations and in most data files. Some differences can be mixed freely. Others depend on data tuned for one formula and cannot be separated.

The ruleset is therefore built from two layers:

1. **A base.** It decides the core combat formulas and the main data set. Server owners pick `renewal` or `pre_renewal`.
2. **Independent flags.** Each one changes one feature and can be set on either base, within the listed dependencies.

#### Base (cannot be mixed)

The base includes these features, because items, mobs, and skills are tuned to them:

| Feature | Renewal | Pre-Renewal | Main rAthena code |
|---|---|---|---|
| ATK model | Status, weapon, equipment, and mastery ATK are separate parts. | One base ATK value. | `battle.cpp` 3422 to 3880, `status.cpp` 2428 to 2498 |
| MATK model | Weapons carry MATK. | MATK comes from INT, with a minimum and maximum. | `status.cpp` 2510, 6240, 7443 |
| DEF model | Flat DEF reduction, values in the hundreds. | Percentage DEF with a cap (`max_def`). | `battle.cpp` 4800 to 4844, `status.cpp` 7860 to 8004 |
| HIT, FLEE, and critical formulas | Renewal stat formulas. | Pre-Renewal stat formulas. | `status.cpp` 2617 to 2725 |
| Card bonus order | Renewal order and rounding. | Pre-Renewal order and rounding. | `battle.cpp` 708 to 1076 |
| Refining | Up to +20, armor levels, enchant grades. | Up to +10. | `status.hpp` 33, `status.cpp` 3932 to 4016 |
| Level cap and classes | Level 200 or 275, third and fourth classes, expanded classes. | Level 99, up to transcendent classes. | `pc.cpp` 1808, `char.cpp` 2825 |
| Data sets | `db/re`, `npc/re` | `db/pre-re`, `npc/pre-re` | |

Why these cannot be mixed:

- Item weapon ATK and MATK values mean different things under each ATK model. The same weapon gives very different damage.
- `mob_db` stats are rebalanced for each model. `Attack2` is MATK in Renewal and maximum ATK in pre-Renewal. A Poring has 1 ATK and gives 150 base exp in Renewal, and has 7 to 10 ATK and gives 2 base exp in pre-Renewal.
- Renewal DEF values are flat reductions in the hundreds. Pre-Renewal DEF is a capped percentage. Every item and mob is tuned to one of them.
- Refine tables, item levels, and the +20 refine limit form one system.
- Level cap, exp tables, job stat tables, and the level damage modifier form one system.
- Card percentages stack differently in each order, so card values are tuned per mode.

#### Independent flags

| Flag | Values | What it changes | Depends on |
|---|---|---|---|
| `cast_time` | `renewal`, `pre_renewal` | Variable plus fixed cast time, or DEX-only cast time. | Pre-Renewal base with `renewal` uses a default fixed cast of 20% when `skill_db` has no `FixedCastTime`. |
| `skill_cooldown` | `on`, `off` | Per-skill cooldowns for players, homunculi, and guild skills. | Cooldown values come from the Renewal `skill_db`, applied as a data overlay on a pre-Renewal base. |
| `aspd` | `renewal`, `pre_renewal` | ASPD formula, shield penalty, and bonus stacking. | Ships with both `job_aspd` tables. |
| `stat_point_cost` | `renewal`, `pre_renewal` | Status point cost formula (`statpoint.yml`). | None. |
| `level_penalty_exp` | `on`, `off` | Exp modifier by level gap between player and monster. | None. |
| `level_penalty_drop` | `on`, `off` | Drop rate modifier by level gap. | None. |
| `level_damage_mod` | `on`, `off` | Skill damage scales with base level. | Only useful with a level cap above 99. |
| `element_table` | `renewal`, `pre_renewal` | Element damage table (`attr_fix.yml`). For example, Neutral against Ghost is 90% or 25%. | None. |
| `size_table` | `renewal`, `pre_renewal` | Size penalties for knuckles and whips (`size_fix.yml`). | None. |
| `status_resist` | `renewal`, `pre_renewal` | Status effect resistance and duration formulas. | None. |
| `heal_formula` | `renewal`, `pre_renewal` | Heal skill and healing item formulas. | None. |
| `boss_protection` | `renewal`, `pre_renewal` | Boss endure, trap immunity, and stun length. | None. |
| `ensemble_songs` | `renewal`, `pre_renewal` | Songs as self buffs and ensemble fatigue. | None. |
| `homunculus` | `renewal`, `pre_renewal` | Homunculus ASPD, exp share, and stat growth. | Ships with both homunculus tables. |
| `busy_restrictions` | `on`, `off` | Blocks NPC, item, skill, and cart actions while busy. | None. |
| `fourth_job_traits` | `on`, `off` | POW, STA, and other trait stats, plus RES and MRES. | Renewal base only. |
| `item_rolls` | `on`, `off` | Items with roll ranges get random values when created. With `off`, every roll uses its default value, which matches rAthena (section 5.4). | None. |
| `skill_rebalance.<skill>` | `renewal`, `pre_renewal` | Ratio and effect of one skill that changed between modes, such as Basilica or Magnum Break. | Base ATK model for damage skills. |

#### Ruleset mods

Rulesets are mods. There is no ruleset built into the server.

- `core` ships two ruleset mods: `core.ruleset.renewal` and `core.ruleset.pre_renewal`. Each sets every flag to its own mode's value.
- Modders publish their own ruleset mods that mix features from both modes.
- A server owner picks one ruleset mod when creating a world. A world cannot change its ruleset later, because items, stats, and levels depend on it.
- `wactl world new` offers `core.ruleset.renewal` as the default choice. Its data set is much larger (29,356 items and 2,675 monsters, against 6,169 items and 1,004 monsters in pre-Renewal), and procedural worlds need a large pool of content to draw from.

A ruleset mod can:

1. **Pick a base and set flags.** This covers every combination in the tables above.

   ```yaml
   # mod.yaml
   id: example.classic-plus
   provides:
     ruleset:
       extends: core.ruleset.pre_renewal
       flags:
         cast_time: renewal
         level_penalty_drop: on
         skill_cooldown: on
   ```

2. **Ship data overlays that rebalance the base.** Features in the base cannot be mixed because data is tuned to one formula. A ruleset mod that also rebalances that data can mix them anyway. For example, a mod can use the Renewal ATK model on the pre-Renewal item and monster lists if it supplies Renewal-style ATK values for those items and monsters.

3. **Replace single formulas with hooks.** With the `rules.override` permission, a ruleset mod can replace a formula such as the HIT calculation or the refine success rate (section 6.8). This covers ideas that neither mode has.

Rules:

- The loader checks flag dependencies for every ruleset mod. A ruleset that uses a base feature with the other mode's data must declare the data overlays that fix it, or loading fails.
- Content mods declare which rulesets they support, by base or by flag (section 6.3). The loader refuses to load a content mod on a ruleset it does not support.
- Scripts in any mod can read the active flags, so one content mod can support several rulesets.
- `wactl mod test` runs a ruleset mod's tests against both `core` parity reference sets, so authors can see how far their mix moves from each mode.

Implementation:

- In Go, `Ruleset` is a struct of small interfaces, one per flag, such as `CastTimeModel` and `ASPDModel`. Each interface has a Renewal and a pre-Renewal implementation.
- Each flag declares the data files it needs. The data loader picks each file from the Renewal or pre-Renewal set to match.
- The loader rejects combinations that break a dependency, with an error naming the flag.
- Scripts check flags with `wa.rules.is("cast_time", "renewal")`. This replaces rAthena's `checkre()`.
- Settings in `conf/battle` whose defaults depend on the mode today, such as `default_fixed_castrate` and `max_def`, become settings of the matching flag.

Testing:

- Both `core` ruleset mods must match rAthena in the parity harness.
- Each flag has unit tests for both values.
- CI runs a sample of mixed combinations to check that the loader and formulas work together.

### 3.7 Capacity target

Public data on private server population is limited. RateMyServer lists servers but does not show player counts. Nostalgic.gg shows online members of each server's Discord, which is a proxy for players online. In September 2026 it showed about 2,000 to 7,000 online for the five largest servers, and tens to a few hundred for most others ([Nostalgic.gg server list](https://nostalgic.gg/en/ragnarok-online)).

Targets:

- One world supports 5,000 concurrent players. This covers every server except the very largest.
- Load tests run at 7,500 players to keep 50% headroom.
- One zone supports 500 players in view of each other, such as a town during an event.
- Larger worlds spread zones across more processes or machines (section 4.1, item 1).

### 3.8 Performance budgets

Server:

| Budget | Target | Reason |
|---|---|---|
| Zone tick | 50 ms (20 ticks per second) | RO movement is timed per cell. Default walk speed is 150 ms per cell (`DEFAULT_WALK_SPEED` in `src/common/mmo.hpp`), and the fastest is 20 ms. rAthena's timer loop runs at least every 20 ms (`TIMER_MIN_INTERVAL`). A 50 ms tick with per-cell timestamps keeps movement exact without running every 20 ms. |
| Tick work | p99 under 10 ms, maximum 25 ms | Leaves 60% of the tick free for bursts. A zone over budget for 10 ticks in a row logs a warning with a profile. |
| View range | 14 cells | Same as rAthena's `area_size` default (`conf/battle/client.conf`). Updates are only sent for entities in range. |
| Memory per online player | Under 256 KB | 5,000 players then use about 1.3 GB, leaving room for maps and Lua. |
| Lua memory | 16 MB per mod per zone | Enforced by the sandbox (section 6.6). |
| Lua CPU per call | 5 ms for events, 50 ms for generation layers | A call over the limit is stopped (section 6.6). |
| Database query | p99 under 20 ms | Zones never wait on queries, but slow queries delay saves. |
| Login to in-game | Under 2 seconds on the server side | Excludes asset loading in the client. |
| World startup | Under 30 seconds | Zones start in parallel. Generated maps load from the disk cache. |

Network, per player:

| Budget | Target |
|---|---|
| Server to client, busy town | Average under 8 KB/s, peaks under 32 KB/s |
| Server to client, field | Average under 2 KB/s |
| Client to server | Average under 1 KB/s. The rate limits in section 7.4 start at 4 KB/s. |
| Largest message | 64 KB. Larger data, such as map files, uses HTTP downloads. |
| Messages per tick | One batched message per player per tick. |

Data loss after a crash:

| Data | Largest loss |
|---|---|
| Trades, vending, buying stores, mail, storage moves, guild storage, cash points | None. Each is written in a transaction before the player sees the result. |
| Character position, exp, zeny from monsters, item drops | 30 seconds. Changed data is written every 30 seconds and at logout and map change. rAthena's default is 300 seconds (`autosave_time` in `conf/map_athena.conf`). |

Web client:

| Budget | Target |
|---|---|
| Frame rate | 60 FPS in a busy town on a mid-range laptop. 30 FPS on mid-range phones. |
| Map ready after warp | Under 1.5 seconds when cached. |
| First start after assets are cached | Under 10 seconds. |
| Browser memory | Under 1.5 GB. |

CI runs the server budgets in load tests (section 12). A change that breaks a budget fails the nightly run.

### 3.9 Deployment modes

| Mode | For | Setup |
|---|---|---|
| All-in-one | Most servers, up to about 2,000 players | One binary, `waserver all`, runs the gateway, account, world, and zone services in one process. PostgreSQL runs next to it. |
| Distributed | Large worlds | Each service runs as its own process. Zones spread across several machines. Services use mutual TLS (section 7.2). |

All-in-one details:

- The repository ships a Docker Compose file with `waserver`, PostgreSQL 18, and a TLS proxy (Caddy) that gets certificates automatically.
- The same Compose file serves the web client build as static files.
- `wactl world new` creates the database, the first admin account, and the world seed. `wactl world backup` and `wactl world restore` wrap `pg_dump` and restore.
- Service calls inside one process skip the network but use the same interfaces as the distributed mode. Tests run both.

Minimum hardware for all-in-one with 500 players: 4 CPU cores, 8 GB RAM, and 50 GB SSD. The load tests in section 12 confirm or correct these numbers.

Updates:

- Releases are single binaries plus database migrations. `waserver` refuses to start if migrations are pending, and `wactl migrate` applies them.
- A restart announces a countdown to players, saves all data, and stops zones cleanly.
- Restarts without downtime are a later goal for the distributed mode.

## 4. Target server architecture

```mermaid
flowchart LR
  WC[Web client] -->|Protocol over WebSocket and TLS| GW[Gateway]
  GW --> ZS
  subgraph ZS[Zone service]
    Z1[Zone goroutine: prontera]
    Z2[Zone goroutine: generated field 3,-1]
    Z3[Zone goroutine: instance 42]
  end
  ML[Mod loader] --> ZS
  EV[Event scheduler] --> ZS
  ZS <-->|gRPC| AS[Account service]
  ZS <-->|gRPC| WS[World service: party, guild, chat, mail]
  ZS --> PL[Persistence layer]
  WS --> PL
  AS --> PL
  PL --> DB[(PostgreSQL)]
```

### 4.1 Changes to make from the start

1. **Zone goroutines.**
   - Each map or instance runs in its own goroutine. It owns all state for that zone and reads commands from one inbox channel.
   - A ticker drives the zone tick. Code inside a zone runs on one goroutine and needs no locks.
   - Actions that cross zones, such as warps, party updates, and whispers, are sent as messages to the other zone's inbox.
   - Each zone has its own Lua state.
   - Zones can be spread across processes or machines.

2. **Entity model.**
   - Split player, mob, NPC, pet, homunculus, and mercenary state into components. Examples are `Position`, `Stats`, `Inventory`, `StatusEffects`, and `Cooldowns`.
   - Stats are totals of modifiers. Each modifier source (equipment, card, buff, passive skill) adds and removes its own modifiers (section 5.3).
   - Final values such as ATK, HIT, and ASPD are recomputed only when a total they depend on changes.

3. **Ruleset at runtime.**
   - The ruleset is built from a base and independent flags (section 3.6).
   - No build tags or compile-time switches for game rules. The only build tag is `procedural`, which includes or leaves out a whole feature (section 8.1).

4. **Protocol layer.**
   - One protocol, designed for the web client. Messages are defined in schema files and code is generated from them.
   - The protocol has an explicit version. The client sends its version at login, and the server rejects versions it does not support.
   - Related updates are grouped into one message per tick to reduce message count.
   - The gateway translates messages into typed game commands and translates game events back into messages. It contains no game logic.

5. **Persistence.**
   - A pool of worker goroutines handles all database access. A zone never waits on the database during a tick.
   - Entities track which fields changed. Saves write only changed data, at intervals and at checkpoints such as logout and map change.
   - Trades, storage moves, mail attachments, and vending use database transactions to prevent item duplication.
   - PostgreSQL constraints reject negative or overflowing zeny and item amounts (section 3.5).

6. **Services.**
   - Merge `login` and `char` into one account service.
   - Guild, party, clan, chat channels, and mail move to a world service.
   - Services talk over gRPC with versioned contracts.

7. **Scheduling.**
   - Timers are typed and registered on an owner (entity, modifier set, or mod).
   - When the owner is removed, all its timers and event listeners are removed.
   - World-wide scheduled events run through the event scheduler (section 8.8).

8. **Determinism and tests.**
   - Clock and random number generator are passed to every system as interfaces.
   - Damage, status, and drop formulas are pure functions over their inputs.
   - A test run with a fixed seed gives the same results every time.

9. **Request validation.**
   - Every client command passes one validation layer before reaching game logic.
   - It checks range, entity state, cooldowns, weight, zeny overflow, and rate limits.
   - The web client is open source and easy to modify, so the server trusts nothing it sends.

10. **Data.**
    - Keep YAML game data files.
    - Validate them against a schema at load time.
    - Support reloading data files without a restart.

11. **Mods from the start.**
    - The mod loader, public Lua API, and data overlays exist before any content is ported.
    - The base game content is packaged as the `core` mod. If `core` cannot do something through the public API, the API is missing a feature.

## 5. Lua scripting

### 5.1 Public API shape

- Functions live under the `wa` table, for example `wa.npc`, `wa.rand`, `wa.chance`.
- Game constants such as `bLuk`, `Size_All`, and `EF_WIND` are global and read-only. Their names match rAthena's `db/const.yml` and `src/map/script_constants.hpp`, so converted scripts and forum knowledge still apply.
- Entities (`pc`, `mob`, `npc`, `it`) are userdata holding an entity ID. Each access looks up the entity again. A player who logs out during a dialog causes a Lua error, not a crash.

### 5.2 NPC scripts

NPC dialogs pause while they wait for the player. Each dialog runs as a Lua coroutine.

- `mes`, `next`, `menu`, `input`, and `sleep` yield the coroutine.
- The zone resumes the coroutine when the matching client response arrives.
- If the player disconnects, the coroutine is closed.

Example:

```lua
wa.npc {
  name = "Healer", map = "prontera", x = 150, y = 180, sprite = "4_F_KAFRA1",
  on_click = function(pc)
    pc:mes("[Healer]", wa.t("healer.greeting"))
    if pc:menu(wa.t("common.yes"), wa.t("common.no")) == 1 then
      if pc.zeny < 500 then
        pc:mes(wa.t("common.not_enough_zeny"))
        return pc:close()
      end
      pc.zeny = pc.zeny - 500
      pc:heal_full()
    end
    pc:close()
  end,
}
```

In procedural mode, NPCs are placed at named places instead of fixed coordinates (section 8.6).

Other rules:

- **Events.** Scripts register callbacks such as `on_login`, `on_mob_kill`, `on_timer`, and `on_map_enter`. These replace label names like `OnPCLoginEvent:`.
- **Variables.** A typed API (`pc.vars`, `account.vars`, `server.vars`, and Lua locals) replaces the `#`, `##`, `$`, and `.@` prefixes. Each mod's variables are stored under its own namespace (section 6.7).
- **Reloading.** Each script file can be reloaded alone. Running coroutines finish on the old version.

### 5.3 Item scripts

Item and card scripts are event-driven. Each item file is loaded once and returns a table of handlers.

Equipping an item creates a modifier set (`mods`) owned by that equipped copy. Unequipping the item deletes the set. This removes all its bonuses, listeners, and timers at once.

Static bonus. Runs once, on equip.

```lua
-- Poring Card
return {
  on_equip = function(pc, it, mods)
    mods:add(bLuk, 2)
    mods:add(bFlee2, 1)
  end,
}
```

Computed bonus. The function reruns only when a value it read changes.

```lua
return {
  on_equip = function(pc, it, mods)
    mods:add(bBaseAtk, function()
      local lv = pc.base_level
      return lv > 70 and (lv - 70) // 10 * 10 or 0
    end)
    mods:add(bAspdRate, function() return 4 + it.refine end)
    mods:add(bBaseAtk, function() return pc.base_luk >= 90 and 20 or 0 end)
  end,
}
```

While a computed function runs, `pc` and `it` record which fields it reads. A level up recomputes only the modifiers that read `base_level`.

Proc on being hit.

```lua
return {
  on_equip = function(pc, it, mods)
    mods:add(bSubSize, Size_All, 15)
    mods:on("hit_taken", function(ev)
      if ev.flag & (BF_WEAPON | BF_MAGIC) ~= 0 and wa.chance(3, 1000) then
        mods:timed(10000, function(t) t:add(bFlee2, 30) end)
        pc:specialeffect(EF_WIND)
      end
    end)
  end,
}
```

Consumable.

```lua
return {
  on_use = function(pc, it)
    pc:itemheal(wa.rand(45, 65), 0)
  end,
}
```

Rules:

1. **Conditions read base stats only.** This matches rAthena. `readparam(bLuk)` returns `sd->status.luk`, the allocated points without bonuses (`src/map/pc.cpp`, `pc_readparam`). Item conditions cannot form cycles.
2. **Every value a script can read sends a change event.** This includes base and job level, base stats, job class, refine, enchants, grade, map and map flags, other equipped items, learned skill levels, active status effects, and time of day.
3. **Computed functions have no side effects.** They only return a value. The runtime blocks calls that change game state while a computed function runs.
4. **Results do not depend on equip order.** The final formula applies modifiers in a fixed order.
5. **Scripts using only constants become static tables.** The converter detects them, and the server applies them without calling Lua.

A debug mode used in tests runs a full recalculation after every change and compares it with the event-driven totals. A mismatch fails the test and names the item.

### 5.4 Rolled item values

Items can have values that are rolled when the item is created. The item's data declares each roll, and its script reads the rolled value.

The Abysmal Knight Card gives +25% damage against bosses in rAthena (`bonus2 bAddClass,Class_Boss,25;`). With a roll, each card gets a value from 15% to 35%:

```yaml
- AegisName: Knight_Of_Abyss_Card
  Rolls:
    boss_damage:
      Min: 15
      Max: 35
      Default: 25
      Distribution: triangular
```

```lua
return {
  on_equip = function(pc, it, mods)
    mods:add(bAddClass, Class_Boss, it.roll.boss_damage)
  end,
}
```

When values are rolled:

- On monster drops, crafting, and quest rewards.
- Scripts choose per call, for example `pc:getitem(id, 1, { roll = "random" })` or `{ roll = "default" }`.
- NPC shops sell items with default values unless the shop's data says otherwise.
- With the `item_rolls` flag set to `off`, every roll uses `Default`. This keeps parity with rAthena (section 3.6).

Distributions:

| Distribution | Result |
|---|---|
| `uniform` | Every value from `Min` to `Max` is equally likely. |
| `triangular` | Values near `Default` are common. `Min` and `Max` are rare. |
| `weighted` | A table of values or ranges with weights, for tiers such as common, rare, and perfect. |

A roll can use a Lua function instead of fixed `Min` and `Max`. The function receives the source of the item, such as the monster's level or, in procedural mode, the region's level band. Higher regions can then give better ranges.

Rules:

- **Server only.** Values are rolled on the server from the zone's secret random stream (section 8.3). The client never sends roll values.
- **Integers only.** Percentages use whole numbers or basis points.
- **Up to 8 rolls per item.** This keeps item messages and storage small.
- **Stored per item instance.** The item instance row has a `rolls` `JSONB` column, such as `{"boss_damage": 31}`. The server checks every value against the item's declared range when loading. Values outside the range are clamped and logged.
- **Cards keep their rolls in slots.** A slot entry stores the card ID and the card's rolls.
- **Stacking.** Stackable items with rolls stack when their rolls are equal.
  - The stack key is the item ID plus a canonical form of its rolls (keys sorted, integer values). Other stack rules, such as bound or unbound, still apply.
  - Two Abysmal Knight Cards with 31% stack together. A card with 31% and a card with 29% form two stacks.
  - Stacks with rolls count toward the inventory slot limit like any other stack. A `weighted` distribution with a few tiers keeps the number of stacks low.
  - Each stack has one serial number. Splitting a stack creates a new serial and logs which stack it came from, so duplication tracking still works.
  - Unidentified items with hidden rolls never stack. If they did, stacking would reveal which hidden values are equal before identification.
  - Trading, vending, and storage move amounts from a stack without changing its rolls.
- **Display.** The client shows the value and the range, for example "Damage against bosses +31% (15 to 35)". Item links in chat show rolls. Vending and buying store searches can filter by roll range.
- **Hidden rolls.** An item can set `RollHidden: true`. Its value is rolled at creation but shown only after identification with a Magnifier, the same as RO's unidentified items. Identifying cannot change the value.
- **Logs.** Item logs record the rolls with the item serial number.
- **Rerolling** is not in `core`. Mods can add reroll services with the `items.modify_rolls` permission.

rAthena's random options (`db/re/item_randomopt_db.yml` with 249 options, `item_randomopt_group.yml`, and the `option_id0` and `option_val0` columns) become extra rolled lines on the same storage.

### 5.5 Integer rules

- `/` always returns a float. Use `//` for integer division.
- `//` rounds toward negative infinity. C truncates toward zero. The results differ for negative numbers. The converter emits `wa.div(a, b)` wherever an operand can be negative.
- Integers are 64-bit and wrap on overflow.
- Scripts use `wa.rand` and `wa.chance`, not `math.random`, so tests with a fixed seed are repeatable.

### 5.6 Converting existing scripts

808k lines cannot be rewritten by hand.

1. Write a converter from rAthena script to Lua. Base its grammar on the parser in `src/map/script.cpp`.
2. Map each of the 676 built-in commands to a Lua API call. Mark commands that have no mapping yet.
3. Convert item scripts to the event-driven format in section 5.3.
4. Convert `checkre()` calls to ruleset flag checks (section 3.6).
5. Convert all of `npc/` into the `core` mod and record which files convert cleanly.
6. Fix remaining files by hand, starting with the most-used towns and quests.
7. Ship the converter as `wactl convert` so community members can migrate their own scripts (section 6.11).

In procedural mode, only scripts that do not depend on fixed maps are used (section 11.4). Converting all of `npc/` still matters for classic mode and for reusing shops, services, and dialogs.

### 5.7 Porting skill code

The skill code is the largest single task. `src/map/skills/` has 1244 files, and `skill.cpp`, `battle.cpp`, and `status.cpp` hold about 42k more lines with skill logic.

Method:

1. **Inventory.** A script lists every skill with its `SkillImpl` class, the functions it overrides, the `#ifdef RENEWAL` branches it contains, and the status effects it uses. The list becomes a tracking table in `docs/skills.md`.
2. **Golden tests first.** The parity harness records, for each skill and level, the damage, status effects, durations, and cooldowns for a fixed set of casters and targets. These are recorded before the skill is ported.
3. **Group by pattern.** Most skills follow a few shapes: single-target damage, area damage, buff, debuff, ground effect, and summon. Each shape gets a Go base type, like rAthena's `WeaponSkillImpl` and `StatusSkillImpl`. Skills that fit a shape become mostly data.
4. **Translate per skill.** A tool turns each `SkillImpl` file into a Go file draft. It handles the common mechanical parts: renamed types, `#ifdef RENEWAL` branches turned into ruleset flag checks, and rAthena helper calls mapped to the new API. AI-assisted translation can speed this step. Every draft is reviewed by hand.
5. **Test.** A skill is done when its golden tests pass on both rulesets.
6. **Order.** First classes, then second classes, then transcendent classes, then monster-only skills used by `core` monsters. Third and later classes follow the first release.

Rules:

- A skill is not merged without golden tests.
- Skill damage uses the ruleset helpers, so mixed rulesets work without special cases.
- Skills defined by mods use the same Go base types through the Lua API (section 6.2).

## 6. Community modding

### 6.1 Principles

- Mods use only the public API. There is no access to Go internals.
- The base game is the `core` mod. It is built with the same API as community mods.
- A broken mod must not crash a zone or corrupt another mod's data.
- Server owners decide what runs on their server. Mods declare what they need, and the owner approves it.
- Installing, testing, and publishing a mod needs one tool (`wactl`) and no C or Go toolchain.

### 6.2 What mods can add

| Content | How |
|---|---|
| NPCs | Lua scripts (section 5.2). |
| Monsters | YAML records plus optional Lua AI hooks. Sprites come from the mod's assets or reuse existing sprites. |
| Items | YAML records plus Lua item scripts (section 5.3). |
| Skills | YAML records for targeting, range, cast, and cooldown, plus Lua handlers for effects. Damage uses ruleset helpers such as `wa.battle.weapon_damage`, so custom skills follow the server's flags. |
| Jobs | YAML records for stats, HP and SP tables, ASPD, skill tree, and sprite references, plus a Lua job change script. |
| Pre-made maps | Map files in the mod's `maps/` folder, such as a starting town or home area. In procedural mode they are placed into the generated world (section 8.6). |
| Generation layers | Lua layers for procedural mode (section 8.7). |
| Automatic events | Event definitions for the scheduler (section 8.8). |
| Rulesets | A base, flag values, rebalancing data overlays, and formula hooks (section 3.6). |

Mods cannot replace `core` content. They change it only through data overlays (section 6.4) and hooks (section 6.8).

### 6.3 Mod package format

A mod is a folder or a `.zip` file.

```text
halloween-event/
  mod.yaml
  data/
    items.yml
    mobs.yml
    skills.yml
    jobs.yml
    spawns.yml
    events.yml
  scripts/
    npc/pumpkin_king.lua
    items/pumpkin_hat.lua
    skills/pumpkin_bomb.lua
  maps/                 # optional pre-made maps
  assets/               # optional original sprites, textures, and sounds
  locales/
    en.yml
    pt-BR.yml
  tests/
    pumpkin_king_test.lua
```

Manifest example:

```yaml
id: example.halloween-event
name: Halloween Event
version: 1.2.0
api: ">=1.0 <2.0"
authors:
  - name: Example Author
license: MIT
depends:
  - id: core
    version: ">=1.0"
load_after:
  - example.seasonal-lib
supports:
  rulesets:
    base: [renewal, pre_renewal]
    flags:
      skill_cooldown: on
permissions:
  - player.give_items
  - player.give_zeny
  - storage.mod
```

The `supports.rulesets` field lists the bases the mod works with and any flag values it needs. The loader refuses to load a mod on a ruleset that does not match. A ruleset mod uses `provides.ruleset` instead (section 3.6).

### 6.4 Data overlays

Mods add or change game data by overlaying YAML records.

- Records are keyed by a string name, such as `AegisName` for items.
- A mod can add records or change fields of existing records. It cannot delete `core` records.
- If two mods change the same field, the later mod in load order wins, and the loader logs a warning. A strict mode turns the warning into an error.
- `wactl mod validate` shows the final merged value of every record and which mod set each field.

### 6.5 IDs

The database stores items, mobs, skills, jobs, quests, and achievements by numeric ID. IDs must never change once assigned.

The `core` mod keeps rAthena's IDs. Player-supplied game files find sprites and icons by these IDs. The data is treated as complete, so `core` does not add IDs from future official updates.

Highest IDs in `core` today, taken from `db/re` and `db/pre-re`:

| Kind | Highest `core` ID | Lowest free ID | Mod range starts at |
|---|---|---|---|
| Items | 1,270,141 | 1,270,142 | 2,000,000 |
| Monsters | 22,668 | 22,669 | 100,000 |
| Skills | 10,019 | 10,020 | 20,000 |
| Jobs | 4,350 (`JOB_SECOND_JOB_END`) | 4,351 | 10,000 |
| Quests | 62,238 | 62,239 | 1,000,000 |
| Achievements | 240,020 | 240,021 | 1,000,000 |

- The "lowest free ID" column is the minimum that avoids conflicts. The mod ranges start higher, so fixes to `core` data have room.
- IDs are stored as 32-bit unsigned integers, so every range has room for millions of entries.
- Maps are identified by namespaced names, not numbers. Numeric map indexes exist only in memory.

Assignment:

- Mods name new records with namespaced strings, such as `example.halloween-event:Pumpkin_Hat`.
- The server assigns the next free ID in the mod range on first load.
- Assigned IDs are saved in `ids.lock` on each server. The server never reuses an ID.
- If a mod is removed, its items stay in the database, hidden from players. They return if the mod is installed again.

### 6.6 Sandbox and permissions

- Each mod runs in its own Lua environment table inside the zone's Lua state. Mods cannot read or change each other's globals.
- Shared tables such as `wa` and the constants are read-only.
- The standard libraries `io`, `os`, `debug`, `package`, and bytecode loading are removed.
- Each call into a mod has CPU and memory limits. A call that exceeds them is stopped and logged.
- If a mod's handler fails too often, it is disabled in that zone and the server owner is alerted. A failed dialog closes cleanly for the player.

Permissions:

| Permission | Default | Allows |
|---|---|---|
| `game.read` | Granted | Read game state. |
| `npc.create`, `event.listen` | Granted | Add NPCs and listen to events. |
| `player.give_items`, `player.give_zeny`, `player.give_exp` | Needs approval | Change a player's economy state. |
| `player.warp`, `player.status` | Needs approval | Move players and apply status effects. |
| `storage.mod` | Granted | Read and write the mod's own saved data. |
| `events.schedule` | Needs approval | Add world events to the scheduler. |
| `rules.override` | Needs approval, flagged | Replace formulas such as damage or drop rates. |
| `admin.commands` | Needs approval, flagged | Add GM commands. |
| `net.http` | Needs approval, flagged | Call external HTTP services, limited to hosts listed in the manifest. |
| `items.modify_rolls` | Needs approval, flagged | Reroll or change the rolled values of existing items (section 5.4). |

There is no permission for raw SQL, the file system, or other mods' data.

### 6.7 Mod data storage

- `wa.store` gives each mod key-value and table storage under its own namespace. Values are stored as PostgreSQL `JSONB` rows keyed by mod ID.
- Player and account variables set by a mod are stored under that mod's namespace.
- Each mod has a storage size limit set by the server owner.
- A mod can ship migration scripts that run when its version changes.
- Uninstalling a mod keeps its data unless the owner runs `wactl mod purge`.

### 6.8 Extending the game

Mods change behavior through events and hooks, not by replacing internal functions.

- **Events** notify mods about something that happened, such as `mob_kill`, `level_up`, or `item_use`. Many mods can listen.
- **Hooks** let a mod change a result, such as the drop rate or the damage of one skill. Hooks run in a chain ordered by priority. Each hook calls `next()` to get the result of the rest of the chain.

```lua
wa.hooks.drop_rate(function(ctx, next)
  local rate = next(ctx)
  if ctx.map.flags.event_zone then
    return rate * 2
  end
  return rate
end)
```

- **Exports** let one mod offer functions to other mods. A mod reads them with `wa.mods.get("example.seasonal-lib").api`. The dependency must be listed in `depends`.

### 6.9 Tooling

The `wactl` tool covers the mod workflow.

| Command | Purpose |
|---|---|
| `wactl mod new <id>` | Create a mod from a template. |
| `wactl mod validate` | Check the manifest, data schemas, IDs, ruleset support, and permissions. |
| `wactl mod test` | Run the mod's tests in a headless server with fake players and a fixed random seed. |
| `wactl dev` | Start a local server that reloads the mod when files change. |
| `wactl mod pack` | Build a versioned `.zip` with checksums. |
| `wactl mod install <source>` | Install from a Git URL, a `.zip` file, or a mod index (section 6.10). |
| `wactl convert <file>` | Convert rAthena scripts into a mod (section 6.11). |

Editor support:

- The binding layer generates type definitions for the Lua Language Server (`---@class` annotations). Editors then show autocomplete and type warnings for the whole `wa` API.
- The API reference website is generated from the same definitions, so it always matches the code.

Test example:

```lua
local t = wa.test

t.case("pumpkin king gives a hat once", function()
  local pc = t.player { map = "prontera", x = 150, y = 150 }
  t.talk(pc, "Pumpkin King", { 1 })
  t.expect(pc:count_item("example.halloween-event:Pumpkin_Hat")).to_be(1)
  t.talk(pc, "Pumpkin King", { 1 })
  t.expect(pc:count_item("example.halloween-event:Pumpkin_Hat")).to_be(1)
end)
```

### 6.10 Distribution

A mod index is a catalog of published mods. Each entry lists the mod ID, versions, download URL, checksum, supported API range, and requested permissions. `wactl` reads the index to find and install mods by name. The index does not host the mods themselves.

Plan:

1. **At first there is no central index.** Mods are installed from a Git URL or a `.zip` file. Server owners review the manifest and permissions before installing.
2. **Community indexes.** The community creates and moderates indexes. The core team does not run one. The core team provides:
   - The index file format and its JSON Schema.
   - `wactl` support for adding an index by URL and installing from it.
   - A template repository with CI that runs `wactl mod validate` and `wactl mod test` on each pull request and shows the requested permissions to reviewers.
3. Server owners choose which indexes they trust. Several indexes can be added at once.

For all sources:

- Releases are signed. `wactl` checks signatures and checksums before installing.
- Approval by an index does not replace the server owner's review of permissions.

### 6.11 Migrating existing community scripts

The rAthena community has many custom scripts on forums and in private repositories.

- `wactl convert` turns rAthena script files into a mod folder with a generated manifest.
- The converter reports commands it could not map and the permissions the result needs.
- The converter guide lists common manual fixes.

### 6.12 API stability

- The public API has a version number. The manifest's `api` field declares which versions a mod supports.
- Stable API follows semantic versioning. Breaking changes only happen in a new major version.
- New API starts under `wa.experimental`. It can change in any release.
- Deprecated functions log a warning for at least one minor release before removal.
- Every release has a changelog entry for API changes.

### 6.13 Localization

- Mods store player-facing text in `locales/<language>.yml`.
- Scripts use `wa.t("key")`. The server picks the player's language, with English as the fallback.
- The converter moves literal strings from `mes` commands into the English locale file.

### 6.14 Mod assets

- Mods can ship original sprites, textures, models, and sounds in `assets/`.
- The web client downloads them from the server when needed and caches them.
- Mods must only ship assets they have the right to share.
- Content from mods that reuses existing sprites refers to them by name. Those files come from each player's own game files (section 10.1).

## 7. Security

### 7.1 Threat model

| Attacker | Goal | Main entry points |
|---|---|---|
| Cheating player | Items, zeny, speed, or information they should not have. | Modified web client, bots, injected messages, replayed messages, predicting random outcomes. |
| Account attacker | Take over accounts to steal items. | Password reuse from other breaches, brute force, phishing, stolen session tokens. |
| Malicious mod author | Steal data, add backdoors, or break the economy. | Published mods, hooks, permissions. |
| Malicious client fork | Steal passwords or sessions from players who use it. | Forked web clients hosted by third parties. |
| Rogue staff member | Give items or read private data. | GM commands, web panel, database access. |
| Network attacker | Take servers offline. | Connection floods, large messages. |
| Web attacker | Run code in players' browsers or act as them. | Chat messages, names, guild emblems, cross-site requests. |

### 7.2 Fixes for current rAthena problems

| Problem in rAthena | Fix in the new server |
|---|---|
| Plain-text or unsalted MD5 passwords. | Argon2id hashes with a per-account salt. |
| Default `s1` / `p1` server account. | No shared passwords between services. Services use mutual TLS with certificates from a private certificate authority. There is no game account for services. |
| SQL built with string formatting. | Only `sqlc`-generated queries with bound parameters. A CI check rejects `fmt.Sprintf` next to query calls. |
| `query_sql` script command. | Not provided. Mods use `wa.store` (section 6.7). The converter reports `query_sql` uses for manual rewrite. |
| No protocol encryption. | All traffic uses TLS. The web client is the only client, so no unencrypted game traffic remains. |
| Unchecked packet parsing. | Go bounds checks on every slice access. Generated decoders check lengths before reading. Fuzz tests for every decoder. |

### 7.3 Accounts and authentication

- Players sign up with a username and password. Passwords are hashed with Argon2id. Parameters are set so one hash takes about 50 ms on production hardware.
- Players can turn on TOTP codes and passkeys (WebAuthn) in their account settings. Both are optional for players.
- Staff accounts with GM permissions must turn on at least one of them before GM commands work.
- Login attempts are rate limited per account and per IP address. After repeated failures, the login form asks for a captcha.
- Session tokens are random 256-bit values. The database stores only a hash of each token. Tokens expire and are bound to the account.
- Changing the password or email revokes all sessions and sends a notice to the old email.
- Email changes need confirmation from the old address.

### 7.4 Network and protocol

- The gateway sets a maximum message size, a per-connection message rate, and a per-message-type rate. Clients that exceed them are disconnected.
- Connections per IP address are limited, which replaces rAthena's `ddos_*` settings in `conf/packet_athena.conf`.
- Decoders never panic on bad input. A decode error closes that one connection. Each connection goroutine also recovers from panics, so one bad client cannot stop the server.
- The gateway checks the WebSocket `Origin` header. Server owners list the client domains they allow, including community forks they trust.
- The game endpoint sits behind a load balancer or proxy that can absorb connection floods. Internal service ports are not public.

### 7.5 Game integrity

- The request validation layer (section 4.1, item 9) checks every client command against server state before it runs.
- Movement is checked against walk speed and path. A player who moves faster than allowed is moved back.
- Attack and skill timing is checked against server-side cooldowns, cast times, and animation delays. The server ignores the client's timing.
- The server sends only what the player can see. Hidden players, mob positions out of range, and secret loot tables are never sent.
- Zeny and item amounts use checked math. An operation that would overflow fails and is logged.
- Every item instance has a serial number. Logs record each move by serial. A job flags serial numbers that appear in two places.
- Gameplay randomness uses a secret stream per zone (section 8.3). Players cannot predict drops or critical hits from what they see.
- Behavior signals for bots, such as exact repeat timing and long sessions with no chat, are logged for staff review. The server does not ban automatically on these signals.

### 7.6 Web client and web services

- Chat, player names, guild names, and NPC text are shown with `textContent`, never `innerHTML`.
- A strict Content Security Policy only allows scripts from the web client's own origin.
- Cookies are `HttpOnly`, `Secure`, and `SameSite=Strict`. Requests that change state need a CSRF token.
- Login happens on the account service's own page. A forked client receives a session token, never the password. Passkeys are bound to the account service's domain, so a fork on another domain cannot use them.
- Guild emblem uploads are size limited, decoded, checked, and re-encoded before storage. The original file is never served.
- The control panel API uses scoped tokens, such as read-only rankings or account self-service. Each token only allows its scope.

### 7.7 Mods

Section 6.6 covers the sandbox and permissions. Additional Lua rules:

- `load` and `loadstring` are removed for mods. Mods cannot run code built from strings.
- Shared tables are read-only proxies. Their metatables are locked with `__metatable`.
- Memory limits stop large string builds such as `string.rep` with a huge count. CPU limits stop slow pattern matches.
- Mod errors that reach players show a generic message. Stack traces go only to server logs.
- `net.http` calls only go to hosts listed in the manifest, never to private IP ranges. This blocks mods from reaching internal services.

### 7.8 Staff and operations

- GM permissions are role-based and set per command. Every GM command is written to an audit log with the account, target, and arguments.
- Commands that create items or zeny need a reason field, and their audit log entries can be reviewed.
- Each service uses its own PostgreSQL role with only the tables and rights it needs. The game services cannot drop tables.
- Secrets such as database passwords and certificate keys come from environment variables or a secret manager, never from files in the repository.
- Backups are encrypted, and restore tests run on a schedule.
- Logs never contain passwords, tokens, or full session IDs.

### 7.9 Secure development

- CI runs `govulncheck`, `gosec`, `golangci-lint`, tests with the race detector, and short fuzz runs on every pull request.
- Longer fuzz runs on message decoders, YAML loaders, and the mod loader run every night.
- Go modules are pinned by `go.sum`. Web client packages are pinned by the lock file. Dependency updates go through review. Releases include a software bill of materials.
- Release binaries are signed.
- `SECURITY.md` explains how to report vulnerabilities privately, and states the target time for a first response.
- An external security review happens before the public beta.

### 7.10 Player safety and privacy

Moderation:

- **Reports.** Players can report a player, a chat message, a guild name, or a guild emblem from the client. Each report includes the last 5 minutes of chat the reporter could see. Reports go to the staff queue in the admin tools (section 10.6).
- **Blocking.** Players can block another player. Blocked players cannot whisper, trade, or invite them, and their chat is hidden.
- **Chat filter.** A server-side word filter checks chat, names, and guild names. Server owners edit the word lists. The filter can replace words or reject the message.
- **Chat limits.** New characters have lower chat rate limits for their first hour. This slows spam bots.
- **Sanctions.** Mute, jail, and ban have durations and reasons. The player sees the reason and end time.
- **Name checks.** Character and guild names pass the chat filter and a list of reserved names, such as staff titles.

Privacy:

| Data | Kept for | Notes |
|---|---|---|
| Account email | Life of the account | Used for login recovery and notices only. |
| IP addresses on login records | 90 days | Needed for ban evasion checks. Then deleted. |
| Chat logs | 30 days | Kept longer only when attached to a report, until the report is closed. |
| Item and zeny logs | 180 days | Needed to trace duplication and theft. |
| GM audit logs | 2 years | Needed to review staff actions. |

Server owners can shorten these periods. Lengthening them is a setting with a warning in the admin tools.

Player rights:

- **Data export.** Players can download their account data (account, characters, inventories, and their own chat logs still kept) as a JSON file from the account page.
- **Account deletion.** Players can delete their account. After a 14-day waiting period, personal data is deleted. Logs that other players need, such as trade records, keep the character ID but drop the account email and IP addresses.
- **Privacy notice.** The account page shows what is stored and for how long. Server owners fill in their own contact details.

The project provides these tools. Each server owner is responsible for following the laws that apply to them, such as the GDPR for players in the EU.

## 8. World modes and procedural generation

### 8.1 World modes

Each world runs in one of two modes, set when the world is created.

| Mode | Maps | Content |
|---|---|---|
| `classic` | rAthena's fixed maps, loaded from each player's own game files. | `core` NPCs, quests, and spawns at fixed places. |
| `procedural` | Fields, dungeons, and towns generated from the world seed. Pre-made maps from mods, such as a home area, are placed into the generated world. | NPCs, quests, and spawns placed by generators and templates. |

The web client can generate or download new terrain at any time, so both modes use the same client.

Procedural mode is included with the Go build tag `procedural`.

- Builds with the tag contain the generator, `wa.gen`, and rotating dungeons. New worlds default to `procedural`.
- Builds without the tag contain only `classic` mode. New worlds default to `classic`.
- The mode is still chosen per world at creation time, in builds that include both.
- The server tells the web client which modes it supports at login. The client loads the generator WebAssembly module only for procedural worlds.
- CI builds and tests both variants on every pull request, so code without the tag cannot break.

This lets a release ship in either order. Classic mode can launch while procedural mode is still in development.

### 8.2 Seeded random number tree

All generation uses a tree of seeds. Each node's seed comes from its parent's seed and a label, never from a shared random stream.

```text
world (root seed, secret)
├── region(3,-1)
│   ├── terrain        public
│   ├── layout         public
│   ├── decoration     public
│   └── secret
│       ├── spawns
│       ├── loot
│       └── puzzles
├── town("t7")
│   ├── layout         public
│   └── secret
│       ├── npcs
│       └── quests
└── dungeon("crypt", 2026-10-31)
    ├── floor(1)
    │   ├── layout     public
    │   └── secret
    └── floor(2)
```

```go
// Node is one point in the seed tree. Its key is 32 bytes.
type Node struct{ key [32]byte }

// NewRoot derives the root node from the world secret.
func NewRoot(secret []byte) (Node, error)

// Child derives a child node with HMAC-SHA256. A child key does not reveal
// its parent key or its siblings' keys.
func (n Node) Child(label string, idx ...int64) Node

// Stream returns a fast deterministic xoshiro256** stream for public generation.
func (n Node) Stream() *Rand

// SecretStream returns a ChaCha8 stream for secret branches. Server only.
func (n Node) SecretStream() *Rand

// Hash returns a stateless hash for noise and per-cell choices.
func (n Node) Hash(x, y, z int64) uint64
```

Properties:

- **Order does not matter.** Generating region (3,-1) before or after region (0,0) gives the same result. Generation can run lazily and in parallel.
- **Any part can be regenerated alone.** Changing the decoration generator does not change terrain or loot.
- **Time-based content** uses the date as an index, such as `dungeon("crypt", date)`. The same dungeon appears for everyone that day.
- **Specification.** The algorithms below are written down in [docs/specs/rng-v1.md](specs/rng-v1.md), with test vectors. Go and TinyGo builds must pass the same vectors.

#### Algorithms

| Use | Algorithm | Why |
|---|---|---|
| Deriving child nodes | HMAC-SHA256. The key is the parent's 32-byte key. The message is `"wa-seed-v1"`, then the label with a length prefix, then each index as a big-endian `int64`. | One-way, so public seeds do not reveal secret branches. In the Go standard library and supported by TinyGo. Speed does not matter, because derivation happens a few thousand times per world. The length prefix stops `("ab", "c")` and `("a", "bc")` from giving the same message. |
| Public generation streams | xoshiro256\*\*, seeded with the node's 32-byte key. An all-zero key is replaced by SplitMix64 output, because xoshiro cannot use a zero state. | Fast in Go and WebAssembly. About 15 lines of code in our own package, so it does not depend on standard library internals that TinyGo may not support. Passes the BigCrush statistical tests. |
| Secret streams | ChaCha8 from Go's `math/rand/v2`, seeded with the node's key. | Players cannot recover the state from outcomes they see. The output is fixed by the C2SP `chacha8rand` spec. Runs on the server only. |
| Stateless per-cell hashing | The key folded to 64 bits, then the SplitMix64 finalizer applied once per coordinate: `h = mix(h XOR x * P1)`, then `y * P2`, then `z * P3`, with three fixed odd 64-bit constants. | Noise and per-cell choices need a value for a coordinate, not the next value in a stream. Any cell can be computed alone, in any order, on any thread. |
| Gameplay randomness | ChaCha8, one stream per zone, seeded from `crypto/rand` at startup. | Not part of the tree (section 8.3). |

Helpers, all in the spec with test vectors:

- **Bounded integers** use Lemire's multiply-and-reject method. The spec fixes the rejection rule, because each rejection uses one more value from the stream.
- **Shuffles** use Fisher-Yates from the last element down.
- **Weighted choices** use integer weights and a cumulative table. The alias method is not used, because building its table in a different order changes results.
- **No floating point.** Fractions use Q16.16 fixed-point integers (16 integer bits and 16 fraction bits).

Why not other options:

- `math/rand` (version 1) does not document its algorithm, so it could change between Go releases.
- The PCG generator in `math/rand/v2` is well specified, but TinyGo support is not guaranteed. Our own xoshiro implementation removes that risk.
- Using ChaCha8 for everything would work, but it is slower in WebAssembly without SIMD, and public streams need no secrecy.

The spec has a version (`rng-v1`). Each world records it. An algorithm never changes within a version.

### 8.3 Public and secret branches

- **Public branches** (terrain, layout, decoration) are sent to the web client as a seed, so the client can generate the map itself.
- **Secret branches** (spawns, loot, NPC placement details, quest answers) stay on the server. They use ChaCha8 streams, so players cannot recover the stream state from outcomes they observe.
- The root world seed is secret. The keyed hash is one-way. Knowing one map's public seed does not reveal another map's seed or any secret branch.
- Combat and drop randomness during play does not come from the tree. Each zone uses a ChaCha8 stream seeded from `crypto/rand` at startup. In test mode, the seed is fixed and logged so runs can be replayed.

### 8.4 Generator rules

- The core generator is written in Go and compiled to WebAssembly with TinyGo for the web client (section 3.3).
- **Integer and fixed-point math only.** The Go compiler may fuse `x*y + z` into one FMA instruction on arm64 but not on amd64. Floating point can then give different results on different machines. Fixed-point math gives the same result everywhere.
- **No iteration over Go maps.** Go map order is random. Use sorted slices.
- **Versioned generators.** Each world records the generator version it used. A changed generator gets a new version number. Old versions stay in the code until no world uses them.
- The server sends a checksum of the generated cell grid. If the web client's result does not match, it downloads the map data from the server instead.

### 8.5 Terrain generation

#### Output

Each generated map produces the same data a classic map has, in the web client's own format:

- **Cell grid.** One entry per cell, with a type and 4 corner heights. Types follow RO's `.gat` values, which rAthena reads in `map_gat2cell` ([src/map/map.cpp:3280](../src/map/map.cpp#L3280)): 0 walkable, 1 blocked, 3 walkable water, and 5 gap (blocks walking but not ranged attacks).
- **Material map.** A ground material per cell, such as grass, dirt, sand, rock, or snow, with blend weights for smooth edges.
- **Water level** for the map.
- **Object list.** Trees, rocks, buildings, and other models, with position, rotation, and scale.
- **Portals, named places, and feature tags** (water, ruins, clearings) for warps (section 8.10) and spawns (section 8.11).
- **Navigation data** for the client's navigation window.
- **Checksum** of the cell grid (section 8.4).

Material and object names are abstract, such as `grass` or `pine_tree`. An asset set maps them to files. The asset set can point to textures and models in the player's own game files, or to a mod's own art.

#### World scale

1. **Region grid.** The map graph (section 8.10) places fields on a grid of regions.
2. **Climate.** Two low-frequency noise fields over region coordinates give temperature and moisture. A biome table maps the pair to a biome. Hot and dry is desert, cold is snow, wet and warm is swamp or jungle, and middle values are grassland or forest.
3. **Level bands** come from graph distance to the home area.
4. **Global coordinates.** Every field samples noise in world coordinates, not local ones. The terrain on two sides of a shared border then continues smoothly, even though they are separate maps.

#### Field steps

Each step uses its own child node, such as `field(3,-1)` then `terrain`, `water`, or `objects`. Changing one step does not change the others.

1. **Height.** Fractal gradient noise in fixed-point, with 4 to 6 octaves. Domain warping, which offsets the sample position by another noise field, makes shapes look less regular. Each biome sets its own values:

   | Biome | Shape |
   |---|---|
   | Grassland | Low amplitude, gentle hills. |
   | Forest | Medium amplitude, rolling hills. |
   | Desert | Ridged noise for dunes, plus flat basins. |
   | Snow | High amplitude, steep slopes. |
   | Swamp | Very flat, with the water level just below the average height. |
   | Beach | A slope from land down into water along one edge. |

2. **Water.** Cells below the water level become walkable water (type 3) when shallow and blocked (type 1) when deep. Rivers start at high points and follow the steepest descent to a lake, the sea, or the map edge. They are carved into the height map as they go.
3. **Cliffs.** A cell whose height differs from a neighbor by more than the biome's slope limit becomes a gap (type 5) or blocked (type 1).
4. **Portal areas.** The map graph fixes where portals sit on each border. The generator flattens a circle around each portal and marks it walkable.
5. **Connectivity.**
   - A flood fill from each portal finds which walkable areas it reaches.
   - If two portals are not connected, the generator carves a road between them with A\* search over a cost map that prefers gentle slopes, then flattens the road.
   - Walkable pockets smaller than 200 cells that no portal reaches become blocked, so no monster spawns where players cannot go.
6. **Places.** Clearings, ruins, camps, and similar spots are placed with Poisson-disk sampling (Bridson's algorithm), which keeps them spread out. They become feature tags and named places.
7. **Objects.** Trees, rocks, and plants are placed with Poisson-disk sampling. Density comes from a biome density map. Each object blocks the cells under its footprint. Roads and portal areas are protected, so objects never break connectivity.
8. **Materials.** Each cell's material comes from biome, height, slope, and moisture. For example, steep cells get rock, cells near water get sand or mud, and high snow biome cells get snow.
9. **Lighting and fog** come from the biome.

#### Validation and retries

Every map is checked after generation:

- All portals are connected.
- The walkable share is within the biome's range, for example 55% to 85% for grassland.
- Each spawn area has enough walkable cells.
- Named places required by the map graph exist.

A map that fails is generated again from the child node `attempt(n)`. Retries are deterministic. After 8 failed attempts, the generator uses a fallback template for that biome and logs the seed for investigation.

#### Towns

Towns use a separate generator.

1. Flatten a plateau at the town's location.
2. Build a road network. Main roads run from each town gate to a central square. Side roads branch from them.
3. Split the land between roads into building plots.
4. Place buildings from town templates. Each template lists building models and their footprints, and the named places inside them, such as `blacksmith`, `inn`, and `storage`.
5. Place walls, gates, decorations, and service NPC spots.

#### Dungeons

| Style | Method |
|---|---|
| Caves | Cellular automata. Start with about 45% of cells blocked at random. Apply 5 rounds of the rule "a cell becomes blocked if 5 or more of its 8 neighbors are blocked". Keep the largest open area and connect the rest with tunnels. |
| Crypts, towers, and ruins | Place rooms without overlap. Connect them with a minimum spanning tree, then add a few extra corridors so the layout has loops. The exit to the next floor goes in the room farthest from the entrance. The boss room is at the end of the last floor. |

#### Performance

- A 400 × 400 cell map has 160,000 cells. With 6 octaves of noise, that is about 1 million noise samples, which takes tens of milliseconds in Go.
- WebAssembly is expected to be 3 to 5 times slower. Generation still finishes in about a second, and the client runs it in a Web Worker.
- Generated maps are cached on disk on the server and in the Origin Private File System in the browser.
- Rows can be generated in parallel, because every cell's noise is computed from its coordinates. Each thread writes to fixed positions, so the result does not depend on thread timing.

#### Golden tests

The repository keeps a list of seeds with the expected checksum of every map they generate. CI generates them with the Go build and the TinyGo build. Any change to a checksum fails CI unless the generator version is raised.

### 8.6 Procedural mode content

Every part of the world is generated in procedural mode.

- **Fields.** Terrain from biome rules. Monster spawns come from biome spawn tables, scaled by the region's level band.
- **Dungeons.** Room and corridor layouts or caves. Floors get harder with depth. Bosses spawn in generated boss rooms.
- **Towns.** Layout, buildings, and roads come from town templates. Service NPCs (storage, shops, inns, job changes) are placed at named places in each town.
- **Named places.** Content refers to places by tag, not coordinates. A script asks for `wa.place("town:any", "blacksmith")`, and the generator resolves it for the current world.
- **Quests.** Quest templates pick targets, items, and places from the generated world. Examples are hunt quests for monsters in nearby fields and delivery quests between towns.
- **Pre-made maps from mods.** A mod map with the `home` tag becomes the starting area. The generator connects it to the nearest generated town.
- **Classic `core` content.** Scripts that depend on fixed rAthena maps do not run in procedural mode. Their shops and services are reused through templates.

### 8.7 Mods and generation

- Mods can add generation layers through `wa.gen`, such as biomes, town templates, structures, spawn tables, loot rules, and quest templates.
- A mod layer receives a tree node, such as `node:child("chests"):rand()`. Mods have no other source of randomness during generation.
- Lua layers run on the server only. The server sends their results to the web client as object lists. Only the Go terrain generator runs on both sides.

### 8.8 Automatic events

An event scheduler in the world service starts world events without staff action. It works in both world modes.

Triggers:

- **Schedule.** Cron-style times, such as every Saturday at 20:00. This replaces rAthena labels like `OnClock2000:`. Times use the server's time zone, set as an IANA name such as `Asia/Manila` in the server config. The server stores all timestamps in UTC and converts only when matching schedules and showing times. On days when clocks change, a time that does not exist runs at the next valid minute, and a time that happens twice runs once. The client shows event times in server time, with the player's local time next to it.
- **Conditions.** Game state, such as a town having no visitors for an hour, or players killing a number of monsters in a region.
- **Random.** A chance per hour, drawn from a secret stream.

Built-in event types:

| Event | What happens | Needs procedural mode |
|---|---|---|
| Boss spawn | A boss appears at a random place in a level band, with a world announcement. | No |
| Invasion | Monster waves attack a town until players defeat them. | No |
| Treasure hunt | Chests spawn in random places, with hints from NPCs. | No |
| Guild siege | Castle war on a schedule, like War of Emperium. | No |
| Rotating dungeon | A new dungeon is generated each day or week, from `dungeon(name, date)`. | Yes |
| World change | A region is regenerated with a new biome or level band at the end of a season. | Yes |

Rules:

- Each event is defined in YAML with a trigger, a duration, rewards, and an optional Lua script.
- Mods add event types with the `events.schedule` permission.
- The next rotation is generated before it starts, so players do not wait.
- Rotating dungeons launch together with procedural mode. The seed tree makes them cheap once the generator exists.

### 8.9 Storage

- The database stores the root seed, the generator version, and per-map state such as opened chests and cleared rooms.
- Generated map data is cached on disk. It can be deleted and rebuilt from the seed at any time.
- A zone generates its map in a background goroutine before it starts.
- Town names and the map graph are also stored, so a new generator version never renames a town or moves a warp (section 8.10).

### 8.10 Names, map identity, and warps

#### Map identity

Every map has a stable ID and a display name.

- The ID is used by the server, the database, scripts, save points, and memo points. It never changes for a world.
- The display name is shown to players. It can change without breaking anything.
- Classic maps keep rAthena's map names as IDs, such as `prontera`. Display names come from locale files.
- Generated maps get IDs from their place in the seed tree, such as `gen:town:7`, `gen:field:3:-1`, and `gen:dungeon:crypt:2026-10-31:2`.

#### Town names

Town names come from the `name` child of the public tree node `town(n)`, so every server with the same seed and generator version gets the same names.

A naming style defines how names are built. Each biome or culture uses one style, for example a Norse-like style for temperate lands, a desert style, and an eastern style. Mods add styles as data:

```yaml
NamingStyles:
  - Name: norse
    Syllables:
      Start: [Val, Hrim, Skar, Eld, Grim, Thor]
      Middle: [a, e, i, un, ar]
      End: [heim, gard, vik, stad, holm]
    Patterns:
      - "{Start}{End}"
      - "{Start}{Middle}{End}"
```

Rules:

- Names are unique within a world. They are 4 to 12 letters long.
- Every name passes a blocked-word filter.
- A rejected name is retried with the next index, `name(1)`, `name(2)`, and so on. Retries are deterministic, so the result is the same on every run.
- Names are stored in the database at world creation.
- Staff can rename a town with a GM command. This changes the display name only.
- Other places take their names from the nearest town and locale templates, such as "North Valheim Fields 2" or "Crypt of Skarvik". Town names are proper nouns and stay the same in every language. Template words such as "Fields" and "Crypt" come from locale files.

#### Map graph and warps

At world creation, the layout step builds a graph. Each node is a map, and each edge is a pair of linked portals.

- Fields sit on a grid of regions. Neighboring fields connect through portals on their shared border, like RO's field maps.
- Each town connects to 2 to 4 fields.
- Dungeon entrances are placed in fields. Dungeon floors link in order.
- Every map must be reachable from the home area. The generator rejects and retries layouts that are not.
- Level bands rise with graph distance from the home area.
- Portals sit on walkable cells on both sides. Arrival places the player on a walkable cell next to the linked portal, not on it, so players do not bounce between maps.

Warp services use the graph:

- Teleport NPCs, like RO's Kafra teleport service, list the towns the player has visited. The price rises with graph distance.
- Save points store a map ID and a cell.
- Warp Portal memo points store map IDs. Maps that can be removed, such as rotating dungeons, have the `nomemo` flag.
- The `@go` command lists towns by display name. `@warp` accepts a map ID or a display name.
- Scripts find places with `wa.towns()`, `wa.town_nearest(pc)`, `town.name`, and `wa.place(...)` (section 8.6).
- The client's world map is drawn from the parts of the graph the player has discovered.

When a map is removed or regenerated, for example by a world change event or when a rotating dungeon expires:

1. Players on the map get a countdown warning.
2. Players still on the map are moved to the nearest town.
3. Save points and memo points on the map move to the nearest town.
4. Portals that led to the map are updated in the same step.

In classic mode, warps come from `core` scripts, as they do in rAthena today (`npc/*/warps`).

### 8.11 Field spawns

In classic mode, spawns come from `core` scripts, as in rAthena (`npc/re/mobs/fields/*.txt`). The line `prt_fild00,0,0 monster Roda Frog 1012,169,5000` (tab-separated in the file) places 169 Roda Frogs anywhere on the map with a 5 second respawn.

In procedural mode, each field builds its spawn list from four inputs:

- **Level band.** Set by graph distance from the home area (section 8.10).
- **Biome.** Set by the terrain generator, such as grassland, forest, desert, snow, swamp, beach, or volcanic.
- **Map size.** The number of walkable cells.
- **Features.** Water, ruins, caves, and other places the generator marks on the map.

#### Reference numbers from rAthena

The 150 Renewal field maps in `npc/re/mobs/fields` average 199 monsters and 6.7 spawn lines per map. Most normal monsters respawn after 5 seconds. Plants and rare monsters use timers of 3 to 30 minutes. Phase 0a computes exact density per walkable cell from these files, and the generator uses those values as its defaults.

#### Monster spawn tags

Every monster has spawn tags. They are built in three ways:

1. **Derived from classic spawns.** Each rAthena map family maps to a biome, for example `prt_fild` to grassland and `moc_fild` to desert. A monster that spawns on `moc_fild` maps gets the desert tag with a weight based on how often it appears there.
2. **Derived from `mob_db`.** Level, race, element, size, and boss class come from the monster's data.
3. **Set by data.** `core` and mods can add or override tags:

```yaml
- AegisName: PORING
  Spawn:
    Biomes: { grassland: 10, forest: 6 }
    Role: common
    Themes: [cute]
```

Roles:

| Role | Share of monsters on a field | Typical respawn |
|---|---|---|
| `common` | 60 to 70% | 5 seconds |
| `uncommon` | 25 to 35% | 5 to 60 seconds |
| `plant` | Small groups near features | 3 to 30 minutes |
| `rare` | 1 to 5 monsters | 30 to 60 minutes |
| `mini_boss` | 1 on some fields, including fields next to towns | 30 to 60 minutes |
| `field_boss` | 1 on some fields away from towns | 1 to 2 hours |

Boss monsters from `mob_db` are never picked as `common` or `uncommon`.

#### Selection steps

The generator runs these steps with the field's secret spawn node (section 8.3), so players cannot predict rare spawns from public seeds.

1. **Filter candidates.** Keep monsters that match the biome, fit the level band, exist in the world's ruleset data, and are allowed by the map's flags.
2. **Pick a theme (optional).** Some fields get a theme, such as undead ruins or insect swarm. The theme raises the weight of monsters with matching tags.
3. **Pick species.** Draw 5 to 8 species by weighted sampling. The weight combines biome affinity, closeness to the band's level, and theme. Constraints:
   - At least 2 `common` species.
   - At most 3 species of the same element.
   - At most 2 aggressive species in bands below level 30.
   - At most 1 `rare` species.
4. **Set counts.** The total is density times walkable cells. Roles share the total as in the table above.
5. **Place spawns.**
   - Most spawns cover the whole map, like rAthena's `0,0` spawns.
   - Plants and some themed monsters spawn in areas near matching features, such as water.
   - No aggressive monster spawns within 15 cells of a portal arrival point, so players are not attacked on arrival.
6. **Add bosses.**
   - **Fields next to towns** (one warp away from a town in the map graph) can get a `mini_boss`. The boss is drawn from boss monsters no more than 20 levels above the band's highest level. rAthena does the same today. Eclipse (level 31) spawns on `prt_fild02` next to Prontera, and Toad (level 27) spawns on `cmd_fild03` near Comodo. Both respawn after 30 to 60 minutes.
   - **Other fields.** In each region, 1 or 2 fields get a `field_boss`. The boss is drawn from boss monsters at or above the band's level.
   - **Placement.** A boss never spawns within 30 cells of a portal that leads to a town, so new players leaving town do not walk into it.
   - **Settings.** `spawns.town_field_boss_chance` sets the chance that a field next to a town gets a `mini_boss` (default 50%). `spawns.town_field_boss_role` can be set to `field_boss` to allow stronger bosses there. Mods can also place bosses on chosen fields through `wa.gen` layers.

Example result:

```yaml
field: gen:field:3:-1
band: [20, 31]
biome: forest
theme: insect_swarm
spawns:
  - { mob: STAINER, count: 70, respawn: 5s }        # level 21
  - { mob: CREAMY, count: 55, respawn: 5s }         # level 23
  - { mob: POISON_SPORE, count: 35, respawn: 5s }   # level 26
  - { mob: DENIRO, count: 20, respawn: 30s }        # level 31
  - { mob: SHINING_PLANT, count: 1, respawn: 30m, area: near_water }
  - { mob: GOLDEN_BUG, count: 1, respawn: 2h, role: field_boss }  # level 65 boss
```

#### Rules

- **Stored at world creation.** Spawn lists are saved in the database with the map. A new generator version does not change the spawns of an existing world.
- **Dungeons use the same steps.** Dungeon biomes, such as crypt, cave, and tower, have their own tags. Floor depth raises the level band.
- **Level coverage.** The generator checks that every level band from 1 to the level cap has enough fields. It retries the layout if a band has fewer than 3 fields.
- **Rolled drops.** Monsters in higher bands can give better roll ranges on their drops (section 5.4).
- **Mods.** Mods change spawns with `wa.gen` layers or the `spawn_table` hook. Mods that add monsters give them spawn tags so the generator can pick them.
- **Events.** Invasions and boss events add spawns on top of the stored list (section 8.8). They do not change it.
- **Density scaling (optional).** A server setting can raise or lower counts in a region based on how many players are there. It is off by default.

### 8.12 Map format

All maps use one project format:

- Generated maps.
- Pre-made maps shipped by mods, such as a home area.
- Classic maps, which the web client converts from the player's GRF files on first use.

Mod authors never need to produce RO's `.gat`, `.gnd`, or `.rsw` files.

A map is a folder:

```text
maps/home_village/
  map.yaml        # ID, display name, size, biome, water level, music, map flags
  cells.bin       # cell types and corner heights
  materials.bin   # ground material per cell, with blend weights
  objects.yaml    # models with position, rotation, and scale
  places.yaml     # portals, named places, and feature tags
```

- `cells.bin` and `materials.bin` start with a small header (format version, width, height) and store one fixed-size record per cell. They compress well and load fast.
- Models referenced in `objects.yaml` are glTF 2.0 files from a mod, or RSM model names from the player's GRF.
- Textures are PNG or KTX2 files from a mod, or texture names from the player's GRF.
- The format has a version number. Readers reject versions they do not know.

Tools:

- `wactl map import <gat> <gnd> <rsw>` converts a classic map for testing.
- `wactl map validate` checks connectivity, portals, and named places, the same checks the generator runs (section 8.5).
- `wactl map preview` renders a top-down image of the cell grid and materials.
- A visual map editor is not planned. Mod authors write `places.yaml` by hand or generate maps with `wa.gen` and save the result.

### 8.13 Economy balance

Procedural worlds decide where monsters spawn, what shops sell, and what quests pay. These choices set how fast players level and how much zeny enters the world. Without checks, one generated world can be much easier or richer than another.

Work needed:

1. **Economy model.** List every zeny and item source and every sink.

   | Sources | Sinks |
   |---|---|
   | Selling monster drops to NPCs | NPC shop purchases (potions, arrows, gems) |
   | Zeny from quest rewards | Refining and enchanting |
   | Items from quests and events | Teleport and storage fees |
   | Vending between players (moves zeny, does not create it) | Vending tax, if enabled |
   | | Repair, stylist, and other services |

2. **Prices.** NPC shop prices come from `item_db` `Buy` and `Sell` values, the same as rAthena. Town templates choose what each shop sells, not the price. Server owners can set a price multiplier per shop type.
3. **Target rates.** Each level band has target rates, written as data:
   - Base exp per hour for a solo player of that band, set so leveling time follows the ruleset's exp curve. For example, the target could be about 1 hour per level at level 30.
   - Zeny income per hour, from NPC sales of normal drops.
   - Zeny spent per hour on basic supplies.
4. **Simulator.** `wactl sim economy` estimates each field's exp and zeny per hour from its spawn list, monster exp and drops, respawn times, and a simple player model (kills per minute by band). It runs on a generated world without starting a server.
5. **Generator checks.** After generation, every field must fall within 30% of its band's target rates. Fields outside the range get their spawn counts or level band adjusted, then are checked again.
6. **Bot runs.** Economy bot scenarios (section 12.1) farm fields in each band and compare real results with the simulator.
7. **Live stats.** Dashboards in the admin tools show total zeny in the world, zeny created and removed per day, average exp per hour by band, the most farmed fields, and roll value spread (section 5.4).
8. **Settings.** Server owners tune exp rate, drop rate, zeny rate, shop multipliers, and fees. Rolled item ranges and field boss frequency have their own settings.

Results of the simulator and bot runs are part of the phase 7 exit criteria (section 15).

## 9. Logic shared by the server and the web client

Both sides are ours, so shared logic uses one implementation where possible (section 3.3).

| Logic | Where it runs | How the two sides stay in sync |
|---|---|---|
| Pathfinding and walk timing | Server and client | Same TinyGo package. Test vectors. |
| Cell data and distance checks | Server and client | Same TinyGo package. Test vectors. |
| Map generation | Server and client | Same TinyGo package. Checksum with download fallback. |
| Message formats and ID tables | Server and client | Generated from schema files. |
| Animation timing (attack motion, damage motion, walk delay after a hit) | Server decides. Client displays. | Values come from server data files and are sent to the client. |
| Damage, status, drops, AI, and scripts | Server only | Not shared. |
| Rendering, effects, sound, camera, UI, and item descriptions | Client only | Not shared. |

The server never trusts the client on range, cooldown, cast completion, weight, equipment requirements, trade and vending values, position, or message timing.

## 10. Web client

### 10.1 Assets

Ragnarok Online sprites, maps, textures, and sounds belong to Gravity. The project never hosts them.

- **Player files.** Each player selects their own installed game folder or GRF file. The browser reads it locally and caches converted assets in the Origin Private File System. No game assets pass through the project's servers.
- **Mod assets.** Mods can ship their own original assets (section 6.14).

#### Reading GRF files

GRF files are workable in the browser. roBrowser already reads them. The details to handle:

- **Size.** A full install has several GB of GRF data. The client never loads a whole file. It reads the file table, then reads single entries with `Blob.slice` when needed.
- **Encryption.** Some entries use DES encryption, either on the header only or mixed with a shuffle (`FILELIST_TYPE_ENCRYPT_HEADER` and `FILELIST_TYPE_ENCRYPT_MIXED` in `src/common/grfio.cpp`). The client ports rAthena's decoder.
- **Compression.** Entries use zlib. The browser's `DecompressionStream` handles it in a Web Worker.
- **Filenames.** Names in the file table are Korean text in CP949. `TextDecoder("euc-kr")` decodes them. The client stores a normalized lowercase name for lookups.
- **Versions.** GRF versions 0x200 and 0x300 are supported. Other versions show a clear error.
- **Storage limits.** The client converts and caches only the files it uses, not the whole GRF. Browsers limit storage differently, and Safari has the tightest limits. When the quota is low, the client removes the least recently used converted files.
- **Several files.** Players can add several GRF files and a `data` folder. Later entries override earlier ones, like the official client's `DATA.INI` order.

If GRF reading turns out unreliable on some browsers, classic world mode is the part that depends on it most. It can be dropped, and the rest of the game still needs only sprites, textures, models, and sounds.

### 10.2 Prior art

roBrowser and roBrowserLegacy are existing JavaScript RO clients using WebGL. They already parse GRF, GND, RSW, RSM, SPR, ACT, PAL, and STR files. Review their format loaders before writing new ones. Check their licenses before reusing code.

Server-side prior art:

- **goAthena** (`github.com/bouroo/goAthena`) is a Go rewrite of rAthena game logic, licensed GPL-3.0 and started in July 2026. It targets the official Thai client, the rAthena database schema, and the rAthena script language, which wAthena does not use. Its `pkg/ro` loaders for rAthena YAML data and its stat and damage packages could be reused under the GPL. Any reused package must pass the phase 0a reference data first. No further review is planned.
- **pAthena** (`github.com/PallasSoftware/pAthena`) is a C# emulator inspired by rAthena.

### 10.3 Client architecture

| Area | Choice |
|---|---|
| Language | TypeScript, strict mode. |
| Build | Vite. |
| Rendering | WebGL2 through three.js first. Move to WebGPU when browser support allows. |
| Sprites | Characters and mobs are 2D sprites drawn in a 3D map. Pack sprite frames into texture atlases and draw them in batches. |
| Maps | Convert GND, RSW, and RSM into GPU-ready meshes once, then cache them. Generated maps use the same mesh format. |
| Shared logic | WebAssembly module built from the Go shared package (section 3.3). |
| Asset decoding | Web Workers, so decoding does not block rendering. |
| UI | HTML and CSS overlay on the canvas, using a small framework such as Solid or Svelte. This gives text input, accessibility, and styling for free. |
| Audio | Web Audio API. |
| Network | One WebSocket per session. Binary frames. Codec generated from the shared schema. |
| Auth | Login on the account service's page, which returns a short-lived session token. The token opens the game WebSocket. |

### 10.4 Community changes to the client

- The web client is open source. Server owners and the community fork it and change its UI, themes, and features.
- The client keeps UI code apart from protocol and game-state code. This keeps forks easy to update from upstream.
- A plugin API comes later. Plugins run in sandboxed iframes and talk to the client through a message API.
- Forks must use the account service's login page (section 7.6). The protocol version check rejects forks that fall too far behind.

### 10.5 Client data tables

The official client keeps many tables inside its executable and its Lua files. The web client needs its own copies. The project ships a minimum set as YAML files in `client/data/`, each with a JSON Schema.

| Table | Content | Source for the minimum set |
|---|---|---|
| `jobs.yml` | Job ID to body sprite name, per gender, plus mount and costume variants. | rAthena job constants and roBrowser's job tables. |
| `weapons.yml` | Weapon type to sprite name, attack sound, and attack motion. | roBrowser's weapon tables. |
| `headgear.yml` | Headgear view ID to accessory sprite name. | Item `View` values in rAthena's `item_db` and roBrowser's accessory table. |
| `monsters.yml` | Monster ID to sprite name, size, and sounds. | `mob_db` `AegisName` values, which match most sprite names. |
| `npcs.yml` | NPC sprite ID to sprite name. | rAthena NPC sprite constants and roBrowser's NPC table. |
| `items.yml` | Item ID to icon, illustration, and display name. | `item_db` names and resource names. |
| `effects.yml` | Effect ID to effect type (sprite, STR animation, particle, or sound), files, and timing. | roBrowser's effect table, limited to the effects `core` uses. |
| `skill_effects.yml` | Skill ID to cast, hit, and ground effects. | roBrowser's skill effect table, limited to skills `core` ships. |
| `maps.yml` | Classic map name to display name, music, and minimap. | rAthena map index and roBrowser's map tables. |

The minimum set covers what `core` uses and no more. Missing entries fall back to a placeholder sprite or no effect, and the client logs the missing ID once.

Guide and template:

- `docs/client-data.md` explains each table, how to find the right sprite or effect in a GRF, and how to test a change.
- A template repository shows how to ship replacement tables as a mod. Tables in a mod override matching entries in `client/data/`.
- `wactl client-data validate` checks tables against the schema and against the files in a given GRF, and lists missing files.

### 10.6 Admin tools

Admin tools are part of the web client. They appear only for accounts with staff roles, and only after two-factor login (section 7.3).

| Tool | Actions |
|---|---|
| Player search | Find accounts and characters by name, ID, IP address range, or item serial. View online status, location, inventory, and storage. |
| Item history | Show every move of an item serial, including trades, drops, and stack splits (section 3.5). Restore items to a player after a confirmed theft. |
| Moderation | Mute, kick, jail, and ban, each with a duration and reason. Work through the report queue (section 7.10). |
| Chat review | Read reported chat logs within the retention period (section 7.10). |
| Events | Start, stop, and schedule automatic events (section 8.8). Edit event settings. |
| World | View zones, player counts, and tick times. Announce messages. Plan restarts. |
| Mods | List installed mods, their permissions, and error counts. Disable a mod in a zone. |
| Economy | View the economy dashboards (section 8.13). |

Rules:

- The tools call an admin API on the account and world services. The API checks the staff role for every call.
- Every action writes to the GM audit log (section 7.8).
- Actions that create items or zeny need a reason and are marked for review.
- Admin UI code is loaded only after the server confirms the staff role, so normal players do not download it.

## 11. Feature inventory

This list covers the systems in rAthena and what happens to each. Tiers:

- **Tier 1.** Needed for the first public release.
- **Tier 2.** After the first release.
- **Tier 3.** Left to mods. The core API must make it possible, but the project does not build it.
- **Dropped.** Not built.

### 11.1 Core gameplay

| System | rAthena source | Tier | Procedural mode notes |
|---|---|---|---|
| Movement, pathfinding | `unit.cpp`, `path.cpp` | 1 | Shared TinyGo package (section 3.3). |
| Stats, status effects | `status.cpp`, `db/*/status.yml` | 1 | None. |
| Combat and damage | `battle.cpp` | 1 | None. |
| Skills, first and second classes | `skill.cpp`, `src/map/skills/` | 1 | None. |
| Skills, transcendent classes | same | 1 | Rebirth NPC placed in the largest town. |
| Skills, third classes | same | 2 | Job change quests from templates (section 11.4). |
| Skills, fourth and expanded classes | same | 2 | Same as third classes. Needs `fourth_job_traits` on a Renewal base. |
| Baby classes | `pc.cpp` | 3 | None. |
| Monsters, AI, mob skills | `mob.cpp`, `db/*/mob_skill_db.txt` | 1 | Spawns from section 8.11. |
| Items, equipment, cards, combos | `itemdb.cpp`, `pc.cpp` | 1 | Rolled values (section 5.4). |
| Item groups, boxes, packages | `db/*/item_group_db.yml`, `item_packages.yml` | 1 | None. |
| Refining | `db/*/refine.yml` | 1 | Refine NPC placed in towns by templates. |
| Enchant grade, reform, enchant | `enchantgrade.yml`, `item_reform.yml`, `item_enchant.yml` | 2 | NPC services placed by templates. |
| Random options | `item_randomopt_*.yml` | 1 | Merged into rolled values (section 5.4). |
| Crafting (forge, brew, arrows) | `db/*/produce_db.txt`, `create_arrow_db.yml` | 1 | None. |
| Laphine synthesis and upgrade | `laphine_*.yml` | 3 | None. |
| Exp tables, level penalty | `db/*/job_exp.yml`, `level_penalty.yml` | 1 | None. |
| Map drops | `map_drops.yml` | 1 | Region drops by biome and level band. |
| Map flags (PvP, GvG, no teleport) | `npc/*/mapflag/` | 1 | Set by generators per map type. |
| Instances | `instance.cpp`, `instance_db.yml` | 2 | Instances copy generated dungeons. rAthena's fixed instances are classic mode only. |
| Achievements | `achievement.cpp` | 2 | Achievements that name classic maps are classic mode only. Generic ones (kill counts, levels) work everywhere. |
| Quests, quest log | `quest.cpp`, `quest_db.yml` | 1 | Quest templates (section 8.6). |
| Hunting boards (Eden Group style) | `npc/` scripts | 2 | Quest board NPC in each town, filled from quest templates. |
| Navigation | `navi.cpp` | 1 | Generated (section 8.5). |
| Warps and Kafra teleport | `npc/*/warps`, Kafra scripts | 1 | Map graph (section 8.10). |
| Storage (personal) | `storage.cpp` | 1 | Storage NPC in every town. |
| Cart | `pc.cpp` | 1 | None. |
| Magic Mushroom, Abracadabra tables | `magicmushroom_db.yml`, `abra_db.yml` | 1 | None. |
| Reputation | `reputation.yml` | 3 | None. |
| Spellbooks | `spellbook_db.yml` | 1 | None. |

### 11.2 Companions

| System | rAthena source | Tier | Notes |
|---|---|---|---|
| Pets, taming, pet evolution | `pet.cpp`, `pet_db.yml` | 2 | Taming items drop from monsters as usual. |
| Homunculus | `homunculus.cpp`, `homunculus_db.yml` | 2 | Homunculus S is tier 3. |
| Mercenaries | `mercenary.cpp`, `mercenary_db.yml` | 2 | Mercenary guild NPC placed in towns by templates. |
| Elementals | `elemental.cpp`, `elemental_db.yml` | 2 | Needs third classes. |

### 11.3 Social and economy

| System | rAthena source | Tier | Notes |
|---|---|---|---|
| Chat: public, whisper, party, guild | `clif.cpp`, `channel.cpp` | 1 | Moderation from section 7.10. |
| Chat channels | `channel.cpp` | 1 | None. |
| Chat rooms | `chat.cpp` | 2 | None. |
| Friends list | char server | 1 | None. |
| Party, party booking | `party.cpp`, web party booking | 1 | Party booking moves into the web client. |
| Guild, guild skills, guild storage | `guild.cpp`, `guild_skill_tree.yml` | 1 | Guild storage NPC placed in towns. |
| Guild emblems | web emblem controller | 1 | Upload rules from section 7.6. |
| Clans | `clan.cpp` | 3 | None. |
| Guild sieges (War of Emperium) | `castle_db.yml`, `npc/guild*/` | 2 | Castles generated from a castle template in chosen regions. Schedule from the event scheduler (section 8.8). |
| Battlegrounds | `battleground.cpp` | 3 | None. |
| Duels | `duel.cpp` | 2 | None. |
| Trade | `trade.cpp` | 1 | Transactional (section 3.5). |
| Vending, autotrade | `vending.cpp` | 1 | Vending search included. |
| Buying stores | `buyingstore.cpp` | 2 | None. |
| Store search | `searchstore.cpp` | 2 | Filter by roll range (section 5.4). |
| Mail (RODEX) | `mail.cpp` | 1 | Transactional. |
| Auction | char server | Dropped | Replaced by vending search and mail. |
| Zeny bank | `openbank` script command | 2 | None. |
| NPC shops, barter shops | `npc.cpp` | 1 | Town shops from templates. Prices from `item_db` (section 8.13). |
| Cash shop, cash points | `cashshop.cpp`, `item_cash.yml` | 3 | Payment handling is outside the project. The API supports cash points so servers can add their own shop. |
| VIP / premium accounts | `VIP_ENABLE` in `src/config/core.hpp` | 3 | Same as cash shop. |
| Roulette | `open_roulette` script command | 3 | None. |
| Stylist | `stylist.yml` | 2 | Stylist NPC in towns. |
| Attendance rewards | `attendance.yml` | 2 | None. |
| Captcha (anti-bot) | `captcha_db.yml` | 2 | Used by the staff tools for bot checks. |

### 11.4 Procedural replacements for fixed-map content

Some rAthena content depends on fixed maps and NPC locations. In procedural mode it is replaced as follows:

| rAthena content | Procedural replacement |
|---|---|
| Job change quests (for example, the Swordman test in Izlude) | One job guild per class family, placed in a town by templates. Tests are quest templates: hunt, fetch, and a short trial map generated like a small dungeon. |
| Rebirth (Valkyrie in Juno) | A rebirth NPC in the largest town of the highest level band reachable at level 99. |
| Third and fourth class job changes | Quest template chains with more steps and a boss trial. |
| Town-specific services (Kafra, refine, stylist, pet services) | Named places from town templates (section 8.6). |
| Fixed instances (Endless Tower, Old Glast Heim) | Generated instance dungeons with floor counts and a final boss. |
| Episode story quests | Not reproduced. Classic mode only. |
| MVP spawns on fixed maps | Field bosses (section 8.11) and boss events (section 8.8). |
| Guild castles | Generated castles from templates (section 11.3). |

### 11.5 Data files

Data files that the tables above do not name directly. `tools/wathena/inventory.py` checks that every file in `db/` appears in this section.

| Data file | Used by | Tier |
|---|---|---|
| `mob_db.yml` | Monsters | 1 |
| `mob_summon.yml` | Summon skills and monster skills | 1 |
| `mob_item_ratio.yml` | Monster drops | 1 |
| `mob_chat_db.yml` | Monster chat lines | 2 |
| `item_db_equip.yml`, `item_db_etc.yml`, `item_db_usable.yml` | Items | 1 |
| `item_combos.yml` | Item combos | 1 |
| `item_randomopt_db.yml`, `item_randomopt_group.yml` | Random options, merged into rolled values | 1 |
| `attr_fix.yml` | Element table (`element_table` flag) | 1 |
| `size_fix.yml` | Size table (`size_table` flag) | 1 |
| `const.yml` | Script constants for the converter and the Lua API | 1 |
| `job_stats.yml`, `job_basepoints.yml`, `job_aspd.yml` | Job HP, SP, stat bonuses, and ASPD | 1 |
| `statpoint.yml` | Status point cost (`stat_point_cost` flag) | 1 |
| `exp_guild.yml` | Guild levels | 1 |
| `exp_homun.yml` | Homunculus levels | 2 |
| `achievement_db.yml`, `achievement_level_db.yml` | Achievements | 2 |
| `job_outfits.yml` | Job outfit costumes | 3 |
| `battleground_db.yml` | Battlegrounds | 3 |
| `laphine_synthesis.yml`, `laphine_upgrade.yml` | Laphine synthesis and upgrade | 3 |
| `reputation_group.yml` | Reputation | 3 |

## 12. Testing tools

### 12.1 Headless bot clients

A Go package, `botclient`, speaks the same protocol as the web client. It uses the generated message code, so it stays in sync with the schema.

- A bot can sign up, log in, walk, fight, chat, trade, and talk to NPCs.
- Scenarios are written in Go or YAML, for example "100 bots walk around town and chat" or "20 bots trade the same item back and forth".
- Bots run without graphics. One machine runs thousands of them.

Uses:

| Use | How |
|---|---|
| Load tests | Nightly runs with 7,500 bots against an all-in-one build and a distributed build. Results are checked against section 3.8. |
| End-to-end tests | Scenarios for login, combat, trade, storage, and mail run on every pull request against a small server. |
| Duplication tests | Many bots trade, vend, and mail the same items at once. A job then checks item serials for copies (section 3.5). |
| Economy tests | Bots farm fields in each level band. Results feed the balance checks in section 8.13. |
| Mod tests | `wactl mod test` uses in-process fake players for speed. Bots are used for end-to-end mod tests. |

### 12.2 Other test layers

- Unit tests for formulas, with golden values from the rAthena parity harness.
- Fuzz tests for message decoders, YAML loaders, and the mod loader (section 7.9).
- Golden map checksums for the generator (section 8.5).
- Replay tests. The server can record the messages and random seed of a zone, then replay them to reproduce a bug.

## 13. Project governance

### 13.1 Repository layout

One repository holds the whole project:

```text
server/        Go services: gateway, account, world, zone
shared/        Go package compiled to WebAssembly (pathfinding, generator)
schema/        Message and data schemas, with codegen
client/        TypeScript web client, including client/data/
mods/core/     The core mod: rulesets, converted scripts, data
tools/wactl/   Command-line tool
tools/convert/ rAthena script and data converter
docs/          Guides, specs, and design proposals
deploy/        Docker Compose files and example configs
```

### 13.2 Contribution rules

- Every commit has a `Signed-off-by` line under the Developer Certificate of Origin. There is no contributor license agreement.
- Commit messages follow Conventional Commits (`feat:`, `fix:`, `docs:`).
- Pull requests need passing CI. With one maintainer, the maintainer reviews every pull request. Trusted contributors can be added as reviewers later.
- Issues use templates for bugs, features, and mod API requests.
- The repository has a code of conduct and `SECURITY.md` (section 7.9).

### 13.3 Design proposals

- Changes to the protocol, the public Lua API, the map format, the seed spec, or the database schema need a short design proposal in `docs/proposals/`.
- A proposal states the problem, the change, and what it breaks. It is merged when accepted.
- Small changes do not need a proposal.

### 13.4 Releases

- The server, client, and `wactl` share one version number and follow semantic versioning.
- Releases are tagged, signed, and built by CI (section 7.9).
- The public Lua API has its own version (section 6.12).

## 14. Solo development plan

The project has one developer. The plan is ordered to reach a playable game early and to avoid work that only matters at large scale.

### 14.1 Principles

- **Generate instead of write.** Codegen for messages, `sqlc` for queries, and converters for scripts and data do the repetitive work.
- **One of each.** One world mode, one ruleset, and one deployment mode first. The others come after the first release.
- **Tests replace reviewers.** Parity tests, golden checksums, and bot scenarios catch mistakes that a second person would.
- **Community later.** Features that others can build as mods (tier 3 in section 11) stay out of the core schedule.

### 14.2 First release scope

- Procedural mode only. It is the default mode, it avoids fixed-map content, and it does not need GRF map files.
- One ruleset: `core.ruleset.renewal`.
- Classes up to transcendent second classes.
- All tier 1 systems in section 11.
- All-in-one deployment.

Classic mode, pre-Renewal, third and later classes, and tier 2 systems follow.

### 14.3 Rough effort

These are estimates for one full-time developer. They will change after phase 0.

| Phase | Estimate |
|---|---|
| 0a. Work in the rAthena repository | 1 month |
| 0b. Foundations in the wAthena repository | 1 month |
| 1. Web client foundation | 3 months |
| 2. Server core skeleton | 3 months |
| 3. Lua runtime and mod loader | 3 months |
| 4. Combat, up to transcendent classes | 6 months |
| 5. Social and economy, tier 1 | 3 months |
| 6. Web client, tier 1 | 3 months |
| 7. Procedural mode | 4 months |
| 8. First public release | 1 month |
| 9. Modding ecosystem | 2 months, after release |
| 10. After the first release | Ongoing |

Total to the first release: about 28 months. The largest risk is phase 4 (section 5.7).

## 15. Phased roadmap

Phases run in order, because one developer does them (section 14). Each phase ends with exit criteria. Estimates are in section 14.3.

### Phase 0a: Work in the rAthena repository

Some phase 0 work needs rAthena's code to run or be read. It happens in a fork of rAthena, on a branch named `wathena-reference`, before the wAthena repository has any game code. Nothing here is sent upstream to rAthena, and the rAthena `master` branch is not changed.

1. **Pin a snapshot.** Record the rAthena commit used as the reference. At the time of writing that is `e98500617`. All reference data names this commit. Later rAthena changes are pulled in on purpose, not by accident.
2. **Build a reference recorder.** Add a small command-line program, `wathena-recorder`, under `src/tool/`. It links the map server code, loads the `db/` data, and calls the formula functions directly with fixed inputs. It builds twice, once with `RENEWAL` and once with `PRERE`.
   - It writes results as JSON, one file per system, with the rAthena commit and build mode in each file.
   - **Randomness.** rAthena uses a global `std::mt19937` seeded from `std::random_device` (`src/common/random.hpp`), and `std::uniform_int_distribution`, whose output differs between standard libraries. The recorder does not try to reproduce these values. It replaces `rnd`, `rnd_value`, and `rnd_chance` with fixed modes: always minimum, always maximum, and a fixed list of values. This gives the damage range and the fixed parts of every formula, which are what wAthena must match.
   - **Clock.** `gettick()` is replaced with a clock the recorder controls, so durations and cooldowns are exact.
3. **Record reference data.**
   - Stats: HP, SP, ATK, MATK, DEF, MDEF, HIT, FLEE, critical, and ASPD for a grid of jobs, levels, base stats, and equipment sets.
   - Damage: normal attacks and every skill of first, second, and transcendent classes, against a set of target monsters with different sizes, races, elements, and DEF values (section 5.7).
   - Status effects: chance, duration, and resistance for each status effect used by those skills.
   - Item and card bonuses: stats with each item equipped alone, and with common combos.
   - Exp and drops: exp and drop rates by level gap, for both level penalty settings.
   - Pathfinding: paths and walk times between sample points on sample maps, from `src/map/path.cpp`.
   - Refining: success rates and bonuses from `refine.yml`.
4. **Static inventories.** Scripts that read the rAthena source and data, without changing them:
   - The ruleset flag list: every `RENEWAL` preprocessor site grouped by flag (section 3.6). A first version already exists from the analysis for this plan.
   - The skill inventory: each skill's `SkillImpl` class, overridden functions, `RENEWAL` branches, and status effects (section 5.7).
   - The script command inventory: all 676 built-in commands with their arguments, for the converter (section 5.6).
   - The feature inventory check: confirm section 11 lists every system in `src/map/` and every file in `db/`.
5. **Data extracts.**
   - Spawn density per walkable cell and monster biome tags from `npc/re/mobs/fields/` and the map cache (section 8.11).
   - Minimum client data tables from rAthena constants and data: job, monster, NPC sprite, and item tables (section 10.5).
   - A snapshot of `db/re`, `db/pre-re`, and the needed parts of `npc/`, used as converter input.
6. **Check prior art.** A short check of goAthena is recorded in section 10.2.

Exit criteria:

- `wathena-recorder` builds in both modes and writes the reference data for the pinned commit.
- Running it twice gives identical files.
- The flag, skill, script command, and feature inventories are complete and checked into the wAthena repository.
- The data extracts and snapshot are checked into the wAthena repository with a note naming the rAthena commit.

The recorder stays in the rAthena fork. The wAthena repository keeps only its output, so wAthena itself does not need a C++ toolchain.

### Phase 0b: Foundations in the wAthena repository

- Create the wAthena repository with the layout in section 13.1, the license and mod exception (section 2.4), `SECURITY.md`, and the code of conduct.
- Import the reference data, inventories, and extracts from phase 0a. Add tests that load the reference data, so later phases can check against it.
- Design the message schema for the web client protocol.
- Benchmark `arnodel/golua` against `gopher-lua` and C Lua on converted item and NPC scripts. Pick the Lua runtime.
- Write the threat model (section 7.1).
- Set up CI with security checks (section 7.9).
- Write the seed tree spec with test vectors (section 8.2).

Exit criteria:

- CI loads the reference data from phase 0a for both modes.
- The Lua runtime decision is recorded with benchmark results.
- CI runs `govulncheck`, `gosec`, the race detector, and fuzz tests on every pull request.

### Phase 1: Web client foundation

- GRF reader with encryption, CP949 names, and partial caching (section 10.1).
- Map format reader (section 8.12), and conversion of a classic map from the player's GRF for testing.
- Sprite, model, and effect rendering, driven by the minimum client data tables (section 10.5).
- Walk around a map offline, using the shared pathfinding module compiled with TinyGo.

Exit criteria:

- A player can load their own game files and walk around a converted Prontera in the browser without a server.
- The WebAssembly pathfinding module passes the shared test vectors.
- The client meets the frame rate budget in section 3.8 on the test map.

### Phase 2: Server core skeleton

- Account service with signup, login, session tokens, and rate limits (section 7.3).
- TOTP and passkey setup in account settings.
- PostgreSQL schema, migrations, and database roles per service.
- Zone goroutines, entity components, scheduler, and persistence layer.
- Ruleset loader for ruleset mods, with flag and dependency checks.
- Seed tree library and per-zone secret random streams (section 8.3).
- Gateway with message size and rate limits, and fuzz tests for every decoder (section 7.4).
- All-in-one mode and the Docker Compose file (section 3.9).
- Headless bot client with login, walk, and chat (section 12.1).
- Walking, map changes, and chat, connected to the web client.

Exit criteria:

- Players can sign up, log in, walk, change maps, and chat in the web client.
- Pathfinding and walk timing pass the parity harness.
- Seed tree results match the spec test vectors in Go and TinyGo builds.
- No plain-text passwords or shared service passwords exist anywhere in the system.
- 500 bots walk and chat on one all-in-one server within the tick budget.

### Phase 3: Lua runtime and mod loader

- Lua binding layer, sandbox, coroutines, events, variables, and hooks.
- Mod loader with manifests, dependencies, load order, data overlays, ID ranges, ruleset support checks, and permissions.
- `wactl mod new`, `validate`, `test`, `dev`, and `install` from Git URLs and `.zip` files.
- Lua Language Server type definitions generated from the binding layer.
- Script converter. For the first release it converts item scripts, shops, services, and dialogs used by town templates. Full `npc/` conversion comes with classic mode (phase 10).
- Event scheduler with schedule and condition triggers, using the server time zone.

Exit criteria:

- Converted item scripts and service NPCs behave the same as the originals in scripted tests.
- A sample community mod adds an NPC, an item, a monster, and a hook without changes to the server source.
- A scheduled boss spawn event runs without staff action.

### Phase 4: Combat, up to transcendent classes

- `core.ruleset.renewal` with all independent flags implemented. `core.ruleset.pre_renewal` follows in phase 10.
- Stat calculation with event-driven modifiers, status effects, and damage formulas.
- Skills for first, second, and transcendent classes, ported with the method in section 5.7.
- Monster spawning, AI, mob skills, drops, and exp.
- Item and card scripts converted to the event-driven format.
- Rolled item values, the `item_rolls` flag, and random options on the same storage (section 5.4).
- Skill and job definitions through the mod API, used by `core` itself.

Exit criteria:

- `core.ruleset.renewal` matches the parity harness.
- Every flag has unit tests for both values. Sampled mixed combinations pass CI.
- Every ported skill has golden tests.
- The full-recalculation check in section 5.3 passes for every item in `core`.
- With `item_rolls` set to `off`, every item matches rAthena. With `on`, 100,000 rolled drops stay inside their declared ranges and follow their distributions.
- A sample mod adds a new skill and a new job.

### Phase 5: Social and economy systems, tier 1

- All tier 1 systems in section 11.3: chat, channels, friends, party, guild, guild storage, emblems, trade, vending, mail, and NPC shops.
- Personal storage and cart.
- Moderation: reports, blocking, chat filter, and sanctions (section 7.10).
- Built-in automatic events: boss spawns, invasions, and treasure hunts.

Exit criteria:

- Each system passes its scripted tests.
- Duplication tests for trade, storage, mail, and vending pass with bots under concurrent load.

### Phase 6: Web client, tier 1

- All UI windows for the systems in phases 4 and 5.
- Admin tools (section 10.6).
- Report and block actions, data export, and account deletion in the account page (section 7.10).
- Mod asset loading and mod client data tables.
- Touch input for mobile browsers.

Exit criteria:

- A player can do every tier 1 activity in the web client.
- Staff can find a player, view item history, and apply a sanction from the admin tools.
- Items, monsters, skills, and jobs added by a mod display correctly.

### Phase 7: Procedural mode

All code in this phase sits behind the `procedural` build tag.

- Generator core in Go with fixed-point math, compiled to WebAssembly with TinyGo (section 8.5).
- Generated fields, dungeons, and towns, with checksum verification and download fallback.
- Named places, town templates, spawn tables, and quest templates.
- Procedural replacements for job changes and rebirth (section 11.4).
- Home area placement from mod maps (section 8.12).
- `wa.gen` API for mod generation layers.
- Rotating dungeons and world change events.
- Town naming styles, map IDs, the map graph, and warp services (section 8.10).
- Monster spawn tags, field and dungeon spawn selection, mini bosses, and field bosses (section 8.11).
- Economy simulator and generator checks (section 8.13).

Exit criteria:

- The same seed and generator version give the same cell grid on the server and in Chrome, Firefox, and Safari, on amd64 and arm64 machines.
- A generated world with at least 5 towns, 20 fields, and 5 dungeons is playable from level 1 to the transcendent level cap.
- A rotating dungeon changes on schedule without staff action.
- Every generated map is reachable from the home area, and no two towns in a world share a name.
- Removing a rotating dungeon moves its players, save points, and memo points to the nearest town.
- Every level band has at least 3 fields, and every generated field meets the spawn constraints in section 8.11.
- Every field is within 30% of its band's target exp and zeny rates in the simulator, and bot runs agree with the simulator within 20%.
- Players cannot predict loot or spawns from public seeds, tested with a known public seed.

### Phase 8: First public release

Built with the `procedural` tag, with procedural mode as the only world mode (section 14.2).

- GM audit logs, role-based GM permissions, and encrypted backups with restore tests (section 7.8).
- Privacy settings and retention jobs (section 7.10).
- Load tests at 2,000 bots on the all-in-one build.
- A security review. With one developer, this can be a paid review or a public bug bounty before the beta.
- Setup guide for server owners and a first-start guide for players.
- Public beta.

Exit criteria:

- Load tests meet the budgets in section 3.8 at 2,000 bots.
- All high and critical findings from the security review are fixed.
- A new server owner can go from an empty machine to a running world with the setup guide in under 30 minutes.

### Phase 9: Modding ecosystem

- `wactl mod pack`, `purge`, and `convert` for community scripts.
- Signed releases and signature checks.
- Generated API reference website and modding guide.
- Client data guide and template repository (section 10.5).
- Stable API version 1.0.
- Index file format, `wactl` index support, and a template repository for community indexes (section 6.10).

Exit criteria:

- Five mods written by other people install from Git URLs and pass `wactl mod validate` and `wactl mod test`.
- A popular rAthena forum script converts and runs with only documented manual fixes.

### Phase 10: After the first release

Ordered by expected demand. Each item gets its own exit criteria when it starts.

1. `core.ruleset.pre_renewal`, matching the parity harness.
2. Tier 2 systems from section 11, starting with pets, homunculus, and guild sieges.
3. Third classes, then fourth and expanded classes.
4. Classic mode: full `npc/` conversion, classic maps from player GRF files, and fixed spawns.
5. Distributed deployment mode and load tests at 7,500 bots (section 3.7).
6. Web client plugin API (section 10.4).

## 16. Risks

| Risk | Impact | Mitigation |
|---|---|---|
| One developer | Slow progress, burnout, and no one else who knows the code. | Small first release (section 14.2). Tests and design proposals written down. Contributors invited after the first release. |
| Skill porting takes longer than planned | The first release slips. | Golden tests first, shared base types, translation tooling, and transcendent classes as the first release limit (section 5.7). |
| Formula parity gaps | Players see different damage, drops, or exp. | Parity harness from phase 0. No system ships without golden tests. |
| Broken flag combinations | Mixed rulesets give unbalanced or wrong results. | Coupled features stay in the base. The loader checks dependencies. CI samples mixed combinations. `wactl mod test` shows ruleset authors how their mix compares with each mode. |
| Scope | About 350k lines of C++ and 808k lines of scripts. | First release uses procedural mode and one ruleset. Classic mode and full script conversion come later. Tier 3 features are left to mods. |
| GRF reading fails on some browsers | Players cannot load their game files. | Partial reading and caching (section 10.1). Classic mode can be dropped, since generated worlds need only sprites, textures, models, and sounds. |
| License or trademark problems | Takedown requests or forced changes. | GPL v3 compatible license (section 2.4). No Gravity trademarks in names or branding. No Gravity assets hosted. |
| `arnodel/golua` maturity or speed | Slow ticks or runtime bugs. | Benchmarked in phase 0b (decision 0001). The binding layer recovers from runtime panics and allows switching to C Lua. Static bonus tables and computed-bonus caching reduce Lua calls. |
| Go garbage collector pauses | Uneven tick times in busy zones. | Allocation budgets per tick, buffer pools, `GOMEMLIMIT`, and tick p99 tracking in load tests. |
| Missing change events for item scripts | Stale bonuses. | Full-recalculation comparison in tests (section 5.3). |
| Missing client data entries | Wrong or missing sprites and effects. | Placeholder fallbacks, logged missing IDs, and `wactl client-data validate` (section 10.5). |
| Players need their own game files | Fewer players can join. | Clear setup guide in the client. Mods can ship original assets. |
| Malicious or broken mods | Economy exploits, data loss, or crashes. | Sandbox, CPU and memory limits, permissions approved by the owner, and signed releases. |
| Malicious client forks | Stolen passwords or sessions. | Login only on the account service's page. Passkeys bound to its domain. Server owners list trusted client domains. |
| Public API maintenance | Breaking changes anger mod authors. Freezing the API slows core work. | Experimental namespace, semantic versioning, deprecation period, and `core` built on the public API. |
| Account takeover | Stolen items and angry players. | Argon2id, rate limits, optional two-factor login, session revocation, and item serial tracking to restore stolen items. |
| Harassment and spam | Players leave. | Reports, blocking, chat filter, new-character chat limits, and sanctions (section 7.10). |
| Generated maps differ between server and client | Walking desyncs on generated maps. | Fixed-point math, one generator in Go compiled with TinyGo, test vectors, and checksum with download fallback. |
| Unbalanced generated worlds | Leveling is too fast or too slow, or zeny inflates. | Economy simulator, generator checks, bot runs, and live dashboards (section 8.13). |
| Rolled items unbalance the economy | Top rolls become required, or prices spread too wide. | `item_rolls` flag per ruleset, triangular distributions by default, and drop logs to watch roll spread. |
| Procedural content feels repetitive | Players leave procedural worlds. | Mod-supplied biomes, towns, and quest templates. World change events. |
| Predictable randomness | Players predict drops or critical hits. | Secret ChaCha8 streams, one-way seed derivation, and no gameplay randomness from public seeds. |

## 17. Open questions

- **Lua API design.** The API in section 5 is a sketch. It will be redesigned before the binding layer is built, for example so that mods do not define custom globals. The redesign needs a design proposal.
