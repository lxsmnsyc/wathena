# Home Village

A sample mod for procedural worlds (proposal 0002, review decision 43). It shows two parts of the world layout:

- `maps/village/` is a home map in the map folder format (strategy section 8.12, docs/specs/map-folder-v1.md). It is tagged `home`, so new characters start there, at its `start` place. The world layout links its `portal` place to the first town.
- `data/naming_styles.yml` adds the naming style `meadow` for grassland towns. It loads after core, so it replaces core's `norse` style for that biome.

The tests in `tests/` run with `wactl mod test` built with `-tags procedural`, on the test world's generated maps. They use `wa.towns`, `maps.get_display_name`, `player:has_visited` and `place:get_band`.

Check the map folder with `wactl map validate mods/examples/home-village/maps/village`. To use the mod, put it in the server's mods folder before `wactl world new --config waserver.yml` makes the world: the layout is made once, with the mods of that config.
