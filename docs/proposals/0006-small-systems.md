# 0006: Small Systems

Eleven systems that rAthena's scripts call and that wAthena lacks: NPC and player chat rooms, marriage, achievements, client navigation, the stylist, the zeny bank, and four item windows (refine, item enchant, enchant grade, item reform). This proposal designs each: the rAthena behavior to match, the data and extracts, the Lua API, the converter rows, the protocol, the database rows, the client windows, the ruleset rules, the admin tools and the tests. It adds to API 1.0 as proposal 0002's review decisions do. It also says which parts leave v0.2.0 because they need classes or systems wAthena does not have.

Words used here:

- **Room**: a chat room (rAthena's `chat_data`, `BL_CHAT`). An **NPC room** belongs to an NPC and is made by `waitingroom`. A **player room** belongs to a player and is made from the client.
- **Trigger**: the number of users at which an NPC room runs its event.
- **Vault**: the zeny a player keeps in the bank, shared by the characters of an account.
- **Grade**: an equipment's enchant grade, 0 for none and 1 to 4 for D to A (`item.enchantgrade`).
- **Enchant**: an item stored in a card position beyond the item's own card slots (rAthena's `card[1..3]` of the item enchant window). It is not a refine and not a grade.

## Problem

- Nothing here exists. No chat room, partner link, achievement, route, style change, vault or item window is in the server, the protocol or the client. The converter refuses every command in the list below with "not supported".
- Counts from the reports (first blocker of a script, renewal; pre-renewal in brackets): `navigateto` 73, `waitingroom` 33 (24), `getpartnerid` 22 (22), `achievementinfo` 14, `refineui` 9 (7), `openstylist` 7, `enablewaitingroomevent` 2 (2), `enchantgradeui` 2, `achievementexists` 1, `openbank` 1, `item_enchant` 4 NPC rows and 7 item rows, and 130 delayed-use items that call `item_reform`. The source has more call sites (comments left out): `navigateto` 162, `F_Navi` 345, `waitingroom` 58, `warpwaitingpc` 27, `marriage` 52, `achievementupdate` 14, `item_enchant` 15, `item_reform` 12. The brief's counts match the reports.
- `npc/re/other/achievements.txt` has 295 NPC lines (the Adventure treasure chests) and converts 0 of them. `npc/other/marriage.txt` and `divorce.txt` convert 8 of 11 and 0 of 1 NPCs.
- Hooks that wait for this work:
  - `items.refine_cost(item, refine, kind)` and `items.refine_chance(..., {enriched})` take a kind, but `refine_db.json` holds the Normal cost only, so HD and Enriched raise an error (decision 0064; server/zone/itemactions.go:255).
  - The map flags `nochat` and `nobank` and the cell `no_chat` are refused with "chat rooms do not exist" and "the bank does not exist" (server/zone/maps.go:812, 1032, 1034; tools/convert/npcmaps.go).
  - `getbattleflag("feature.refineui")`, `feature.achievement` and `feature.stylist` convert to constant 0 (tools/convert/npcgroup3.go:58), so the refine NPCs take their no-window branch.
  - `equipped.enchant_grade` returns 0 (server/zone/equipment.go:554). `heal.go:71` says partners are not ported.
  - Converted guides send `<NAVI>[name]<INFO>map,x,y,0,000,0</INFO></NAVI>` in dialog text (`F_Navi` converts whole, PACKETVER is 20211103), and `client/src/viewer/dialog.ts:19` has no token for it, so players see raw markup.
  - Item copies have no grade and no enchant positions (`item.Slot` has `Refine` and `Cards`, a list of filled card slots).

What rAthena does, by system:

- **Chat rooms** (`chat.cpp`, `script.cpp` 13089 to 13335). A room has a title (36), a password (8, player rooms), a public flag, a limit (at most `MAX_CHAT_USERS` 20), a trigger, a trigger event, a zeny fee, and a level range.
  - Join (`chat_joinchat`) fails, in this order: no such room or another map, the joiner is vending, running a buying store or already in a room, or the room is full (an NPC room counts the NPC, so it takes `limit - 1` players); wrong password unless public or the `join_all_chat` permission; level out of range; zeny below the fee (the fee is only checked here); on the kick list (player rooms). A join stops the walk, sends the room to the joiner and the others, and runs the trigger.
  - Leave: the first remaining user becomes owner of a player room, and an empty player room is deleted. An NPC room stays. Kicked users of a player room cannot return. Owners change the title, password, limit and public flag, hand over ownership and kick (not staff with `no_chat_kick`).
  - Trigger: after a join, `users >= trigger` runs the room's event on the NPC (`npc_event_do`, no player). `enablewaitingroomevent` clears bit 0x80 and runs the check at once; `disablewaitingroomevent` sets it. The trigger is at most 127.
  - `warpwaitingpc map,x,y{,n}` warps the first `n` users (default: the trigger) in join order. Each pays the fee, and the loop stops at the first that cannot pay or, for `SavePoint`, stands on a `noteleport` map. It fills `$@warpwaitingpc[]` with the warped players' account IDs and `$@warpwaitingpcnum` with their count. `Random` is a random cell of the user's map.
  - `getwaitingroomstate(type{,npc})`: 0 users, 1 limit, 2 trigger, 3 disabled, 4 title, 5 password, 16 event, 32 full (`users >= limit`, which differs from the join test), 33 triggered; -1 for none.
  - While in a room a player cannot use items or skills (clif.cpp 12103; skill.cpp 8241, 9331), speaks to the room only, and leaves on a map change or logout. Creating needs no `nochat` map or `CELL_NOCHAT` cell, no NPC within `min_npc_vendchat_distance`, Basic Skill 4 (`basic_skill_check`) and no `SC_NOCHAT` room ban. Unloading an NPC deletes its room.
- **Marriage** (`pc.cpp` 12826 to 12925, `script.cpp` 14408 to 14530). `partner_id` is symmetric. `pc_marriage` refuses a married or baby player. `pc_divorce` clears both and deletes the wedding rings (2634, 2635) from both inventories; if the partner is offline the char server does it (`chrif_divorce`). `getpartnerid` gives the partner's character ID. `wedding` shows an effect. The two rings' item scripts give WE_MALE, WE_FEMALE, WE_CALLPARTNER, WE_CALLALLFAMILY and WE_ONEFOREVER.
  - WE_MALE heals the partner by the HP the caster paid (10 percent of max); WE_FEMALE does the same for SP; both target the partner only (unit.cpp 2158). WE_CALLPARTNER puts a ground unit that warps the partner to the cell. WE_CALLALLFAMILY warps the partner and the child (same map, same party, alive). WE_ONEFOREVER revives a dead partner or child with 30 percent HP. A female Super Novice's Heal on her partner doubles (skill.cpp 602). The wedding skills never count as learned skills.
  - Adoption (`pc_adoption`, the baby skills WE_BABY, WE_CALLPARENT, WE_CALLBABY and WE_CHEERUP, `father`, `mother`, `child`) needs baby classes. No script command uses `adopt`, `getchildid`, `getmotherid` or `getfatherid`.
- **Achievements** (`achievement.cpp`, `db/{re,pre-re}/achievement_db.yml`, `achievement_level_db.yml`). 361 records in renewal and 359 in pre-renewal (130005 and 200035 are renewal only); 20 levels, identical in both modes. A record has a group, an optional condition script, targets (count and monster), dependents, a reward (item, script, title 1000 to 1046) and a score.
  - Groups and what fires them: `Battle` and `Taming` (a kill or a tame of the target monster; with `achievement_mob_share` the party members in range too, default off); `Goal_Level` (base and job level), `Goal_Status` (a stat change and a level up), `Job_Change`, `Get_Item` (the item's sell value), `Get_Zeny`, `Spend_Zeny` (vending), `Add_Friend`, `Party` (creating one), `Marry`, `Baby`, `Enchant_Success` and `Enchant_Fail` (refines), `Chatting_Create`, `Chatting_Count`, `Chatting_Dying`, `Goal_Achieve` (a level change). `Adventure` records are given by scripts (`achievementcomplete`) or complete when their dependents do. `Eat` and `Chatting` are never fired: their 15 renewal records cannot complete.
  - A condition reads `ARG0..` (the trigger's values), `Class`, `BaseLevel`, `JobLevel`, `readparam`. 93 renewal records have one. 51 have a reward script and 203 a reward item. 17 have a title.
  - At login every group is checked with no arguments. Completing moves the record to the completed block, recalculates level and score, completes dependents, and fires `Goal_Achieve` on a level change. Claiming a reward (`CZ_REQ_ACH_REWARD`) needs it completed and unclaimed, saves the time, runs the reward script, and mails the item from "GM" (`int_achievement.cpp` 303). The title list at login holds every completed record's title, claimed or not. The player picks a title (`CZ_REQ_CHANGE_TITLE`), shown with its name.
  - `achievementadd` fails if present; `achievementinfo` types are the counts 1 to 10, complete 11, date 12, reward 13, level 14, score 15, with -1 when absent; `achievementupdate` sets a count, `completed` or `rewarded` and resends; `achievementcomplete` adds if needed.
- **Navigation** (`clif_navigateTo`, `F_Navi`). The server sends a map, a cell, a monster, a service flag (`NAV_*`) and a hide-window flag. The client draws the route. Renewal only in content: all call sites are under `npc/re`.
- **Stylist** (`clif.cpp` 22861 to 23030, `db/re/stylist.yml`). The window sends a hair color, hair style, cloth color, top, mid and bottom accessory and a third-class body style. Each part is charged in that order: zeny, and one required item or a box holding it (human or Doram price). Looks apply through `pc_changelook`; accessories arrive by mail from "Styling Shop". Renewal data: 71 options in seven looks, 52 with a required item. Pre-renewal has no data.
- **Bank** (`clif.cpp` 7392 to 7600, `pc.cpp` 14927). The vault is the account variable `#BANKVAULT`, at most `MAX_BANK_ZENY` (2,147,483,647). Deposit refuses a non-positive or overflowing amount and too little zeny; withdraw refuses a non-positive amount, too much, and a wallet past `MAX_ZENY`. `feature.banking` is on; `feature.banking_state_enforce` is off because kRO clients do not report the window state, so any client may bank anywhere. The map flag `nobank` refuses it. One script uses it (`openbank`, Rock Ridge).
- **Refine window** (`clif.cpp` 22539 to 22800; `db/{re,pre-re}/refine.yml`). The window lists, for an identified, unbroken, refineable item, each cost type's ore, chance and zeny, and the Blacksmith Blessing amount. A request names the ore and whether to use the blessing. It pays zeny and ore (and blessings), then draws: success adds 1 and may broadcast; failure with a blessing changes nothing; failure otherwise breaks the item with `BreakingRate`, else downgrades by `DowngradeAmount`, else changes nothing. Weapons fire `Enchant_Success`; every failure fires `Enchant_Fail`.
- **Item enchant window** (`clif.cpp` 24514 to 25040; `db/re/item_enchant.yml`, 138 groups). `item_enchant(group)` opens it (refused at 70 percent weight). Four requests: random (the first empty position of `Order`; fee, materials, a chance with grade bonuses; a retry draw across the grade's enchants), perfect (a chosen enchant), upgrade (an enchant to its next tier) and reset (clear every enchant, with a chance). Targets must be unequipped, unbroken, in the group's list, over its minimum refine and grade, and have no random options unless allowed.
- **Enchant grade window** (`clif.cpp` 23932 to 24320; `db/re/enchantgrade.yml`). `enchantgradeui()` opens it. An item at refine `r` and grade `g` shows a success chance, a catalyst (Blessed Etel Dust, up to 10 steps, each adding chance) and cost options (ore, zeny, breaking, downgrade). Success adds a grade and resets the refine to 0; failure may break, downgrade or change nothing; the result may be announced. The grade adds a percentage of the refine ATK bonus in renewal (`status.cpp` 3936 to 3965).
- **Item reform window** (`clif.cpp` 24324 to 24512; `db/re/item_reform.yml`, 131 items, 1,839 base rows). `item_reform{(item)}` opens it for a reform item (from the item's own script, or by name). A request names an inventory item. It must be a listed base, identified, unequipped, in the refine range, without cards when forbidden, and with the required random options. It consumes the materials and the reform item, applies a random option group, changes the refine, optionally clears the card slots and enchants and the grade, and swaps the item ID.
- Mode differences, checked in the fork: chat rooms, marriage, achievements, navigation (engine), the bank and the refine window exist in both modes; stylist data, the item enchant, the enchant grade and the item reform data exist in renewal only (their db files import `db/re`); `refine.yml` has Normal, HD and Enriched rows in renewal (160, 140, 90) and Normal and Enriched in pre-renewal (70, 70); the grade bonus is renewal only (`#ifdef RENEWAL`).

## Change

### 1. What leaves v0.2.0

Checked by grep in the repo: no baby class can be reached (jobchange refuses baby jobs, no core path adds one, strategy section 11.1 lists baby classes as tier 3), and the cash shop is tier 3 (strategy section 11.3).

- **Adoption and the baby skills** (`pc_adoption`, WE_BABY, WE_CALLPARENT, WE_CALLBABY, WE_CHEERUP, `child`, `father`, `mother`, `Baby` achievements, `getchildid`, `getmotherid`, `getfatherid`, `adopt`) go to `tier3-systems`, v0.3.0, with the baby classes. No script command needs them. The marriage tables leave room: v0.3.0 adds columns.
- **Kafra points paying part of a cash shop price** (review decision 60's G8) goes to `tier3-systems` with the cash shop.
- **Service routes of navigation** (airship, Kafra teleport, scroll edges) are not extracted in v0.2.0 (section 5).
- **Doram and third-class body styles in the stylist.** The data loads; the Doram price applies once Summoner exists, and `Body2` options need third classes. They stay in the records.

Everything else is in v0.2.0. Item reform, item enchant and the enchant grade do not need third or fourth classes: they change items, and the items exist in the extracts (many reform bases are fourth-class gear, which nobody can wear until v0.3.0).

### 2. Chat rooms

Rooms are zone-local, as joining needs the same map. They are not saved. The package `server/chatroom` holds the pure state (members in join order, kick list, limits, join checks in rAthena's order, owner transfer, trigger state) with its tests; `server/zone/chatroom.go` wires it to players, NPCs, the map flag and events.

- **NPC room.** `npc:open_chat_room{title, limit, event?, trigger?, zeny?, min_level?, max_level?}` as `waitingroom`: `trigger` defaults to `limit`, `min_level` to 1, `max_level` to the level cap. A second call on an NPC that has a room is ignored with a log line, as rAthena. The room sits at the NPC's cell, takes `limit - 1` players, and is deleted with the NPC or by `npc:close_chat_room()`. `event` names a label (`"Lv50 Waiting Room::OnStartArena"`) or a function. A label goes out as an NPC event with no player (`world.fire_npc_event`), as 0004's `OnAgit*` labels do. Other NPC calls take an optional NPC name, as the commands do.
- **`wa.ChatRoom`** (a handle, `npc:chat_room()` or `player.chat_room`): fields `title`, `limit`, `trigger`, `trigger_disabled`, `event`, `users` (count), `password` (empty for an NPC room), `public`, `full` (`users >= limit`), `triggered` (`users >= trigger`), `owner`; `members()` in join order. Methods (need `chat_rooms.manage`): `enable_event()`, `disable_event()`, `kick(name)`, `kick_all()`, `warp_members(map, x, y, n?)`, which returns the warped players and takes the fee as above. `map` may be `"Random"` or `"SavePoint"`.
- **Player room.** `ChatRoomCreate` needs: Basic Skill 4 (`basic_skill_check`), no `SC_NOCHAT` room ban, no room yet, not vending or running a store, no NPC within `min_npc_vendchat_distance`, a map without `nochat`, a cell without `no_chat`. Title 1 to 36 characters, password up to 8, limit 1 to 20, public or private. Joining, leaving, owner change, kick and status change follow the rules above. Kicks skip staff who hold the staff right `no_chat_kick`, as rAthena's permission.
- **Behavior while in a room**: items, skills, trades, shops, NPC clicks and attacks are refused; text goes to the room (`CHAT_CHANNEL_ROOM`); a warp, logout or death removes the member. A member that sends `WalkTo` leaves first (rAthena lets a modified client walk; ours does not need that).
- The map flag `nochat` and the cell `no_chat` stop being refused (`maps.set_flag`, `maps.set_cells`; `maps.control`). `checkchatting` is `player.in_chat_room`.
- `warpwaitingpc` stays the converter's `R.warpwaitingpc`, which calls `warp_members` and writes the server variables `$@warpwaitingpc` (account IDs) and `$@warpwaitingpcnum`.
- Battleground rooms (`waitingroom2bg`) wait for battlegrounds (v0.3.0). A converted arena that needs PvP maps stays unplaced because PvP is not ported; its room call converts.

### 3. Marriage

- **Record.** `character_partners` (section 9): two rows per marriage, one for each partner, tied so that one cannot exist without the other. A character delete removes both rows (`ON DELETE CASCADE`). `character.Character.Partner` loads with the character. The registry `server/social/marriage.go` keeps online partners, as `social` keeps friends, and handles a partner in another zone through the world's player directory.
- **`player:marry(other)`** as `pc_marriage`: both online, neither married, neither a baby class. It fires `marry` and the achievement hook. **`player:divorce()`** as `pc_divorce`: deletes the wedding rings from both inventories (online in the zone; offline in the same transaction as the row delete), clears both rows, and tells both with `Divorced`. The partner's zone applies it between steps.
- **Reads.** `player.partner_id` (character ID, 0 for none, as `getpartnerid`), `player.partner` (`wa.Player?` while online, `ispartneron`), `player:warp_partner(map, x, y)` (`warppartner`; it needs the partner online and married). `wedding` is `player:wedding_effect()` and `npc:wedding_effect()` (an area effect).
- **Skills** in `server/skill/wedding.go` and `server/zone/skills_wedding.go`: WE_MALE, WE_FEMALE, WE_CALLPARTNER (a ground unit of 20 s; the partner is warped when the unit is placed; `nomemo` refuses it), WE_CALLALLFAMILY (partner only until v0.3.0), WE_ONEFOREVER (partner only, 30 percent HP, refused on GvG and battleground maps). The partner must be online and not in autotrade. rAthena's `disable_call` is a client setting wAthena does not have; the check waits for it. Heal doubles for a female Super Novice's partner (`heal.go:71`). Core's ring items already give the skills through converted `skill` calls.
- Wedding NPCs convert with the commands above. `npc/other/marriage.txt` needs `vip_status` for its VIP branch: `VIP_SCRIPT` is 0 in the extract, so that branch converts out.

### 4. Achievements

- **Records.** Kinds `mod.AchievementKind` and `mod.TitleKind`. `data/achievements.yml`: `name` (core: `ach_<id>`), `display_name` (rAthena's `Name`), `id` (fixed; core 110000 and up, mods from 1,000,000), `group` (the lower-case group), `targets` (`[{index, count, monster?}]`), `condition` (a function in `scripts/achievements/<name>.lua`), `dependents` (record names), `reward` (`item`, `amount`, `title`, `script`), `score`, `map` (unused: only the never-fired `Chatting` group uses it). `data/titles.yml`: `name`, `id` (fixed; core 1000 to 1046), `display_name`. Core's records convert at start from the new extract `achievements.json` per mode, as items convert from `item_scripts.json`; the condition and reward strings become Lua. Titles have no text in the extracts (the client's `TitleTable` is not extracted): core's title text is the granting achievement's name until a client data node supplies it.
- **State.** Per character, the engine keeps for each started achievement: ten counts, the completed time and the claimed time. Level and score are computed from the records at load, so a changed `score` applies at once (rAthena stores the score at add time). `Level` uses `achievement_level_db`: 20 rows of points; `achievement_level(sd)` is ported with its left and right numbers.
- **Triggers.** One call, `z.achieve(player, group, args...)` in `server/zone/achieve.go`, does what `achievement_update_objective` does: set `ARG0..`, test every record of the group (dependents first, then the condition or targets), add and complete, then clear. Call sites, each one line, exist or come with the system: kill (`mob` kill path, party share by `achievement_mob_share`), base and job level up, stat change, job change, item pickup, zeny gain, vending spend, friend add, party create, refine success and failure (`successrefitem`, `failedrefitem`, the refine window), marriage, chat room create and join (chat-rooms), taming (pets). The login check runs every group with no arguments. The groups `Eat` and `Chatting` have no trigger, as in rAthena.
- **Claim.** `AchievementClaim`: refused unless completed and unclaimed. It saves the time, mails the item from "GM" with the title "Achievement Reward Mail" (the mail layer's system sender), runs `reward.script` with the player attached, and adds the title. Titles at login are every completed record's title.
- **Title.** `TitleChange{title}` sets `characters.title` (0 clears) and tells everyone in sight (`EntityTitle`; `EntityAppear.title` carries it for players too).
- Items: `achievement_condition` is rAthena's wrapper; the converter emits a function of `(player, args)` where `ARGn` is `args[n + 1]`.

### 5. Navigation

- **Server.** `player:navigate_to(map, opts?)` with `x`, `y`, `monster`, `services` (`NAV_*` bits, default `NAV_KAFRA_AND_AIRSHIP`) and `hide_window` (default true). It sends `NaviRoute`. The route comes from `server/navi.Graph`, built at world load from the placed warp NPCs (map to map, each with the cell of the warp), and found by breadth first search over maps. Extra edges (`service: airship|kafra|scroll`) come from an optional `data/navi.yml`, used only when the flag allows the service. Core ships none (question 7): Kafra teleport lists and the airships are scripts, not data. A target with no route sends the destination alone: the client shows its name and cell and marks it on the minimap when it is the player's map.
- **Request.** A `<NAVI>` link in a dialog sends `NaviRequest{map, x, y, monster, services, hide_window}`; the server checks that the map exists, finds the route and answers. A route is recomputed when the player changes map, until the target map is reached or `NaviCancel` arrives. At most one request per second.
- **Client.** `dialog.ts` gets a token for `<NAVI>name<INFO>map,x,y,flag,monster,hide</INFO></NAVI>` that draws a link. The trailing three fields are read as the arguments of `navigateto` in that order; the builder checks them against the kRO format, which the fork does not have. `navi.ts` draws the route as map names and an arrow toward the next warp on the minimap, and a marker on the target.
- Procedural worlds have no classic map names, so `navigateto` returns false there. All call sites are classic content.

### 6. The stylist

- **Data.** `data/stylist.yml` per mode (renewal only has rows): `look` (`hair_color`, `hair`, `clothes_color`, `head_top`, `head_mid`, `head_bottom`, `body2`), and options `{index, value, human: {price, item?, box?}, doram: {...}}`. Core converts from `stylist.json` (new extract).
- **API.** `player:open_stylist()` (no permission: it only spends the player's own zeny and items; fails when the rule `stylist` is 0). The window shows the options the player's race prices (human, or Doram when Summoner exists).
- **Purchase.** `StylistBuy` names up to six indexes. They are charged in rAthena's order; any failure stops with `StylistResult{failed}` and keeps what was paid, as rAthena does (the order is hair color, hair style, cloth color, top, mid, bottom). The charge checks zeny, then the item or the box that holds it, deletes the item, then pays. Hair, hair color and cloth color apply with `player:set_look`'s internals; accessories mail the item from "Styling Shop". `Body2` needs a third class and is refused until then. One upstream quirk stays: with a box and no item the lookup uses item 0 and fails (stylist.yml has no such row).

### 7. The zeny bank

- **Vault.** A core currency `bank` (id 3, `owner: account`, max 2,147,483,647) in `server/currency/core/data/currencies.yml`, saved by the existing `account_currencies` table. No migration. `#BANKVAULT` reads and writes convert to it.
- **API.** `player:open_bank()` (no permission), and `BankDeposit` and `BankWithdraw` commands. Deposit and withdraw move zeny through `server/transfer` with the log actions `bank_deposit` and `bank_withdraw`, with rAthena's refusals as `BankResult` codes (success, no money, overflow, error). A withdraw that would pass `MAX_ZENY` refuses. A map with the flag `nobank` refuses all of it (the flag stops being refused).
- **State.** The window is open from `player:open_bank()` until `BankClose` or a map change. The rule `bank_state_enforce` is 1 here (rAthena's is 0 because kRO clients do not report the state); with 0 the server accepts requests with no window open (question 9).
- Two characters of one account share the vault and the currency layer's one account object, as for storage.

### 8. The refine window

- **Data.** `refine_db.json` gains, per refine level, `costs` (a list of `{type, rate, price, material, breaking_rate, downgrade}` for Normal, HD and Enriched), `blessing` (the Blacksmith Blessing amount), `broadcast_success` and `broadcast_failure`. The old `normal` stays for the loader. The extractor is `tools/wathena/extract.py`'s `build_refine`; the field list is in the fork's refine note. `server/refine` holds the tables (`refine.Table`, `Costs(item, refine)`), replacing the Normal-only reads in `server/status/equip.go` and `server/zone/refine.go`; `status` keeps the bonus parts.
- **API.** `player:open_refine_window()` (`refineui`, no permission) and the rule `refine_ui`. `items.refine_cost(item, refine, kind)` and `items.refine_chance(item, refine, {enriched})` read the new data, so decision 0064's error goes. `getequippercentrefinery` and `getequiprefinecost` convert with the kind argument (renewal: Normal, HD, Enriched; pre-renewal: Normal, Enriched, and HD refuses with "pre-renewal has no HD cost").
- **Window.** `RefineOpen`, `RefineSelect{slot}` (answered with `RefineMaterials`: each cost with its ore, rate in percent, zeny, and the blessing amount), `RefineStart{slot, material, blessing}` (answered with `RefineResult`), `RefineClose`. Checks and outcomes are as rAthena's: identified, unbroken, below `MAX_REFINE` (20 in renewal, 10 in pre-renewal), the ore in the inventory, the blessing in the amount the level lists, zeny; pay first; one draw of `rate` in 10,000; a broadcast through `world.announce`-class logic when the level says so. The refine fires the achievement hook for weapons and every failure. It uses the same transfer layer and log lines as the refiner of procedural towns, which calls the same code.
- Converted refine NPCs read `rules.get_limit("refine_ui")` and take the window branch.

### 9. The item enchant window

- **Data.** `data/item_enchants.yml`, keyed by `group` (the client Lua index, 1 to 138): `targets` (item names), `minimum_refine`, `minimum_grade`, `allow_random_options`, `reset {zeny, materials, chance}`, `order` (positions), `slots` with `normal {zeny, materials, chance, grade_bonus, enchants by grade [{item, chance}]}`, `perfect [{item, zeny, materials}]`, `upgrade [{from, to, zeny, materials}]`. Renewal only; `item_enchant.json` is the new extract. Mods may add groups.
- **Item copies.** `item.Extra.Enchants` holds three item IDs for positions 1 to 3 (0 for empty). They count in `Equal`, stacking and saves, apply their item's card script as cards do, and are cleared with the cards by reform. An enchant position must be at or past the item's card slots.
- **API.** `player:open_enchant_window(group)` (`item_enchant`; refused at 70 percent weight, as the client). `equipped` and `player:get_inventory()` report enchant IDs through `getequipcardid` positions 1 to 3.
- **Window.** `EnchantOpen{group, targets...}`, `EnchantRequest{mode, slot, item?}` (`random`, `perfect`, `upgrade`, `reset`), `EnchantResult`. Checks and draws follow `clif_parse_enchantwindow_*`: base checks, the first empty position of `order`, the grade's list (error message 829 when there is none), pay zeny and materials, one draw against the chance plus grade bonuses, then up to three times the list size random picks each tested against the pick's chance and a last forced pick.

### 10. The enchant grade window

- **Data.** `enchantgrade.json` (new extract, renewal): type (`armor`, `weapon`, shadow types not listed), item level, current grade, `chances` by refine, `bonus`, `catalyst {item, amount_per_step, maximum_steps, chance_increase}`, `options [{index, item, amount, zeny, breaking_rate, downgrade_amount}]`, `announce_success`, `announce_fail`. `data/enchant_grades.yml` carries it; mods may edit it.
- **Item copies.** `item.Slot.EnchantGrade` (0 to 4), saved in `inventory`, `storage_items`, `guild_storage` and `mail_items`; it joins stacking, `getinventorylist` (`@inventorylist_enchantgrade`, already named by the converter), `getenchantgrade`, `equipped.enchant_grade`, and `player:give_item`'s `enchant_grade` option.
- **Stats.** In renewal the grade adds `refine_bonus * grade_bonus / 100` to weapon ATK and the matching DEF bonus (`status.cpp` 3936 to 3965; the exact sites are listed in the decision record, with a `refine.json` golden by grade).
- **API and window.** `player:open_enchant_grade_window()` (`enchantgradeui`). `EnchantGradeOpen`, `EnchantGradeSelect{slot}` (answered with `EnchantGradeInfo`), `EnchantGradeStart{slot, option, catalyst_steps}`, `EnchantGradeResult`. Success adds one grade and resets refine to 0; failure breaks (when the option has a `breaking_rate` draw), downgrades or does nothing; announcements use `world.announce`-class text.

### 11. The item reform window

- **Data.** `data/item_reforms.yml`, keyed by the reform item: `bases [{item, min_refine, max_refine, cards_allowed, required_options, materials, result, change_refine, option_group, clear_slots, remove_grade}]`. Renewal only; `item_reform.json` is the new extract (131 items, 1,839 base rows). Random option groups already exist (`random_options`).
- **API.** `player:open_reform_window(item?)` (`item_reform`): without an argument it needs the item script's context and a delayed-use item, as rAthena; the reform item is consumed when the item opened the window.
- **Window.** `ReformOpen{item, bases...}`, `ReformStart{slot}`, `ReformResult`. Checks and effects as in the Problem list. The result item replaces the stack with the same serial transfer and log lines as rAthena's `LOG_TYPE_REFORM` (two lines: removal and gain).

### 12. Lua API

Reads need `game.read`. New mod permissions (approval needed, not flagged): `chat_rooms.manage` ("open, close and empty NPC chat rooms and warp their members"), `marriage.manage` ("marry and divorce players"), `achievements.manage` ("add, complete, change and remove players' achievements"). The window openers need no permission: they spend the player's own items. Staff right `no_chat_kick` is a staff role flag, as rAthena's `PC_PERM_NO_CHAT_KICK`.

| Call | Returns | rAthena | Permission |
|---|---|---|---|
| `npc:open_chat_room{...}`, `npc:close_chat_room()` | `wa.ChatRoom?`, boolean | `waitingroom`, `delwaitingroom` | `chat_rooms.manage` |
| `npc:chat_room()`, `player.chat_room`, `player.in_chat_room` | `wa.ChatRoom?`, boolean | `getwaitingroomstate`, `checkchatting` | `game.read` |
| `wa.ChatRoom` fields and methods | section 2 | the commands | `chat_rooms.manage` for methods |
| `player:marry(other)`, `player:divorce()` | boolean | `marriage`, `divorce` | `marriage.manage` |
| `player.partner_id`, `player.partner`, `player:warp_partner(map, x, y)`, `player:wedding_effect()`, `npc:wedding_effect()` | integer, `wa.Player?`, boolean | `getpartnerid`, `ispartneron`, `warppartner`, `wedding` | `game.read`, `player.warp` for the warp |
| `achievements.get(name)`, `achievements.id(name)` | `wa.AchievementInfo?`, integer | `achievement_db` | `game.read` |
| `player:achievement(a)`, `player.achievement_level`, `player.achievement_score` | `wa.Achievement?`, integer | `achievementexists`, `achievementinfo` | `game.read` |
| `player:add_achievement(a)`, `player:complete_achievement(a)`, `player:remove_achievement(a)` | `wa.Achievement?`, boolean, boolean | `achievementadd`, `achievementcomplete`, `achievementremove` | `achievements.manage` |
| `wa.Achievement:set_count(n, value)`, `:set_completed(epoch)`, `:set_rewarded(epoch)` | boolean | `achievementupdate` | `achievements.manage` |
| `player:navigate_to(map, opts?)` | boolean | `navigateto` | none |
| `player:open_stylist()`, `open_bank()`, `open_refine_window()`, `open_enchant_window(group)`, `open_enchant_grade_window()`, `open_reform_window(item?)` | boolean | `openstylist`, `openbank`, `refineui`, `item_enchant`, `enchantgradeui`, `item_reform` | none |

Fields of `wa.AchievementInfo`: `id`, `name`, `display_name`, `group`, `score`, `dependents`, `targets`. Fields of `wa.Achievement`: `info`, `counts` (ten values), `completed`, `rewarded`. Constants: `NAV_*`, `ACHIEVEINFO_*`.

Events (kinds `mod`, `npc`, `event`): `chat_room_join`, `chat_room_leave`, `marry`, `divorce`, `achievement_complete`, `achievement_claim`, `bank_change`, `refine_result`, `enchant_result`, `reform_result`. Hooks: none new. `wa.testing`: `world:new_player` options `partner`, `achievements`; `world:open_chat_room`.

### 13. Converter

| Source | Becomes | Mode |
|---|---|---|
| `waitingroom`, `delwaitingroom`, `kickwaitingroomall`, `waitingroomkick`, `enablewaitingroomevent`, `disablewaitingroomevent` (and `enablearena`, `disablearena`), `getwaitingroomstate`, `getwaitingroomusers`, `warpwaitingpc`, `checkchatting` | the room calls; `R.waitingroomstate(type, npc)`, `R.warpwaitingpc` | both |
| `marriage`, `divorce`, `getpartnerid`, `ispartneron`, `warppartner`, `wedding` | the marriage calls | both |
| `achievementinfo`, `achievementexists`, `achievementadd`, `achievementcomplete`, `achievementremove`, `achievementupdate` | the achievement calls; `ACHIEVEINFO_*` | both |
| `navigateto` | `player:navigate_to` | renewal |
| `refineui`, `openstylist`, `openbank`, `enchantgradeui`, `item_enchant`, `item_reform` | the openers | `refineui`, `openbank` both; the rest renewal |
| `getbattleflag("feature.refineui")`, `.achievement`, `.stylist`, `.banking` | `rules.get_limit("refine_ui")` and the like | both |
| `getequippercentrefinery`, `getequiprefinecost` with a type | `items.refine_chance`, `items.refine_cost` with the kind | both |
| `achievement_db.yml`, `achievement_level_db.yml` | `data/achievements.yml`, the level table, `data/titles.yml` | 361 and 359 |
| `stylist.yml`, `item_enchant.yml`, `enchantgrade.yml`, `item_reform.yml` | the data files above | renewal |

The converter's refusals with a reason go: "not supported" for these commands, the constant 0 of the three battle flags, and the reason strings in `npcmaps.go` for `nochat`, `nobank` and `no_chat`. Item-script commands `item_reform` and `item_enchant` leave the delayed-use blockers (130 and 7 rows). `F_Navi` converts as before; its markup now renders. `ARGn` in achievement conditions converts to the trigger's arguments. A reward script or condition that does not convert is refused with its reason in a new section of the NPC reports.

### 14. Protocol

Proposal 0001's rules: new fields and messages only, so the version stays 1. Event numbers start at 128 (event.proto has 114; 0003 takes 115 and 116, 0004 117 to 120, 0005 121 to 127) and move with merge order. Command numbers are the next free at build time (32 at the time of writing).

```proto
// event.proto, in Event.body
ChatRoomShow chat_room_show = 128;       // a room appears above its owner or NPC
ChatRoomClear chat_room_clear = 129;
ChatRoomJoined chat_room_joined = 130;   // the member's own view
ChatRoomMember chat_room_member = 131;
ChatRoomResult chat_room_result = 132;
Divorced divorced = 133;
AchievementList achievement_list = 134;
AchievementUpdate achievement_update = 135;
AchievementClaimResult achievement_claim_result = 136;
EntityTitle entity_title = 137;
NaviRoute navi_route = 138;
StylistOpen stylist_open = 139;
StylistResult stylist_result = 140;
BankOpen bank_open = 141;
BankResult bank_result = 142;
RefineOpen refine_open = 143;
RefineMaterials refine_materials = 144;
RefineResult refine_result = 145;
EnchantOpen enchant_open = 146;
EnchantResult enchant_result = 147;
EnchantGradeOpen enchant_grade_open = 148;
EnchantGradeInfo enchant_grade_info = 149;
EnchantGradeResult enchant_grade_result = 150;
ReformOpen reform_open = 151;
ReformResult reform_result = 152;

message ChatRoomShow {
  uint32 room = 1;           // per connection
  uint32 owner = 2;          // entity ID of the owner or the NPC
  string title = 3;
  uint32 users = 4;
  uint32 limit = 5;
  bool public = 6;
  bool npc = 7;
  uint32 min_level = 8; uint32 max_level = 9; uint32 zeny = 10;   // NPC rooms
}
message ChatRoomClear { uint32 room = 1; }
message ChatRoomJoined { uint32 room = 1; repeated string members = 2; string owner = 3; string title = 4; uint32 limit = 5; bool public = 6; }
message ChatRoomMember { uint32 room = 1; string name = 2; ChatRoomChange change = 3; }
enum ChatRoomChange { CHAT_ROOM_CHANGE_UNSPECIFIED = 0; CHAT_ROOM_CHANGE_JOIN = 1; CHAT_ROOM_CHANGE_LEAVE = 2;
  CHAT_ROOM_CHANGE_KICKED = 3; CHAT_ROOM_CHANGE_OWNER = 4; CHAT_ROOM_CHANGE_STATUS = 5; }
message ChatRoomResult { ChatRoomOutcome outcome = 1; }
enum ChatRoomOutcome { CHAT_ROOM_OUTCOME_UNSPECIFIED = 0; CHAT_ROOM_OUTCOME_OK = 1; CHAT_ROOM_OUTCOME_FULL = 2;
  CHAT_ROOM_OUTCOME_WRONG_PASSWORD = 3; CHAT_ROOM_OUTCOME_KICKED = 4; CHAT_ROOM_OUTCOME_NO_ZENY = 5;
  CHAT_ROOM_OUTCOME_LEVEL_LOW = 6; CHAT_ROOM_OUTCOME_LEVEL_HIGH = 7; CHAT_ROOM_OUTCOME_NOT_ALLOWED = 8; }

message Divorced { string partner = 1; }

// AchievementList is sent after login and after any record change that adds or removes one.
message AchievementList {
  uint32 level = 1; uint32 score = 2; uint32 exp = 3; uint32 next_exp = 4;
  repeated AchievementRow rows = 5;
  repeated uint32 titles = 6; uint32 title = 7;
  repeated AchievementTitle title_names = 8;
}
message AchievementRow {
  uint32 id = 1; string name = 2; string group = 3; repeated uint32 counts = 4;
  repeated AchievementTarget targets = 5; uint64 completed = 6; uint64 rewarded = 7;
  uint32 score = 8; bool has_reward = 9; uint32 reward_item = 10; uint32 reward_title = 11;
}
message AchievementTarget { uint32 index = 1; uint32 count = 2; uint32 monster = 3; }
message AchievementTitle { uint32 id = 1; string name = 2; }
message AchievementUpdate { uint32 level = 1; uint32 score = 2; uint32 exp = 3; uint32 next_exp = 4; AchievementRow row = 5; bool removed = 6; }
message AchievementClaimResult { uint32 id = 1; bool ok = 2; }
message EntityTitle { uint32 id = 1; string title = 2; }

message NaviRoute {
  string map = 1; uint32 x = 2; uint32 y = 3; uint32 monster = 4; uint32 services = 5; bool hide_window = 6;
  bool found = 7; repeated NaviStep steps = 8;   // each step: leave `map` at (x, y) toward the next
  bool cancel = 9;
}
message NaviStep { string map = 1; uint32 x = 2; uint32 y = 3; }

message StylistOpen { repeated StylistLook looks = 1; }
message StylistLook { StylistLookKind kind = 1; repeated StylistOption options = 2; }
message StylistOption { sint32 index = 1; uint32 value = 2; uint32 price = 3; uint32 item = 4; uint32 box = 5; }
enum StylistLookKind { STYLIST_LOOK_KIND_UNSPECIFIED = 0; STYLIST_LOOK_KIND_HAIR_COLOR = 1; STYLIST_LOOK_KIND_HAIR = 2;
  STYLIST_LOOK_KIND_CLOTHES_COLOR = 3; STYLIST_LOOK_KIND_HEAD_TOP = 4; STYLIST_LOOK_KIND_HEAD_MID = 5;
  STYLIST_LOOK_KIND_HEAD_BOTTOM = 6; STYLIST_LOOK_KIND_BODY2 = 7; }
message StylistResult { bool failed = 1; }

message BankOpen { uint64 vault = 1; uint64 zeny = 2; }
message BankResult { BankAction action = 1; BankOutcome outcome = 2; uint64 vault = 3; uint64 zeny = 4; }
enum BankAction { BANK_ACTION_UNSPECIFIED = 0; BANK_ACTION_DEPOSIT = 1; BANK_ACTION_WITHDRAW = 2; BANK_ACTION_CHECK = 3; }
enum BankOutcome { BANK_OUTCOME_UNSPECIFIED = 0; BANK_OUTCOME_OK = 1; BANK_OUTCOME_NO_MONEY = 2;
  BANK_OUTCOME_OVERFLOW = 3; BANK_OUTCOME_ERROR = 4; BANK_OUTCOME_NOT_ALLOWED = 5; }

message RefineOpen {}
message RefineMaterials { uint32 slot = 1; uint32 blessing = 2; repeated RefineCostRow costs = 3; }
message RefineCostRow { RefineKind kind = 1; uint32 material = 2; uint32 rate = 3; uint32 zeny = 4; bool can_break = 5; bool can_downgrade = 6; }
enum RefineKind { REFINE_KIND_UNSPECIFIED = 0; REFINE_KIND_NORMAL = 1; REFINE_KIND_HD = 2; REFINE_KIND_ENRICHED = 3; }
message RefineResult { uint32 slot = 1; RefineOutcome outcome = 2; uint32 refine = 3; }
enum RefineOutcome { REFINE_OUTCOME_UNSPECIFIED = 0; REFINE_OUTCOME_SUCCESS = 1; REFINE_OUTCOME_FAILED = 2;
  REFINE_OUTCOME_BROKEN = 3; REFINE_OUTCOME_DOWNGRADED = 4; REFINE_OUTCOME_REFUSED = 5; }

message EnchantOpen { uint32 group = 1; uint32 minimum_refine = 2; uint32 minimum_grade = 3; repeated EnchantSlotInfo slots = 4;
  EnchantCost reset = 5; repeated uint32 targets = 6; }
message EnchantSlotInfo { uint32 position = 1; EnchantCost normal = 2; repeated EnchantChoice perfect = 3; repeated EnchantUpgrade upgrade = 4;
  repeated EnchantGradeItems by_grade = 5; }
message EnchantCost { uint32 zeny = 1; repeated ItemAmount materials = 2; uint32 chance = 3; }
message EnchantChoice { uint32 item = 1; EnchantCost cost = 2; }
message EnchantUpgrade { uint32 from = 1; uint32 to = 2; EnchantCost cost = 3; }
message EnchantGradeItems { uint32 grade = 1; repeated uint32 items = 2; }
message ItemAmount { uint32 item = 1; uint32 amount = 2; }
message EnchantResult { bool success = 1; uint32 item = 2; }

message EnchantGradeOpen {}
message EnchantGradeInfo { uint32 slot = 1; uint32 chance = 2; uint32 catalyst = 3; uint32 catalyst_per_step = 4;
  uint32 catalyst_steps = 5; uint32 catalyst_bonus = 6; repeated EnchantGradeOption options = 7; }
message EnchantGradeOption { uint32 index = 1; uint32 item = 2; uint32 amount = 3; uint32 zeny = 4; bool can_break = 5; bool can_downgrade = 6; }
message EnchantGradeResult { uint32 slot = 1; EnchantGradeOutcome outcome = 2; uint32 grade = 3; }
enum EnchantGradeOutcome { ENCHANT_GRADE_OUTCOME_UNSPECIFIED = 0; ENCHANT_GRADE_OUTCOME_SUCCESS = 1;
  ENCHANT_GRADE_OUTCOME_FAILED = 2; ENCHANT_GRADE_OUTCOME_BROKEN = 3; ENCHANT_GRADE_OUTCOME_DOWNGRADED = 4; ENCHANT_GRADE_OUTCOME_REFUSED = 5; }

message ReformOpen { uint32 item = 1; repeated ReformBase bases = 2; }
message ReformBase { uint32 base = 1; uint32 min_refine = 2; uint32 max_refine = 3; bool cards_allowed = 4; uint32 required_options = 5;
  repeated ItemAmount materials = 6; uint32 result = 7; sint32 change_refine = 8; bool clear_slots = 9; bool remove_grade = 10; bool random_options = 11; }
message ReformResult { uint32 slot = 1; bool ok = 2; }

// command.proto, in Command.body
ChatRoomCommand chat_room = <next>;
AchievementCommand achievement = <next>;
NaviCommand navi = <next>;
StylistBuy stylist_buy = <next>;
BankCommand bank = <next>;
RefineCommand refine = <next>;
EnchantCommand enchant = <next>;
EnchantGradeCommand enchant_grade = <next>;
ReformCommand reform = <next>;

message ChatRoomCommand { ChatRoomAction action = 1; uint32 room = 2; string title = 3; string password = 4; uint32 limit = 5; bool public = 6; string name = 7; }
enum ChatRoomAction { CHAT_ROOM_ACTION_UNSPECIFIED = 0; CHAT_ROOM_ACTION_CREATE = 1; CHAT_ROOM_ACTION_JOIN = 2;
  CHAT_ROOM_ACTION_LEAVE = 3; CHAT_ROOM_ACTION_CHANGE = 4; CHAT_ROOM_ACTION_OWNER = 5; CHAT_ROOM_ACTION_KICK = 6; }
message AchievementCommand { oneof action { uint32 claim = 1; uint32 title = 2; } }   // title 0 clears
message NaviCommand { string map = 1; uint32 x = 2; uint32 y = 3; uint32 monster = 4; uint32 services = 5; bool hide_window = 6; bool cancel = 7; }
message StylistBuy { sint32 hair_color = 1; sint32 hair = 2; sint32 clothes_color = 3; sint32 head_top = 4; sint32 head_mid = 5; sint32 head_bottom = 6; bool close = 7; }
message BankCommand { BankAction action = 1; uint32 amount = 2; bool close = 3; }
message RefineCommand { oneof action { uint32 select = 1; RefineStart start = 2; bool close = 3; } }
message RefineStart { uint32 slot = 1; uint32 material = 2; bool blessing = 3; }
message EnchantCommand { uint32 group = 1; EnchantMode mode = 2; uint32 slot = 3; uint32 item = 4; bool close = 5; }
enum EnchantMode { ENCHANT_MODE_UNSPECIFIED = 0; ENCHANT_MODE_RANDOM = 1; ENCHANT_MODE_PERFECT = 2; ENCHANT_MODE_UPGRADE = 3; ENCHANT_MODE_RESET = 4; }
message EnchantGradeCommand { oneof action { uint32 select = 1; EnchantGradeStart start = 2; bool close = 3; } }
message EnchantGradeStart { uint32 slot = 1; uint32 option = 2; uint32 steps = 3; bool catalyst = 4; }
message ReformCommand { uint32 item = 1; uint32 slot = 2; bool close = 3; }
```

- `ChatChannel` gains `CHAT_CHANNEL_ROOM`. `InventoryItem` and `InventoryAdd` gain `enchant_grade` and `enchants`. `EntityAppear.title` is sent for players with a title.
- Clients older than this change skip the new events and send no new commands.

### 15. Database

Numbers are taken in merge order. Proposals 0004 and 0005 planned 00043 to 00047; this proposal names 00048 onward.

- **00048_character_partners.sql** (marriage): `character_partners (character_id bigint PRIMARY KEY REFERENCES characters ON DELETE CASCADE, partner_id bigint NOT NULL UNIQUE REFERENCES characters ON DELETE CASCADE, married_at timestamptz NOT NULL, CHECK (character_id <> partner_id), UNIQUE (character_id, partner_id), FOREIGN KEY (partner_id, character_id) REFERENCES character_partners (character_id, partner_id) DEFERRABLE INITIALLY DEFERRED ON DELETE CASCADE)`. Both rows go in one transaction, and a delete of one removes the other. Game role: SELECT, INSERT, DELETE.
- **00049_achievements.sql** (achievements): `character_achievements (character_id bigint FK CASCADE, achievement text CHECK (char_length BETWEEN 1 AND 64), counts integer[] CHECK (cardinality(counts) = 10), completed_at timestamptz, rewarded_at timestamptz, PRIMARY KEY (character_id, achievement))` and `characters.title text` (a title record name, or null). Achievements are stored by name so an ID change keeps rows. Game role: SELECT, INSERT, UPDATE, DELETE.
- **00050_item_enchants.sql** (item-windows): `ALTER TABLE inventory, storage_items, guild_storage, mail_items ADD COLUMN enchant_grade smallint NOT NULL DEFAULT 0 CHECK (enchant_grade BETWEEN 0 AND 4), ADD COLUMN enchants integer[] NOT NULL DEFAULT '{}' CHECK (cardinality(enchants) <= 3)`. `item_log` gains both values in its detail text. The down migration drops the columns.
- Chat rooms, navigation, the stylist, the refine, enchant and reform windows add no table. The bank uses `account_currencies`.
- Saves: the partner and title load with the character; achievement rows write in the character save transaction (`zeny` log lines of the bank go through the transfer layer). The marriage registry writes partner rows at once in their own transaction, because both characters must change together.

### 16. Client windows

In `client/src/viewer`, each with a unit test beside it. All text goes through the locale tables. Windows follow `windows.ts` (one open per kind, keyboard and touch).

- `chatroom.ts` (chat-rooms): the room list above heads (a bubble with the title, a lock for private, `users/limit`), the create dialog (title, limit, password, public), the member window (members, owner badge, change, hand over, kick, leave), the join prompt (password), and the room chat tab.
- `marriage.ts` (marriage): the divorce notice, the partner name line in the character menu, the `WE_CALLPARTNER` notice. No window.
- `achievements.ts`, `titles.ts` (achievements): the window (level and score bar, filter all, in progress, completed, a row with name, counts against targets, score, reward icon and Claim), the title picker, and an update toast.
- `navi.ts` and the `<NAVI>` token of `dialog.ts` (navigation).
- `stylist.ts`, `bank.ts`, `refine.ts` (ui-windows): the stylist (a list for each look with price and item, a preview of the character, Buy), the bank (balance, wallet, amount field, Deposit, Withdraw), the refine window (a slot picker, the cost list with ore, rate, zeny, the blessing check, Refine, and the result text).
- `enchant.ts`, `enchantGrade.ts`, `reform.ts` (item-windows): each with an item picker, the requirements with held amounts, and the result line.

### 17. Restart behavior

- Rooms are not saved. A restart empties player rooms and NPC rooms; converted NPCs make theirs again in `OnInit`, as rAthena does.
- A marriage survives; both rows are on disk. A partner who is offline at a divorce loses the ring in the same transaction.
- Achievement rows save with the character and with each completion (a completion is saved at the next character save, up to the autosave interval, as rAthena's `save` flag; a claim saves at once so a reward cannot repeat). Title and score come back at login.
- The vault is an account currency: saved with the account, written in the same commit as the zeny.
- Open windows (bank, stylist, refine, enchant, grade, reform) close at a restart and at a map change; nothing is half done, since each request is one transaction.
- Navigation is not saved.

### 18. Limits and ruleset rules

Rules (`server/rules/limits.go`, rAthena's shipped values; booleans 0 or 1): `chat_warpportal` 0 (Warp Portal in a room), `achievements` 1 (`feature.achievement`), `achievement_mob_share` 0, `refine_ui` 1 (`feature.refineui`), `stylist` 1 (`feature.stylist`), `banking` 1 (`feature.banking`), `bank_state_enforce` 1 (rAthena 0, section 7), `chat_room_max_users` 20 (rAthena's constant), `chat_room_title` 36. Existing rules read: `basic_skill_check`, `min_npc_vendchat_distance`. The rules apply in both modes with the same defaults. The ruleset flag `renewal` decides the data (which db files load) and the enchant grade bonus.

Limits: one room per player and per NPC; a room of at most 20; a title of at most 36 characters, a password of 8; one open window of each kind; navigation requests one per second; `NaviRequest` maps must exist.

### 19. Admin and observability

- `GET /players/{id}` (`players.read`) shows the partner, the achievement level and score, the title, and the vault.
- Staff permissions: `players.marriage` ("marry and divorce players"; `admin` and `game_master`), `players.achievements` ("add, complete and remove a player's achievements"; `admin` and `game_master`), `players.bank` ("change a player's vault"; `admin` only). Routes: `POST /players/{id}/marriage`, `DELETE /players/{id}/marriage`, `POST` and `DELETE /players/{id}/achievements/{name}`, `POST /players/{id}/bank` with `{ delta, reason }`. Audit actions `marriage.create`, `marriage.end`, `achievements.add`, `achievements.remove`, `achievements.complete`, `bank.change`.
- GM commands: `@marry`, `@divorce`, `@refineui`, `@enchantgradeui` (rAthena's four), under `commands.small_systems`.
- Metrics: gauges of rooms by kind; counters of room joins by outcome, room events fired, marriages and divorces, achievements completed by group, claims, bank zeny moved by direction, window opens by kind, refine, enchant, grade and reform results by outcome, stylist purchases. Info logs for divorces, claimed rewards, enchant and refine breaks of items worth selling (item and owner).

### 20. Procedural worlds

- Player rooms, the bank, the refine, enchant, grade and reform windows and achievements work in any world. Core's achievement records name core monsters, which procedural bands spawn, so Battle and Taming records can complete. Adventure chests, the wedding NPCs, the bank clerk, the stylist NPCs and the enchant NPCs are classic content. A procedural town gets none until a template adds them (strategy section 11.4 names refine and stylist NPCs as town templates); mods place them through the API.
- `navigateto` returns false in procedural worlds.

### 21. Performance

Rooms add one bubble entity per room in view and `ChatRoomMember` events to members only. Achievements add one record scan per trigger: indexed by group (and by monster for Battle and Taming), so a kill tests only records for that monster (`achievement_db.mobexists` is the same index). The login check runs every group once per login: 361 records. Item windows run only on request. No measurement node is needed; achievements adds a micro-benchmark of a kill trigger with 361 records.

### 22. Tests

Unit and golden tests:

- chat-rooms: join checks in rAthena's order (full, NPC counts itself, password, level, zeny, kick list), owner transfer, empty deletion, trigger on join, `enable` runs the check, `disable` blocks, `warp_members` order with fees, the SavePoint and `noteleport` stop, `getwaitingroomstate` types including `full` versus the join test, the `nochat` flag and cell, items and skills refused in a room, converter goldens, e2e of an arena room.
- marriage: marry and divorce rules, offline divorce with ring removal (PostgreSQL), the tied rows and a character delete (PostgreSQL), WE_MALE, WE_FEMALE (HP and SP amounts), WE_CALLPARTNER, WE_CALLALLFAMILY, WE_ONEFOREVER against recordings, the Super Novice heal doubling, converter goldens of `marriage.txt` and `divorce.txt`.
- navigation: graph and route on a sample classic world, route recompute on a map change, `NaviRequest` limits, the `<NAVI>` token (client), converter goldens of `F_Navi` and `navigateto`.
- achievements: record loading in both modes (361, 359), the level table against the recording, every group's trigger against `achievement_life`, dependents, condition conversion for each of the 93 conditions, reward scripts, claim, mail, titles, login check, saves (PostgreSQL), converter goldens, e2e of kill, level, claim and title.
- ui-windows: stylist options and charge order against `stylist_buy`, bank cases against `bank_life`, refine outcomes (success, break, downgrade, blessing, no change) against `refine_ui` with forced draws in both modes, the new `refine_db.json` fields against rAthena's yml, `items.refine_cost` of every kind, converter goldens, e2e of refine with an Enriched ore and a deposit.
- item-windows: enchant modes against `enchant_ui`, grade outcomes and the stat bonus against `enchantgrade_ui` and `refine.json` by grade, reform results against `item_reform` including random option groups with forced draws, item copy saves with `enchant_grade` and `enchants` (PostgreSQL), converter goldens of the 130 reform items and the enchant NPCs.
- client: a unit test for each new viewer file; browser checks of the room, achievement, stylist, bank, refine and item windows in the existing viewer run.

Golden against rAthena. The fork's recorder (`src/map/wathena_recorder.cpp`) gains, per mode where it applies:

- `chat_room`: scripted joins and leaves of sample players in an NPC room and a player room (join order, results, owner changes, kick list), triggers, and `warpwaitingpc` with fees (needs hooks that record `npc_event_do` and `pc_setpos` instead of running them).
- `achievement_life`: each group's `achievement_update_objective` calls with sample players, the resulting counts, completion, dependents and level, and the level table for scores 0 to the maximum (needs `achievement_data` set directly, as the char server would).
- `marriage_life`: `pc_marriage`, `pc_divorce`, and the five wedding skills' effects on sample pairs.
- `refine_ui`: `clif_parse_refineui_refine`'s outcome for each type, level and cost type with `rnd_force_min` and `rnd_force_max`, and the blessing case. `refine.json` gains `by_grade` (renewal).
- `enchant_ui`, `enchantgrade_ui`, `item_reform`: the parse functions' effects on sample items with forced draws (needs a hook that calls the bodies without a socket session).
- `stylist_buy`, `bank_life`: charges and reasons.
- Extracts: `achievements.json`, `stylist.json`, `item_reform.json`, `item_enchant.json`, `enchantgrade.json` from `extract.py`, and `refine_db.json` extended as in section 8; `map_flags.json` gains `nochat` and `nobank` if it lacks them.

e2e (`e2e/`, a classic world with kRO assets, PostgreSQL, headless bots), acceptance of each node:

1. chat-rooms: two bots create, join and leave a player room; a bot joins an arena waiting room until the trigger and is warped by `warpwaitingpc`.
2. marriage: two bots marry through the converted wedding NPC (the e2e sets the items), cast WE_CALLPARTNER, restart, divorce.
3. navigation: a bot clicks a guide's link and receives a route that updates after a warp.
4. achievements: a bot kills a target monster, levels up, claims a reward with a title and an item mail, restarts.
5. ui-windows: a bot refines with the window using Enriched ore, buys a style, deposits and withdraws.
6. item-windows: a bot enchants, raises a grade and reforms an item, restarts.

### 23. Who builds what

- **chat-rooms**: section 2, the room rows of 12 to 14, 16 to 19, `server/chat/**` for the room channel. Brief changes: owned `server/chatroom/**`, `client/src/viewer/chatroom*`; shared adds `server/chat/**`, `server/rules/**`, `tools/convert/npcmaps.go` and `tools/convert/npc*.go`.
- **marriage** (split from marriage-and-navigation): section 3, the marriage rows. Owned `server/social/marriage*.go`, `server/skill/wedding*.go`, `server/zone/skills_wedding.go`, `client/src/viewer/marriage*`; shared adds `server/character/**`, `server/skill/**`, `server/staff/**`, `server/admin/**`.
- **navigation** (new): section 5. Owned `server/navi/**`, `client/src/viewer/navi*`; shared adds `client/src/viewer/dialog.ts`.
- **achievements**: section 4, the achievement rows. Owned `server/achievement/**`, `server/zone/achieve.go`, `client/src/viewer/achievements*`, `client/src/viewer/titles*`; shared adds `server/mod/**`, `server/gamedata/**`, `server/mail/**`, `server/character/**`, `reference/rathena/extracts/**`. Builds `z.achieve` and the call sites that exist. Later nodes add their own site (the contract lists them).
- **ui-windows** (scope cut): refine (section 8), stylist (6), bank (7). Owned `server/refine/**`, `server/stylist/**`, `server/bank/**`, `client/src/viewer/refine*`, `stylist*`, `bank*`; shared adds `server/status/**`, `server/currency/**`, `server/transfer/**`, `reference/rathena/extracts/**`. Depends on extract-sync for the extended refine extract. Kafra points leave.
- **item-windows** (new): sections 9, 10, 11 and migration 00050. Owned `server/itemwindow/**`, `client/src/viewer/enchant*`, `reform*`; shared adds `server/item/**`, `server/status/**`, `server/character/**`, `server/store/*.go`, `tools/convert/use.go`. Order inside the node: the enchant grade first (it adds the item fields), then the enchant window, then reform.
- Suggested order: navigation, marriage, chat-rooms, achievements, ui-windows, item-windows. The contract's stub (`z.achieve` as a no-op in `server/zone/achieve.go`) lets nodes run in any order; whichever node runs first adds the stub and the achievements node replaces its body.
- Milestones: all stay v0.2.0. item-windows is renewal only; it is the first to move to `should` if the release is late (question 11).

## Breaks

- `waitingroom` and 40 other commands stop being refused. NPCs that were not placed because of them place, and scripts that guard on `feature.refineui`, `feature.achievement` or `feature.stylist` take the window branch instead of the no-window one. Refine NPCs in both modes open the refine window.
- `nochat`, `nobank` and the cell `no_chat` become real; maps that set them change.
- Players can marry, join rooms and bank. No existing world has any.
- Item copies gain `enchant_grade` and `enchants` (migration 00050); `item.Extra.Equal` includes them, so copies that differ stop stacking. Existing rows read as 0 and empty.
- `items.refine_cost` and `items.refine_chance` stop raising an error for HD and Enriched.
- `refine_db.json` changes shape (new fields, `normal` kept); tests that read it update.
- Converted guides' `<NAVI>` markup renders as links instead of raw text.
- Achievement completion saves at the next character save, not at once.
- Mods that want these APIs request `chat_rooms.manage`, `marriage.manage` or `achievements.manage`.

## Alternatives

- **Rooms as zone entities with their own kind.** A new `EntityKind` for rooms would reuse view events. But a room is a label on an owner, not a unit: it has no cell of its own to walk, no HP, and the owner moves. `ChatRoomShow` keyed to the owner entity is smaller. Not chosen.
- **Rooms saved or shared between zones.** Joining needs the same map, so cross-zone state buys nothing. Not chosen.
- **Marriage as `partner_id` columns on `characters`.** Two columns, no new table, as rAthena. But nothing keeps the two sides equal, and a partial update leaves a half marriage. The tied rows make that impossible. Not chosen.
- **Marriage in the achievements-style generic table.** No.
- **Achievement records as Lua only.** Fits conditions, but 361 records of data (names, scores, targets, dependents) belong in data files that mods can override. Chosen: data plus converted functions, as items.
- **Per-character achievement score stored at add time** (rAthena). Faster to read, but a score change would never apply. Not chosen.
- **A full kRO-style navigation graph with Kafra, airship and scroll edges.** Needs the client's service tables, which the repo lacks, or a static analysis of Kafra scripts. Not chosen for v0.2.0.
- **Navigation computed in the client.** The client would need the whole warp graph; the server already has it and a hostile client gains nothing. Not chosen.
- **Bank as a table.** Typed rows and a migration. The currency layer already has account-wide, capped, logged amounts that move through the transfer layer. Not chosen.
- **A bank state that is not enforced** (rAthena's default). Only exists because kRO clients do not report the state. Ours does. Not chosen as the default.
- **One `ui-windows` node for six windows.** One node, but it adds the item fields, three renewal-only data sets and five windows to an L brief, and a stuck window blocks the refine NPCs that most scripts need. Not chosen.
- **Keep `marriage-and-navigation` as one node.** The brief's own sizing says one system per node; navigation unblocks 73 scripts and touches only the dialog and a graph, while marriage touches saves, items and skills. Not chosen.
- **Build adoption now.** Needs baby classes (tier 3) and unreachable jobs. Not chosen.
- **Fake the enchants as cards.** Cards list only filled slots, so an enchant at position 3 of an item with no card slots cannot be told from a card. Not chosen; `Extra.Enchants` keeps positions.

## Decisions in review

The maintainer accepted every recommendation on 2026-10-27.

1. Adoption, the baby skills and the family columns move to v0.3.0 with the baby classes (`tier3-systems`). Nothing in the scripts needs them and no baby job is reachable.
2. Kafra points in cash shop prices (G8) move to `tier3-systems` with the cash shop.
3. Split `marriage-and-navigation` into `marriage` and `navigation` (section 23).
4. Split `ui-windows` into `ui-windows` (refine, stylist, bank) and `item-windows` (enchant, enchant grade, reform).
5. Achievements are data (`data/achievements.yml`, `data/titles.yml`) plus converted Lua functions, with score computed from the records at load.
6. Title text comes from the granting achievement's name until a client data node extracts `TitleTable`.
7. Navigation routes over warp NPCs only in v0.2.0; airship, Kafra and scroll edges come from an optional `data/navi.yml` that core does not fill.
8. Marriage is two tied rows in `character_partners` (00048), not columns on `characters`.
9. The bank is a core account currency `bank` with no migration, and `bank_state_enforce` defaults to 1 (rAthena's 0).
10. A member of a player room who walks leaves it (rAthena lets a modified client walk).
11. Item windows are `must` for v0.2.0 as the brief says, and the first to become `should` if the release is late. They stay `must`; the 130 reform items and 4 enchant NPCs are renewal rows.
12. The recorder files and extracts of section 22 are added by the node that needs them, in the fork's local branch, until the open question on pushing it is settled.
13. The converter's `getbattleflag("feature.refineui")` and its two siblings become rule reads once the window exists, so every refine NPC takes the window branch in both modes. rAthena's default is on.
14. The three new mod permissions (`chat_rooms.manage`, `marriage.manage`, `achievements.manage`) need approval and are not flagged; the staff permissions are `players.marriage`, `players.achievements` (`admin`, `game_master`) and `players.bank` (`admin`).
