---
name: small-systems
version: 1
status: draft
defined_by: proposal-small-systems
---

# Contract: small-systems

## Purpose

The interface between the small-systems design and the nodes that build on it: chat-rooms, marriage (split from marriage-and-navigation), navigation, achievements, ui-windows and item-windows. `proposal-small-systems` freezes it when design proposal 0006 is accepted.

Source: [docs/proposals/0006-small-systems.md](../../../proposals/0006-small-systems.md). Section numbers below are the proposal's. Draft: the maintainer's answers to its "Questions for review" may change it.

## Interface

Lua (section 12), new permissions `chat_rooms.manage`, `marriage.manage`, `achievements.manage` (approval, not flagged); reads need `game.read`:

- Chat rooms: `npc:open_chat_room{title, limit, event?, trigger?, zeny?, min_level?, max_level?}`, `npc:close_chat_room()`, `npc:chat_room()`, `player.chat_room`, `player.in_chat_room`; `wa.ChatRoom` fields `title`, `limit`, `trigger`, `trigger_disabled`, `event`, `users`, `password`, `public`, `full`, `triggered`, `owner`, `members()`; methods `enable_event`, `disable_event`, `kick`, `kick_all`, `warp_members(map, x, y, n?)`.
- Marriage: `player:marry(other)`, `player:divorce()`, `player.partner_id`, `player.partner`, `player:warp_partner(map, x, y)`, `player:wedding_effect()`, `npc:wedding_effect()`.
- Achievements: `achievements.get`, `achievements.id`, `player:achievement(a)`, `player.achievement_level`, `player.achievement_score`, `player:add_achievement`, `player:complete_achievement`, `player:remove_achievement`; `wa.Achievement` (`info`, `counts`, `completed`, `rewarded`, `set_count`, `set_completed`, `set_rewarded`); `wa.AchievementInfo`; constants `ACHIEVEINFO_*`.
- Navigation: `player:navigate_to(map, {x, y, monster, services, hide_window})`; constants `NAV_*`.
- Openers (no permission): `player:open_stylist()`, `open_bank()`, `open_refine_window()`, `open_enchant_window(group)`, `open_enchant_grade_window()`, `open_reform_window(item?)`.
- Events: `chat_room_join`, `chat_room_leave`, `marry`, `divorce`, `achievement_complete`, `achievement_claim`, `bank_change`, `refine_result`, `enchant_result`, `reform_result`. `items.refine_cost` and `items.refine_chance` accept `hd` and `enriched`.
- `wa.testing`: `new_player` options `partner`, `achievements`; `world:open_chat_room`.

Zone: `z.achieve(player, group, args...)` in `server/zone/achieve.go`, the one call every trigger site makes (kill, level, stat, job change, item, zeny, vending, friend, party, refine, marriage, chat room, taming). Groups are the lower-case names of rAthena's `e_achievement_group`. `server/chatroom` (pure room state), `server/navi.Graph`, `server/refine` (tables of every cost type), `item.Slot.EnchantGrade` and `item.Extra.Enchants`.

Data (sections 4, 6, 8 to 11): kinds `mod.AchievementKind`, `mod.TitleKind`; files `data/achievements.yml` (fixed `id`; core from 110000, mods from 1,000,000), `data/titles.yml` (core 1000 to 1046), `data/stylist.yml`, `data/item_enchants.yml`, `data/enchant_grades.yml`, `data/item_reforms.yml`, optional `data/navi.yml`. Core converts them from new extracts `achievements.json`, `stylist.json`, `item_enchant.json`, `enchantgrade.json`, `item_reform.json`, and from `refine_db.json` extended with `costs`, `blessing`, `broadcast_success`, `broadcast_failure` (`normal` stays). Core currency `bank` (id 3, account, max 2,147,483,647) in `server/currency/core/data/currencies.yml`.

Rules (section 18), in `server/rules/limits.go`: `chat_warpportal`, `achievements`, `achievement_mob_share`, `refine_ui`, `stylist`, `banking`, `bank_state_enforce`, `chat_room_max_users`, `chat_room_title`. Map flags `nochat`, `nobank` and the cell `no_chat` stop being refused.

Database (section 15): 00048 `character_partners` (marriage); 00049 `character_achievements` and `characters.title` (achievements); 00050 `enchant_grade` and `enchants` on `inventory`, `storage_items`, `guild_storage`, `mail_items` (item-windows). Numbers are taken in merge order after 00047 (proposal 0005). Game role: SELECT, INSERT, UPDATE, DELETE (`character_partners`: no UPDATE).

Converter (section 13): `waitingroom`, `delwaitingroom`, `kickwaitingroomall`, `waitingroomkick`, `enablewaitingroomevent`, `disablewaitingroomevent`, `getwaitingroomstate`, `getwaitingroomusers`, `warpwaitingpc`, `checkchatting`, `marriage`, `divorce`, `getpartnerid`, `ispartneron`, `warppartner`, `wedding`, the six `achievement*` commands, `navigateto`, `refineui`, `openstylist`, `openbank`, `enchantgradeui`, `item_enchant`, `item_reform`; helpers `R.waitingroomstate`, `R.warpwaitingpc`; the three `feature.*` battle flags as rule reads; `getequippercentrefinery` and `getequiprefinecost` with a type.

Protocol (section 14): events from 128 (`ChatRoomShow` to `ReformResult`, 25 events), commands `ChatRoomCommand`, `AchievementCommand`, `NaviCommand`, `StylistBuy`, `BankCommand`, `RefineCommand`, `EnchantCommand`, `EnchantGradeCommand`, `ReformCommand`; `CHAT_CHANNEL_ROOM`; `InventoryItem` and `InventoryAdd` fields `enchant_grade` and `enchants`. Field numbers are the next free at build time.

Client (section 16), in `client/src/viewer`: `chatroom.ts`, `marriage.ts`, `achievements.ts`, `titles.ts`, `navi.ts` and the `<NAVI>` token of `dialog.ts`, `stylist.ts`, `bank.ts`, `refine.ts`, `enchant.ts`, `enchantGrade.ts`, `reform.ts`.

Admin (section 19): staff permissions `players.marriage`, `players.achievements`, `players.bank`; routes under `/players/{id}/`; audit actions `marriage.create`, `marriage.end`, `achievements.add`, `achievements.remove`, `achievements.complete`, `bank.change`; GM commands `@marry`, `@divorce`, `@refineui`, `@enchantgradeui`.

## Behavior

- Rooms are zone-local and not saved. An NPC room takes `limit - 1` players; `full` is `users >= limit`. Join checks run in rAthena's order. The trigger event goes to the NPC with no player. `warp_members` pays the fee per user in join order and stops at the first failure. A member who walks, warps, dies or logs out leaves.
- A marriage is two tied rows; divorce removes the wedding rings of both, online or offline, and tells both. The wedding skills come from the ring items. Adoption and the baby skills are not in v0.2.0.
- Achievements use rAthena's groups and triggers; `Eat` and `Chatting` never fire. Level and score come from the records at load. A claim mails the item from "GM", runs the reward script and adds the title; titles of completed records are on at login.
- `navigate_to` sends a route over warp NPCs; a missing route sends the target alone. Procedural worlds return false.
- The vault is the account currency `bank`; deposits and withdrawals go through the transfer layer. With `bank_state_enforce` on, the window must be open.
- A refine request pays first, then draws once; failure with a blessing changes nothing; otherwise break, downgrade or nothing per the cost row. Pre-renewal has no HD row.
- Item copies keep `enchant_grade` and three enchant positions; stacking and the item log include them.
- Rules take rAthena's shipped values in both modes, except `bank_state_enforce` (1).

## Stub

`server/zone/achieve.go` holds `z.achieve` as a no-op. The first node that needs it adds the file; achievements replaces the body and adds the call sites that exist. A node that adds a trigger site calls `z.achieve` with the group's arguments.

## Contract tests

Named in proposal 0006 section 22:

- chat-rooms: join check order, NPC count, trigger and enable and disable, `warp_members` fees and stops, `getwaitingroomstate` types, `nochat`, items and skills refused, e2e arena room.
- marriage: marry and divorce rules, offline divorce (PostgreSQL), tied rows, the five wedding skills against recordings, heal doubling.
- navigation: graph route, recompute on map change, request limit, the `<NAVI>` token.
- achievements: record loading (361, 359), level table, every group against `achievement_life`, dependents, the 93 conversions, claim and mail, titles, saves (PostgreSQL).
- ui-windows: stylist order, bank cases, refine outcomes in both modes, the extended refine extract, every cost kind.
- item-windows: enchant modes, grade outcomes and stat bonus, reform results, item copy saves (PostgreSQL).
- Recorder files the fork adds: `chat_room`, `achievement_life`, `marriage_life`, `refine_ui`, `enchant_ui`, `enchantgrade_ui`, `item_reform`, `stylist_buy`, `bank_life`, `refine.json` `by_grade`; extracts `achievements.json`, `stylist.json`, `item_reform.json`, `item_enchant.json`, `enchantgrade.json`, extended `refine_db.json`.
- Micro-benchmark (achievements): a kill trigger with 361 records.

## Changes

- v1: initial draft from proposal 0006.
