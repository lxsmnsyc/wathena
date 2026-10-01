# convert

Converter from rAthena scripts to wAthena Lua and data (strategy section 5.6). It converts item scripts today.

- `parse.go` reads rAthena's script language, following `src/map/script.cpp`.
- `equip.go` runs equipment scripts symbolically. A condition on a game value, such as the refine level, becomes a computed bonus.
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

`go test ./tools/convert -update` writes the golden files and reports again.
