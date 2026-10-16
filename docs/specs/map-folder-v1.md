# Map Folder Format Version 1, map-folder-v1

A map folder is the project's map format for maps that are not generated (strategy section 8.12). A mod ships its home map in it (proposal 0002, review decision 43, [decision 0041](../decisions/0041-world-layout.md)). This spec fixes the files, their encodings, and the checks a reader runs.

- The Go reader is `server/mapfolder`. `wactl map validate <folder>` runs its checks.
- The sample is [`mods/examples/home-village/maps/village`](../../mods/examples/home-village/maps/village).
- Readers reject a format version they do not know.

## Layout

A mod keeps each map folder under `maps/<id>/`, where `<id>` is the folder's ID in `map.yaml`:

```text
maps/village/
  map.yaml        ID, display name, size, biome, water level, music, tags, flags, materials
  cells.bin       cell types and corner heights
  materials.bin   ground material per cell, with a blend
  objects.yaml    models with position, rotation and scale
  places.yaml     named places
```

Every file is required. `objects.yaml` and `places.yaml` may hold an empty list.

## map.yaml

| Key | Content |
|---|---|
| `format` | `1`. Required. |
| `id` | 1 to 32 lowercase letters, digits, `_` and `-`. The map ID is `gen:home:<id>` for a home map. Required. |
| `name` | The display name: 1 to 64 characters, without control characters or spaces at the ends. Required. |
| `size` | `[width, height]` in cells, each 8 to 1024. Required. |
| `biome` | A biome of the map generator, such as `grassland`, or left out. |
| `water_level` | The water level in cells, -256 to 256, or left out for no water. |
| `music` | The background music's asset name, or left out. |
| `tags` | Map tags, each 1 to 64 lowercase letters, digits, `_`, `-`, `.` and `:`. `home` makes the map a home map. |
| `flags` | Map flags. Version 1 knows `nomemo`, which forbids Warp Portal memo points. |
| `materials` | 1 to 255 texture asset names, such as `texture/gen/grassland/grass`. `materials.bin` names them by index. Required. |

Unknown keys are errors.

## cells.bin and materials.bin

Both start with a header of 9 bytes:

```text
4 bytes   magic: "WCEL" for cells.bin, "WMTL" for materials.bin
u8        the format version, 1
u16       the width in cells, big-endian
u16       the height in cells, big-endian
```

The width and height must be `map.yaml`'s `size`. One record per cell follows, row by row from y = 0, the south edge, as in WMAP. The file has no other bytes.

- A `cells.bin` record is 17 bytes: the cell's type as a u8, then its 4 corner heights as big-endian two's complement i32 values in Q16.16 cells, up positive, in WMAP's corner order: (x, y), (x+1, y), (x, y+1), (x+1, y+1). Types are rAthena's: 0 walkable, 1 blocked, 3 walkable water, 5 gap. Heights are -256 to 256 cells.
- A `materials.bin` record is 3 bytes: the cell's material index, the index of the material it blends toward, and the blend's weight from 0 to 255. Both indexes are below the number of `materials`. Version 1 readers draw the first material only.

This is the encoding of `gen.Grid.Checksum` without its prefix, so a reader's grid checksum works on a map folder as on a generated map.

## objects.yaml

A list of models:

| Key | Content |
|---|---|
| `model` | An asset name, such as `model/gen/town/house`. Required. |
| `x`, `y` | The model's point in cells, from 0 to the map's width and height. Decimals are allowed. |
| `rotation` | The turn about the up axis in degrees, -360 to 360. 0 by default. |
| `scale` | Above 0, at most 16. 1 by default. |

At most 4,096 models. A model blocks no cell: the cells under it are blocked in `cells.bin`.

## places.yaml

A list of named places, at most 256:

| Key | Content |
|---|---|
| `tags` | 1 to 8 tags, the main tag first, each as in `map.yaml`. |
| `x`, `y` | The place's cell, inside the map. |

Tags a home map uses:

- `portal`: where the world layout's warp to the first town stands. Required on a home map; the first one is used.
- `start`: where new characters start. Without it, they start on the walkable cell nearest the map's middle.
- Others are for content, as on generated maps: `wa.places` finds them, and NPC records with `place` stand at them.

## Checks

A reader refuses a folder that breaks any rule above. Then it runs the map generator's checks (strategy section 8.5):

- Every place stands on a walkable cell: type 0 or 3.
- Every place is reached from the first place, by walks of 8 directions where a diagonal step needs both cells beside it walkable, as rAthena's path search.
- A map tagged `home` has a `portal` place.

## In the game

- The server serves a home map as a generated map: `/gen/maps/<id>.json` with the kind `home`, no `key` and no `params`, and its WMAP at `/gen/maps/<id>.wmap`. A client cannot generate it, so it downloads the WMAP.
- The WMAP has one ground surface per 2 × 2 cells, at the outer corners of its cells, with the material of its south-west cell, the placeholder lightmap, a light from the north-east, water at `water_level`, and the objects standing on the ground at their points.
