# Outcome: converter-commands

spent: ~546k tokens (worker, review and fix rounds), 1 active day.

## Built

- None of the ten items converted before: every row below was refused at commit 16d728b.
- Decision record 0072 (docs/decisions/0072-converter-commands.md). Review decision number taken: 65 (proposal 0002, "Decisions taken in review"). No migration (00044 stays free). No design proposal.
- E3 `jobname` -> `jobs.name(job)` (new API, `game.read`). rAthena's `job_name` table is server/modscript/jobnames.go (made from pc.cpp and map_msg.conf), a mod job gives its display_name (`status.Tables.ModJobName`, `modscript.Jobs.ModJobName`), other numbers "Unknown Job".
- G5 `delitem2` -> `player:take_item(item, n, { refine, cards | signed_by_character })`. New `wa.FromOptions.signed_by_character`; take_item takes the exact match, unequipped copies first (`ItemActionHost.TakeItemSpec`, server/zone/itemactions.go). card 1 254 (CARD0_CREATE) converts to the signer; computed cards go through `R.match`. `countitem2` uses the same rule (`matchOptions` in tools/convert/npcstorageitems.go).
- G12 `isequippedcnt` -> `player:count_equipped(item)` (new API, `ItemActionHost.CountEquipped`), `R.count_equipped` for several items.
- G13 `getequiprandomoption` -> `R.equip_option`; new field `wa.RandomOption.id` (the record's rAthena option ID) for ROA_ID.
- G14 `mesitemlink` every form: brackets dropped (the link has none), a name of its own through `R.item_link`.
- G7 barter: a cost item of data/npcs.yml takes `refine` 1 to 20 (copies at it, else the lowest above it, server/zone/shops.go `costSlots`), and a cost takes 6 lines (`mod.MaxCostLines`, was 5). An item a barter sells twice is still refused (see Gotchas).
- G10 computed `callshop` -> `R.callshop(player, name, shops)` over the shops the script's text names and those starting with the name's fixed text (`callshopComputed`, tools/convert/npcshop.go).
- G11 `freeloop` -> a comment.
- G16 `getpartymember <party>,<type>,<.@array>` -> `R.party_members_into`.
- G15 `setunitdata`: nothing was left to map. The 5 refused uses are `UNPC_GROUP_ID`, the client's group mark, which NPC handles lack; the reason now says so.
- Tests: tools/convert `TestItemCommandsConvert`, `TestRunItemCommands`, `TestComputedCallshopConvert`, `TestLoopAndUnitConvert`, new cases in `TestPartyCommandsConvert` and `TestShopKindsConvert`; server/zone `TestCountEquippedAndOptionID`, `TestTakeItemExact`, `TestBarterRefinedCost`; server/mod `TestShopTable`; server/gamedata `TestItemJobNames`.

Rows of the generated reports, commit 16d728b -> this branch. Command rows: scripts | NPC full | NPC partly | NPC not | this part converts.

| Row | renewal before | renewal after | pre-renewal before | pre-renewal after |
|---|---|---|---|---|
| jobname | 2/0/0/2/0 | 2/2/0/0/2 | not in the report | not in the report |
| delitem2 | 1/0/0/1/0 | 1/1/0/0/1 | 1/0/0/1/0 | 1/1/0/0/1 |
| countitem2 | 1/0/0/1/1 | 1/1/0/0/1 | 1/0/0/1/1 | 1/1/0/0/1 |
| isequippedcnt | 6/0/0/6/0 | 8/7/0/1/8 | not in the report | not in the report |
| getequiprandomoption | 1/0/0/1/0 | 1/1/0/0/1 | not in the report | not in the report |
| mesitemlink | 33/17/0/16/20 | 35/33/0/2/35 | not in the report | not in the report |
| callshop | 75/56/0/19/64 | 76/67/0/9/74 | not in the report | not in the report |
| freeloop | 4/0/0/4/0 | 4/3/0/1/4 | 1/0/0/1/0 | 1/0/0/1/1 |
| setunitdata | 26/20/0/6/21 | 26/20/0/6/21 (reason changed) | not in the report | not in the report |
| getpartymember | 7/5/0/2/5 | 7/6/0/1/7 | 6/5/0/1/5 | 6/6/0/0/6 |
| barter lines (full/partly/not of 114) | 103/6/5 | 112/2/0 | none | none |
| script NPCs placed / full / not | 6840 / 6837 / 2379 | 6874 / 6871 / 2345 | 3674 / 3673 / 668 | 3676 / 3675 / 666 |
| script duplicates placed | 6453 | 6478 | 4378 | 4385 |

Remaining reasons: callshop `Pet_Items` and `tool_shop_lasagna#Extended` are not shops in the extract; the 2 partly barters are barter_refine_1 and barter_refine_2 (an item sold twice). The scripts left "not" in the jobname, freeloop, isequippedcnt and mesitemlink rows fail on other commands.

## Review round 1

- Rentals: `count_item` with a refine leaves rentals out again (countitem2 reads expire_time, script.cpp 7192); `take_item`'s exact match takes rentals (delitem2 does not read it). Two predicates in server/zone/itemactions.go: `specMatches` (count) and `exactMatches` (take).
- `delitem2` ends the script when the player has too few (`if not player:take_item(...) then R.stop() end`). Plain `delitem` is unchanged; the orchestrator fixes it apart.
- card 1 254 needs card 2 = 0 (refused otherwise, `R.match` raises), and cards 3 and 4 are masked to 16 bits.
- Barter: a cost refine above the mode's MAX_REFINE asks for MAX_REFINE (zone, as barter_parseBodyNode caps it); a cost refine on an item that cannot be refined fails to load (gamedata npcShops).
- mesitemlink: a computed brackets argument is evaluated (`R.first`); a name of its own keeps `<` and `>`, as rAthena.
- The report lists scripts whose computed callshop may reach a shop that does not convert. None does now, so the note is absent.
- `jobs.name` of a mod job without a display name gives its name in the mod.
- Decision 0072 names the node barter-entries for the barter items sold twice.
- New tests: tools/convert `TestRunCommandHelpers` (R.match, R.count_equipped, R.equip_option on a worn copy, R.party_members_into, R.callshop, R.item_link), more cases in `TestItemCommandsConvert`; server/zone `TestTakeItemExactOrder` (rentals, equipped last, cards), `TestBarterRefineCapped`.
- Report rows did not change.

## Deviations

- G13 uses a new `wa.RandomOption.id` field instead of the name table in rathena.lua the review named: the zone knows the IDs, and it covers mods' options.
- G5 adds `signed_by_character` to `wa.FromOptions`: the one delitem2 user takes a named item, which refine and cards alone cannot match.
- G7 is built in part: refined costs and 6 cost lines. An item sold twice needs a protocol field (`ShopBuyLine` names the item only) and a `shop_stock` key per entry (a migration). GOAL.md asks for a design proposal before a protocol change, so it is left for the orchestrator.
- Files outside the brief's owned and shared paths, each changed minimally: server/status/jobinfo.go (`ModJobName`, since `*status.Tables` is the `modscript.Jobs` of tools/convert's tests), server/mod/npcs.go and shop_test.go (cost refine, 6 lines), server/gamedata/mods.go (passes the refine) and jobs_test.go, tools/convert/npcgroup3.go (countitem2 through `matchOptions`), npcpresent.go (the UNPC_GROUP_ID reason), npclua.go (the array form marks its array), npcstorage.go dispatch, npcreport.go (the barter note), the tests npc_builtins_test.go, npc_party_test.go, npc_shop_test.go, group3host_test.go, new test files npc_commands_test.go, npcshopcomputed_test.go, npcstorageitems_test.go, server/zone/itemcommands_test.go and barter_test.go, mods/README.md ("Shops"), docs/proposals/0002-lua-api.md (review decision 65).
- The brief's note to split into two PRs (party/chat/guild, then storage/items/shops) was not needed: the diff is about 1,500 lines without generated files.

## Stubs left

- None.

## Gotchas

- `wa.jobs` exists only when the constants are loaded (prelude builds it from `H.constants()`). Zone tests without constants cannot call `jobs.name`; tools/convert's run tests can.
- `count_item` with a refine now matches the signer (0 = unsigned). A signed copy no longer counts for a plain countitem2 match. `take_item`'s exact match takes rentals, `count_item`'s does not.
- A cost item with a refine still leaves out copies with cards, and the shop window does not show the refine (no protocol field). The server checks it.
- Plain `delitem` does not end the script when the player has too few, where rAthena does. `delitem2` does.
- `mesitemlink`'s brackets never show; the web client draws a link's item from its own names.
- An item a barter sells twice (5 items, barter_refine_1 and _2) is the open part of G7, for the node barter-entries.

## Proposed learnings

- Read NPC sources from reference/rathena/extracts/<mode>/npcs*.json, not the fork's npc/ tree | the fork's tree lacked the delitem2 user that the extract has | tools/convert | no
- A shop cost or purchase change touches server/mod, server/shop, server/gamedata, server/zone and maybe the protocol and shop_stock | the barter refine needed four packages | server | no

## Verification

- `go test -count=1 -timeout 45m -run 'TestRunConvertedNPCs|TestNPCReportsCurrent|TestClassicNotesCurrent' ./tools/convert`: pass.
- `go test -count=1 ./tools/convert`: pass (after fixing `TestBuiltinsConvert`'s old mesitemlink expectation).
- `go test -p 1 -count=1 ./server/modscript/... ./server/mod ./server/shop ./server/status ./server/gamedata`: pass. `go test -p 1 -count=1 ./server/zone`: pass (192 s).
- `scripts/check.sh vet lint`: pass.
- After review round 1: `go test -count=1 ./tools/convert` (includes the three report and run tests), `go test -p 1 -count=1 ./server/gamedata ./server/status ./server/mod ./server/modscript/... ./server/zone`, `scripts/check.sh vet lint`: pass.
- Reports regenerated with `go run ./tools/wactl convert npcs --mode <renewal|pre-renewal> --report ...` and `convert classic`; Lua docs with `go test ./server/modscript/luaapi -update`.

## Size

- Estimated L, actual L.
