# 0041: World Layout

- Status: proposed.
- Date: 2026-10-16.
- Code: [server/layout](../../server/layout) (`layout.go`, `names.go`, `dev.go`, `atlas.go`, `options.go`, `records.go`, `core/data/naming_styles.yml`), [server/mapfolder](../../server/mapfolder), [server/fields](../../server/fields) (`layout.go`, `world.go`), [server/store](../../server/store) (`layout.go`, migration 00031, `queries/layout.sql`), [server/zone](../../server/zone) (`worldmap.go`, `gmcommands.go`, `npcs.go`), [server/modscript](../../server/modscript) (`towns.go`, `prelude.lua`, `luaapi/spec.go`), [server/staff/staff.go](../../server/staff/staff.go), [server/cmd/waserver](../../server/cmd/waserver) (`fields.go`, `config.go`, `main.go`), [tools/wactl](../../tools/wactl) (`world.go`, `layout.go`, `maps.go`), [tools/modkit](../../tools/modkit) (`generated.go`, `validate.go`), [schema/proto](../../schema/proto/wathena/v1) (`WorldMap`, `Memo`), [client](../../client) (`src/viewer/worldMap.ts`, `src/world/fields.ts`, `scripts/layout-check.ts`), [mods/examples/home-village](../../mods/examples/home-village), [e2e/layout_test.go](../../e2e/layout_test.go).
- Specs: [generator-v3](../specs/generator-v3.md), "World layout", and [map-folder-v1](../specs/map-folder-v1.md).
- Phase 7 step 4 (strategy section 15, "Phase 7 in full"), and proposal 0002, review decision 43.
- Lua API: `wa.towns`, `wa.Town`, `player:has_visited`, `maps.get_display_name` and `place:get_band`, as review decision 43 lists them.

## Context

Steps 2 and 3 made fields, towns and dungeons, and joined them with dev links for development (decisions 0039 and 0040). Step 4 makes a world's map graph at creation, names its towns, places a home area, and runs the warp services of strategy section 8.10. Review decision 43 fixed the shape. The open questions were:

- Where the layout is made, since it needs the mods' records: town templates, dungeon styles, naming styles, the chat filter and home maps.
- How portals of the graph meet the portals that maps make.
- How content reaches the graph's distances, which no approved Lua call gives.
- What becomes of step 3's dev links.

## Decision

### Where the layout is made

- `wactl world new --mode procedural` builds the layout and writes it with the world row, in one transaction. `--config waserver.yml` gives it the mods (`mods.dir`, `mods.approve`, `mods.disable`, `mods.ruleset`) and `procedural.layout`. Without it, it uses core's records and the defaults.
- It reads only the records a layout needs. Core's files are its dungeon styles, town templates, naming styles and chat filter, with no converted scripts, so the command stays fast.
- `waserver` reads the layout from the database and never makes one for a world row. A procedural world made before this step has no layout, and `waserver` refuses it with a message to make the world again.
- The database keeps each town's and floor's params, the canonical encoding of its template or style (generator-v3, "Inputs"). A mod that changes or leaves keeps the made maps as they were. A home map comes from its mod at each start, so that mod must stay installed.

### The graph

- A field's portal sits on each edge (generator-v2). Each end of a link names a place: the index-th place with a tag, such as a field's `portal:east` 0, a town's `portal` 2, or a floor's `entrance` 0. The server finds the cell when it generates the map.
- Borders between fields open with a chance of 3 in 4. Closed borders and the grid's outer edges leave free portals, where towns and dungeon entrances link. A town's gates link to free portals of distinct fields near its seat, so each town links to 2 to 4 fields, its template's gates. A dungeon entrance takes a free portal of a field.
- A layout that leaves a map unreachable from the home area, or a town without enough free portals, is tried again from `attempt(n)` of the layout node, at most 8 times. One of the six golden layouts needed a retry. An exploratory run of 500 default layouts found none that failed every attempt.
- Dungeon IDs are their style's name, then `-2` and on for more of one style, so `places.find("dungeon:crypt", ...)` still names crypts.
- Every core town template now has a `teleport` place: hamlet's and village's storehouse has `storage` and `teleport`, as the Kafra offers both.

### Bands

Bands rise with the warps from the home area to the farthest field or town, which gets the top band under the max level. Dungeon floors start at their entrance field's band and rise by their style's `level_step`. Normalizing to the farthest map spreads bands over levels 1 to 99 in a world of any size. A small world climbs fast: in a 2 × 1 dev layout, the first town is at levels 45 to 54.

### Names

- Naming styles are records of `data/naming_styles.yml`: `biome`, `syllables` by part, and `patterns`. Core has one per biome: norse for grassland, sylvan for forest, dune for desert, frost for snow, marsh for swamp and coast for beach. The last record of a biome, in load order, is its style, so a mod replaces core's by adding its own.
- Names follow strategy section 8.10: 4 to 12 letters, unique in any case with the home map too, the chat filter, and retries from `name(i)`.
- Other names come from the nearest town and English templates, "North Valheim Fields 2" and "Crypt of Skarvik F1", until locale files exist. They are derived, not drawn, so `@renametown` renames them with the town in the same transaction.

### Home area

- The home area is the first town, or a mod's map folder tagged `home` (map-folder-v1). `procedural.layout.home` picks one when several mods have one, or `none` for the first town.
- The home map's first `portal` place links to the first town's first gate, so the first town takes a template with 3 or more gates when one exists, and keeps 2 or more fields.
- New characters start on the home map's `start` place, or a town's `square`, and their save point is the home area until they save elsewhere.
- `wactl map validate <folder>` and `wactl mod validate` run the map folder's checks: the format, places on walkable cells, every place reached from the first one, and a home map's portal.

### Runtime

- Every map of the layout gets a zone at start, as before. Generation is fast enough not to need lazy zones: the default layout of 47 maps, with fields of 300 cells, generates in about 0.2 s on the development Mac, with a heap of 285 MB at start.
- A warp stands on each end's place and covers its 3 × 3 cells. A player arrives 3 cells inward from a field's portal, as between touching fields, else on a walkable cell 3 to 6 cells from the place that no warp covers.
- Save points and memo points store map IDs, which is what zones run generated maps by. Map rows carry `nomemo`. No layout map is removable yet, so only a home map can set it today, with its `flags`.
- Each character's entered maps are saved (`character_maps`), in their order, for the teleport service, `player:has_visited` and the world map.
- `/memo` is a new command, as rAthena's `pc_memo`: Warp Portal at level 2 or higher, 3 points, the newest first, one per map. Warp Portal itself is not ported yet, so memo points have no use in play.

### The teleport service

The price needs graph distances, which the approved Lua API does not give. So the server runs the service, as it runs shops: an NPC "Teleport Service" with the Kafra sprite at each town's `teleport` place, whose menu the server drives without a script. It offers Save, which sets the save point at the player's cell, each visited town but this one at 500 zeny plus 100 per warp between the towns, and Cancel. `procedural.teleport_base` and `procedural.teleport_step` change the prices. Mods cannot set an NPC's service.

### GM commands

- `@go` lists the towns by number and display name, and `@go <number, ID or name>` goes next to its teleport place, or its square. It needs `commands.warp` and writes `commands.go` audit rows.
- `@warp` takes a display name in any case, with spaces, before an optional cell.
- `@renametown <town> <name>` needs the new permission `world.rename`, which the default admin role has through `world.*`. The name follows the rules of town names. The rename saves the town and the maps named after it in one transaction, then the audit row `world.rename`.

### Lua

As review decision 43 lists: `wa.towns` with `list`, `list({ visited_by = player })`, `nearest` and `get`, the handle `wa.Town` with `get_id`, `get_name`, `get_map` and `get_band`, `player:has_visited(town)`, `maps.get_display_name(map)` and `place:get_band()`. All need `game.read`. Classic worlds have no towns and no display names.

### The client

- `WorldMap` comes after login and with each newly entered map: the visited maps with names and bands, and the maps their warps lead to without them. The World Map window (Ctrl+`) draws it as SVG at each field's grid cell.
- The map name shows the display name from the map's info, `displayName`.
- A map of kind `home` has no node, so the client downloads its WMAP.

### Dev worlds

`--fields`, `--towns` and `--dungeons` stay as a dev shortcut that uses the layout's code: `layout.Dev` turns them into a layout with the links of step 3's dev links, and the server runs it as a stored one, with names and bands. Its home area is the first field, where dev worlds started before. `--layout` builds a whole layout from `procedural.layout` and `--seed`. The test worlds of `wactl mod test` use `layout.Dev` too.

## Determinism

The 6 golden layouts of `server/layout/testdata/golden-layout.json` give the same checksums, town names and bands in Go on darwin/arm64 and on darwin/amd64 under Rosetta, which `scripts/check.sh gen` runs. Only the server makes layouts, so Node and the browsers do not run them. The maps the layout lists are the generator's, whose checks are unchanged.

## Tests and checks

- `server/layout`: `TestGoldenLayout`, `TestLayoutRules` (20 default layouts against the rules of review decision 43), `TestLayoutDeterministic`, `TestLayoutRetries`, `TestLayoutChecks`, `FuzzLayout`, and `TestParseNamingStyle`, `TestNaming`, `TestTownNames`, `TestDeriveNames`, `TestAtlas`, `TestBands` and `TestOptions`.
- `server/mapfolder`: `TestSampleHomeMap`, `TestRead`, `TestCheck` and `FuzzRead`.
- `server/fields`: `TestLayoutWarps` and `TestSetPlaces` on a dev layout.
- `server/store`: `TestWorldLayout` and `TestCharacterMaps`, with PostgreSQL.
- `server/zone`: `TestVisitedMaps`, `TestTeleportService`, `TestMemo` and `TestGoWarpAndRename`.
- `server/modscript`: `TestTowns` and `TestTownsClassic`.
- `server/cmd/waserver`: `TestFieldMaps` with a stored layout, and `TestAddMaps` with the dev links, the teleport NPCs and `--layout`.
- `tools/wactl`: `TestWorldLayout`, with the Home Village sample's home map.
- `mods/examples/home-village`: 3 `wactl mod test` cases of `wa.towns`.
- e2e, with PostgreSQL: `TestWorldLayout`. `wactl world new` makes the world with the Home Village sample. A new character starts on its start place, walks through its portal into the first town, saves at the teleport service, visits the second town, teleports there for its price, dies to the fixture's new Slayer, and comes back at its save point. Its entered maps are in the database.
- Browsers: the check `layout` starts on the home map, which the page downloads, walks into the first town, checks both display names, and opens the world map.

## Lua API

The calls of review decision 43, and nothing else. Proposal 0002, next step 29, has the details to review.

## Left

- Rotating dungeons and world change events, which move players, save points and memo points (step 7). No map of a layout is removable yet.
- Warp Portal itself, which would use the memo points.
- Locale files for the name templates.
- Field spawns by band (step 5), and service NPCs from templates (step 6).
- A run on a real amd64 machine.

## Review items

These need a decision before they are built, since they would be new API or change approved behavior:

- A Lua way to read graph distances or teleport prices, such as `town:get_distance(other)`, so a mod could write its own teleport service. Today the server runs it.
- A record field for server services on NPCs, so mods can place or restyle the teleport service.
- A `wa.testing` way to start a test player on a mod's home map. Today test worlds have no home map.
- The band formula: bands normalized to the farthest map climb fast in small worlds.
- Whether `@renametown` names should allow spaces or more than 12 letters.
- Whether the data export should hold a character's entered maps and memo points.
