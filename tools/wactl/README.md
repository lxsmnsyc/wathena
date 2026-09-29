# wactl

Command-line tool for worlds, mods, maps, and assets. See docs/strategy.md, sections 6.9 and 10.1.

## Commands

`wactl assets scan --client <folder> [--read]`

Lists what a game client folder contains, grouped by asset type. The folder needs a `DATA.INI` and the GRF files it names. With `--read`, it also decompresses every file and reports the speed and any damaged entries.
