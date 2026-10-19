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

- `parse.go` also reads NPC scripts: labels, `goto`, `menu`, `callsub`, `function` and `switch`. As rAthena's parser does:
  - Statements before the first `case` of a `switch` run after the value is read and before the cases are compared.
  - The condition of `if`, `while` and `switch` is the whole expression from its `(`, so `if (a) || (b)` reads as one condition.
  - Digits before a letter or `_` start a word, such as the sprite constant `4_M_CACTUSMAN20`.
  - `set` on a reference that is not a variable, such as `set getvariableofnpc(.x, "NPC"), 1;`, does not parse yet.
- `npc.go` places NPCs and shops from `npcs.json`, its part files of the `quests`, `warps` and `events` folders (review decision 55), and the barter shops of `shops.json`, and writes core's `data/npcs.yml`. The part files come after `npcs.json`, so the record names of its NPCs do not change. Shops become data. A script NPC is placed when its own script converts whole. A duplicate uses its source's script, which converts even when the source is not placed, such as a source without a map. NPCs on instance maps, such as `1@tower`, are not placed, as wAthena has no instances yet.
- `npcstatus.go` converts `sc_start`, `sc_start2`, `sc_start4`, `sc_end` and `getstatus` in NPC scripts. NPC scripts set `cannot_be_avoided`, as rAthena's default flag for NPCs is `SCSTART_NOAVOID`.
- `npclua.go`, `npcexpr.go` and `npcscript.go` translate scripts to Lua with the API of proposal 0002: conversations, `wa.variables`, zeny, items and save points. Labels become Lua labels, and a function's `end` becomes `R.stop()`.
- Event labels run on into the next label until an `end`, as in rAthena. These event labels convert:
  - `OnInit` runs at the end of the NPC's setup function.
  - Clock labels become schedules: `OnClockHHMM` and `OnHourHH` daily, `OnMinuteMM` hourly, and `OnSunHHMM` to `OnSatHHMM` weekly. `OnDayMMDD` does not convert, since the API has no yearly schedule.
  - A label that a `monster` command of the same NPC names runs when each of its monsters dies, with the player that killed it.
  - `OnTouch` becomes a `touch` handler, and `OnTouch_` a handler through `R.touch_one`, which runs it for one player at a time, not for a hiding or dead player, and for the next player in the area when that one leaves (`npc_touchnext_areanpc`). `OnTouch_` wins over `OnTouch`, as in rAthena. The line's span becomes the record's `touch` field, and a line without a sprite (-1) that has a touch area gets `HIDDEN_NPC` (review decision 55).
  - A touch handler runs in a conversation with the player (review decision 56), so an `OnTouch` or `OnTouch_` part with a dialog converts, and so does one that calls a function with one. `R.touch_one(fn, true)` marks a part with a dialog: it does not run for the next player already in the area, who has no conversation, and runs for the next player who comes in instead.
- `jobchange <job>,Upper`, rAthena's `Job_Change`, converts in a function for a normal player, as `jobchange <job>` does. A transcendent or baby player's change fails when it is reached, so the function stays partly converted. Other second arguments do not convert.
- `skill` and `addtoskill` convert to `player:give_skill` (review decision 57), so the renewal first job changers convert:
  - `SKILL_PERM` (0) is `"permanent"`, `SKILL_TEMP` (1) `"temporary"` and `SKILL_TEMPLEVEL` (2) `"temporary_add"`.
  - Without a flag, `skill` is `"temporary"` and `addtoskill` is `"temporary_add"`, as in rAthena.
  - A skill by name or by a fixed ID converts. A computed skill or flag does not, and neither does `SKILL_PERM_GRANT` (3), which the API does not have.
- `percentheal` with a negative amount converts to `player:heal_percent`, which takes that percent of the maximum and never takes HP below 1 (review decision 56). rAthena kills at -100.
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
- `quests.go` converts rAthena's quests from `quests.json` to core's `data/quests.yml` (proposal 0002, review decision 45, [decision 0043](../../docs/decisions/0043-quests.md)). A record's name is the quest's ID. A `TimeLimit` with "+" becomes a duration, such as `3h`, and one without becomes `{ at, days }` or `{ at, weekday }`. Races, sizes and elements take the names of `wa.races`, `wa.sizes` and `wa.elements`. `wactl convert quests --report docs/generated/quests-<mode>.md` writes the coverage report, which `TestQuestReportsCurrent` checks.
- `npcquest.go` converts the quest commands to the quest calls of review decision 45:
  - `setquest`, `completequest`, `erasequest` and `changequest` become `player:add_quest`, `player:complete_quest`, `player:remove_quest` and `player:change_quest`, with the quest's ID.
  - `checkquest` becomes `R.checkquest` and `isbegin_quest` `R.isbegin_quest`, which return rAthena's numbers. rAthena counts a quest without a time limit as timed out for `PLAYTIME` and `HUNTING`, since its end time is 0. The converter passes whether the mode's quest has a limit; for a computed ID it passes nil, and the quest's own limit decides.
  - A fixed ID that the mode's `quest_db` lacks converts to what rAthena does: the commands become a comment, as rAthena only logs an error, `checkquest` is -1, and `isbegin_quest` 0.
  - `showevent` becomes `npc:show_quest_icon` with the icon's name. `QTYPE_DAILYQUEST`, `QTYPE_NONE` and other values remove the icon, as `buildin_showevent` at PACKETVER 20211103. A minimap mark color other than `QMARK_NONE`, a computed icon, and every command for another character do not convert.
- `rathena.lua` is `scripts/lib/rathena.lua`: helpers for what Lua lacks, such as rAthena's arrays, `menu`, `getarg`, `input` with its bounds, `getitem` and `delitem` with a computed amount, which do nothing for 0 or less, `monster` and `mobcount`, `OnTouch_` (`R.touch_one`) and `pc_ishiding` (`R.hiding`). `R.checkquest` and `R.isbegin_quest` read a computed quest ID that `data/quests.yml` lacks as a quest the player does not have, as `quest_check` does.
- Kafra pushcart rental converts: `F_KafCart` reads `BaseClass`, `checkcart()` and `getskilllv`, takes the Free Ticket for the Cart Service or the fee, and calls `setcart`, which is `player:set_cart`. With Niflheim's negative `percentheal`, every Kafra converts fully.
- `npcreport.go` writes the coverage report: by kind, by folder, the touch areas and their dialogs, by command, by function and by file.
- `classicnotes.go` writes the release notes list of classic mode, `docs/generated/classic-coverage.md`, from renewal's coverage (review decision 55): the NPC lines by folder and the folders the extract leaves out, the most common reasons the rest is not placed, and the systems that are not built.

```text
go run ./tools/wactl convert npcs --mode renewal --out /tmp/core-npcs
go run ./tools/wactl convert npcs --mode renewal --report docs/generated/npc-scripts-renewal.md
go run ./tools/wactl convert classic --report docs/generated/classic-coverage.md
```

NPC tests:

- `TestRunConvertedNPCs` starts every converted NPC of both modes in a Lua state per map, as each zone has, steps into and out of their touch areas, kills the monsters they spawn, runs their schedules, and fails on any error.
- `TestNPCGolden` keeps the output of sample NPCs and functions in `testdata/golden/npcs`, such as Prontera's Kafra and its functions, `F_KafCart` among them, a dialog NPC, an inn and a tool dealer.
- `TestParseNPCScripts` parses every script of both modes. Only three may fail, besides `set` on a reference that is not a variable.
- `TestNPCEventsConvert` converts NPCs written for the test, with clock labels and monsters, and keeps two in `testdata/golden/npcs/events_*.lua`. `TestRunNPCEvents` runs them: their schedules, and their monsters' deaths.
- `TestNPCTouchConvert` converts NPCs written for the test with `OnTouch` and `OnTouch_`: the handlers, the records' `touch` field, `HIDDEN_NPC`, and the dialogs, which convert. It keeps three in `testdata/golden/npcs/touch_*.lua`. server/zone's `touch_test.go` runs a converted `OnTouch_` in a zone, with and without a dialog.
- `TestNPCReportsCurrent` checks the reports in `docs/generated`, and `TestClassicNotesCurrent` the release notes list.
- `TestQuestsConvert` converts both modes' quests and builds every record as the server does. `TestQuestCommandsConvert` checks the converted text of each quest command, and `TestRunQuestCommands` runs them against a quest log, with rAthena's numbers for a quest without a time limit. `TestNPCGolden` keeps Dancer Kim, of the Wanderer job change, which gives and checks a hunt quest.
- `TestShopsConvert` checks that every shop line and barter shop of both modes converts. Only a barter whose items all fail may be left out.
- `TestShopKindsConvert` and `TestShopCommandsConvert` check the records of each shop kind and the converted text of each shop, mail, store and currency command. `TestRunShopCommands` runs a converted script with them against a test host.
- server/zone's `kafra_test.go` runs Prontera's converted Kafra: save, teleport with its price and points, storage with its fee, the free ticket and the Basic Skill check, and pushcart rental with its fee, points, free ticket and refusals.
- e2e's `TestClassicKRO` runs the converted NPCs in a classic world of a full kRO asset build, when `WATHENA_KRO_ASSETS` names one: a Kafra's save, storage and teleport, a shop, a field's warp and monsters, and the Taekwon job change (e2e/README.md).

`go test ./tools/convert -update` writes the golden files and reports again.
