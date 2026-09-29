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
| `sprite/head/<sex>/<style>` | Sprite | `sprite/head/m/12` | Hairstyle. |
| `sprite/headgear/<sex>/<name>` | Sprite | `sprite/headgear/f/goggle` | Headgear, by the name in `headgear.yml`. |
| `sprite/weapon/<look>/<sex>/<name>` | Sprite | `sprite/weapon/swordman/m/sword` | Weapon drawn on a character. |
| `sprite/shield/<look>/<sex>/<name>` | Sprite | | Shield drawn on a character. |
| `sprite/garment/<look>/<sex>/<name>` | Sprite | | Garment drawn on a character. |
| `sprite/mob/<name>` | Sprite | `sprite/mob/poring` | Monster. |
| `sprite/npc/<name>` | Sprite | `sprite/npc/4_f_kafra1` | NPC. |
| `sprite/<other>/<name>` | Sprite | `sprite/item/red_potion` | Other sprites: `item` (dropped items), `pet`, `homun`, `merc`, `effect`, `cursor`. |
| `palette/hair/<sex>/<style>/<color>` | Palette | `palette/hair/f/3/5` | Hair dye. |
| `palette/body/<look>/<sex>/<color>` | Palette | | Clothes dye. |
| `texture/<path>` | Texture | `texture/prontera/ground01` | Textures that maps and models use. |
| `ui/<path>` | Texture | `ui/login/background` | Interface images. |
| `icon/item/<name>` | Texture | `icon/item/red_potion` | Item icons. |
| `image/<kind>/<name>` | Texture | `image/card/poring_card` | Larger images: `item`, `card`, `illust`, `emblem`. |
| `model/<path>` | Model | `model/prontera/fountain` | 3D models. |
| `map/<name>` | Map | `map/prontera` | Maps. |
| `effect/<name>` | Effect | `effect/firewall` | Keyframed effects. |
| `sound/<path>` | Sound | `sound/effect/hit_sword` | Short sounds. |
| `music/<name>` | Music | `music/08` | Background music. |
| `placeholder/<type>` | Any | `placeholder/sprite` | Fallbacks, below. |

### Placeholders

When a name is missing, the client uses a placeholder and logs the missing name once:

| Missing | Replaced by |
|---|---|
| Any sprite | `placeholder/sprite` |
| Palette | The sprite's own default palette. |
| Texture, UI image, icon, or image | `placeholder/texture` |
| Model | `placeholder/model` |
| Effect or sound | Nothing is shown or played. |
| Map | The map cannot be entered, and the client shows an error. |

The repository ships a placeholder pack under CC0 in `assets/placeholder/`. It covers every `placeholder/*` name, plus a minimal body, head, monster, ground texture, and map, so the client runs with no other assets. It is original art made for the project.

## Formats

All multi-byte numbers are little-endian. Every binary format starts with a 4-byte magic and a `u16` format version.

| Type | Stored as | Extension | Content type |
|---|---|---|---|
| Sprite | WSPR binary, below. | `.wspr` | `application/octet-stream` |
| Palette | WPAL binary, below. | `.wpal` | `application/octet-stream` |
| Texture, UI, icon, image | WebP, lossless, straight alpha, sRGB. | `.webp` | `image/webp` |
| Model | glTF 2.0 binary, profile below. | `.glb` | `model/gltf-binary` |
| Map | WMAP binary. **Draft.** | `.wmap` | `application/octet-stream` |
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

### Sound and music

- Sounds are Opus in a WebM container, mono or stereo, at 48 kHz, which is Opus's native rate.
- The build encodes sounds at 32 kbit/s for mono and 64 kbit/s for stereo. At these rates, 22 kHz game sounds have no audible loss.
- Opus has no encoder delay padding at the end, so looping sounds loop without gaps.
- Music can be Opus in WebM, or MP3. The build passes MP3 through unchanged, because re-encoding lossy audio loses quality. New music in packs should be Opus.
- Accepted inputs: WAV (PCM, IEEE float, and IMA ADPCM), MP3, Ogg Vorbis, and Opus in Ogg or WebM.
- The container is WebM because browsers have supported Opus in WebM the longest. Phase 1 checks `decodeAudioData` with Opus in WebM on Chrome, Firefox, and Safari. If a browser fails, the container can change to Ogg without re-encoding.

### Model (glTF profile)

- glTF 2.0 binary with one scene.
- Images point to textures by name, with a URI of the form `asset:texture/...`. The client resolves them through the manifest. Embedded images are not allowed, so shared textures download once.
- Allowed: meshes, node hierarchy, `KHR_materials_unlit`, alpha masking, double-sided materials, and animations of translation, rotation, and scale.
- Not allowed: skins, morph targets, cameras, lights, and other extensions. The build rejects them.
- RO's RSM and RSM2 models convert to this profile. Their per-face textures become one primitive per texture.

### Map (WMAP) **Draft**

A map holds everything needed to show it, in sections:

| Section | Content |
|---|---|
| Header | Size in cells, water level and type, and the format version. |
| Cells | Cell type and 4 corner heights per cell (strategy section 8.12). The client uses this for pathfinding and ground height. |
| Ground | Ground mesh vertices, texture coordinates, and texture names. |
| Lightmap | A lightmap atlas and its coordinates, for classic maps. |
| Objects | Model names with position, rotation, and scale. |
| Sounds | Positioned ambient sounds by name. |
| Dependencies | Every texture, model, and sound name the map uses, so the client can fetch them in parallel. |

The byte layout will be fixed when the first map converter is written in phase 1.

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
  - `placeholder`: the pack in `assets/placeholder/`.
  - `grf:<folder>`: a game client folder with `DATA.INI` (strategy section 10.1). The importer maps game paths to names with its own rules, such as `data\sprite\몬스터\poring.spr` to `sprite/mob/poring`.
  - `pack:<folder>`: original art in authoring formats (below).
- **Output.** Each asset is stored under the SHA-256 of its content, as `<first 2 hex digits>/<hash>.<extension>`. Binary formats also get a Brotli copy, `<hash>.<extension>.br`, which the server sends with `Content-Encoding: br`. The hash is of the uncompressed content, so the client can check it after the browser decompresses.
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
