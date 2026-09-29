# Asset Contract, assets-v1

This spec defines what the web client can load: how assets are named, which formats they use, and how a build publishes them. wAthena ships the runtime and this contract, not art. Any source that produces assets in this form works, including the GRF importer, original art, and mods.

- Version: `assets-v1`. The manifest records it. A client rejects a manifest with a version it does not know.
- Sections marked **Draft** can still change during phase 1. The other sections are stable within `assets-v1`.

## Names

Every asset has a name, such as `sprite/mob/poring` or `texture/prontera/ground01`.

Rules:

- A name is UTF-8 in Unicode normalization form NFC. Non-ASCII letters are allowed, because many texture and model names from the game are Korean.
- ASCII letters are lowercase.
- Segments are separated by `/`. A segment is not empty, not `.` or `..`, and has no control characters, `\`, or `:`.
- A name has no file extension. Its type comes from its first segment.
- A name is at most 255 bytes.

Game data never refers to files or GRF paths. It refers to names. The client data tables (strategy section 10.5) map game IDs to names, for example monster 1002 to `sprite/mob/poring`.

### Namespaces

| Namespace | Type | Example | Use |
|---|---|---|---|
| `sprite/body/<look>/<sex>` | Sprite | `sprite/body/swordman/f` | Character body. `<look>` is the look name from `jobs.yml`, including mount and costume looks. `<sex>` is `m` or `f`. |
| `sprite/head/<race>/<sex>/<style>` | Sprite | `sprite/head/human/m/12` | Hairstyle. `<race>` is `human` or `doram`, from `jobs.yml`. |
| `sprite/headgear/<sex>/<view>` | Sprite | `sprite/headgear/f/1` | Headgear, by view ID, the number items use in rAthena's `item_db` `View` field. |
| `sprite/weapon/<look>/<sex>/<view>` | Sprite | `sprite/weapon/swordman/m/2` | Weapon drawn on a character, by weapon view ID. |
| `sprite/shield/<look>/<sex>/<view>` | Sprite | `sprite/shield/knight/f/1` | Shield drawn on a character, by shield view ID. |
| `sprite/garment/<look>/<sex>/<view>` | Sprite | `sprite/garment/archer/m/1` | Garment drawn on a character, by garment view ID. |
| `sprite/mob/<name>` | Sprite | `sprite/mob/poring` | Monster. |
| `sprite/npc/<name>` | Sprite | `sprite/npc/4_f_kafra1` | NPC. |
| `sprite/<other>/<name>` | Sprite | `sprite/item/red_potion` | Other sprites: `item` (dropped items), `pet`, `homun`, `merc`, `effect`, `cursor`. |
| `palette/hair/<race>/<sex>/<style>/<color>` | Palette | `palette/hair/human/f/3/5` | Hair dye. |
| `palette/body/<look>/<sex>/<color>` | Palette | | Clothes dye. |
| `texture/<path>` | Texture | `texture/prontera/ground01` | Textures that maps and models use. |
| `ui/<path>` | Texture | `ui/login/background` | Interface images. |
| `icon/item/<name>` | Texture | `icon/item/red_potion` | Item icons. |
| `image/<kind>/<name>` | Texture | `image/card/poring_card` | Larger images: `item`, `card`, `illust`, `emblem`. |
| `model/<path>` | Model | `model/prontera/fountain` | 3D models. |
| `map/<name>` | Map | `map/prontera` | Maps. |
| `lightmap/<name>` | Texture | `lightmap/prontera` | A map's lightmap atlas. |
| `effect/<name>` | Effect | `effect/firewall` | Keyframed effects. |
| `sound/<path>` | Sound | `sound/effect/hit_sword` | Short sounds. |
| `music/<name>` | Music | `music/08` | Background music. |
| `placeholder/<type>` | Any | `placeholder/sprite` | Fallbacks, below. |

### Textures from a game client

The GRF importer names images in `data\texture\` by their path without the extension:

| GRF path | Name |
|---|---|
| `data\texture\<path>` | `texture/<path>` |
| `data\texture\유저인터페이스\item\<name>` | `icon/item/<name>` |
| `data\texture\유저인터페이스\collection\<name>` | `image/item/<name>` |
| `data\texture\유저인터페이스\cardbmp\<name>` | `image/card/<name>` |
| `data\texture\유저인터페이스\illust\<name>` | `image/illust/<name>` |
| `data\texture\유저인터페이스\<path>` | `ui/<path>` |

Some folders have the same image name in two formats, such as `fire.bmp` and `fire.tga`, and the two images differ. The preferred format gets the plain name, and the other gets `~` and its format appended, as in `texture/effect/fire~tga`. The order of preference is BMP, TGA, PNG, then JPEG, because maps and models refer to BMP files.

### Sounds and music from a game client

| Client path | Name |
|---|---|
| `data\wav\<path>.wav` in the GRF | `sound/<path>` |
| `BGM\<name>.mp3` in the client folder | `music/<name>` |

Sprite events that name a sound file, such as `ef_hit1.wav`, become the same sound names.

### Placeholders

When a name is missing, the client uses a placeholder and logs the missing name once:

| Missing | Replaced by |
|---|---|
| Weapon, shield, garment, or headgear sprite | Nothing is drawn. |
| Any other sprite | `placeholder/sprite` |
| Palette | The sprite's own default palette. |
| Texture, UI image, icon, or image | `placeholder/texture` |
| Model | `placeholder/model` |
| Effect or sound | Nothing is shown or played. |
| Map | The map cannot be entered, and the client shows an error. |
| Lightmap | The ground is drawn without baked light. |

wAthena provides a placeholder pack as the `placeholder` build source. Its art is drawn by code in `internal/assets/placeholder`, so it is original and CC0. It will cover every `placeholder/*` name, plus a minimal body, head, monster, ground texture, and map, so the client runs with no other assets. Each part is added when its format's converter exists. `placeholder/sprite` exists now.

## Formats

All multi-byte numbers are little-endian. Every binary format starts with a 4-byte magic and a `u16` format version.

| Type | Stored as | Extension | Content type |
|---|---|---|---|
| Sprite | WSPR binary, below. | `.wspr` | `application/octet-stream` |
| Palette | WPAL binary, below. | `.wpal` | `application/octet-stream` |
| Texture, UI, icon, image | WebP, lossless, straight alpha, sRGB. | `.webp` | `image/webp` |
| Model | glTF 2.0 binary, profile below. | `.glb` | `model/gltf-binary` |
| Map | WMAP binary, below. | `.wmap` | `application/octet-stream` |
| Effect | WEFF binary. **Draft.** | `.weff` | `application/octet-stream` |
| Sound | Opus in WebM. | `.webm` | `audio/webm` |
| Music | Opus in WebM, or MP3. | `.webm`, `.mp3` | `audio/webm`, `audio/mpeg` |

### Sprite (WSPR)

A sprite holds its images, packed into pages, and its animations. It follows the structure of RO's `spr` and `act` files, so converted sprites keep their exact timing and offsets.

```text
Header
  magic         "WSPR"
  version       u16 = 1
  pageCount     u16
  imageCount    u32
  actionCount   u32
  eventCount    u16

Pages, pageCount times
  kind          u8     0 = palette indexes, 1 = RGBA
  width         u16
  height        u16
  pixels        width × height bytes for indexes, × 4 for RGBA

Palette         256 × 4 bytes RGBA, present if any page has kind 0.
                Index 0 is transparent.

Images, imageCount times
  page          u16
  x, y          u16, u16    position in the page
  width, height u16, u16

Actions, actionCount times
  intervalMs    u16         time per frame
  frameCount    u16
  Frames, frameCount times
    event       i16         index into Events, or -1
    anchorCount u8
    Anchors     anchorCount × (x i16, y i16)
    layerCount  u16
    Layers, layerCount times
      image     i32         index into Images, or -1 for none
      x, y      i16, i16    offset from the anchor point
      mirror    u8          1 = flipped horizontally
      color     4 × u8      RGBA multiplier
      scaleX    f32
      scaleY    f32
      rotation  u16         degrees

Events, eventCount times
  length        u8
  name          UTF-8 bytes of a sound name, such as "sound/effect/hit_sword",
                or a marker without "/", such as "atk" for the moment a hit lands
```

Rules:

- Palette index pages are kept as indexes, so the client can swap palettes for hair and clothes dyes without new images.
- Pages are at most 2048 × 2048.
- Actions use RO's layout: action number × 8 + direction, with direction 0 facing south and turning clockwise. Character actions: 0 idle, 1 walk, 2 sit, 3 pick up, 4 ready, 5 attack, 6 hurt, 7 freeze, 8 dead, 9 freeze again, 10 attack, 11 attack, 12 cast. Monster actions: 0 idle, 1 walk, 2 attack, 3 hurt, 4 dead.
- Anchors are where layers attach, such as a head on a body.

### Palette (WPAL)

```text
magic    "WPAL"
version  u16 = 1
colors   256 × 4 bytes RGBA. Index 0 is transparent.
```

### Texture

- Textures are lossless WebP, so the client gets the source pixels exactly.
- The color of a fully transparent pixel is not kept. The build sets it to the average of its opaque neighbors, or black, so that filtering does not blend in stray colors at the edges.
- In BMP sources, magenta and colors close to it (red and blue above 230, green below 20) become transparent, as in the game.
- Accepted inputs: BMP (uncompressed, 1 to 32 bits per pixel), TGA (uncompressed and run-length encoded), PNG, and JPEG.
- The build encodes with libwebp at lossless level 6. libwebp is compiled to WebAssembly and run by the build tool, so the build needs no C compiler and makes the same bytes on every platform.

### Sound and music

- Sounds are Opus in a WebM container, mono or stereo, at 48 kHz, which is Opus's native rate.
- The build encodes sounds at 32 kbit/s for mono and 64 kbit/s for stereo. At these rates, 22 kHz game sounds have no audible loss.
- Sources at other rates are resampled to 48 kHz with the speexdsp resampler at its highest quality.
- The decoded sound has exactly the source's length. The track's `CodecDelay` holds the encoder's pre-skip, and the last block's `DiscardPadding` removes the padding in the last packet. Phase 1 checks that browsers apply both, so looping sounds loop without gaps.
- Music can be Opus in WebM, or MP3. The build passes MP3 through unchanged, because re-encoding lossy audio loses quality. New music in packs should be Opus.
- Accepted inputs: WAV (8- to 32-bit PCM, IEEE float, IMA ADPCM, and Microsoft ADPCM), MP3, Ogg Vorbis, and Opus in Ogg or WebM. The GRF importer reads WAV now. The other inputs come with the pack importer.
- For ADPCM WAV files, the `fact` chunk gives the length, since the last block is usually only partly filled.
- libopus and the resampler are compiled to WebAssembly and run by the build tool, like libwebp. Tests pin the encoder's output bytes, so a platform or build change that alters them fails.
- The container is WebM because browsers have supported Opus in WebM the longest. Phase 1 checks `decodeAudioData` with Opus in WebM on Chrome, Firefox, and Safari. If a browser fails, the container can change to Ogg without re-encoding.

### Model (glTF profile)

- glTF 2.0 binary with one scene.
- Images point to textures by name, with a URI of the form `asset:texture/...`. Each name segment is percent-encoded, since many names are Korean. The client resolves them through the manifest. Embedded images are not allowed, so shared textures download once. glTF validators warn about these URIs, which is expected.
- Allowed: meshes, node hierarchy, `KHR_materials_unlit`, alpha masking, alpha blending, double-sided materials, and animations of translation, rotation, and scale.
- Not allowed: skins, morph targets, cameras, lights, and other extensions. The build rejects them.
- Y points up. A model stands on y = 0 and is centered on x and z.
- Primitives have positions, normals, and one set of texture coordinates. The client lights models with the normals, as RO does, so materials are not unlit.
- An animation can set `extras.loopSeconds`, the length it loops over, which can be longer than its last keyframe.
- Nodes can have negative scales. As glTF defines, a node whose world transform has a negative determinant has clockwise front faces.

#### RSM conversion

- `data\model\<path>.rsm` and `.rsm2` become `model/<path>`. When both exist, the RSM2 model gets `~rsm2` appended. Granny (`.gr2`) models are not supported.
- RO's space has X east, Y down, and Z north, which is right-handed. The converter turns it half a turn around X with R = diag(1, -1, -1): vertices become R × v, and node transforms become R × X × R, which keeps them as translation, rotation, and scale. Y points up and north is -Z, glTF's forward direction. R is a rotation, so nothing is mirrored. Map objects are placed the same way.
- Each RSM node becomes a glTF node with RO's position, rotation, and scale. The node's offset and 3 × 3 matrix are applied to its vertices.
- The root node holds RO's centering: the box center on x and z, and the box's lowest point on y.
- Faces are grouped into one primitive per texture and sidedness. Two-sided faces get double-sided materials.
- Normals follow the model's shading type: flat face normals, smooth normals averaged within each smoothing group, or, for unshaded models, normals that point up in the world.
- Materials use alpha masking at 0.5. A model with an alpha below 255 uses alpha blending with that opacity.
- Rotation keyframes become a rotation animation. Keyframe times are milliseconds.

#### RSM2 conversion

RSM2 (versions 2.2 and 2.3) differs from RSM, following BrowEdit3's reading of the format:

- Model space has X east, Y up, and Z north, which is left-handed, and models are not centered. The converter mirrors through Z with Q = diag(1, 1, -1): vertices become Q × v, node transforms become Q × X × Q, and triangles swap two corners.
- Nodes store their matrix and position in model space. A node's transform relative to its parent is T(position) × M × S. Position is the position keyframe, or the parent's inverse matrix times (position - parent position). M is the rotation keyframe, or the parent's inverse matrix times the node's matrix. S is the scale keyframe, or none.
- Before a node's first keyframe, rotation starts from none, scale from 1, and position from the stored position. After its last keyframe, the last value holds.
- Keyframe times are frames at the model's frame rate, usually 30 per second. `loopSeconds` is the animation length in frames divided by the frame rate.
- A node whose subtree does not move is baked into its vertices, since an RSM2 matrix can shear and a glTF node matrix cannot. A node above a moving node keeps a matrix node. In the kRO client, one model (`model/ilusion/bathwall_s_01`) has a shearing matrix there, which glTF validators report.
- Texture animations in version 2.3 are not converted yet. The glTF profile has no way to express them.

### Map (WMAP)

A map holds everything needed to show it and walk on it, except the textures, models, and sounds it names.

**Map space.** Units are cells. X points east, Y up, and -Z north. Cell (x, y), in the server's coordinates, covers X from x to x + 1 and Z from -(y + 1) to -y. Models are placed in the same space.

**Layout.** A header, then sections. Each section is a 4-byte tag, a `u32` length, and its body. A reader skips tags it does not know. Every section below is required except `WATR`, and each appears once.

```text
Header
  magic      "WMAP"
  version    u16 = 1
  width      u16    cells, at most 1024
  height     u16

Strings are a u8 length and UTF-8 bytes. Names are asset names.

"CELL"  width × height cells, row by row from y = 0 (the south edge)
  flags     u8     1 walkable, 2 shootable (projectiles pass), 4 water
  heights   4 × f32   corners (x, y), (x+1, y), (x, y+1), (x+1, y+1), up positive

"GRND"  the ground, a grid of surfaces of 2 × 2 cells
  width, height   u16, u16    surfaces
  textureCount    u16
  textures        textureCount × string
  tileCount       u32
  Tiles, tileCount times
    u, v          4 × f32, 4 × f32   texture coordinates, corners in cell order
    texture       u16                index into textures
    lightmap      u32                lightmap block
    color         4 × u8             RGBA, multiplies the texture of top tiles
  Surfaces, width × height, row by row from the south
    heights       4 × f32            corners, as for cells
    top, north, east   3 × i32       tile index, or -1 for none

"LMAP"
  name      string   the lightmap atlas, a texture
  count     u32      blocks
  columns   u16      blocks per row

"LGHT"
  longitude, latitude   2 × i32    degrees, as RO stores them
  diffuse, ambient      2 × 3 × f32
  opacity               f32        shadow strength
  direction             3 × f32    unit vector toward the light

"WATR"
  zone          the default water zone, below
  splitWidth, splitHeight   u16, u16
  zones         splitWidth × splitHeight zones, row by row, overriding the default
  textureSets   u8
  Sets, textureSets times
    type        i32
    frames      32 × string    the animation frames of this water type
  A zone is: level f32 (up positive), type i32, waveHeight f32, waveSpeed f32,
  wavePitch f32, animSpeed i32.

"OBJS"
  count     u32
  Objects, count times
    model       string
    matrix      16 × f32   column-major, from model space to map space
    animType, animSpeed, blockType   i32, f32, i32

"SNDS"
  count     u32
  Sounds, count times
    name        string
    position    3 × f32
    volume, width, height, range, cycle   5 × f32   range in cells, cycle in seconds

"EFCT"
  count     u32
  Effects, count times
    id          i32    RO effect ID
    position    3 × f32
    delay       f32
    params      4 × f32

"DEPS"
  count     u32
  names     count × string    every name the map uses, so the client can fetch them in parallel
```

**Building the ground.** Surface (i, j) covers X from 2i to 2i + 2 and Z from -(2j + 2) to -2j. Corner k is at X = 2i + 2 × (k & 1), Z = -(2j + 2 × (k >> 1)), at its height. For each surface:

- The top tile, if any, is a quad over the four corners. Tile corner k goes on surface corner k.
- The north tile, if any, is a wall on the north edge. Tile corners 0, 1, 2, and 3 go on this surface's corners 2 and 3, then the north neighbor's corners 0 and 1. Surfaces on the last row have no north wall.
- The east tile, if any, is a wall on the east edge. Tile corners 0, 1, 2, and 3 go on this surface's corners 3 and 1, then the east neighbor's corners 2 and 0. Surfaces on the last column have no east wall.
- Each tile corner k has the tile's texture coordinates (u[k], v[k]). Its lightmap coordinates are texel 1 or 7 of the tile's 8 × 8 block: 7 across when k & 1, 7 down when k >> 1. Block b starts at (b % columns × 8, b / columns × 8) in the atlas.

**Lighting.** A ground texel is: texture × tile color (top tiles only, filtered between neighboring tiles) × clamp(diffuse × max(0, normal · direction) + ambient), then × the lightmap's alpha, then + the lightmap's RGB. Models use the same light without a lightmap.

**Lightmap atlas.** `lightmap/<map>` is a lossless WebP texture of 8 × 8 blocks: RGB is baked light color, alpha is shadow, with 255 for none. Its pixels are exact, with no color key and no edge filling.

#### Maps from a game client

- `data\<name>.rsw`, with `<name>.gnd` and `<name>.gat`, becomes `map/<name>` and `lightmap/<name>`.
- RO units are 1/5 cell, with X east, Y down, and Z north, relative to the map's center. The importer scales by 1/5, moves the origin to the south-west corner, and turns by R = diag(1, -1, -1), as for models. An object's matrix is R × T(position / 5 + center) × Rz × Rx × Ry × S(scale / 5) × R, with RO's rotations in degrees.
- Water comes from the GND file from version 1.8, and from the RSW file before that.
- Objects, sounds, and effects with undefined (NaN or infinite) values are left out, since the game cannot show them either. Undefined cell heights take the average of the cell's other corners.
- GAT cell types map to flags: 0, 2, 4, and 6 are walkable and shootable, 3 is also water, 5 is shootable only, and other types block.

### Effect (WEFF) **Draft**

A keyframed effect, converted from RO's STR format: layers of textured quads with keyframes for position, size, rotation, color, and blend mode. The byte layout will be fixed with the effect converter.

## Builds

`wactl assets build` combines sources into a build:

```text
wactl assets build \
  --source placeholder \
  --source grf:/srv/client \
  --source pack:/srv/our-art \
  --out /srv/wathena/assets
```

- **Layering.** Sources are applied in order. A later source replaces names an earlier one provides. Mods are applied after all `--source` values, in mod load order.
- **Sources.** Each source is an importer:
  - `placeholder`: the placeholder pack, drawn by code.
  - `grf:<folder>`: a game client folder with `DATA.INI` (strategy section 10.1). The importer maps game paths to names with its own rules, such as `data\sprite\몬스터\poring.spr` to `sprite/mob/poring`.
  - `pack:<folder>`: original art in authoring formats (below).
- **Output.** Each asset is stored under the SHA-256 of its content, as `<first 2 hex digits>/<hash>.<extension>`. Binary formats (WSPR, WPAL, WMAP, WEFF, and glTF) are stored only as Brotli, as `<hash>.<extension>.br`. The server sends them with `Content-Encoding: br`, and decompresses them itself for the rare client that does not accept Brotli. The hash is of the uncompressed content, so the client can check it after the browser decompresses.
- **Cache.** The build keeps `.cache.json` in the output folder, keyed by each input's identity and converter version. A later build reuses an asset whose key and stored file are unchanged, so slow compression happens once per changed asset.
- **Report.** The build prints how many names each source provided and replaced, and every asset it rejected, with the reason.

### Manifest

```text
manifest.json
  contract   "assets-v1"
  build      build ID, a timestamp and a short hash
  shards     256 shard file hashes, one per value of the first byte of SHA-256(name)

<shard hash>.json
  { "<name>": { "h": "<content hash>", "t": "<type>", "s": <size in bytes> }, ... }
```

- The client loads `manifest.json` first. It is small, and is the only file served without long caching.
- The client loads a shard only when it needs a name in it.

### Pack format for original art

A pack is a folder:

```text
our-art/
  pack.yaml                      name, license, contract version
  sprite/mob/poring.png          frames, in a grid or listed in the YAML
  sprite/mob/poring.yaml         frames, actions, anchors, and events
  texture/grass/01.png
  model/tree/pine.glb
  sound/effect/hit.wav             or .ogg, .opus, .mp3
  map/home_village/              map folder, strategy section 8.12
```

- A file's path without its extension is its name.
- Sprite images can be 8-bit indexed PNG, which keeps the palette and allows dyes, or RGBA PNG.
- Textures can be PNG, JPEG, or WebP. The build converts them to lossless WebP.
- Models are glTF, converted to the profile above.
- Every pack must state its license in `pack.yaml`.

## Checks

`wactl assets check --build <folder> --data <client data folder>` lists:

- Names the client data tables or maps refer to that no source provides. These will show as placeholders.
- Names provided but never referred to.
- Assets over the size limits.

## Limits

The build rejects assets over these limits, and the client refuses them if a server sends them anyway:

| Type | Limit |
|---|---|
| Sprite | 32 MB uncompressed, 64 pages. The largest kRO sprite, an event monster with full-color frames, is 21.8 MB. |
| Texture | 4096 × 4096 pixels. |
| Model | 16 MB, 65,536 vertices per primitive. |
| Map | 64 MB uncompressed, 1024 × 1024 cells. |
| Sound | 8 MB. |
| Music | 32 MB. |

## Security

- Importers parse untrusted files. Every parser checks lengths before reading and has fuzz tests.
- Names are checked against the rules above before use, so a crafted name cannot escape the output folder.
- The client checks each file's SHA-256 against the manifest before using or caching it.
