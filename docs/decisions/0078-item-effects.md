# 0078: Item effects: statuses in equipment, autobonus actions, transforms and bonus scripts

- Status: built.
- Date: 2026-11-25.
- Code: [tools/convert](../../tools/convert) (`use.go`, `usestatus.go`, `equipitem.go`, `expr.go`, `item.go`, `output.go`), [server/statuschange](../../server/statuschange) (`end.go`, `ids.go`), [server/item](../../server/item) (`heal.go`), [server/modscript](../../server/modscript) (`playerapi.go`, `prelude.lua`, `luaapi/spec.go`), [server/zone](../../server/zone) (`playeractions.go`, `inventory.go`, `equipment.go`, `zone.go`).
- Docs: [tools/convert/README.md](../../tools/convert/README.md), [lua-api.md](../generated/lua-api.md), [item-scripts-renewal.md](../generated/item-scripts-renewal.md), [item-scripts-pre-renewal.md](../generated/item-scripts-pre-renewal.md).
- Builds the node item-effects of docs/roadmaps/v0.2-parity, and review decision 60's E4. Review decision 67 of [proposal 0002](../proposals/0002-lua-api.md) records the API additions.
- Lua API: adds to API 1.0. Protocol: none. Settings: none. Data: none. Store: none.

## Context

The item converter refused the scripts that use `heal`, `transform`, `active_transform`, `showscript`, `skilleffect`, `setfont`, `getexp2`, `getitempos`, and `sc_start` or `sc_end` in equipment and in autobonus scripts. Decision 0064 left `active_transform` waiting for options, and review decision 60 asked that `percentheal -100` kill.

## Decision

### Use scripts and unequip scripts

| Command | Lua |
|---|---|
| `heal hp,sp` | `player:heal(hp, sp)`, which is `status_heal` with flag 1 and may kill with a negative amount |
| `getexp base,job`, `getexp2 base,job` | `player:give_exp(base, job)`, and with `{ raw = true }` for `getexp2` |
| `skilleffect skill,lv` | `player:show_skill_effect(skill, lv)`. A third argument other than the player's own ID does not convert |
| `showscript "text"{,id{,target}}` | `player:say_aloud(text)`, or with `{ to = "self" }` for `SELF`. Another unit or another target does not convert |
| `setfont n` | `player:set_font(n)` |
| `transform mob,ms{,sc,v1..v4}` | `player:transform(mob, ms, { status = sc, values = { v1, ... } })`. `SC_NONE` leaves the option out |
| `active_transform mob,ms` | `player:transform(mob, ms, { active = true })` |

- `showscript` is a bubble over the head in rAthena, with no name. `say_aloud` shows the speaker's name before the text, so the text reads a little different. A packet of its own would need a protocol change, which this node does not make.
- `transform` with a status starts both for the same time. The transform keeps the status in val2, and its end ends the status, as `status_change_end` does (status.cpp 13814). `SC_ACTIVE_MONSTER_TRANSFORM` ends it the same way. rAthena starts the active status for the icon only, so the zone starts nothing else.
- `player:set_font` toggles as `setfont` does: the font, or 0 when the player already had it. The zone keeps it on the player and shows nothing, as the client has no fonts.

### Equipment

- `sc_start`, `sc_start2`, `sc_start4` and `sc_end` in an equip script convert as they do in a use script and run when the item is equipped. The unequip script ends the status, as it does in rAthena. A status under a condition does not convert (`conditional sc_start`).
- The actions of an autobonus's bonus script (`sc_start*`, `sc_end`, `heal`, `specialeffect2`, `showscript`) run once when the autobonus triggers, before its effect script. rAthena runs the bonus script at each recalculation while the autobonus lasts, so its `sc_start` would start again each time; once is what the script means.
- `getitempos()` reads `equipped.position`, the EQP_ bit of the slot the copy sits in (`s.Equip`), through the new field of `wa.EquippedItem`. In a card's script it is the host item's slot, as `current_equip_item_index`.
- A `bonus_script` that uses `rand` rolls once when the item is used, and the timed bonus keeps the value. rAthena rolls again at each recalculation. A computed amount other than a roll still does not convert.
- The script of an item in the group `THROWABLE` does not run when it is equipped in rAthena (`status_calc_pc_`, status.cpp 4047): the throw skills run it on their target. wAthena has none, so these items do not convert, with the reason `throwable ammo`. Before this, their `sc_start` ran on equip in wAthena, which rAthena does not do.

### Heal and kill

- `player:heal_percent` with -100 or less kills, as `percentheal` does (`status_percent_change` with its kill flag). A rate above -100 still stops at 1 HP. This replaces review decision 56's rule for -100. The Poison Bottle and the 8 NPC files that review decision 60's E4 names (the brief says 10) now kill. `Zone.percentLoss` takes the HP and SP through `zap`, so a kill runs the death flow.

## Tests

- `tools/convert`: `TestConvertUse` and `TestConvertEquipEffects` (each command, the autobonus actions, `getitempos`, a rolled bonus script), `TestGolden` with 10 more items (`Trans_Scroll_Gazeti`, `Cool_Towel`, `Giant_Lance`, `Dea_Staff`, `Ghoul_Leg_IL`, `VIT_Biscuit_Stick`, `Red_Envelope`, `Two_Dragon_Ring`, `Rudo_Card`, `Mysterious_Powder`), `TestRunConvertedScripts` (every converted script runs), `TestEquipmentParity` (10,046 of 10,046 renewal equipment records and 1,539 of 1,539 cards match, up from 9,996 and 1,503; its test host now gives the real position of each copy).
- `server/zone`: `TestTransformStatusAndActive`, `TestEquippedPosition`, and `TestHealPercentTakes` with the kill at -100.
- `server/modscript/luaapi`: `TestGeneratedFiles` (the Lua reference).

## Result

| Report | Row | Before | After |
|---|---|---|---|
| renewal, equipment | `heal` | 0 full, 57 partly, 3 not | 58 full, 2 partly |
| renewal, equipment | `sc_start` | 21 full, 21 not | 29 full (13 scripts moved to `throwable ammo`) |
| renewal, equipment | `sc_end` | 65 full, 1 not | 65 full |
| renewal, equipment | `active_transform` | 0 full, 25 partly, 8 not | 34 full |
| renewal, equipment | `showscript` | 0 full, 9 partly, 3 not | 12 full |
| renewal, equipment | `getitempos` | 11 not | row gone, all 11 convert |
| renewal, equipment | `autobonus`, `autobonus2`, `autobonus3` | 320, 24, 102 full | 362, 43, 109 full |
| renewal, equipment | `throwable ammo` | none | 17 not |
| renewal, usable | `transform` | 0 full, 5 partly, 13 not | 17 full, 1 partly (monster 21530 is unknown) |
| renewal, usable | `showscript`, `skilleffect`, `getexp2`, `setfont`, `heal` | 0 full | all full |
| renewal, usable | `bonus_script` | 13 full, 4 partly | 16 full, 1 partly |
| renewal, usable | `sc_start` | 422 full, 20 partly | 442 full |
| pre-renewal, equipment | `heal` | 0 full, 20 partly | 20 full |
| pre-renewal, equipment | `sc_start`, `sc_end` | 1 and 12 full, 14 and 1 not | 1 and 12 full |
| pre-renewal, equipment | `autobonus`, `autobonus3` | 34 and 1 full | 35 and 2 full |
| pre-renewal, equipment | `throwable ammo` | none | 17 not |
| pre-renewal, usable | `setfont`, `skilleffect`, `getexp2`, `heal` | 0 full | all full |

## Left

- `EF_ENCHANCE` (a typo constant of rAthena), `UNLUCKY`, `Eff_Confuse` and a `Hp` read in an autobonus's condition still refuse: they are constant names and reads that belong to the converter's names, not to item effects.
- `Ep19_Rgan_Scroll` needs monster 21530, which the client tables lack.
- The throwable ammo (17 items in each mode) waits for throw skills.
- `showscript` has no packet of its own, and `setfont` shows nothing, until the client takes them.
