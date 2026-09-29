# wactl

Command-line tool for worlds, mods, maps, and assets. See docs/strategy.md, sections 6.9 and 10.1.

## Commands

`wactl assets build --source <source> [--source <source> ...] --out <folder> [--fast]`

Builds assets from sources, applied in order, into an output folder with a manifest (docs/specs/assets-v1.md). Sources are `placeholder` and `grf:<client folder>`. The GRF source converts monster, NPC, body, head, headgear, weapon, shield, and garment sprites, hair and body palettes for human and doram characters, textures, UI images, item icons, and illustrations, sounds, RSM models, maps with their lightmaps, STR effects, and the MP3 music in the client's `BGM` folder. Body, weapon, shield, and garment names use the looks in `client/data/jobs.yml`. Rebuilds reuse unchanged assets from the cache in the output folder. `--fast` uses a low Brotli level for development builds.

`wactl assets check --build <folder> [--list] [--json]`

Verifies every file in a build against its hash and decodes it, and lists names that maps, models, sprites, effects, or the client data tables refer to but the build lacks. It exits with status 1 if any file is broken. Missing names are warnings; the client shows placeholders for them.

`wactl assets gc --build <folder> [--dry-run]`

Removes files the current build no longer uses, such as outputs of older converter versions.

`wactl assets scan --client <folder> [--read]`

Lists what a game client folder contains, grouped by asset type. The folder needs a `DATA.INI` and the GRF files it names. With `--read`, it also decompresses every file and reports the speed and any damaged entries.

`wactl assets sprites --client <folder> [--preview <folder> --names <names>] [--brotli <0-11>] [--every <n>]`

Converts every monster and NPC sprite into WSPR and reports sizes and failures. With `--preview`, it writes a PNG of the first idle frame of each listed sprite. This checks the sprite importer. The full build comes later.
