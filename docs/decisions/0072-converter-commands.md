# 0072: Converter Commands: Job Names, Exact Item Matches, Equipment Counts, Refined Barter Costs, Computed callshop

- Status: built. Details to review.
- Date: 2026-11-05.
- Code: [tools/convert](../../tools/convert) (`npcstorageitems.go`, `npcshop.go`, `npcparty.go`, `npcexpr.go`, `npcgroup3.go`, `npcpresent.go`, `npclua.go`, `npcstorage.go`, `npcreport.go`, `rathena.lua`), [server/modscript](../../server/modscript) (`jobnames.go`, `jobs.go`, `itemapi.go`, `services.go`, `market.go`, `prelude.lua`, `luaapi/spec.go`), [server/zone](../../server/zone) (`itemactions.go`, `rolls.go`, `shops.go`, `scripts.go`), [server/mod](../../server/mod) (`npcs.go`), [server/shop](../../server/shop) (`shop.go`), [server/status](../../server/status) (`jobinfo.go`), [server/gamedata](../../server/gamedata) (`mods.go`).
- Docs: [lua-api.md](../generated/lua-api.md), [mods/README.md](../../mods/README.md) ("Shops"), [npc-scripts-renewal.md](../generated/npc-scripts-renewal.md), [npc-scripts-pre-renewal.md](../generated/npc-scripts-pre-renewal.md).
- Builds the converter-commands items of review decision 60 of [proposal 0002](../proposals/0002-lua-api.md): E3, G5, G7 (but an item sold twice), G10, G11, G12, G13, G14, G15 (`setunitdata`) and G16. Review decision 65 records the API details.
- Lua API: adds to API 1.0. Protocol: none. Settings: none. Data: a cost item's `refine`, and 6 cost lines. Store: none. Permission: none new.

## Context

- The reports refused `jobname`, `delitem2`, `isequippedcnt`, `getequiprandomoption`, `freeloop`, `callshop` of a computed name, `mesitemlink` without brackets or with a name, `getpartymember` into a script's own array, `setunitdata`'s `UNPC_GROUP_ID`, and barter items that take refined items, more than 5 kinds of items, or that a barter sells twice.

## Decision

### jobname (E3)

- `jobname(<job>)` becomes `jobs.name(job)`. For core's jobs it gives rAthena's `job_name` (pc.cpp), whose names come from `conf/msg_conf/map_msg.conf`: "Swordsman", "Magician", and the mounted forms' own names ("Knight" for `JOB_KNIGHT2`). The table is `server/modscript/jobnames.go`, made from those two files. A mod job gives its record's `display_name`, or its name in the mod when the record has none. Any other number gives "Unknown Job", msg 655. `display_name` of core's records stays as it is ("Swordman"): it is what the client shows.

### delitem2 and countitem2 (G5)

- `delitem2 <item>,<amount>,<identify>,<refine>,<attribute>,<card1>..<card4>` becomes `player:take_item(item, amount, { refine, cards })`. Only identified copies that are not broken convert, as for `countitem2`.
- card 1 `CARD0_CREATE` (254) is a copy named for the character in cards 3 and 4, as getnameditem makes it. It converts to `signed_by_character`. Computed cards go through `R.match`, which reads them when the script runs. A forged or pet mark stays refused. `countitem2` uses the same options, so the sign quest's check (the_sign_quest.txt, Knight#ss) now runs.
- `take_item` with these options takes the copies that are not equipped first, then the equipped ones, in slot order, as `buildin_delitem_search` with an exact match. Too few take nothing.
- With a refine, `count_item` also matches the signer (0 for none). It still leaves rentals out, unless `rentals` is set, as `script_countitem_sub` (script.cpp 7192). `take_item` takes rentals too: `buildin_delitem_search` reads no rental time.
- A card 1 of 254 needs card 2 to be 0, as a named copy has: rAthena compares all four cards, so another card 2 matches nothing. Such a call is refused, and `R.match` raises an error for it. Cards 3 and 4 are read as 16-bit halves.
- When the player has too few, `delitem2` ends the script, as rAthena's, which ends it and closes the dialog (script.cpp 8771): `if not player:take_item(...) then R.stop() end`. Plain `delitem` still goes on; that is a separate fix.

### isequippedcnt (G12)

- `isequippedcnt(<item>...)` becomes `player:count_equipped(item)`, or `R.count_equipped` for several items, which skips IDs of 0 or less and repeated IDs (script.cpp 16735). A worn copy counts its amount once, even when it fills two slots. For a card, each place it fills in a worn copy counts one. wAthena's copies hold no forged or named mark among their cards, so `itemdb_isspecial` needs no check.

### getequiprandomoption (G13)

- `getequiprandomoption(<slot>, <index>, <type>)` becomes `R.equip_option`. ROA_ID reads the new `id` field of `wa.RandomOption`, the option record's rAthena ID, ROA_VALUE `value` and ROA_PARAM `param`. An index outside 0 to 4 or an empty slot gives -1, an index past the copy's options 0, and another type 0, as rAthena's. The type must be fixed.
- The review asked for a table of option names in `rathena.lua`. The zone has each record's ID (`item.RandomOption.ID`), so a field is smaller and covers mods' options too. A mod's option has `id` 0, the same as a slot without an option.

### mesitemlink (G14)

- Every form converts: `mesitemlink(<item>{, <brackets>{, <name>}})`. `text.item_link`'s link has no brackets, and the web client draws a link's item from its own item names (client/src/viewer/dialog.ts), with the link's text as its title. So the brackets argument changes nothing. It is still evaluated when it is not fixed, for its effects (`R.first`). A name of its own becomes the link's text through `R.item_link`, as rAthena writes it, with `<` and `>` kept. The web client's link pattern stops at `<INFO>`, so only a name with `<INFO>` in it would break the link. `R.item_link` is "" for an item that does not exist, as rAthena's.

### Barter costs (G7)

- A cost item of `data/npcs.yml` takes `refine`, 1 to 20. It takes copies at that refine, then at the lowest refine above it, as `npc_barter_purchase`'s search for each copy (npc.cpp 3266). Costs with a refine take their copies after the unrefined costs of the same item. A cost item without a refine keeps its rule: unrefined copies.
- A refine above the mode's MAX_REFINE (10 in pre-renewal) asks for MAX_REFINE, as barter_parseBodyNode caps it. A cost refine on an item that cannot be refined fails to load, as rAthena refuses the line.
- A cost takes at most 6 items and currencies, as `MAX_BARTER_REQUIREMENTS`, up from 5. `shop:add_item`'s cost follows.
- A barter that sells an item twice does not convert yet. A purchase names the item (`ShopBuyLine.item_id`), and the shop's stock is saved by shop and item (`shop_stock`). Selling one item twice needs an index in the protocol and in the table. The roadmap node barter-entries (a `should` node) builds it. See Details to review.

### callshop of a computed name (G10)

- `callshop` of a computed name becomes `R.callshop(player, name, shops)`. `shops` maps the shop names the script's text has, and the shops whose names start with the name's fixed text (`"shop_ext171_" + .@str$`), to their records, or to false for a shop that does not convert. Another name opens nothing, as rAthena's, which logs "Shop not found" and goes on. A false one raises an error. The report lists the scripts that may reach such a shop, since their rows count as converted.

### freeloop (G11)

- `freeloop` converts to a comment. The call keeps its CPU budget. The loops it guards walk quest IDs and item lists, far below the budget.

### getpartymember into an array (G16)

- `getpartymember <party>,<type>,<array>` fills a `.@` array of the type's kind from index 0 and keeps its other elements, as `setd_sub_num` and `setd_sub_str`. `$@partymembercount` is set as before. Other arrays stay refused.

### setunitdata (G15)

- The `UNPC_` looks and the `UMOB_` types the API has were built (decisions 0063 and 0065). The only refused type left, `UNPC_GROUP_ID` (5 shop marks of Extended_Ammunition.txt), sets the group mark the client shows by a unit's name (`clif_name`'s groupId). NPC handles have no such field, so it stays refused with that reason.

## Breaks

- None for mods. `player:take_item`'s options take more fields, `count_item` with a refine counts signed copies only for their signer, and a cost may have 6 lines.

## Alternatives

- **A name table for random options in `rathena.lua`** (G13), as the review wrote. Larger, and blind to mods' options.
- **Clicking a barter item by index** (G7). It needs a protocol field and a stock key per entry: left for a decision.
- **Changing core's job display names to rAthena's** (E3). The client shows them. jobname's names are a script's text.

## Details to review

- Plain `delitem` goes on when the player has too few, where rAthena ends the script and closes the dialog. `delitem2` now ends it. delitem is left for a separate fix.
- A barter item that a barter sells a second time (barter_refine_1 and barter_refine_2: 5 items) does not convert. It needs `ShopBuyLine` to name the entry, a protocol change, and `shop_stock` keyed by entry, a migration. None of the 5 has a stock. The node barter-entries builds it.
- A cost item with a refine never takes copies with cards, as cost items always did. rAthena's barter takes them. `shop:add_item`'s cost has no refine, so a script cannot make a refined cost (marketscripts.go). The shop window does not show the refine a cost asks for, since `ShopCostItem` has no refine: a protocol field. The server checks it.
- rAthena's barter with a required refine of 0 takes +0 copies first, then the lowest above. The converter writes no refine for 0, which takes unrefined copies only. No barter of the tree asks for refine 0 on a refinable item.
- `jobs.name` follows `job_name` exactly, also where rAthena gives one name for two jobs, such as `JOB_SUMMER2`, which is "Summer".
- `mesitemlink`'s brackets do not show, in either form. rAthena's default link has them. A name of its own keeps `<` and `>`, as rAthena's, where `text.item_link` strips them from item names.
- `callshop` of a static name that is not a shop stays refused (`Pet_Items`, `tool_shop_lasagna#Extended`), where rAthena logs an error and goes on.
- `getequiprandomoption`'s index reads `random_options` in order. wAthena lists the copy's options without gaps. rAthena's slots may have an empty slot before a filled one.
- `isequippedcnt` counts a card in any worn copy. rAthena skips copies whose card 1 is a forged, named or pet mark. wAthena's copies keep those marks outside the card list, so the result is the same.

## How to check it worked

- `go test ./tools/convert -run 'TestItemCommandsConvert|TestRunItemCommands|TestComputedCallshopConvert|TestLoopAndUnitConvert|TestPartyCommandsConvert|TestShopKindsConvert'`.
- `go test ./server/zone -run 'TestCountEquippedAndOptionID|TestTakeItemExact|TestBarterRefinedCost'`.
- `go test ./server/mod -run TestShopTable` and `go test ./server/gamedata -run TestItemJobNames`.
