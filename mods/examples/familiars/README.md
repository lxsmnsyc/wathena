# Familiars

A sample mod for companions and mounts of proposal 0002, review decision 25 (mods/README.md, "Companions, mounts and carts").

- `data/companions.yml` adds the Owl. It shows the falcon's sprite and acts as a falcon, so Blitz Beat and the other falcon skills work with it. It needs Falconry Mastery and gives DEX +2.
- `data/mounts.yml` adds the Horse. It shows the Peco Peco's sprite and follows its rules, with a walk speed bonus of 20% in place of 25%. It needs Riding and gives 200 HP.
- `data/skills.yml` adds Owl Strike, a strike for 150% damage whose requirements name the falcon: a falcon or the Owl passes. `data/skill_trees.yml` lets Hunters learn it.
- The Familiar Keeper in Prontera, at 143,191, hands out the Owl and the Horse with `player:set_companion` and `player:set_mount`.

Run `wactl mod test mods/examples/familiars`. `TestFamiliars` in `tools/modkit` checks in Go what a test cannot read: a cast of Owl Strike with and without a companion.
