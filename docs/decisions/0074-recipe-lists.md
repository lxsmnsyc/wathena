# 0074: Recipe Lists: Prepare Potion, Create Elemental Converter, produce and cooking

- Status: built. Details to review.
- Date: 2026-11-06.
- Code: [server/zone](../../server/zone) (`produce.go`, `skillitems.go`, `skills_alchemist.go`, `skills_assassin.go`, `skills_sage.go`), [server/skill](../../server/skill) (`sage.go`), [server/modscript](../../server/modscript) (`produce.go`, `runtime.go`, `prelude.lua`, `luaapi/spec.go`), [tools/convert](../../tools/convert) (`use.go`), [client/src/viewer](../../client/src/viewer) (`skillItems.ts`, `online.ts`, `main.ts`).
- Docs: [proposal 0001](../proposals/0001-protocol.md) ("Breaks"), [proposal 0002](../proposals/0002-lua-api.md) (review decision 66), [server/README.md](../../server/README.md), docs/generated (Lua API, item script reports).
- Builds the rest of step 1 of the window-skills node (docs/roadmaps/v0.2-parity/nodes/window-skills/brief.md), after [decision 0073](0073-skill-item-lists.md). Weapon forging is left out (below).
- Lua API: `player:open_produce(level)`, `player:open_cooking(level)`. Protocol: `SkillItemList.item_level`, `SkillItemList.cooking`, `SkillListItem.materials`, `SkillListMaterial`, additive. Data: none new (`crafting.json`'s `produce`, from decision 0073). Store: the core character variable `COOK_MASTERY`. Permission: none.

## Context

- Prepare Potion (`AM_PHARMACY`) and Create Elemental Converter (`SA_CREATECON`) failed before they started: rAthena's cast sends a recipe list (`clif_skill_produce_mix_list` with item level 22, `clif_elementalconverter_list` with 23), and the pick runs `skill_produce_mix`.
- The item scripts `produce <level>` and `cooking <level>` (smithing hammers, the Mini Furnace and the cooking kits) were refused by the converter.
- Twilight Alchemy and Create Deadly Poison held their recipes as hardcoded copies of `produce_db.txt` rows.

## Decision

### One produce routine

- `canProduceMix` is `skill_can_produce_mix` over `skill.DB.Crafting().Produce`: the first row of the product whose skill the player has, skipping a row of another skill than the open list's (menuskill_id); Bioethics for the Homunculus Supplement; room for the products; a row of the trigger's item level (above 20: the same level; 11 to 20: any cooking level; up to 10: that level or lower); and the materials, where amount 0 must be held but is not used.
- `produceMix` is `skill_produce_mix`: the materials go, then each product succeeds at rAthena's `make_per` in 10,000. The formulas ported: Prepare Potion and Twilight Alchemy (with the per-potion bonuses and the baby penalty), the ore skills (Iron, Steel, Enchanted Stone, Star Crumb at 100%), Create Deadly Poison (its failure costs 25% of max HP), Holy Water and Create Elemental Converter at 100%, cooking (the kit's level, base level, DEX, LUK, the dish's level, its material count and `COOK_MASTERY`; level 15 kits always succeed), and weapon forging without additions (job level, DEX, LUK, weapon level, the skill, Weaponry Research and the anvils). Any other row is 50%.
- A dish raises `COOK_MASTERY` on success and lowers it on failure, as rAthena, capped at 0 to 1999. It is a core character variable, saved with the character.
- `twilightRecipes` and `deadlyPoisonRecipe` are gone; both skills read the extract.
- The answer is the area effect rAthena's `clif_misceffect` or `clif_specialeffect` shows (pharmacy, refine or cooking, success or failure) and a system message ("X was made.", "Making X failed."), since `clif_produceeffect` has no event.

### Lists and picks

- The lists reuse `SkillItemList` and `SkillItemPick` (decision 0073). A recipe line holds the product and its materials. `item_level` is the list's level. A script's list has skill 0, and `cooking` marks a cooking kit's.
- Prepare Potion lists the level 22 rows of its own skill. Create Elemental Converter lists every level 23 row. A script's `produce` lists its level's rows with no menu skill. A script's `cooking` is a Prepare Potion menu, as `clif_cooking_list`, so the pick uses the cooking formula.
- The pick checks the line again (`skill_can_produce_mix` with the list's level and menu skill) and runs `produce_mix` with the row's skill, or with `SA_CREATECON` for the converters, as `clif_parse_ProduceMix`, `clif_parse_Cooking` and `clif_parse_SelectArrow`. Decision 0073's checks still hold: death, warps, an NPC talk, vending, a buying store and a trade.
- The list is built without the open list's menu skill: a new list replaces the open one (decision 0073's deviation), so the old menu does not filter it.

### Lua API and converter

- `player:open_produce(level)` and `player:open_cooking(level)` open the lists (review decision 66 of proposal 0002). Levels 1 to 10, weapon forging, raise an error. They need no permission: the pick checks the skills and materials.
- The converter writes `produce N` and `cooking N` with a fixed level as those calls. `produce 1` to `3` (the smithing hammers) are refused as "weapon forging", and so is a computed level.

### Left out

- Weapon forging from the smithing hammers: the pick cannot name star crumbs or element stones (rAthena's `material` slots of `CZ_REQMAKINGITEM`), and wAthena's copies hold no forged mark (`CARD0_FORGE` with the maker, element and star crumbs) that battle could read. A forged weapon without them would be a different item. The Combination Kit's Novice Cutter (renewal, `cooking 30` below base level 20) uses the forging rate and comes without the mark.
- The maker's name on products (`produce_item_name_input`), fame (forged weapons, potion rankings), Instruction Change's bonus from a homunculus, and the Genetic, Rune Knight and Guillotine Cross lists (third classes).

## Tests

- `server/zone`: `TestPreparePotion` (both modes), `TestPreparePotionFails`, `TestCreateElementalConverter` (both modes), `TestOpenProduce`, `TestOpenCooking`, `TestOpenProduceFromLua`; `TestTwilightAlchemy` and `TestCreateDeadlyPoison` run on the extract now.
- `server/skill`: `TestSoulSiphonDamage` checks that Create Elemental Converter is registered.
- `tools/convert`: `TestConvertUse` (`produce`, `cooking`, the refusal), `TestRunConvertedScripts`, `TestReportsCurrent`.
- `client`: `skillItems.test.ts`.
