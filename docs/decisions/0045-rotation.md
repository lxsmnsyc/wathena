# 0045: Rotating Dungeons and World Changes

- Status: proposed.
- Date: 2026-10-17.
- Code: [server/rotation](../../server/rotation) (`rotation.go`, `keys.go`), [server/layout](../../server/layout) (`rotation.go`, `atlas.go`, `layout.go`), [server/fields](../../server/fields) (`layout.go`, `fields.go`, `spawns.go`, `records.go`, `core/data/dungeon_styles.yml`), [server/places/places.go](../../server/places/places.go), [server/world/world.go](../../server/world/world.go), [server/zone](../../server/zone) (`worldchange.go`, `warp.go`, `zone.go`, `headless.go`), [server/store](../../server/store) (`worldchanges.go`, migration 00035, `queries/world_changes.sql`), [server/modscript](../../server/modscript) (`worldevents.go`, `runtime.go`, `prelude.lua`, `luaapi/spec.go`), [server/admin](../../server/admin) (`tools.go`, `world.go`, `admin.go`), [server/adminworld/tools.go](../../server/adminworld/tools.go), [server/cmd/waserver](../../server/cmd/waserver) (`worldchange.go`, `fields.go`, `main.go`, `config.go`), [server/metrics/registry.go](../../server/metrics/registry.go), [tools/wactl](../../tools/wactl) (`layout.go`, `world.go`), [tools/modkit](../../tools/modkit) (`changes.go`, `generated.go`, `test.go`), [schema/proto/wathena/admin/v1/admin.proto](../../schema/proto/wathena/admin/v1/admin.proto), [client/src/admin](../../client/src/admin) (`events.ts`, `api.ts`), [client/scripts/rotation-check.ts](../../client/scripts/rotation-check.ts), [mods/examples/market-town](../../mods/examples/market-town), [e2e](../../e2e) (`rotation_test.go`, `startup_test.go`, `server.go`).
- Spec: [generator-v3](../specs/generator-v3.md), "Rotations and seasons".
- Phase 7 step 7 (strategy section 15, "Phase 7 in full"), and proposal 0002, review decision 47.
- Lua API: the world events `dungeon_rotated` and `region_changed`, and `world:rotate(dungeon)` and `world:change_region(x, y)` in `wa.testing`, as review decision 47 lists them, and nothing else.
- Protocol: none. The admin API gains two calls and the events list's rotations and seasons.

## Context

Steps 1 to 6 made a procedural world whose layout, spawns and content `wactl world new` fixes for good. Strategy sections 8.8 and 8.10 ask for a dungeon that rotates on a schedule and for world changes at the end of a season, and for a procedure that removes or replaces a map without losing players, save points or memo points. Review decision 47 fixed the shape. The open questions were:

- How a world that the database keeps as made runs maps that change with the date.
- How the game server picks the spawns of new floors and fields when it never holds the secret branch.
- How zones, warps, places and the map graph change while the server runs, since each was fixed at start.
- What "safe under restarts" needs when the server stops before, during or after a change.
- What a region is, and which one a season end changes.

## Decision

### The layout that runs

The database keeps the layout as `wactl world new` made it. A state, each rotating dungeon's active date and each region's season, turns it into the layout that runs (`layout.Layout.Apply`):

- A rotating dungeon's floors take dated map IDs, `gen:dungeon:<id>:<date>:<floor>`, the `nomemo` flag, and their stored IDs as `Base`. Their links follow. Names and bands stay.
- A changed region's fields take their season and the region's new biome. IDs, links and bands stay.

The atlas keeps both: the stored layout with the current names, and the one that runs. A rename saves a rotating floor's name by its stored ID, so the next rotation keeps it. `places.find("dungeon:crypt", tag)` finds dated floors.

### Rotating dungeons

- Dungeon styles take `rotation: { every, at, weekday }`. Core's four styles all have one, so the first dungeon of a default world always rotates: cave and tower daily at 04:00, crypt weekly on Friday at 20:00, ruins weekly on Monday at 04:00.
- "Core marks one dungeon per world": `wactl world new` marks the first dungeon in the layout's order whose style has a rotation, and stores it with its rule in `world_rotations`, so a changed record never changes a made world. A world without such a style has no rotating dungeon.
- A date is the local date of the rotation's time in the server's time zone, as the event scheduler reads clock times. The dungeon's public node of a date is `child(child(public, "dungeon"), id, yyyymmdd)`, so the stored floor's node, `child(child(public, "dungeon"), id)`, stays apart.
- The floors that run are those of the latest rotation time at or before now. The next date's floors are generated right after each change, so they are ready at the time.
- The stored spawn lists and content of the stored floors are not used. Generation layers and the `spawn_table` hooks run only at world creation, as review decisions 44 and 46 approved them, so they do not run on rotating floors.

### World changes

- A region is the fields whose nearest town by warps is one town, ties to the lower number, as for field bosses (decision 0042). It is named by the town's ID, such as `t2`. `world:change_region(x, y)` changes the region that holds the field at x, y.
- A region has one biome per season after the first change: season k draws a biome other than season k - 1's from the public node `child(child(child(public, "region"), town), "season", k)`. Season 0's biome is the one most of its fields have.
- Each field of a region in season n generates from `child(field(x, y), "season", n)`, the decision's `field(x, y, "season", n)`, with the new biome. IDs, links and bands stay, so save points and portals keep their map IDs. The layers' places and objects on the field are left out, since they stood on the old cells.
- With `procedural.seasons.days` set, a season ends at 00:00 every that many days from the day seasons started, and the next region by town number changes, in turn. Seasons are off by default. Staff can change a region either way.

### Secrets

`wactl world new` stores, for the game role (migration 00035):

- `world_rotations.key`: `child(child(secret, "rotation"), dungeon)` for each rotating dungeon. The decision's `child(secret, "rotation", name)` puts the name in the label, since rng-v1's indexes are numbers.
- `world.season_key`: `child(secret, "season")`.

The game server runs spawn selection with `child(rotation key, "date", yyyymmdd)` or `child(child(season key, town), "season", n)` as the secret of `spawns.Plan`, the same code as at world creation. It still never reads `world.seed` or the secret branch. The keys reveal those maps' future spawns and nothing else, which the threat model records.

### Removing or replacing a map

A rotation removes the old floors and adds the new ones. A region's change replaces its fields' zones with new zones of the same IDs. Both follow strategy section 8.10:

1. Warnings at 10, 5 and 1 minutes and 30 seconds go to the players on the maps the change removes or replaces, as announcements: "Crypt of Skarvik F1 closes for new floors in 5 minutes. Anyone still here then goes to Skarvik."
2. At the time, in one step on the change manager's goroutine: the generated maps, the atlas and the places index change; the new zones join the world, and the old ones leave it; the zones whose warps changed, such as the entrance field, get their new warps.
3. The players still on an old map go to the nearest town by the map graph, at its save spot: a walkable cell 3 to 6 cells from its `inn` place, else its `square`, that no warp covers. A player who took a warp to the old map a moment before goes on there when it arrives.
4. Save points and memo points on the old maps move to the same spot: in memory for the players online, and in the database for every character, with the change's record, in one transaction. A removed map's points also move at any later join, for a character that was in transit. A replaced field's points move only this once, since points made on it later stay. A character keeps one memo point per map.
5. The `dungeon_rotated` or `region_changed` handlers run in every zone. The old zones stop 30 seconds later.

The world, the zone directory, the generated maps, the atlas and the places index are now safe to change while the server runs. A place keeps its handle's index; the places of a removed map leave `find` and `nearest`.

### Restarts

- The database records a change with its moves in one transaction. Until then the stored state is the old one.
- At start, `rotation.Due` turns the stored state into the state the world runs now: a world's first date, a rotation that came while the server was stopped, and each season end that came. The start records these with the moves of their old maps before anyone joins, and logs `world changed while the server was stopped`. A record that fails stops the start.
- So a change that a stop cut short before its record happens again at the next start, and one that the database recorded is done. Players online at the stop were moved in memory, and their last saves name the town.
- A forced change that waits for its countdown lives only in the running server.

### Staff

The events screen lists the rotating dungeon with its rule, the date that runs and the next change, and the regions with their seasons. `POST /rotations/{dungeon}/rotate` and `POST /regions/{region}/change` need `events.control` and write `events.rotate` and `events.change_region` audit rows. A forced change happens 10 minutes later, after the warnings. A forced rotation takes the next date's floors, so the scheduled time of that date changes nothing.

### Dev worlds

`waserver --dev --layout` marks its rotating dungeon as a new world does, with keys from `--seed`, and keeps its state in memory. `--dev`'s clock moves rotations and seasons. Worlds of `--fields`, `--towns` and `--dungeons` do not rotate, so the existing checks keep their floor IDs. Test worlds of `wactl mod test` rotate only through `world:rotate`.

### The startup stall

A procedural world on PostgreSQL stopped after the jail warning when its config named an event map that did not run. `runAll` started the server variables' writer, which runs until the server's context ends, and deferred the wait for it. A later error, here `CheckEvent`'s, returned from `runAll`, and the deferred wait ran before the deferred cancel, so it waited for ever. Without a database the writer returns at once, which hid it. Any error after that point stalled the start, and so did the waits of the zones' last saves after `go w.Run`. The waits now end the context first (`background` in `worldchange.go`). `TestBackground` and the e2e test `TestStartFailsAtOnce` cover it; the e2e test stalls without the fix.

## Determinism

The 2 golden worlds of `server/rotation/testdata/golden-rotation.json`, each in two rotation dates and two seasons of a region, give the same layout checksums, nodes, biomes, grid checksums, WMAP hashes and spawn lists in Go on darwin/arm64 and on darwin/amd64 under Rosetta, which `scripts/check.sh gen` runs. The client generates the new floors and fields with the generator it has, from the nodes the server sends, so no client-side generation is new; the browser check `rotation` shows Chromium generating a new floor with the server's checksum.

## Tests and checks

- `server/rotation`: `TestDue`, `TestRotationTiming` (the warnings at 10, 5 and 1 minutes and 30 seconds on the server clock, the change at its time, its record and the next build), `TestClockJump`, `TestForce`, `TestSeasons`, `TestRetry`, `TestDaylightSaving`, and, with the procedural tag, `TestGoldenRotation` and `TestRotationRules` (other floors and IDs per date, one new biome per season with IDs and bands kept, and other spawns from the public branch or another date).
- `server/layout`: `TestParseRotation`, `TestFloorIDs`, `TestApply` and `TestAtlasReplace`.
- `server/places`: `TestUpdate`.
- `server/world`: `TestRemoveAndEvacuate`, with the race detector too: a zone added while the world runs, the old one removed, its player moved, the points of a player online and of a later login moved.
- `server/zone`: `TestRelocationMoves`.
- `server/store`: `TestWorldChanges`, with PostgreSQL: the keys and rotation, the moves of characters, save points and memo points, the seasons, a failed change left out whole, and what the game role cannot change.
- `server/adminworld`: `TestForceChange`, with PostgreSQL. `server/admin`: `TestEventCalls` with the two calls.
- `server/modscript`: `TestWorldChanged`, and `TestAPIDefinitions` with the new events.
- `server/cmd/waserver`: `TestBackground` and `TestChangeHost`, a `--dev --layout` world whose rotation moves a player, points the entrance at the new floor and replaces a region's field zones.
- `tools/wactl`: `TestProceduralWorld` checks the rotating dungeon and the keys. `tools/modkit`: `TestTestWorldChanges` and `TestMarketTown`.
- `wactl mod test` of the Market Town: 2 new cases, a rotation of the cellar heard by the Town Guide and a region's change.
- e2e, with PostgreSQL: `TestRotation` proves the two exit criteria. The server clock moves to 9 minutes before the dungeon's next time; the player inside hears the warning; at the time, without staff action, the new floors run, the old ones are gone, and the player inside is in the nearest town; an offline player's save point and memo point on the old floor are at that town's save spot in the database. Then the server stops, the stored date goes back years, and the next start rotates and moves the points. `TestStartFailsAtOnce` covers the startup stall.
- Browsers: the check `rotation` warps into the rotating floor, sees the warning in the chat log, follows the server to the town at the time, and warps into the new floor, which the page generates.
- Client: `events.test.ts` with the screen's texts.

## Lua API

The events and the two test helpers of review decision 47, and nothing else. Proposal 0002, next step 33, has the details to review.

## Left

- The `spawn_table` hooks and generation layers on rotating floors and changed fields, which would run outside world creation (review items).
- World quests that name monsters of a changed region's fields keep their monsters; a board quest may name a monster no field has after a change.
- Jail return points on removed maps: a jailed player released after its return map went is placed as a character on a map that does not run.
- A forced change that waits does not last across a restart.
- Mod switches that staff made after the start do not reach the zones a change adds until the next start.
- A run on a real amd64 machine.

## Review items

These need a decision before they are built, since they would be new API or change approved behavior:

- Whether the `spawn_table` hooks and generation layers should run on rotating floors and changed fields while the server runs, which review decisions 44 and 46 limit to world creation.
- A Lua way to read a rotating dungeon's date or next rotation, or a region's season, such as `maps.get_rotation(dungeon)`.
- Whether staff's forced change should last across a restart, and a way to cancel one.
- Whether a world change should give a region a new level band too, as strategy section 8.8 allows; decision 47 keeps bands.
- Whether more than one dungeon per world should rotate, by a config list or a style field.
- The warnings' text in locale files, and a client banner for them.
