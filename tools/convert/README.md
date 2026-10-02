# convert

Converter from rAthena scripts to wAthena Lua and data (strategy section 5.6). It converts item scripts, and NPC scripts and shops.

- `parse.go` reads rAthena's script language, following `src/map/script.cpp`.
- `equip.go` runs equipment scripts symbolically. A condition on a game value, such as the refine level, becomes the `when` option of the bonus. A game value in an amount becomes a computed bonus.
- `use.go` translates scripts that run once: use handlers, equip and unequip scripts, and the effects of `autobonus`.
- `usestatus.go` converts `sc_start`, `sc_start2`, `sc_start4` and `sc_end` to `player:start_status` and `player:end_status` (proposal 0002, review decision 19), and `bonus_script` to `player:add_timed_bonuses`. Item scripts set `keep_duration`, as rAthena's default flag for items is `SCSTART_NOTICKDEF`. A timed bonus's key is a hash of its script's text, as rAthena tells `bonus_script` copies apart by their text. The flags `BSF_FORCE_REPLACE` (0x400) and `BSF_FORCE_DUPLICATE` (0x800) become `on_repeat = "extend"` and `on_repeat = "add"`. Without them, the API's default keeps the first copy, as rAthena does.
- `status.go` converts the scripts of `status.yml`, such as `bonus bStr,getstatus(SC_INCSTR,1);`, to bonus templates that `statuschange` computes for each active status ([decision 0010](../../docs/decisions/0010-status-effects.md)).
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
- `TestEquipmentParity` equips every fully converted equipment item and card, with its static bonuses and its Lua script, on the recorder's sample player, and compares the stats and the damage with rAthena's `item_bonuses.json`, and every refine level with `refine.json`, in both modes. The results are `testdata/equipment_parity_<mode>.md` (server/README.md, "Equipment").
- `TestHealParity` checks that every plain heal converts to the amounts the items extract read.
- `TestGolden` keeps the output of sample items in `testdata/golden`.
- `TestStatusScripts` converts every status script of both modes and keeps the templates in `testdata/golden/status_scripts_<mode>.txt`.
- `TestReportsCurrent` checks the reports in `docs/generated`.

NPCs ([decision 0008](../../docs/decisions/0008-converted-npcs.md)):

- `parse.go` also reads NPC scripts: labels, `goto`, `menu`, `callsub`, `function` and `switch`.
- `npc.go` places NPCs and shops from `npcs.json`, and writes core's `data/npcs.yml`. Shops become data. A script NPC is placed when its own script converts whole.
- `npcstatus.go` converts `sc_start`, `sc_start2`, `sc_start4`, `sc_end` and `getstatus` in NPC scripts. NPC scripts set `cannot_be_avoided`, as rAthena's default flag for NPCs is `SCSTART_NOAVOID`.
- `npclua.go`, `npcexpr.go` and `npcscript.go` translate scripts to Lua with the API of proposal 0002: conversations, `wa.variables`, zeny, items and save points. Labels become Lua labels, and a function's `end` becomes `R.stop()`.
- Event labels run on into the next label until an `end`, as in rAthena. These event labels convert:
  - `OnInit` runs at the end of the NPC's setup function.
  - Clock labels become schedules: `OnClockHHMM` and `OnHourHH` daily, `OnMinuteMM` hourly, and `OnSunHHMM` to `OnSatHHMM` weekly. `OnDayMMDD` does not convert, since the API has no yearly schedule.
  - A label that a `monster` command of the same NPC names runs when each of its monsters dies, with the player that killed it.
- `monster` converts on the NPC's own map, with the monster's own name, the default size and AI, and an event label of the NPC or none. `mobcount` converts for such a label. `killmonster` does not convert.
- `rathena.lua` is `scripts/lib/rathena.lua`: helpers for what Lua lacks, such as rAthena's arrays, `menu`, `getarg`, `input` with its bounds, `getitem` and `delitem` with a computed amount, which do nothing for 0 or less, and `monster` and `mobcount`.
- `npcreport.go` writes the coverage report.

```text
go run ./tools/wactl convert npcs --mode renewal --out /tmp/core-npcs
go run ./tools/wactl convert npcs --mode renewal --report docs/generated/npc-scripts-renewal.md
```

NPC tests:

- `TestRunConvertedNPCs` starts every converted NPC of both modes in a zone's Lua state, kills the monsters they spawn, runs their schedules, and fails on any error.
- `TestNPCGolden` keeps the output of sample NPCs and functions in `testdata/golden/npcs`, such as Prontera's Kafra and its functions, a dialog NPC, an inn and a tool dealer.
- `TestNPCEventsConvert` converts NPCs written for the test, with clock labels and monsters, and keeps two in `testdata/golden/npcs/events_*.lua`. `TestRunNPCEvents` runs them: their schedules, and their monsters' deaths.
- `TestNPCReportsCurrent` checks the reports in `docs/generated`.
- `TestShopsConvert` checks that every zeny shop converts, and that cash, item, point and market shops are reported instead.
- server/zone's `kafra_test.go` runs Prontera's converted Kafra: save, and teleport with its price and points.

`go test ./tools/convert -update` writes the golden files and reports again.
