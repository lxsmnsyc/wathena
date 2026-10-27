# Log: proposal-small-systems

## 2026-10-27

- Read GOAL.md, the roadmap CLAUDE.md, the brief, the empty contract, proposals 0004 and 0005 (shape and depth), the companions contract and outcome, the four downstream briefs (chat-rooms, marriage-and-navigation, achievements, ui-windows), 0002b category C, review decision 60 and decision 0064.
- Counts checked against the reports. First-blocker rows, renewal: navigateto 73, waitingroom 33, getpartnerid 22, achievementinfo 14, refineui 9, openstylist 7, item_reform 130 delayed items, item_enchant 4 NPC rows and 7 item rows, enchantgradeui 2, enablewaitingroomevent 2, achievementexists 1, openbank 1. Pre-renewal: waitingroom 24, getpartnerid 22, refineui 7, enablewaitingroomevent 2. The brief's numbers match. Source call sites in the fork's npc folder (comments left out) are larger: navigateto 162, F_Navi 345, marriage 52, waitingroom 58, warpwaitingpc 27, achievementinfo 14, achievementupdate 14, item_enchant 15, item_reform 12.
- Grep with `-I` skips files with non-UTF-8 bytes (quests_16_2.txt holds the achievementinfo calls). Use `-a` when counting scripts.
- Nothing exists in wathena for any of the eleven systems: no chat room, partner link, achievement, navigation, stylist, bank or window. Existing hooks: `items.refine_cost` and `items.refine_chance` take a kind but the extract has Normal only (server/zone/itemactions.go:255); the map flags `nochat`, `nobank` and the cell `no_chat` are refused with "does not exist" reasons (server/zone/maps.go:812, 1032, 1034; tools/convert/npcmaps.go); `feature.refineui`, `feature.achievement`, `feature.stylist` convert to constant 0 (tools/convert/npcgroup3.go:58); heal.go:71 says partners are not ported; dialog.ts has no `<NAVI>` token, so converted guides send raw markup.
- rAthena findings that shape the design:
  - Chat rooms are zone-local: joining needs the same map. An NPC room holds users+1 against the limit. `warpwaitingpc` writes `$@warpwaitingpc[]` with account IDs and `$@warpwaitingpcnum`.
  - Marriage is two symmetric `partner_id` values. WE_MALE, WE_FEMALE, WE_CALLPARTNER, WE_CALLALLFAMILY and WE_ONEFOREVER come from the two wedding rings' item scripts. The baby skills and `pc_adoption` need baby classes, which are strategy tier 3, and no path reaches a baby job in wathena.
  - Achievements: 361 renewal and 359 pre-renewal records, 20 levels. AG_EAT and AG_CHATTING are never triggered by rAthena. Item rewards are mailed. Titles come from completed achievements at login, claimed or not.
  - Navigation is client side in rAthena; the server sends a target. F_Navi builds a `<NAVI>` link; PACKETVER 20211103 makes it active.
  - Stylist, enchant grade, item enchant and item reform data are renewal only (db files import db/re). Refine and bank work in both modes. refine.yml has Normal, HD and Enriched in renewal (160, 140, 90 rows) and Normal and Enriched in pre-renewal (70 each).
  - The bank vault is the account variable #BANKVAULT: wAthena has account currencies (server/currency), so no migration.
  - rAthena's refine UI, bank, stylist and item windows read their state from the client packets; wAthena's client is ours, so enforcement can be tighter.
- Decided: split marriage-and-navigation (the brief says one system per node) and ui-windows (refine, stylist, bank vs the three renewal-only item windows); adoption and baby skills, Kafra points in the cash shop move to v0.3.0; bank is a core account currency.
- Wrote docs/proposals/0006-small-systems.md and filled contracts/small-systems.md.
