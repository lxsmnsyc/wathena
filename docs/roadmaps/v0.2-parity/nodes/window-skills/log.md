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
