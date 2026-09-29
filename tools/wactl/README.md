# wactl

Command-line tool for worlds, mods, maps, and assets. See docs/strategy.md, sections 6.9 and 10.1.

## Commands

`wactl assets scan --client <folder> [--read]`

Lists what a game client folder contains, grouped by asset type. The folder needs a `DATA.INI` and the GRF files it names. With `--read`, it also decompresses every file and reports the speed and any damaged entries.

`wactl assets sprites --client <folder> [--preview <folder> --names <names>] [--brotli <0-11>] [--every <n>]`

Converts every monster and NPC sprite into WSPR and reports sizes and failures. With `--preview`, it writes a PNG of the first idle frame of each listed sprite. This checks the sprite importer. The full build comes later.
