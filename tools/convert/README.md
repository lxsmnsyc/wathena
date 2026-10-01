# convert

Converter from rAthena scripts to wAthena Lua and data (strategy section 5.6). It converts item scripts, and NPC scripts and shops.

- `parse.go` reads rAthena's script language, following `src/map/script.cpp`.
- `equip.go` runs equipment scripts symbolically. A condition on a game value, such as the refine level, becomes the `when` option of the bonus. A game value in an amount becomes a computed bonus.
- `use.go` translates scripts that run once: use handlers, equip and unequip scripts, and the effects of `autobonus`.
- `bonusmap.go` maps every bonus form to a `bonuses.add` call of [proposal 0002a](../../docs/proposals/0002a-bonus-names.md): names, sign flips, chance units, argument order, and options.
- `output.go` writes core's `data/items.yml` and `scripts/items/*.lua`. Equipment with only constant bonuses becomes item data.
- `report.go` writes the coverage report.

`waserver` runs the converter when it starts ([decision 0007](../../docs/decisions/0007-converted-item-scripts.md)). `wactl convert items` writes the files and the report:

```text
go run ./tools/wactl convert items --mode renewal --out /tmp/core-items
go run ./tools/wactl convert items --mode renewal --report docs/generated/item-scripts-renewal.md
```

Tests:

- `TestRunConvertedScripts` loads and runs every converted script of both modes, with equipment against a recording bonus sink.
- `TestItemBonusesParity` compares the base stats of converted equipment with rAthena's `item_bonuses.json`. The result is `testdata/item_bonuses_parity.md`.
- `TestHealParity` checks that every plain heal converts to the amounts the items extract read.
- `TestGolden` keeps the output of sample items in `testdata/golden`.
- `TestReportsCurrent` checks the reports in `docs/generated`.

NPCs ([decision 0008](../../docs/decisions/0008-converted-npcs.md)):

- `parse.go` also reads NPC scripts: labels, `goto`, `menu`, `callsub`, `function` and `switch`.
- `npc.go` places NPCs and shops from `npcs.json`, and writes core's `data/npcs.yml`. Shops become data. A script NPC is placed when its own script converts whole.
- `npclua.go`, `npcexpr.go` and `npcscript.go` translate scripts to Lua with the API of proposal 0002: conversations, `wa.variables`, zeny, items and save points. Labels become Lua labels, and a function's `end` becomes `R.stop()`.
- `rathena.lua` is `scripts/lib/rathena.lua`: helpers for what Lua lacks, such as rAthena's arrays, `menu`, `getarg`, and `input` with its bounds.
- `npcreport.go` writes the coverage report.

```text
go run ./tools/wactl convert npcs --mode renewal --out /tmp/core-npcs
go run ./tools/wactl convert npcs --mode renewal --report docs/generated/npc-scripts-renewal.md
```

NPC tests:

- `TestRunConvertedNPCs` starts every converted NPC of both modes in a zone's Lua state, and fails on any error.
- `TestNPCGolden` keeps the output of sample NPCs and functions in `testdata/golden/npcs`, such as Prontera's Kafra and its functions, a dialog NPC, an inn and a tool dealer.
- `TestNPCReportsCurrent` checks the reports in `docs/generated`.
- `TestShopsConvert` checks that every zeny shop converts, and that cash, item, point and market shops are reported instead.
- server/zone's `kafra_test.go` runs Prontera's converted Kafra: save, and teleport with its price and points.

`go test ./tools/convert -update` writes the golden files and reports again.
