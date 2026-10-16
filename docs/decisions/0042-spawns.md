# 0042: Spawns and Level Bands

- Status: proposed.
- Date: 2026-10-16.
- Code: [server/spawns](../../server/spawns) (`spawns.go`, `themes.go`, `select.go`, `plan.go`, `check.go`, `data.go`, `core/data/spawn_tags.yml`, `core/data/spawn_themes.yml`), [tools/spawntags](../../tools/spawntags), [server/layout](../../server/layout) (`layout.go`, `options.go`, `dev.go`), [server/fields/spawns.go](../../server/fields/spawns.go), [server/gamedata](../../server/gamedata) (`spawns.go`, `world.go`, `mods.go`), [server/store](../../server/store) (`spawns.go`, migration 00032, `queries/spawns.sql`), [server/zone](../../server/zone) (`spawntable.go`, `monster.go`, `headless.go`), [server/modscript](../../server/modscript) (`spawns.go`, `world.go`, `prelude.lua`, `luaapi/spec.go`), [server/mod/permission.go](../../server/mod/permission.go), [server/cmd/waserver](../../server/cmd/waserver) (`fields.go`, `config.go`, `main.go`), [tools/wactl/layout.go](../../tools/wactl/layout.go), [tools/modkit](../../tools/modkit) (`generated.go`, `test.go`), [mods/examples/mossy-fields](../../mods/examples/mossy-fields), [e2e/spawns_test.go](../../e2e/spawns_test.go).
- Spec: [generator-v3](../specs/generator-v3.md), "World layout" and "Spawns".
- Phase 7 step 5 (strategy section 15, "Phase 7 in full"), and proposal 0002, review decision 44.
- Lua API: the hook `spawn_table` and the permission `spawns.modify`, as review decision 44 lists them.

## Context

Step 4 made a world's layout with bands that spread over levels 1 to 99 by the farthest map. Review decision 44 fixed the bands to 8 levels per warp from a home area of levels 1 to 10, asked for 3 fields in every band up to 99, and fixed how fields and dungeon floors pick their monsters (strategy section 8.11). The open questions were:

- How a 6 × 5 grid reaches level 99 at 8 levels per warp.
- What "every band has 3 fields" counts.
- Who picks the spawns, since the game server holds only the public branch.
- Where the spawn_table hook runs, since world creation has no zone.

## Decision

### Bands

- A map at d warps from the home area starts at level 1 + 8 × max(0, d − d0), where d0 is the fewest warps of any field. So the home area is the home map, the first town and the fields nearest them, at levels 1 to 10. Bands stop at 90 to 99. `procedural.layout.band_step` sets the 8. It is 1 to `band_width`, and left out it is at most `band_width`.
- The coverage check counts the bands of `band_width` levels from level 1: 1 to 10, 11 to 20, and on to 91 to 99. Each needs 3 fields whose band overlaps it, or the layout tries its next attempt. A field's band of 9 to 18 counts for both 1 to 10 and 11 to 20. Counting the 13 bands of the step instead would need 39 fields, more than the 30 of the default grid.
- A grid with fewer than 3 fields per counted band, such as the 2 × 2 grids of tests, skips the check, since it can never pass.
- The default grid is 6 × 5.

### A layout that reaches the top

At 8 levels per warp, the top band needs fields 12 warps from home. Step 4's borders, open with a chance of 3 in 4, gave at most about 9 warps on a 6 × 5 grid, so no attempt could pass the check. The borders now grow as a tree in layers from the first town's seat: each layer takes 2 fields, or 3 with a chance of 1 in 3, among those that touch the layer before, preferring fields with few unreached neighbors so the layers sweep the grid. Fields left over join their deepest neighbor. Loops then open between fields at most one layer apart, which shortens no way.

Towns could still make shortcuts: a town whose gates reach fields 10 warps apart joins them through 2 warps. A town now links only fields within 2 warps of its seat over open borders, and, after the first town, only while its fields stay within 2 warps of each other from home. Each town after the first tries its seats in a shuffled order until its gates find fields.

In 2,000 default layouts, none failed, and 82% passed on the first attempt. The fields spread evenly: about 2.2 per band step, with the top band holding the rest. The 6 golden layouts changed and all pass on their first attempt now.

### Spawn tags

- `tools/spawntags` reads `reference/rathena/extracts/renewal/spawns.json`, the extract that phase 0a made from rAthena's `npc/re/mobs`, and writes `server/spawns/core/data/spawn_tags.yml`. No new extract was needed: the file already holds each map's spawn lines and walkable cells.
- Each rAthena map family maps to a biome of wAthena: the six field biomes of generator-v2 and the four dungeon tilesets of generator-v3. `tools/spawntags/families.go` lists them, and the data file repeats them. Towns, guild maps, quest maps and the later lands are left out.
- A monster's weight in a biome is its share of that biome's monsters per 1,000, at least 1. Its role is the role of most of its monsters' lines, by section 8.11's rules. A boss class monster is a mini boss, and an MVP a field boss.
- The densities are rAthena's medians of monsters per 10,000 walkable cells: 41 for field maps and 48 for dungeon maps.
- Level, race, element, size and boss class come from monster data. A monster record's `spawn: { biomes, role, themes }` replaces its tags field by field. A new mod monster has none until it sets them. A role left out is uncommon, or mini_boss or field_boss for a boss. A boss as common or uncommon is an error.
- Spawn themes are records of `data/spawn_themes.yml`: `biomes`, `races`, `elements` and `boost`. Core has nine.

### Selection

`server/spawns` follows section 8.11, and generator-v3, "Spawns", fixes each draw. The parts the strategy leaves open:

- The band fits through 14 tiers: the map's biome within 0, 5 and 10 levels of the band, then any biome at those, then 15, 20, 30 and 50 levels. Some biomes lack monsters at some levels, such as towers at levels 1 to 20.
- A map draws a theme with a chance of 1 in 3.
- "Bands below level 30" are those whose lowest level is below 30.
- Rares and plants take at most a quarter of a map's monsters, and every species at least 2 monsters, so small dungeon floors still hold all their species.
- A region is the set of fields whose nearest town, by warps, is one town. Its fields that are not next to a town draw 1 or 2 field bosses.
- A mini boss is at most 20 levels above the band and not below it. A field boss is at or above the band, best within 30 levels above it.
- Each dungeon's last floor gets a field boss in its boss room. Other floors get no boss.
- Plants spawn within 8 cells of water or a clearing on fields, or a room on floors.
- No aggressive monster spawns within 15 cells of an arrival, and no boss within 30 cells of a warp to a town, by the larger axis distance. The stored line keeps these cells, and the zone never places a monster of it there, also when it comes back.

### Who reads what

| Who | Reads | Never reads |
|---|---|---|
| `wactl world new` (migrator role) | The new seed, the public and secret branches, the mods, the game data, every generated map | |
| The database | `world.seed`, `world_maps`, `world_links`, `world_spawns` | |
| `waserver` (game role) | `world.public_node`, `world_maps`, `world_links`, `world_spawns` | `world.seed`, so no secret branch |
| The web client | Each map's public node, params, places and band, the monsters it sees | The public branch, any secret node, the spawn lists |

The spawn lists come only from `child(root, "secret")`. The secret is never derived from the public branch, so no client can compute or predict them. `TestPublicSeed` checks that the public branch, and nodes derived from it, give other lists, that other secrets with the same maps give other lists, and that no map data names a monster.

### World creation

- `wactl world new --mode procedural` loads the game data of the ruleset's mode and the mods of `--config`, as `waserver` does, builds the layout, generates every map, computes the warps, and picks the spawns. It writes the world row, the layout and `world_spawns` in one transaction. This takes about 10 seconds, most of it loading the game data.
- `waserver` reads `world_spawns` at start and makes each line a zone spawn with the loaded monster's stats, by name. A line whose monster no longer loads, such as one of a removed mod, is left out with a warning. A new generator version or a mod change never changes a made world's spawns.
- Dev worlds without a database have no spawns.
- The settings `spawns.town_field_boss_chance`, `spawns.town_field_boss_role` and `spawns.density.field` and `.dungeon` are read by `wactl world new`. `waserver` checks them at start.

### The hook

- `hooks.on("spawn_table", function(map, spawns, next) ... end)` needs `spawns.modify` in place of `rules.override`. It is a mod context's hook.
- `map` is read-only: `id`, `kind`, `biome`, `band` as `{ lowest, highest }`, and `features`. `spawns` is a copy of the list the hook may change: each `{ monster, count, respawn, role, area }`, with `respawn` in milliseconds and `area` `{ x, y, w, h }` or nil. `next(spawns)` runs the rest of the chain on a list, the hook's own by default. The innermost result is the list it gets.
- A hook that fails or returns a bad list is logged and left out, as for other hooks.
- `wactl world new` runs it only when a mod with `spawns.modify` has an entry script: a Lua state runs the mods' entry scripts once, on no map, and then each list goes through the chain in map order.
- `wactl mod test` runs it in each test world: a case that runs on a generated field or floor gets the map's list, from the test world's secret, through the hooks of that world.

## Tests and checks

- `server/spawns`: `TestGoldenSpawns` (2 worlds, every line stored), `TestSpawnDeterministic`, `TestSpawnRules` (12 default worlds, with the settings, against `Check`: species, roles, elements, aggressive limits, arrivals, warps to towns, and the boss rules), `TestSpawnDensity`, `TestPublicSeed`, `FuzzSelect`, `TestParseTags`, `TestParseTheme`, `TestResolve` and `TestLoadCore`.
- `tools/spawntags`: `TestCommitted`, `TestFamily` and `TestDerive`.
- `server/layout`: `TestCoverage` (300 default worlds), `TestBands`, and `TestLayoutRules` with the new bands and the check. The golden layouts changed.
- `server/store`: `TestWorldSpawns`, with PostgreSQL.
- `server/zone`: `TestSpawnClearCells`.
- `server/modscript`: `TestSpawnTable` and `TestSpawnTableRules`.
- `server/cmd/waserver`: `TestZoneSpawns` and `TestSpawnsConfig`.
- `tools/wactl`: `TestProceduralWorld`, with the Home Village and Mossy Fields samples.
- `tools/modkit`: `TestMossyFields`, and the sample's 3 `wactl mod test` cases.
- e2e, with PostgreSQL: `TestWorldSpawns`. `wactl world new` makes the world with both samples. Every field and floor has a list, the hook put 12 Moss Porings on low fields, and a hidden player who looks at each quarter of such a field sees Moss Porings and only the monsters its list stores.
- `scripts/check.sh gen` also runs the golden spawns on both architectures.

## Lua API

The hook `spawn_table` and the permission `spawns.modify`, and nothing else. Proposal 0002, next step 30, has the details to review.

## Left

- Better drop rolls in higher bands (step 8), density scaling by player count, and bosses placed by mods through `wa.generation` (step 6).
- Spawns in dev worlds without a database.
- Floors are small, so a floor holds 10 to 20 monsters at rAthena's density.
- The map flags of section 8.11's first step: generated maps have no flags that limit monsters yet.
- A run on a real amd64 machine.

## Review items

These need a decision before they are built, since they would be new API or change approved behavior:

- A way for the hook to read a monster's level, race or element, or the spawn tags, such as `monsters.get_level(id)`. Today a hook knows only IDs.
- A way to read a map's stored spawns in Lua, so a mod or a test can check them without counting monsters.
- A dungeon style field for its spawn biome. Today a floor's biome is its tileset.
- The theme chance of 1 in 3 as a setting or a record field.
- Whether `spawns.modify` should be a flagged permission, as `rules.override` is.
