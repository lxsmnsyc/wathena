# Wayside Shrines

A sample mod for procedural worlds (proposal 0002, review decision 46). Its generation layer runs when `wactl world new` makes a world, once on every generated map:

- Each town gets a place tagged `shrine`, where the Shrine Keeper of `data/npcs.yml` stands.
- Each field gets a ring of four stones, props of the field's tileset, around its first clearing, a place tagged `stone_ring`, and a Vagabond Wolf as a mini boss.
- `generation.quest_template` adds the template `offerings`, which the quest boards draw from.

It needs the permission `world.generate`. `wactl mod test` runs its 3 cases in the test world, with a `wactl` built with `-tags procedural`. mods/README.md, "Generation layers", explains each part.
