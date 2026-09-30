# Cells for the pathfinding tests

Each `<mode>-<map>.cells` file holds one map's cells from rAthena's map caches (GPL-3.0), at the commit that recorded `reference/rathena/<mode>/pathfinding.json`: `db/map_cache.dat`, with `db/re/map_cache.dat` for renewal or `db/pre-re/map_cache.dat` for pre-renewal over it.

The file is `CELL`, the width and height as `u16`, then the GAT cell types, one byte per cell, row by row from y = 0, compressed with zlib.

The tests check each map against the walkable-cell hash in the reference, so they fail if a file differs from what rAthena used.
