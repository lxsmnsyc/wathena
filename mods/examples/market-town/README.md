# Market Town

A sample mod for procedural worlds (proposal 0002, review decision 42).

- `data/town_templates.yml` adds `market_town`: a town of 96 cells with two gates, a smithy, an inn and a lamp shop, whose door has the place `shop:lamp`.
- `data/dungeon_styles.yml` adds `cellar`: two floors of small rooms on the tower tileset. A cellar that a world rotates gets new floors daily at 06:00 (review decision 47).
- `data/npcs.yml` places two NPCs by `place` instead of `map`, `x` and `y`:
  - the Lamp Seller, a shop, at every place tagged `shop:lamp` in any town;
  - the Town Guide on every town square, who tells a player where the nearest blacksmith is, with `wa.places`, and the rotations and world changes it heard of, with `dungeon_rotated` and `region_changed`.
- `tests/market_town_test.lua` runs on the test world's generated maps. It needs a `wactl` built with `-tags procedural`. Two cases rotate the cellar with `world:rotate` and change the field's region with `world:change_region`.

In a classic world the NPCs placed by tag do not appear, since classic maps have no places.

Run it on a development server:

```sh
waserver --dev --mods mods/examples --fields 0:0 --towns 4 --dungeons example.market-town:cellar
```

Town t4 is the market town: core has three templates, and this mod's comes after them.
