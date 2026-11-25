# Log: window-skills

## 2026-11-06, session 1 (step 1, part A)

- Checked first: Arrow Crafting, Weapon Repair, Upgrade Weapon, Prepare Potion and Create Elemental Converter were `notPorted` (fail before start). `cooking` and `produce` were refused by the converter (tools/convert/builtins.go). Twilight Alchemy already has a partial `skill_produce_mix` with 5 hardcoded recipes (server/zone/skills_alchemist.go).
- `create_arrow_db.yml` and `produce_db.txt` were not extracted. Added `build_crafting` to the fork's extract.py (fork commit `7885f522f`) and copied `crafting.json` for both modes. Produce is in it already for part B.
- Step 1 alone is too big for one PR (protocol, server, client for 4 windows plus 2 script commands). Split: part A = the item list skills (Arrow Crafting, Weapon Repair, Upgrade Weapon); part B = the recipe skills (Prepare Potion, Create Elemental Converter, `cooking`, `produce`).
- Protocol: one generic `SkillItemList` event and `SkillItemPick` command (proposal 0001 amended). The server keeps the list and the pick names a line, so the client cannot pick what was not offered. Step 2's Item Appraisal and step 3's Hindsight can reuse it (Hindsight picks a skill, so it may need a skill field on the line).
- Decision 0073. Golden zone tables have no refine data: tests copy the tables and SetRefine (`withRefine`).
- Weapon Repair has a 2.5 s fixed cast; zone tests need ~4 s windows.

## Next session: step 1, part B

- Prepare Potion (`AM_PHARMACY`, `clif_skill_produce_mix_list(sd, skill, 22)`), Create Elemental Converter (`SA_CREATECON`, `clif_elementalconverter_list`, item level 23, picked through `clif_parse_SelectArrow`), and the script commands `produce <level>` and `cooking <list>` (script.cpp 11093, 11108).
- Needs a recipe list event (product + materials, maybe quantity) and a produce command, a generic `skill_can_produce_mix` / `skill_produce_mix` over `skill.DB.Crafting().Produce`, replacing `twilightRecipes`. Smithing (`produce` levels 1-3, 21) adds star crumbs and element stones and the forge rate.
- Converter: `cooking` and `produce` need a Lua API (proposal 0002 amendment) and converter rules in tools/convert.
- Then step 2 (Warp Portal memo list, Item Appraisal, Change Cart, Decorate Cart, Open Buying Store) and step 3 (Hindsight, Abracadabra, party reach, ensembles).

## 2026-11-06, review fixes

- Fresh review: picks while dead, during an NPC talk, vending or buying store, and of a slot that now holds another copy. Fixed: death, warps and leaving the map clear the list; pc_istrading's four cases fail the pick; the list keeps each line's item ID and serial. Tests added. A new cast replacing the list is kept and recorded as a deviation in 0073.

## 2026-11-06, session 2 (step 1, part B)

- Checked first: Prepare Potion and Create Elemental Converter still `notPorted`; `cooking` and `produce` still refused by the converter; Twilight Alchemy's `twilightRecipes` (5 hardcoded rows) still in skills_alchemist.go. produce_db is in crafting.json since part A.
- Plan: reuse `SkillItemList`/`SkillItemPick` for recipe lists (additive fields: line materials, the list's item level; skill 0 for a script's list), a generic `canProduceMix`/`produceMix` over `Crafting.Produce`, `player:open_produce` and `player:open_cooking` (review decision 66), converter rules for `produce` and `cooking`.
- Weapon forging (`produce 1` to `3`) left out: it needs the pick to carry star crumbs and element stones, and forged marks (element, star crumbs, maker) on items, which wAthena's items do not hold. The converter refuses it with a reason. Ores (`produce 21`) and the equip path for the Combination Kit's Novice Cutter use rAthena's formulas, without the forged mark.
- Create Deadly Poison's `deadlyPoisonRecipe` was a second hardcoded produce_db row; it now goes through `produceMix` too.
- Decision 0074, review decision 66. Converter: renewal usable 4940 -> 4947 full, pre-renewal 1333 -> 1339.

## Next session

- Weapon forging (rest of step 1): additions on `SkillItemPick` (star crumbs, element stones), a forged mark on copies (maker, element, star crumbs) and its battle effects, then lift the level 1-10 refusal in `OpenProduce` and the converter.
- Step 2 (Warp Portal memo list, Item Appraisal, Change Cart, Decorate Cart, Open Buying Store) and step 3 (Hindsight, Abracadabra, party reach, ensembles).
