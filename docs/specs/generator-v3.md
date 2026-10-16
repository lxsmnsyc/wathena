# Map Generator Version 3, generator-v3

Generator version 3 adds dungeon floors, towns and named places (strategy sections 8.5 and 8.6, proposal 0002, review decision 42). The world layout (review decisions 43 and 44) and the spawns (review decision 44) build on it. This spec fixes their inputs, their seed tree, what they return, and how the server and the web client share them. A client from strategy section 10.7 that generates maps follows it.

- [generator-v1](generator-v1.md) still holds for world modes, version rules, the grid, its cell types and its checksum. [generator-v2](generator-v2.md) still holds for fields.
- The Go implementation is `shared/gen` (`maps.go`, `v3*.go`). The web client runs it as `client/src/world/gen.wasm`.
- The golden file is [`shared/gen/testdata/golden-v3.json`](../../shared/gen/testdata/golden-v3.json).
- Decisions [0040](../decisions/0040-dungeons-towns-places.md), [0041](../decisions/0041-world-layout.md) and [0042](../decisions/0042-spawns.md) have the reasons.

## Seed tree

```text
root (secret)
├── public
│   ├── climate
│   ├── field(x, y)                a field's public node
│   │   ├── version 2's children
│   │   └── places                 its clearings
│   ├── layout                     the world layout (decision 0041)
│   │   ├── edges, towns, dungeons
│   │   └── attempt(n)             a retry, n from 1 to 7
│   ├── town(n)                    a town's public node, n from 1
│   │   ├── roads, buildings, decorations, heights
│   │   ├── name(i)                a name attempt, i from 0 to 99
│   │   └── attempt(n)             a retry, n from 1 to 7
│   └── dungeon
│       └── <id>                   a dungeon, such as crypt
│           └── floor(f)           a floor's public node, f from 1
│               ├── rooms or cave, props, materials, heights
│               └── attempt(n)
└── secret                         only `wactl world new` holds it
    ├── spawns
    │   └── <map ID>               a field's or floor's spawns (decision 0042)
    │       ├── theme, count, species(k), counts, respawn, place
    │       ├── boss               a boss next to a town, or in a boss room
    │       └── field_boss         a region's field boss
    ├── regions
    │   └── <town ID>              a region's field bosses
    │       └── bosses
    └── hooks                      the random stream of the spawn_table hooks
```

- `town(n) = child(public, "town", n)`. The town's ID is `t<n>` and its map ID `gen:town:t<n>`.
- `dungeon(id) = child(child(public, "dungeon"), id)`, and `floor(f) = child(dungeon(id), "floor", f)`. A dungeon's ID is 1 to 32 lowercase letters, digits, `_` and `-`. A floor's map ID is `gen:dungeon:<id>:<f>`.
- A map's area is its map ID without `gen:`: `town:t7`, `dungeon:crypt:2` or `field:3:-1`.
- A client gets the node of each map it enters, never the public branch, so a floor's node does not reveal another floor's.

## Inputs

A field takes a node, a size and a biome, as in version 2. A dungeon floor or a town takes a node and its params: a byte string that the server encodes from a record and sends as hex. The client passes the bytes to the module unread.

### Dungeon params

```text
u8 1, u8 1              kind (dungeon) and params version
u8 method               0 cave, 1 rooms
u16 width, u16 height   the floor's size in cells, big-endian
u16 room_min, room_max  room sides in cells
str tileset             u8 length and bytes
u8 floors, u8 level_step   the style's values
u8 floor, u8 of         this floor, from 1, and the dungeon's floors
```

### Town params

```text
u8 2, u8 1              kind (town) and params version
u16 size                the side in cells
u8 gates, u8 walls      2 to 4; 0 or 1
str tileset
u8 n, n × str           the decorations
u8 n, n × building      the buildings, in order
building: str model, u8 width, u8 depth, u8 count, u8 k, k × str tag
```

The params must pass the record checks below. A decoder refuses bytes that are cut short, too long, or fail a check. The longest params are 16 KiB.

| Record | Checks |
|---|---|
| Dungeon style | `method` is cave or rooms. `floors` 1 to 20. The floor's sides are even, 48 to 512. Room sides are even, 6 to 40, the smaller first, and the largest at most a third of the floor's smaller side. `tileset` is 1 to 32 lowercase letters, digits and `_`. `level_step` 1 to 50. A floor is 1 to the dungeon's floors, at most 99. |
| Town template | `size` is even, 64 to 256. `gates` 2 to 4. 1 to 32 buildings, with counts that add up to at most 200, and at most 8 decorations. A building's model is a slot name, its footprint is even, 2 to 12 cells on each side, its count 1 to 64, and it has at most one place per 2 cells of its width. Tags are 1 to 64 lowercase letters, digits, `_`, `-`, `.` and `:`. |

## Blocks

Dungeon floors and towns are built on a grid of blocks of 2 × 2 cells: block (i, j) holds cells 2i to 2i + 1 and 2j to 2j + 1. A block is the size of a WMAP ground surface, so each block is one surface. Each block has:

- whether it is walkable,
- a flat height, which its 4 cells' corners take,
- a texture of the tileset,
- at most one prop.

Every cell is walkable (0) or blocked (1). The water level is -16 cells, below every map.

## Tilesets

| Tileset | Kind | Floor, accent, wall | Props |
|---|---|---|---|
| cave | dungeon | dirt, mud, rock | stalagmite, rock |
| crypt | dungeon | stone, moss, brick | pillar, coffin, bones, brazier |
| tower | dungeon | plank, stone, brick | pillar, crate, brazier |
| ruins | dungeon | cobble, grass, rock | pillar, rock, bush |
| town | town | lots grass, main roads paving, side roads dirt, square cobble, walls stone, outside grass | fountain, lamp, tower, gate, tree, bush, well, crate |

Every dungeon tileset also has the props `stairs_up`, `stairs_down` and `altar`. Walls stand 3 cells high, 2 in ruins and 2.5 in towns. `shared/gen/v3.go` holds the full table, with the clutter weights and the light. A town template's building models join the town tileset's model slots (decision 0040, "Art").

## Dungeon floors

An attempt runs from its node `a`.

### Rooms

1. **Rooms.** The stream of `child(a, "rooms")` draws rooms. Each has sides from `room_min / 2` to `room_max / 2` blocks, then a place with a block of margin from the floor's edge. A room that comes within one block of another is dropped. It aims at `clamp(blocks / (avg² × 6), 4, 40)` rooms, where `avg` is the mean side in blocks, and stops after 30 times that many draws. Fewer than 3 rooms fail the attempt.
2. **Joins.** Prim's algorithm joins the rooms in a minimum spanning tree from the first room, by the Manhattan distance between the rooms' middle blocks, ties to the lower indexes. Then the other pairs, nearest first, ties by index, each join with a chance of one in three from the stream, up to a fifth of the rooms, at least one, and none longer than three quarters of the floor's width plus height.
3. **Corridors.** Each join is an L of one block wide between the middle blocks: across then up, or up then across, by the stream.
4. **Entrance and goal.** The first room holds the entrance. The room whose middle is farthest from it by walkable blocks, ties to the first, holds the exit, or the boss room on the last floor.

### Caves

1. **Fill.** Inside a border of one block, each block starts blocked with a chance of 45 in 100 from the stream of `child(a, "cave")`.
2. **Rounds.** 5 rounds of the rule: a block becomes blocked when 5 or more of its 8 neighbors are blocked, and a blocked block stays blocked with 4.
3. **Areas.** Open blocks join along the 4 axes. The largest area stays, ties to the first. Areas below 6 blocks are filled. Each other area gets a tunnel: the way one breadth-first search from the largest area finds to the area's first block.
4. **Entrance and goal.** The entrance is a block whose 8 neighbors are open, picked by the stream. The goal is the farthest such block, ties to the first. A goal nearer than a sixth of the width plus height in blocks fails the attempt. On the last floor, a circle of radius 4 blocks around the goal opens as the boss chamber.

### Props, textures and heights

- **Places and stairs.** The entrance gets `stairs_up`, and the exit `stairs_down`. Neither blocks. The blocks of the places and their 8 neighbors are kept free of other props.
- **Props** from the node `child(a, "props")`. Each blocks its block and needs its 8 neighbors open, so no prop splits a walkable area:
  - On the last floor, an `altar` two blocks north of the boss room's middle.
  - The tileset's pillar at the four inner corners of rooms of 6 or more blocks on each side.
  - Clutter on an open block when `hash(props, x, y, 0) mod 100 < 4`, picked by the hash from the tileset's weighted clutter.
- **Textures.** A blocked block without a prop takes the wall texture. Others take the floor texture, or the accent where a value noise of `child(a, "materials")` with a spacing of 8 blocks is above 0.7.
- **Heights.** Rooms' floors are flat at 0, and walls stand at the wall height. A cave's blocks roll by a value noise of `child(a, "heights")` times 3/4 of a cell, in steps of 1/8, and its walls stand that much above the wall height.

## Towns

An attempt runs from its node `a`. With B the side in blocks, the margin is 4 blocks and the wall, or a hedge when the template has no walls, is the ring of blocks 4 from the edge. The middle block is `c = B / 2`, and the square's half side `max(2, B / 14)` blocks. Main roads are 1 block wide, or 2 from a side of 64 blocks.

1. **Square.** The square's blocks are paved. A fountain fills the 2 × 2 blocks at the middle. The square's place is the cell (2c, 2c - 3), just south of it.
2. **Gates** from the stream of `child(a, "roads")`. 4 gates take every side. 3 gates leave out the side the stream picks. 2 gates take south and north, or east and west, by the stream. Each gate's lateral block is `c` plus a draw from minus to plus a sixth of the inner width, kept 2 blocks from the corners.
3. **Main roads.** Each runs from the map's edge, through its gate, inward. When its lateral block is outside the square's, it turns at a row the stream picks, runs across to the square's columns, and turns inward again to the square.
4. **Side roads.** Along each main road's straight parts, every `2d + 3` or `2d + 4` blocks by the stream, where `d` is the buildings' mean side in blocks, rounded up, a side road leaves on both sides and runs over empty lots until it meets anything else or the wall.
5. **Buildings** from the stream of `child(a, "buildings")`, in the template's order, each `count` times. A lot is a rectangle of empty blocks inside the wall, a block away from other buildings, with the whole side its door faces on a street: a main road, a side road or the square. The door faces south, east, north or west, and the footprint turns with it. A building with places takes one of the 6 lots nearest the square, by the stream. One without takes any lot, while lots remain. A building with places that finds no lot fails the attempt.
6. **Places at doors.** Each of a building's tags is a place on the street cell next to the door: the door's middle first, then 2 cells to one side, 2 to the other, 4 to the first, and on.
7. **Decorations** from the node `child(a, "decorations")`:
   - A `lamp` beside the main roads every 4 blocks, on alternate sides.
   - The template's decorations on half the Poisson-disk points 4 blocks apart, picked by the hash.
   - `tree` props on the Poisson-disk points 2 blocks apart outside the wall, from `child(child(a, "decorations"), "outside")`.
   Each lamp and decoration blocks its block and needs its 8 neighbors walkable.
8. **Walls.** With walls, a `tower` stands at each corner of the ring and a `gate` arch in each gate. Neither blocks: the ring's wall blocks already do.
9. **Heights.** The plateau inside is flat at 0. Walls stand at the wall height. The ground outside rolls by a value noise of `child(a, "heights")` times 3/4 of a cell, in steps of 1/8, and is blocked but for the roads.

## Named places

| Map | Places, in this order |
|---|---|
| Field | each portal, as `portal` and `portal:<edge>` with edge south, east, north or west, then up to 8 `clearing`s: Poisson-disk points 24 cells apart from the stream of `child(a, "places")`, whose 7 × 7 cells are walkable and dry, at least 12 cells from every portal |
| Dungeon floor | `entrance`, then `exit` or, on the last floor, `boss_room`, then `room` at the middle of each other room |
| Town | `square`, then for each gate `gate` and `gate:<side>` just inside the wall and `portal` and `portal:<side>` 3 cells from the map's edge, then each building's tags in the order it was placed |

A place's cell is a dungeon block's south-west cell, or the cell the rules above name. The client does not need places, but makes them, so the golden files can hash them. The canonical encoding (`gen.EncodePlaces`) is, for each place: its x and y as big-endian u16 values, its tag count as a u8, and each tag with a u8 length.

## Validation and retries

| Map | An attempt passes when |
|---|---|
| Dungeon floor | the walkable share is 10% to 75%, and at least 25% in caves, and every place's block is reached from the entrance's |
| Town | every building with places has a lot, every place's block is reached from the square's, and at least 25% of the town is walkable |

Attempt 0 uses the map's node, and attempt n uses `child(node, "attempt", n)`. After 8 failed attempts the map is the error `ErrNoMap`, which lists each attempt's problem. The server does not start with such a map and logs its public node.

## Output

- The grid of generator-v1: each block's 4 cells take its walkability and its height on all 4 corners. Its checksum is generator-v1's.
- The props and the places.
- The map as WMAP ([assets-v1](assets-v1.md), "Map (WMAP)"), the same bytes on every platform:
  - `CELL`: the cells.
  - `GRND`: one surface per block, with all 4 corners at its height. The top tile is the block's texture. The north and east tiles are the side tile, of the wall texture, toward a neighbor at another height. One top tile per texture in use, in the order of the tileset's sorted textures, then the side tile.
  - `LMAP`: `placeholder/lightmap`. `LGHT`: the tileset's light.
  - No `WATR`.
  - `OBJS`: each prop's model `model/gen/<tileset>/<prop>`, turned and scaled, at its point in half cells and the height of the block there. A building stands at its footprint's middle, at scale 1.
  - `SNDS` and `EFCT` are empty. `DEPS` lists every name, sorted.

## World layout

`wactl world new` makes a procedural world's layout, the map graph of strategy section 8.10, from the public branch alone (proposal 0002, review decision 43, [decision 0041](../decisions/0041-world-layout.md)). The database keeps it, so a later change to these rules or to a record never changes a made world. The rules below are those of `server/layout`, which only the server runs. They use integer math and slices, and the golden layouts of `server/layout/testdata/golden-layout.json` hold on darwin/arm64 and darwin/amd64. The map generator's version does not change.

### Nodes

- `layout = child(public, "layout")`. Attempt 0 uses it, and attempt n `child(layout, "attempt", n)`, up to 8 attempts, then the error `ErrNoLayout` with each attempt's problem.
- Each attempt draws from the streams of its children `edges`, `towns` and `dungeons`.
- Town n's name attempt i is `child(town(n), "name", i)`, from 0 to 99, independent of the layout's attempt.
- Each field's biome is the climate of generator-v2 at its coordinates.

### Steps of an attempt

The config gives a grid of W × H fields, T towns and D dungeons: 6 × 5, 5 and 6 by default. Field (x, y), for x from 0 to W - 1 and y from 0 to H - 1, has a portal on each edge (generator-v2), named by its place `portal:south`, `portal:east`, `portal:north` or `portal:west`.

1. **Edges** (review decision 44), from the `edges` stream. The streams of this section are xoshiro256\*\*, as all public streams.
   - The root: `IntN` over the fields, by row then column. It is the first town's seat.
   - A tree of open borders grows from the root in layers. The root is layer 0. Each next layer draws its size: 3 when `IntN(3) == 0`, else 2. For each pick it lists the fields not yet reached that touch a field of the layer before, keeps those with the fewest neighbors not yet reached, and draws one by `IntN` in row and column order. The picked field opens its border to a neighbor of the layer before, drawn by `IntN` in edge order (south, east, north, west). The growth stops when no field touches the last layer.
   - Each field still unreached, by row and column, again until none is left, opens its border to its reached neighbor of the highest layer, the first by edge on a tie.
   - Loops: each closed border whose two fields' layers differ by at most 1 opens when `IntN(2) == 1`. The borders between columns go first, row by row from y = 0, then those between rows.
   - An open border links the two portals that face each other, the field to the west or south first. The other portals are free.
2. **Towns**, by number from the `towns` stream:
   - A template: `IntN` over the templates in record order. With a home map, the first town draws among templates with 3 or more gates when one exists.
   - A seat. The first town's is the root. Each other town shuffles the fields at least 2 grid steps from every earlier seat, else 1, else any, by `Shuffle`, and takes the first whose gates all find fields.
   - Gates: each gate's portal links to a free portal of a different field, the first free one by edge. The fields come by their warps from the seat over open borders, at most 2, then by row and column. For a town after the first, a field joins only while the town's fields stay within 2 warps of each other from the first town, so a town never shortens a way. With a home map, the first town's first gate links to the home map's first `portal` place instead. A town whose every seat finds too few fields fails the attempt.
3. **Dungeons**, by number from the `dungeons` stream: a style by `IntN` over the styles, then a field by `IntN` among the fields with a free portal and no dungeon and no town seat, else with no dungeon, else any with a free portal. Its first free portal, by edge, links to the first floor's `entrance`, and each floor's `exit` to the next floor's `entrance`. A dungeon's ID is its style's name without the mod prefix, then `-2`, `-3` and on for more of one style. No free portal fails the attempt.
4. **Reach.** Every map must be reached from the home area over the links, or the attempt fails.
5. **Bands**, as below, then **coverage**: with W the band width, the bands of levels 1 to W, W + 1 to 2W and on, up to level 99 or the max level, each need 3 fields whose band overlaps them, or the attempt fails. A grid with fewer than 3 fields per such band skips the check.

A link's end names a place: the index-th place, from 0, with a tag, such as a town's `portal` 2 or a field's `portal:east` 0.

### Bands and names

- **Bands** (review decision 44). Distances count warps from the home area: the home map, or the first town. With d0 the fewest warps of any field, a field, a town or the home map at distance d starts at level 1 + step × max(0, d − d0), at most top = max level − band width + 1, and its band spans the band width. The defaults are a width of 10, a step of 8 and a max level of 99, so the home area and its nearest fields are levels 1 to 10, and each further warp adds 8 levels up to 90 to 99. A floor starts at its entrance field's lowest level plus (floor − 1) × the style's `level_step`, at most top.
- **Town names.** Town n takes the naming style of its seat's biome: the last record for that biome, in load order. Attempt i draws a pattern by `IntN` from the stream of `name(i)`, then a syllable by `IntN` for each part of the pattern. The name is the syllables in lower case but its first letter. It passes with 4 to 12 ASCII letters, no other town or the home map with the same name in any case, and no word the chat filter blocks. After 100 attempts the layout fails with `ErrNoName`.
- **Other names.** Each field and floor takes the town the fewest warps away, ties to the lower number. A field is "<direction> <town> Fields", with the direction by the larger grid step from the town's seat, north or south on a tie, and none on the seat. A dungeon is "<style> of <town>", with the style's name in words, and each floor adds " F<floor>". Fields or dungeons that share a name get a number in map order. A rename of a town names them again.

### Dev worlds

`waserver --dev` with `--fields`, `--towns` and `--dungeons` makes a layout without a stream, with the links of step 3's dev links: touching fields link, the portals that touch no field are free ends in field ID order, each town by number links its first portal to the first free end and its other portals become the first free ends, and each dungeon by ID opens from the next free end. Its home area is its first field. `waserver --dev --layout` builds a layout as above from `--seed`.

## Spawns

`wactl world new` picks each field's and dungeon floor's spawns from the secret branch, `child(root, "secret")`, after it makes the layout and generates every map (strategy section 8.11, proposal 0002, review decision 44, [decision 0042](../decisions/0042-spawns.md)). The database keeps them. The rules below are those of `server/spawns`, which only `wactl` runs. The streams are ChaCha8 (rng-v1's secret streams), and the golden worlds of `server/spawns/testdata/golden-spawns.json` hold on darwin/arm64 and darwin/amd64.

### Inputs

- Monsters: every monster with stats and spawn tags, by name. Core's tags are `server/spawns/core/data/spawn_tags.yml`, which `tools/spawntags` derives from rAthena's spawn files. A monster record's `spawn` replaces them field by field. A boss, mob_db's boss class or an MVP, is never common or uncommon.
- Themes: the records of `data/spawn_themes.yml`, by name.
- Each map: its biome (a field's, or a floor's tileset), band, walkable cells (ground and water), features and feature points (the first water cell of each 32 × 32 block and each `clearing` on fields, each `room` on floors), the arrival cell of every warp to it, the warps on it that lead to a town, its `boss_room`, whether a field is one warp from a town, and its region: the town the fewest warps away, ties to the lower number.
- Settings: `spawns.town_field_boss_chance` (50), `spawns.town_field_boss_role` (`mini_boss`) and `spawns.density.field` and `.dungeon`, monsters per 10,000 walkable cells (rAthena's medians by default).

### One map

With `m = child(child(secret, "spawns"), <map ID>)`:

1. **Theme.** From `child(m, "theme")`: when `IntN(3) == 0`, a theme by `IntN` among those, by name, whose biomes hold the map's biome or are empty. A monster matches it by race, by element, or by naming it in its tags, and weighs `boost` percent of its weight.
2. **How many.** `5 + IntN(4)` species, from `child(m, "count")`.
3. **Species**, from `child(m, "species", k)` for the k-th of 14 tiers: the map's biome within 0, 5 and 10 levels of the band, then any biome within the same, then 15 and 20 levels, the biome first, then 30, then 50. The candidates of a tier are the common, uncommon, plant and rare monsters within it, by name, weighing their biome weight (1 for another biome) times their closeness: 100 inside the band, 4 less per level outside, at least 10. The draw takes 2 commons, then the rest from every candidate, each by `Weighted` without putting back, and skips a draw that would make 4 species of one element, 3 aggressive species in a band below level 30, or 2 rares. A tier that ends with fewer than 2 commons or 5 species passes to the next.
4. **Counts**, from `child(m, "counts")`: the total is density × walkable cells / 10,000, at least 2 per species. Each rare draws `Range(1, 5)` and each plant `Range(3, 8)`, cut in proportion to a quarter of the total, at least 1 each. Commons and uncommons share the rest: with uncommons, commons take `c / (c + u)` of it for `c = Range(60, 70)` and `u = Range(25, 35)`. Each species of a role draws a part `Range(2, 5)` and takes its share, at least 1, the last the remainder.
5. **Respawns**, from `child(m, "respawn")`, in whole seconds: common 5, uncommon `Range(5, 60)`, plant `Range(180, 1800)`, rare and mini boss `Range(1800, 3600)`, field boss `Range(3600, 7200)`.
6. **Areas**, from `child(m, "place")`: a plant spawns within 8 cells of a feature point drawn by `IntN`, others anywhere. An aggressive monster keeps 15 cells from every arrival, by the larger axis distance.

### Bosses

- A field one warp from a town: from `child(m, "boss")`, when `IntN(100)` is below the chance, a boss of the setting's role.
- A dungeon's last floor: from `child(m, "boss")`, a field boss within 3 cells of its `boss_room`.
- Each region's other fields, by town number: from `child(child(child(secret, "regions"), <town ID>), "bosses")`, `1 + IntN(2)` of them, the first after a `Shuffle`, each get a field boss from `child(m, "field_boss")`.
- A mini boss comes from the band's lowest level to 20 above its highest, a field boss from its lowest to 30 above its highest, by `Weighted` on its biome weight plus 1. Without one there, the closest mini boss by level, or the lowest field boss above the band, else the highest. Bosses keep 30 cells from every warp to a town.
- A list sorts by role (common, uncommon, plant, rare, mini boss, field boss), then by count from the largest, then by name.

### Hooks

Then the mods' `spawn_table` hooks run on each list, in map order, in a Lua state that ran the mods' entry scripts on no map, with the random stream `child(secret, "hooks")`. The server keeps its rules over their lists: each line's clear cells follow its monster and role, and a boss as common or uncommon is left out.

## Server endpoints

`GET /gen/maps/<id>.json` gives, besides version 2's fields:

| Field | Content |
|---|---|
| `kind` | `field`, `dungeon`, `town`, or `home` for a mod's home map ([map-folder-v1](map-folder-v1.md)), which has no `key` and which a client downloads. |
| `params` | A floor's or a town's params, as hex. |
| `name` | The style's or the template's record name. |
| `town` | A town's number. |
| `dungeon`, `floor`, `floors` | A floor's dungeon ID, its floor and the dungeon's floors. |
| `levels` | The map's level band: the lowest and the highest level. Every map of a world layout has one. |
| `displayName` | The map's display name, from the world layout. |
| `places` | The map's places, each `{tags, x, y}`. |

The tileset table at `/gen/tilesets.json` holds the dungeon and town tilesets too.

## Client

As for fields, with the kind and params:

1. Fetch the map's JSON.
2. Generate the map from the node, and for a floor or a town from its params, in a Web Worker.
3. Use the map's WMAP when the grid's checksum matches the server's, else download it.

## WebAssembly module

The module adds:

| Export | Use |
|---|---|
| `params() → ptr` | The 16 KiB buffer for a floor's or a town's params. |
| `generate_map(version, kind, length) → code` | Makes a dungeon floor (kind 1) or a town (kind 2) from the key and the first `length` bytes of the params. 0, -1 for a version without the kind, -2 for params that do not decode, -3 when every attempt failed, -4 for a tileset or prop the version lacks. |
| `width() → n`, `height() → n` | The size of the last map. |
| `places() → ptr`, `places_size() → n` | The last map's places in their canonical encoding, or 0 bytes. |

## Golden file

`golden-v3.json` adds to each map of generator-v2's format:

- `kind`, `params` and `name` for a floor or a town.
- `town`, or `dungeon` and `floor`: the map's node below the public branch of `secret`.
- `places`: the SHA-256 of the places' encoding.

The maps cover two fields, the first and the last floor of each core style, a 256 × 256 floor, each core template, a town of the largest size, and a floor and a town from an all-zero key.
