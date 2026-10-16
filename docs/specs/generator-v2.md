# Map Generator Version 2, generator-v2

Generator version 2 makes fields from biome rules (strategy section 8.5). This spec fixes its inputs, its seed tree, what it returns, and how the server and the web client share a field. A client from strategy section 10.7 that generates fields follows it.

- [generator-v1](generator-v1.md) still holds for world modes, version rules, the grid, its cell types and its checksum. Version 1 stays callable.
- The Go implementation is `shared/gen` (`v2*.go`). The web client runs it as `client/src/world/gen.wasm`.
- The golden file is [`shared/gen/testdata/golden-v2.json`](../../shared/gen/testdata/golden-v2.json).
- Decision [0039](../decisions/0039-generated-fields.md) has the reasons.

## Seed tree

```text
root (secret: the world row's seed)
├── public                         the public branch
│   ├── climate                    biomes of fields, server only
│   └── field(x, y)                a field's public node, sent to clients
│       ├── terrain, warp, rivers, portals, thickets, objects, density, materials
│       └── attempt(n)             a retry, n from 1 to 7, with the same children
└── secret                         spawns, loot and other server branches (later steps)
```

- `public = child(root, "public")`. `wactl world new` stores its key in the world row's `public_node`. The game role reads it and never the seed.
- `field(x, y) = child(public, "field", x, y)`. The map ID of the field is `gen:field:<x>:<y>` (strategy section 8.10).
- A field's node does not reveal the public branch, another field's node, or the secret branch.

## Inputs

- The field's public node.
- The width and height, each from 32 to 1024 cells.
- The biome's index in the table below. The server picks it from the climate or from its config, and sends it with the node.

## Biomes

Heights and lengths are in cells.

| Index | Name | Height | Water | Cliffs | Walkable share | Props |
|---:|---|---|---|---|---|---|
| 0 | grassland | 5 octaves, amplitude 3, terraces of 2 | 6% below the level, 1 river | above 0.6, gaps | 55% to 85% | tree, bush, rock; thickets of tree, 12% |
| 1 | forest | 5 octaves, amplitude 5, terraces of 3 | 5%, 2 rivers | above 0.7, gaps | 45% to 80% | tree, pine, bush; thickets of tree, 25% |
| 2 | desert | 4 octaves of ridged noise, amplitude 4, flat basins | none | above 0.7, gaps | 55% to 95% | cactus, rock; thickets of rock, 4% |
| 3 | snow | 6 octaves, amplitude 9, terraces of 3 | 3%, 1 river | above 0.9, gaps | 40% to 80% | pine, rock; thickets of pine, 10% |
| 4 | swamp | 4 octaves, amplitude 1 | 28%, walkable to 1.5 deep | above 0.5, blocked | 60% to 95% | deadtree, reed; thickets of deadtree, 8% |
| 5 | beach | 5 octaves, amplitude 1.5, a slope of ±4 toward one edge | 30% | above 0.6, gaps | 45% to 85% | palm, rock; thickets of palm, 6% |

The table in `shared/gen/v2biomes.go` is the full rule set: octave spacings, warp amplitudes, shallow depths, prop spacing and density, ground materials and light.

## Field steps

Each attempt runs these steps from its node `a`:

1. **Height.** Each vertex samples fractal gradient noise of `child(a, "terrain")` at a point moved by two fields of warp noise of `child(a, "warp")`. Gradients are one of 8 directions with parts -1, 0 or 1, picked by the top 3 bits of the hash of the lattice point. Samples use the quintic fade. Each octave has half the amplitude and half the spacing of the one before. Then terraces, basins and the beach slope apply.
2. **Water.** The water level is the cell mean height below which the biome's share of cells lies, rounded up to 1/64 cell. Rivers start on cells in the top 20% of heights, picked by the stream of `child(a, "rivers")`, and step to the lowest unvisited neighbor until they reach water or an edge. Their bed is half the shallow depth under the level.
3. **Portal areas.** One portal on each edge, 4 cells in, at a place from the stream of `child(a, "portals")`. A circle of radius 4 around it is flat and above the water.
4. **Cells.** Below the level minus the shallow depth: blocked. Below the level: walkable water. Corners that differ by more than the slope limit: the biome's cliff type. Else walkable.
5. **Thickets.** Dry walkable cells where a value noise of `child(a, "thickets")` is highest, the biome's share, are blocked. Portal areas stay open.
6. **Connectivity.** Walkable cells connect along the 4 axes. Every portal, and every walkable area of 200 cells or more, that the first portal does not reach gets a road: the cheapest way to the first portal's area by one Dijkstra search, with costs for slopes, cliffs and deep water. A road's heights rise at most a third of the slope limit per cell. The search runs up to 8 rounds, with at most 24 roads in all. Walkable cells that still do not reach the first portal become blocked.
7. **Objects.** Props stand on Poisson-disk points (Bridson's algorithm with integer offsets) from the stream of `child(a, "objects")`. Thickets get a prop every 3 cells or so. Elsewhere a point becomes a prop when its cell and its 8 neighbors are dry walkable ground, off roads and portal areas, and its hash passes the density there. Each prop blocks its cell. Since its 8 neighbors stay walkable, a prop never splits a walkable area.
8. **Materials.** Roads, then water and its shore within 2 cells, then rock where corners differ by more than half the slope limit, then patches over the biome's ground.

Banks of rivers, portal areas and roads slope at a fixed rate: each vertex stays within that rate per cell of the seed heights, by chamfer distance (2 along an axis, 3 along a diagonal), in two passes over the vertex grid.

Version 2 does not place named places yet, and does not use global coordinates or a fallback template (decision 0039, "Left").

## Validation and retries

An attempt passes when:

- every portal reaches the first one, and
- the walkable share, walkable water included, is in the biome's range.

Attempt 0 uses the field's node. Attempt n uses `child(node, "attempt", n)`. After 8 failed attempts the field is the error `ErrNoField`, which lists each attempt's problem. The server does not start with such a field and logs its public node.

## Output

- The grid of generator-v1, with its checksum.
- The portals, one per edge, and the props.
- The field as a WMAP map ([assets-v1](assets-v1.md), "Map (WMAP)"), the same bytes on every platform:
  - `CELL`: the cells' flags and corner heights.
  - `GRND`: surfaces of 2 × 2 cells. A surface takes the material of the highest rank among its cells: road, rock, shore, patch, ground. One white tile per material covers a surface once.
  - `LMAP`: `placeholder/lightmap`, one block. `LGHT`: 45 degrees of longitude and latitude, with the biome's colors.
  - `WATR`, when a cell is below the level: the level, a wave height of 1/8, and the 32 frames `texture/gen/<tileset>/water`.
  - `OBJS`: each prop's model `model/gen/<tileset>/<prop>`, turned by its 16th of a turn and scaled, standing on its cell's mean height.
  - `SNDS` and `EFCT` are empty. `DEPS` lists every name, sorted.
- Ground textures are `texture/gen/<tileset>/<material>`. A tileset table on the server maps these names to assets, with `placeholder/ground/<slot>` and `placeholder/prop/<slot>` by default.

f32 values come from fixed-point values by integer math: the nearest single-precision value, ties to even. `TestF32Bits` compares it with the hardware's conversion.

## Server endpoints

| Path | Content |
|---|---|
| `GET /gen/maps/<id>.json` | The field's ID, generator version, rng spec, public node, size, biome index and name, grid checksum, and the path, SHA-256 and length of its WMAP. |
| `GET /gen/maps/<id>.wmap` | The WMAP. gzip for a client that accepts it, and the SHA-256 as its ETag. |
| `GET /gen/tilesets.json` | `{"version": 1, "aliases": {...}}`: the asset each `texture/gen/...` and `model/gen/...` name stands for. |

The paths need no login, since public nodes are public by design (strategy section 8.3). Any origin may read them.

## Client

1. Fetch the field's JSON.
2. Generate the field from the node, the size and the biome, in a Web Worker.
3. Use the field's WMAP when the grid's checksum matches the server's.
4. Otherwise, or when the module is missing or lacks the version, download the WMAP and check its SHA-256.

## WebAssembly module

The module of generator-v1 changes as follows:

| Export | Use |
|---|---|
| `generate(version, width, height, biome) → code` | Makes a field for a version with fields, else a grid. 0, -1 for a version the module lacks, -2 for a size outside 1 to 1024 or below 32 for a field, -3 when every attempt failed, -4 for a biome the version lacks. |
| `biomes(version) → n` | The biome count, 0 for a version without fields, -1 for one the module lacks. |
| `wmap() → ptr`, `wmap_size() → n` | The field's WMAP, or 0 bytes for a grid. |
| `attempt() → n` | The attempt that made the field. |

## Golden file

`golden-v2.json` adds to each map of generator-v1's format:

- `field`: the field's coordinates below the public branch of `secret`, instead of `region`.
- `biome`, `attempt` and `objects`: the biome index, the attempt that passed and the prop count. A zero value is left out.
- `wmap`: the WMAP's SHA-256.

The maps cover every biome, the smallest field, two 400 × 400 fields, a 1024-cell side, field indexes near 2^40, and an all-zero key.
