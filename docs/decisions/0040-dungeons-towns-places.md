# 0040: Dungeons, Towns and Named Places

- Status: proposed.
- Date: 2026-10-16.
- Code: [shared/gen](../../shared/gen) (`maps.go`, `v3.go`, `v3dungeon.go`, `v3town.go`, `wasm`), [server/fields](../../server/fields) (`maps.go`, `records.go`, `core/data`), [server/places](../../server/places), [server/mod](../../server/mod) (`generated.go`, `npcs.go`), [server/gamedata/mods.go](../../server/gamedata/mods.go), [server/modscript](../../server/modscript) (`places.go`, `prelude.lua`, `luaapi/spec.go`), [server/zone/scripts.go](../../server/zone/scripts.go), [server/cmd/waserver](../../server/cmd/waserver) (`fields.go`, `config.go`, `main.go`), [tools/modkit](../../tools/modkit) (`generated.go`, `test.go`, `validate.go`), [tools/convert/core.go](../../tools/convert/core.go), [internal/assets/placeholder](../../internal/assets/placeholder), [internal/assets/build/sources.go](../../internal/assets/build/sources.go), [client/src/world](../../client/src/world) (`generator.ts`, `fields.ts`, `fieldGenerator.ts`, `fieldWorker.ts`, `gen.wasm`), [client/src/viewer/main.ts](../../client/src/viewer/main.ts), [client/scripts](../../client/scripts) (`places-check.ts`, `browser-check.ts`, `game-check.ts`), [mods/examples/market-town](../../mods/examples/market-town), [scripts/check.sh](../../scripts/check.sh).
- Spec: [generator-v3](../specs/generator-v3.md).
- Phase 7 step 3 (strategy section 15, "Phase 7 in full"), and proposal 0002, review decision 42.
- Lua API: `wa.places` and `wa.Place`, as review decision 42 lists them. No protocol change: the maps' data goes over HTTP.

## Context

Steps 1 and 2 built the generator core and generated fields (decisions 0038 and 0039). Step 3 adds the other two kinds of generated maps, dungeons and towns, and the named places that content uses instead of coordinates (strategy sections 8.5 and 8.6). Review decision 42 fixed the shape: dungeon styles and town templates as records, places with tags and cells, `wa.places`, NPC records with `place`, and a dev world. The open questions were:

- How a client gets a mod's town template or dungeon style, since it generates the map itself.
- How walls and buildings look with the placeholder art, and how a building's art fits its footprint.
- How towns and dungeons join the fields before the map graph of step 4.
- How `wactl mod test` runs a case on a generated map.

## Decision

### Generator version 3

- Version 3 is a new `Generator` in the registry. Its fields are version 2's, with places. `Map` makes a dungeon floor or a town from a node and params, and `Kinds` lists `dungeon` and `town`. Versions 1 and 2 stay as they were.
- New worlds get version 3. A world on version 2 runs fields only, and refuses `--towns` and `--dungeons` with an error.
- Integer and Q16.16 math only, no map types, and no clock. `TestGuard` covers the new files.

### Params: records the client generates from

A town template or a dungeon style is a record of the server's data, which mods change and add. The client must generate the same map, so the server sends the record's values with the map's node, as params: a canonical byte encoding that `shared/gen` writes and reads (spec, "Inputs"). The client passes the bytes to the module unread, and the module checks them as the server does. A mod's template then reaches every client without a client data table and without a new protocol message.

### Seed tree and IDs

- A town's node is `child(public, "town", n)`, its ID `t<n>` and its map ID `gen:town:t<n>`. A dungeon's floors are `child(child(child(public, "dungeon"), id), "floor", f)`, with map IDs `gen:dungeon:<id>:<f>`.
- A place's area is its map ID without `gen:`, so `get_area()` gives `town:t7`, `dungeon:crypt:2` or `field:3:-1`, as review decision 42 writes them.
- Strategy section 8.10 wrote `gen:town:7`. The town's map ID carries its ID, `t7`, so that the area and the scope `town:t7` name the same thing.

### Blocks

Dungeon floors and towns are built on blocks of 2 × 2 cells, a WMAP ground surface each. Walls are blocks raised 2 to 3 cells, and the WMAP's side tiles stand them up between surfaces at different heights. A wall's surface then covers exactly its blocked cells, and the existing renderer draws walls with no change. Fields keep their smooth terrain of shared vertices.

### Dungeons

- **Caves.** Cellular automata as section 8.5 says: 45% of blocks start blocked and 5 rounds run. The rule "blocked if 5 or more of its 8 neighbors are blocked" alone left caves 83% to 87% open, outside every reasonable range. The usual 4-5 rule, where a blocked block also stays blocked with 4 blocked neighbors, keeps the walls, and this version uses it. The largest area stays, small areas fill, and the rest join by tunnels.
- **Crypts, towers and ruins** use the rooms method: rooms without overlap, a minimum spanning tree by Prim's algorithm, and a few loops. They differ in their records: floor and room sizes, floors, and tileset.
- The exit is in the room farthest from the entrance by walkable blocks. On the last floor that room is the boss room, with an altar, and the floor has no exit. A cave's goal is its farthest open spot, and its last floor opens a round chamber there.
- **Level bands.** Each floor's band is `level_step` above the floor before it. The first floor's band comes from where the dungeon stands, which step 4's map graph decides. Until then it is levels 1 to 10. The band is server data: `/gen/maps/<id>.json` shows it, and the Lua API does not.
- **Styles** are records of `data/dungeon_styles.yml`: `method`, `floors`, `floor_size`, `room_size`, `tileset` and `level_step`. Core has cave, crypt, tower and ruins (`server/fields/core/data`).

### Towns

- A flat plateau inside a wall, or a hedge of trees when the template has no walls, with rough blocked ground outside.
- Main roads from each gate to the central square, side roads that branch from them, and lots along the streets.
- Buildings from the template in order. A lot's whole door side faces a street, so a building's places stand on the street at its door. A building with places goes near the square and must fit. Others fill lots while lots remain.
- Lamps along the main roads, the template's decorations on empty lots, a fountain on the square, and towers and gate arches on the wall.
- **Templates** are records of `data/town_templates.yml`: `size`, `gates`, `walls`, `tileset`, `buildings` and `decorations`. A building has `model`, `footprint`, `count` and `places`. Core has hamlet, village and city.
- Towns have IDs and no names. Names come with step 4, as review decision 42 says.

### Named places

- Every generated map records its places, each with tags and a cell (spec, "Named places"): portals and clearings on fields, the entrance, the exit or boss room, and rooms on dungeon floors, and the square, gates, portals and the buildings' tags in towns.
- A place has tags, not one tag. A gate is `gate` and `gate:south`, so content can ask for any gate or one side.
- Places are public layout: the generator makes them from public nodes, and `/gen/maps/<id>.json` lists them. The client makes them too but does not use them. The browser checks read them from the JSON to find their way.
- The server's zones share one index of every place of every generated map (`server/places`), in a fixed order: by map ID, then the generator's order.

### wa.places

- `places.find(scope, tag)` returns every match in the index's order. The scopes are `town:any`, `town:<id>`, `dungeon:any`, `dungeon:<id>` and `map:<id>`. Another form raises an error.
- `places.nearest(player, tag)` returns the place with the fewest warps from the player's map, then the shortest distance. On another map, the distance counts from the arrival cell of the first warp on the shortest way. Maps no warp reaches are left out.
- A place handle has `get_map()`, `get_position()`, `get_tags()` and `get_area()`. Both calls need `game.read`.
- A classic world has no index: `find` returns an empty list, and `nearest` nil.

### NPCs at places

- An NPC record may give `place: {scope, tag}` in place of `map`, `x` and `y`. The loader checks the scope and the tag, and refuses `map`, `x` or `y` beside it.
- The NPC stands at every matching place, once the server generated its maps, with the record's other fields. A shop at places is one shop: its stock is shared.
- In a classic world, or a world without a match, the NPC does not appear, and the server logs it at info level.

### Art

- The new tilesets are cave, crypt, tower and ruins for dungeons, and town. Their names differ from the field tilesets', so each texture or model name is one slot of one tileset.
- The placeholder pack gains ground textures for stone, brick, moss, plank, cobble and paving, the props of the new tilesets, and `placeholder/building/<w>x<d>`: a house with a pitched roof and a door, for every even footprint from 2 to 12 cells.
- A building's model is a slot of its template's tileset, such as `model/gen/town/smithy`. The tileset table maps it to `placeholder/building/<w>x<d>` by default, and the operator's table may point it at a model of the build. A building stands at scale 1, so its art is made to its footprint. A model has one footprint in every template, or the load fails.
- A building model may not share a name with a prop of its tileset.
- Mods name tilesets of the generator. A mod's own tileset is new API, a review item below.

### The server

- `--towns <n>` (`procedural.towns`) runs towns t1 to t<n>. Town n takes the template n - 1 modulo their number, in record order: core's, then the mods' in load order. `--dungeons <style>[:<floors>],...` (`procedural.dungeons`) runs a dungeon per style, with the style's floors when the count is left out. A dungeon's ID is its style's name without its mod prefix.
- With `--dev` and no world row, `--towns` and `--dungeons` make a procedural world, as `--fields` does. A world row's generator must make them.
- The towns and dungeons generate after the mods load, since the mods give their records. A failed map stops the start and names its node.
- **Dev links.** Until step 4's map graph, `DevLinks` joins the maps with warps: free field portals, those that touch no other field, are free ends in field order. Each town links its first gate's portal to the first free end, and its other portals become the first free ends. Each dungeon's first floor links its entrance to the first free end, and each floor's exit to the next floor's entrance. A map left without a free end is logged with a warning.
- The places index goes to every zone (`zone.Mods.Places`), and the NPCs placed by tag join their maps' zones.

### The client

- The map info adds `kind`, `params` and `name`. The worker runs `generate_map` with the params for a dungeon floor or a town, and the loader compares the checksum and falls back to the download as for fields.
- `viewer.fieldLoads()` records each map's kind, and its style or template in `biome`.
- `gen.wasm` grew from 146 to 228 KB: the town generator's sorting, the params' decoding and the new exports.

### Mod tests on generated maps

A `wactl` built with `-tags procedural` gives every test world of a mod the same small procedural world, from the secret `wactl-mod-test`: the field `gen:field:0:0` of 96 cells, a town per town template, and a dungeon per dungeon style, joined by the dev links. A case finds places with `wa.places` at any time, and `world:new_player { map = "gen:town:t4", ... }` puts the test world on a generated map, with the NPCs placed by tag. No `wa.testing` call is new. A `wactl` without the tag finds no places, and a case that names a generated map fails with a message that asks for the tag. `scripts/check.sh mods` builds `wactl` with the tag. `wactl mod validate` generates the test world's maps too, so a template that cannot make a town fails validation.

### The sample mod

`mods/examples/market-town` adds the town template `market_town` with a lamp shop, the dungeon style `cellar`, a Lamp Seller at every `shop:lamp`, and a Town Guide on every square, who finds the nearest blacksmith with `places.nearest`. Its tests run on the test world's generated maps.

## Performance

Generation on the development Mac (Apple M2). Go is the best of 20 nodes, and the others are the golden maps over two runs:

| Map | Go, darwin/arm64 | Node 26 | Chromium | WebKit | Firefox |
|---|---:|---:|---:|---:|---:|
| Cave floor, 128 × 128 | 1.4 ms | 4 to 6 ms | 4 to 8 ms | 3 to 7 ms | 31 to 35 ms |
| Crypt floor, 128 × 128 | 0.6 ms | 2 ms | 2 to 4 ms | 2 ms | 22 to 27 ms |
| Ruins floor, 160 × 160 | 0.8 ms | 3 ms | 4 to 5 ms | 3 to 4 ms | 32 to 40 ms |
| Crypt floor, 256 × 256 | 2.1 ms | 9 ms | 11 to 13 ms | 8 to 9 ms | 86 to 94 ms |
| Hamlet, 96 × 96 | 1.7 ms | 2 to 4 ms | 2 to 5 ms | 2 to 4 ms | 19 to 24 ms |
| Village, 128 × 128 | 5.9 ms | 9 ms | 9 to 12 ms | 10 to 11 ms | 59 to 62 ms |
| City, 192 × 192 | 30 ms | 22 ms | 32 to 40 ms | 35 to 38 ms | 262 to 274 ms |
| City, 256 × 256 | 58 ms | 42 ms | 43 to 59 ms | 40 to 42 ms | 485 to 531 ms |

- Section 8.5 budgets about a second in a Web Worker. Every dungeon floor and town is far within it in every engine. The slowest, a town of the largest size in Firefox, takes about half of it.
- The browser check measured the worker's generation of a hamlet at 27 to 72 ms and a crypt floor at 37 to 47 ms in Chromium, with the module's start, and the whole load at 144 to 338 ms.
- A town's time goes to the lots: each building looks for lots over the whole town, with prefix sums. 200 buildings in a town of 256 cells is the slowest case the records allow.

## Determinism

The 17 golden maps of `golden-v3.json`, with their WMAP and places hashes, and the golden files of versions 1 and 2, match on:

| Platform | Where |
|---|---|
| Go, darwin/arm64 | `scripts/check.sh gen` and `unit` |
| Go, darwin/amd64 under Rosetta | `scripts/check.sh gen` |
| TinyGo, native and wasip1 in wasmtime | `scripts/check.sh tinygo` |
| `gen.wasm` in Node 26 | `scripts/check.sh gen` and `client` |
| `gen.wasm` in Chromium, Firefox and WebKit | `scripts/check.sh browsers`, the check `gen` |

## Tests and checks

- `shared/gen`: `TestGolden` with version 3, `TestDungeonV3` (every floor of each core style from 4 dungeons: the entrance first, the exit or the boss room, every place reached, the WMAP's cells, and the same floor twice), `TestTownV3` (8 towns of each core template: the square first, a gate and a portal per gate, every building's places, all reached), `TestFieldV3Places`, `TestMapV3Retries`, `TestMapV3Inputs`, `TestParams`, `TestMapIDs`, `FuzzDungeonV3`, `FuzzTownV3`, `BenchmarkDungeonV3` and `BenchmarkTownV3`. No core floor or town of the tests needed a retry.
- `server/places`: `TestFind`, `TestParseScope` and `TestNearest`.
- `server/fields`: `TestCoreRecords`, `TestParseRecords`, `TestParseDungeonSpec`, and with the tag `TestGenerateMaps`, `TestDevLinks`, `TestSetPlaces`, `TestServeMaps` and `TestTilesetBuildings`.
- `server/mod`: `TestNPCPlaces`, and `TestSampleMod` counts the sample's NPCs placed by tag.
- `server/modscript`: `TestPlaces` and `TestPlacesClassic`, Lua tests of `wa.places`.
- `server/cmd/waserver`: `TestTownsConfig` and `TestAddMaps`.
- `tools/modkit`, with the tag: `TestMarketTown` and `TestGeneratedTestWorld`. `scripts/check.sh mods` runs the sample's 4 cases with `wactl mod test`.
- `internal/assets/placeholder`: `TestBuildings`, and the props and ground tests cover the new ones.
- The client: `generator.test.ts` runs version 3's golden maps, with their places, and `runRequest`. `fields.test.ts` loads a town and a dungeon floor from their params.
- `pnpm check:browsers` has the check `places`: a `--dev` world with a field, a hamlet and a crypt of two floors, and the Market Town mod. A player walks from the field into the town, which the page generates with the server's checksum, talks to the Town Guide that stands on the square by its place, and walks into the crypt's first floor, which the page generates too. The check `gen` compares version 3's checksums, WMAP and places hashes in all three engines.
- `scripts/check.sh gen` runs the dungeon and town tests on both architectures. `fuzz` runs `FuzzDungeonV3` and `FuzzTownV3`.

## Lua API

`wa.places` with `find` and `nearest`, and the handle `wa.Place` with `get_map`, `get_position`, `get_tags` and `get_area`, as review decision 42 lists them. Nothing else. Proposal 0002, next step 28, has the details to review.

## Left

- The map graph, town names, the home area, and the level bands of dungeons by where they stand (step 4). The dev links and the band of levels 1 to 10 stand in until then.
- Spawns in dungeon floors and boss rooms (step 5), and service NPCs from templates (step 6). Today a mod places NPCs by tag.
- Lua generation layers, mods' own tilesets and props, and interior maps of buildings (step 6, `wa.generation`).
- Cave chambers have no `room` places. Fields have clearings, but no ruins or camps.
- A placeholder building is a house of its footprint; it does not tell a smithy from an inn.
- Generated maps are not cached on disk. The server generates every map at each start: about 10 ms for the dev world of 2 towns and 5 floors.
- A run on a real amd64 machine and in Safari, as for steps 1 and 2.

## Review items

These need a decision before they are built, since they would be new API or change approved behavior:

- A `wa.testing` way to pick a test world's town template or map size. Today a case finds its town with `wa.places`.
- A mod's own tilesets and props, through records or `wa.generation`.
- The level band of a floor in Lua, such as `place:get_levels()`, for spawns and quests.
- Whether an NPC placed by tag in a world without a match is a warning or an error. Today it is an info line, since a classic world has no places at all.
