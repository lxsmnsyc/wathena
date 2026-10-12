# convert

Converter from rAthena scripts to wAthena Lua and data (strategy section 5.6). It converts item scripts, and NPC scripts and shops.

- `parse.go` reads rAthena's script language, following `src/map/script.cpp`.
- `equip.go` runs equipment scripts symbolically. A condition on a game value, such as the refine level, becomes the `when` option of the bonus. A game value in an amount becomes a computed bonus.
- `use.go` translates scripts that run once: use handlers, equip and unequip scripts, and the effects of `autobonus`.
- `usestatus.go` converts `sc_start`, `sc_start2`, `sc_start4` and `sc_end` to `player:start_status` and `player:end_status` (proposal 0002, review decision 19), and `bonus_script` to `player:add_timed_bonuses`. Item scripts set `keep_duration`, as rAthena's default flag for items is `SCSTART_NOTICKDEF`. A timed bonus's key is a hash of its script's text, as rAthena tells `bonus_script` copies apart by their text. The flags `BSF_FORCE_REPLACE` (0x400) and `BSF_FORCE_DUPLICATE` (0x800) become `on_repeat = "extend"` and `on_repeat = "add"`. Without them, the API's default keeps the first copy, as rAthena does.
- `status.go` converts the scripts of `status.yml`, such as `bonus bStr,getstatus(SC_INCSTR,1);`, to bonus templates that `statuschange` computes for each active status ([decision 0010](../../docs/decisions/0010-status-effects.md)).
- `randomopt.go` converts rAthena's random options and groups from `random_options.json` to core's `data/random_options.yml` and `data/random_option_groups.yml` (proposal 0002, review decision 23, and [decision 0014](../../docs/decisions/0014-rolled-items.md)). An option's script converts as a status script does: `getrandomoptinfo(ROA_VALUE)` becomes `{ roll: value }`, a division or a minus around it `divide` or `multiply: -1`, and a `for` loop unrolls. Its readable name and display name come from its first bonus. `wactl convert random-options --report docs/generated/random-options-<mode>.md` writes the coverage report, which `TestRandomOptionReportsCurrent` checks.
- `bonusmap.go` maps every bonus form to a `bonuses.add` call of [proposal 0002a](../../docs/proposals/0002a-bonus-names.md): names, sign flips, chance units, argument order, and options. An autospell's `i` flag becomes an explicit `target`: for `bAutoSpell` and `bAutoSpellWhenHit`, bit 1 casts on the enemy and its absence on the wearer, as `pc_bonus_autospell` reads it; for `bAutoSpellOnSkill`, bit 1 casts on the wearer.
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
- `npc.go` places NPCs and shops from `npcs.json` and the barter shops of `shops.json`, and writes core's `data/npcs.yml`. Shops become data. A script NPC is placed when its own script converts whole. A duplicate uses its source's script, which converts even when the source is not placed, such as a source without a map. NPCs on instance maps, such as `1@tower`, are not placed, as wAthena has no instances yet.
- `npcstatus.go` converts `sc_start`, `sc_start2`, `sc_start4`, `sc_end` and `getstatus` in NPC scripts. NPC scripts set `cannot_be_avoided`, as rAthena's default flag for NPCs is `SCSTART_NOAVOID`.
- `npclua.go`, `npcexpr.go` and `npcscript.go` translate scripts to Lua with the API of proposal 0002: conversations, `wa.variables`, zeny, items and save points. Labels become Lua labels, and a function's `end` becomes `R.stop()`.
- Event labels run on into the next label until an `end`, as in rAthena. These event labels convert:
  - `OnInit` runs at the end of the NPC's setup function.
  - Clock labels become schedules: `OnClockHHMM` and `OnHourHH` daily, `OnMinuteMM` hourly, and `OnSunHHMM` to `OnSatHHMM` weekly. `OnDayMMDD` does not convert, since the API has no yearly schedule.
  - A label that a `monster` command of the same NPC names runs when each of its monsters dies, with the player that killed it.
- `monster` converts on the NPC's own map, with the monster's own name, the default size and AI, and an event label of the NPC or none. `mobcount` converts for such a label. `killmonster` does not convert.
- `npcparty.go` converts the party commands and reads to `player.party` and `wa.parties` (proposal 0002, review decision 27). A party ID of `getcharid(1)` becomes `player.party`, and any other ID `parties.get(id)`. The write commands convert as statements for the attached player only. `getpartymember` converts for names, and `$@partymembername$` and `$@partymembercount` read its result in the same function. Character IDs, account IDs and the leader's job, map and level do not convert.
- `npcstorage.go` converts the storage commands to the calls of proposal 0002, review decision 29: `openstorage` and `openstorage2 0` to `player:open_storage()`, `storagecountitem`, `cartcountitem` and `guildstoragecountitem` to `count_item` with `from` or `guild:count_item` (-1 where rAthena gives it), `storagedelitem` and `cartdelitem` to `take_item` with `from`, and `getitembound` to `give_item` with `bound`. `basicskillcheck()` reads its rule with `rules.get("basic_skill_check")` (review decision 32). `countbound`, the `2` forms, other storage IDs and modes, and other accounts do not convert, with the reason in the report.
- `checkre(0)` is the converter's mode, as rAthena's `RENEWAL` define. `checkre(1)` to `checkre(5)` read the ruleset flag of their define with `rules.is`: `cast_time`, `level_penalty_drop`, `level_penalty_exp`, `level_damage_mod` and `aspd` (review decision 32). `TestRuleReadsConvert` checks them.
- `npcguild.go` converts the guild reads and commands to `player.guild` and `wa.guilds` (proposal 0002, review decision 28). A guild ID of `getcharid(2)` becomes `player.guild`, and any other ID `guilds.get(id)`. `guild_has_permission` converts for a fixed permission, and `getguildmember` for names, as `getpartymember` does. `guildgetexp` converts for the attached player's guild. `guildopenstorage` calls `R.guild_storage`, which uses `player:open_guild_storage()` and returns rAthena's `GSTORAGE_` codes. `guildopenstorage_log` calls `R.guild_storage_log`, which uses core's `wa.internal` module and says that the log is not available in other mods. Character IDs, guild skill raises, castles and the other guild commands do not convert, with the reason in the report.
- `npcshop.go` converts shops, currencies and mail to proposal 0002, review decision 30:
  - `shop` lines keep the list form. `itemshop`, `pointshop`, `cashshop`, `marketshop` and barter shops use the table form, which players cannot sell to, as in rAthena.
  - An `itemshop` price is an amount of its item, a `pointshop` price an amount of its variable's currency, and a `cashshop` price an amount of `cash`. A `marketshop` item has a zeny price and its stock, and no stock for -1. A barter item costs its zeny and items, with its stock when it is above 0.
  - A barter item that is refined, that takes a refined item or more than 5 kinds of items, or that the barter sells a second time, does not convert. Neither does a `marketshop` item that starts with no stock, or a second entry for an item in a table shop. The shop then converts partly.
  - A `pointshop` variable becomes a currency: `#CASHPOINTS` is `cash`, `#KAFRAPOINTS` is `kafra`, and any other variable is a new record of core's `data/currencies.yml` with an ID from 3, owned by the account for `#` and `##` variables and by the character otherwise. A temporary variable does not convert.
  - Reads of these variables become `player:get_currency`. `x += n`, `x -= n`, `x = x + n` and `x = x - n` become `give_currency` and `take_currency`, or `R.add_currency` for a computed amount. Other assignments become `R.set_currency`, which gives or takes the difference.
  - `callshop` opens the converted shop or barter by its record name. `npcshopupdate`, `npcshopadditem` and `npcshopdelitem` use `world.get_shop`: `set_stock` for a stock, `add_item` for a new price or item, and `remove_item`. A new price converts only for an item the shop's line sells, and a new item only when the shop does not sell it already.
  - `mail` becomes `player:send_mail`, with `R.mail_items` reading the item and amount arrays when the script runs. `openmail`, `searchstores` and `buyingstore` become `player:open_mail()`, `player:search_stores` and `player:open_buying_store`.
  - `npcshopitem`, `npcshopattach`, `sellitem`, `stopselling`, mail with refine, bound, card or option arrays, and `searchstores` on another map do not convert, with the reason in the report.
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
- `TestShopsConvert` checks that every shop line and barter shop of both modes converts. Only a barter whose items all fail may be left out.
- `TestShopKindsConvert` and `TestShopCommandsConvert` check the records of each shop kind and the converted text of each shop, mail, store and currency command. `TestRunShopCommands` runs a converted script with them against a test host.
- server/zone's `kafra_test.go` runs Prontera's converted Kafra: save, teleport with its price and points, and storage with its fee, the free ticket and the Basic Skill check.

`go test ./tools/convert -update` writes the golden files and reports again.
