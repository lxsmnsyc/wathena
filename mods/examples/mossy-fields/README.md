# Mossy Fields

A sample mod for procedural worlds (proposal 0002, review decision 44). It shows three parts of spawns:

- `data/monsters.yml` adds the Moss Poring and the Moss Spore with `spawn` tags: every field biome with the highest weight, and a role. A new world draws them like core's monsters, by biome, level and role.
- `data/spawn_themes.yml` adds the theme `mossy`, which raises plants, earth monsters and monsters whose tags name it.
- `scripts/main.lua` adds a `spawn_table` hook, with the permission `spawns.modify`. It puts 12 Moss Porings on every field whose band starts below level 20.

The world draws spawns once, when `wactl world new --config waserver.yml` makes it, so put the mod in the server's mods folder and approve `spawns.modify` before that. A world made without the mod never gets its monsters, and removing the mod later leaves its lines out at the next start.

The tests in `tests/` run with `wactl mod test` built with `-tags procedural`, on the test world's field.
