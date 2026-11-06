# Outcome: window-skills (partial: step 1, parts A and B)

Partial outcome. Weapon forging (the rest of step 1), step 2 and step 3 remain (log.md, "Next session").

## Built

- Arrow Crafting, Weapon Repair and Upgrade Weapon cast and work in both modes (server/zone/skillitems.go: `makingArrow`, `weaponRepair`, `upgradeWeapon`, `arrowCreate`, `repairWeapon`, `weaponRefine`, `skillItemPick`). Before this, all three failed before they started.
- Protocol: event `SkillItemList` with `SkillListItem`, command `SkillItemPick` (schema/proto, generated Go and TypeScript). Proposal 0001 "Breaks" amended.
- Data: `reference/rathena/extracts/<mode>/crafting.json` (create_arrow_db.yml and produce_db.txt), from fork commit `7885f522f`. Loaded into `skill.Data.Crafting`, read through `skill.DB.Crafting()`, `Crafting.Arrow`.
- Client: `client/src/viewer/skillItems.ts` (`SkillItemsWindow`), opened from `onSkillItemList`, picks through `Connection.skillItemPick`.
- Decision 0073 (docs/decisions/0073-skill-item-lists.md). Numbers taken: decision 0073. No review decision, proposal or migration.
- Already there before: nothing of these three skills. Twilight Alchemy's 5 hardcoded produce recipes stay until part B.

Part B (decision 0074):

- Prepare Potion and Create Elemental Converter cast and open recipe lists in both modes; the pick makes the product with rAthena's `skill_produce_mix` rate (server/zone/produce.go: `canProduceMix`, `produceMix`, `openRecipes`, `recipePick`, `preparePotion`, `createConverter`). Before this, both failed before they started.
- One produce routine over `crafting.json`'s `produce` replaces Twilight Alchemy's `twilightRecipes` and Create Deadly Poison's `deadlyPoisonRecipe`. It ports the potion, ore, Deadly Poison, converter, cooking and forging rates, and `COOK_MASTERY`.
- Item scripts: `produce N` (N above 10) and `cooking N` convert to `player:open_produce(N)` and `player:open_cooking(N)` (review decision 66 of proposal 0002). Reports: renewal usable 4940 -> 4947, pre-renewal 1333 -> 1339 full. `produce 1` to `3` stay refused as "weapon forging".
- Protocol: `SkillItemList.item_level`, `SkillItemList.cooking`, `SkillListItem.materials`, `SkillListMaterial` (proposal 0001 "Breaks" amended). Skill 0 is a script's list.
- Client: the same window titles and shows recipe lines with their materials.
- Numbers taken: decision 0074, review decision 66. No proposal or migration.
- Already there before: the `SkillItemList` window and pick (part A), the produce extract, Twilight Alchemy's partial `skill_produce_mix`.

## Deviations

- Edited outside owned and shared paths, minimally: server/zone (the skill behaviors live there, not in server/skill), server/gateway (command dispatch and rate kind), server/gamedata/skills.go (load crafting.json), reference/rathena (new extract, embed list, embed test count, README), internal/reference/reference_test.go (extract list), client/src/gen (generated from the proto), docs/decisions/0073, server/README.md (skill list), and the rAthena fork (`extract.py`, commit `7885f522f`, not pushed, like the fork's other commits).
- Step 1 split in two PRs: this one covers Arrow Crafting, Weapon Repair and Upgrade Weapon.
- A new cast while a list is open replaces it; rAthena refuses the cast instead (decision 0073).
- Part B edited, minimally: server/modscript (`produce.go`, `runtime.go`: the host call behind the two Lua methods), tools/convert (`use.go` rules, `runtime_test.go` test host), server/skill/sage.go (registers SA_CREATECON), docs/decisions/0073 and 0074, server/README.md.
- Weapon forging (`produce 1` to `3`) left out: the pick cannot name star crumbs or element stones, and copies hold no forged mark. The Combination Kit's Novice Cutter uses the forging rate without the mark (decision 0074).
- A recipe list is built without the open list's menu skill, since a new list replaces the open one.

## Stubs left

- Weapon forging: `player:open_produce` refuses levels 1 to 10, and the converter refuses `produce 1` to `3`. It needs additions in `SkillItemPick` (star crumbs, element stones) and a forged mark on copies that battle reads (`CARD0_FORGE`: maker, element, star crumbs).
- Maker names on products (`produce_item_name_input`), fame, Instruction Change's bonus: those systems do not exist yet.

## Gotchas

- Golden zone job tables have no refine table; tests that refine copy them and call `SetRefine` (`withRefine` in skillitems_test.go).
- `SkillItemPick` names a line of the server's own list; part B and step 2 can reuse `Player.skillMenu`.
- Golden items have no display name, so produce answers in tests use the Aegis name ("Making White_Slim_Potion failed.").
- Create Deadly Poison now goes through `produceMix`, so an open recipe list of another skill makes it fail, as rAthena's menuskill_id check.
- Broken items do not save yet, so repair matters only within a session.

## Proposed learnings

- A window skill keeps the offered list on the player and the pick names a line | the client cannot pick what was not offered, and rAthena's checks still run | server/zone | yes

## Verification

- `go test -p 1 -parallel 2 ./server/skill ./server/gamedata ./server/gateway ./internal/reference ./schema ./reference/rathena`: ok.
- After review: `go test -p 1 -parallel 2 ./server/zone -run 'TestArrowCrafting|TestUpgradeWeapon|TestWeaponRepair|TestSkillPick'`: ok (adds death, warp, busy, changed slot, cancel, index, other skill and not refineable cases).
- `go test -p 1 -parallel 2 ./server/zone -run 'Skill|Smith|Arrow|Upgrade|Repair|Alchemist|Twilight|Homunculus|Refine|Vending|Trade|Quest|Memo|Blacksmith|Maximize|Golden'`: ok.
- client: `tsc --noEmit`, `vitest run src/viewer/skillItems.test.ts`, oxlint on the touched files: ok.
- `scripts/check.sh lint`: passed. The full check is left to the orchestrator.
- Part B: `go test -p 1 -parallel 2 ./server/skill ./server/modscript/... ./schema`: ok. `./server/zone -run 'Produce|Potion|Converter|Cooking|Twilight|Deadly|Homunculus|Skill|Arrow|Upgrade|Repair|Alchemist|Sage|Smith|Blacksmith|Golden|NPC|Present|API'`: ok. `./tools/convert -run 'TestConvertUse|TestReportsCurrent|TestRunConvertedScripts|TestParse|TestConvert'`: ok (0 failures in the converted scripts run). client: `tsc --noEmit`, `vitest run src/viewer/skillItems.test.ts`, oxlint and oxfmt on the touched files: ok. `scripts/check.sh lint`: passed.

## Size

- Estimated L, actual L for the node; this part is M. The data extract and the generic protocol took most of it.

spent: 1 active day (2026-11-06, parts A and B), tokens not measured by the worker.
