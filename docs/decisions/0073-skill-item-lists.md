# 0073: Skill Item Lists: Arrow Crafting, Weapon Repair and Upgrade Weapon

- Status: built. Details to review.
- Date: 2026-11-06.
- Code: [server/zone](../../server/zone) (`skillitems.go`, `skillimpls.go`, `skills_blacksmith.go`, `zone.go`), [server/skill](../../server/skill) (`crafting.go`, `data.go`, `mods.go`), [server/gamedata](../../server/gamedata) (`skills.go`), [server/gateway](../../server/gateway) (`gateway.go`, `limits.go`), [client/src/viewer](../../client/src/viewer) (`skillItems.ts`, `main.ts`, `online.ts`), [client/src/net](../../client/src/net) (`connection.ts`).
- Docs: [proposal 0001](../proposals/0001-protocol.md) ("Breaks"), [reference/rathena/README.md](../../reference/rathena/README.md) ("Crafting"), [server/README.md](../../server/README.md).
- Builds part of step 1 of the window-skills node (docs/roadmaps/v0.2-parity/nodes/window-skills/brief.md). Prepare Potion, Create Elemental Converter, `cooking` and `produce` follow.
- Lua API: none. Protocol: `SkillItemList`, `SkillListItem` and the command `SkillItemPick`, additive. Data: `crafting.json` per mode. Store: none. Permission: none.

## Context

- Arrow Crafting (`AC_MAKINGARROW`), Weapon Repair (`BS_REPAIRWEAPON`) and Upgrade Weapon (`WS_WEAPONREFINE`) failed before they started, since the client had no item list. rAthena calls them menu skills: the cast sends a list (`clif_arrow_create_list`, `clif_item_repair_list`, `clif_item_refine_list`), sets `menuskill_id`, and the client's answer runs `skill_arrow_create`, `skill_repairweapon` or `skill_weaponrefine`.
- `create_arrow_db.yml` and `produce_db.txt` were not in the extracts.

## Decision

### Data

- The fork's `extract.py` writes `crafting.json` per mode (fork commit `7885f522f`): `arrows` is `create_arrow_db.yml` as `SkillArrowDatabase` loads it, and `produce` is `produce_db.txt` as `skill_parse_row_producedb` loads it. Produce is extracted now for the next part of the step.
- `gamedata.LoadSkillData` reads it into `skill.Data.Crafting`, and `skill.DB.Crafting()` returns it. `Clone` keeps it, so mods' databases have it too. Mods cannot add arrow or produce recipes yet.

### Protocol

- One event and one command serve every menu skill that picks an item. `SkillItemList` holds the skill and its lines (item, refine, cards). `SkillItemPick` names the skill and a line by its place in the list, or cancels.
- rAthena's client sends back an item ID (arrows) or an inventory index (repair, refine), and rAthena accepts any item the player has. The server here keeps the list (`Player.skillMenu`) and runs only a listed line, then checks it again as rAthena does. A pick of another skill does nothing. The list closes after any pick.
- A pick during a trade fails with `SkillFailed`, as `pc_istrading` makes it.
- Answers that rAthena sends as their own packets (`clif_upgrademessage`, `clif_item_repaireffect`) are system messages. Item changes are the usual inventory events.

### Behavior

- Arrow Crafting lists the sources of `create_arrow_db.yml` the player holds, in file order, leaving out a copy that is worn or not identified. A pick takes one source and adds each product. A product that does not fit is lost, as `skill_drop_items_full` is off, and the player sees why.
- Weapon Repair lists the target player's broken items, or fails when there are none. A pick takes one ore from the caster by the item's level (Iron Ore, Iron, Steel, Rough Oridecon for weapon levels 1 to 4; Steel for armor level 1; none for renewal's level 5 weapons and level 2 armors, which fail). Another player out of the skill's range gets "could not be repaired". Both players are told.
- Upgrade Weapon lists identified weapons below the skill's learned level, with the ore of their weapon level held (Phracon, Emveretarcon, Oridecon), not worn in a hand. The list may be empty, as rAthena sends it. A pick needs the ore of refine.yml's Normal cost to match the hardcoded one. The chance is the Normal rate in percent plus `(job level - 50) / 2`, or plus 10 for third classes. Success refines by 1 and puts the weapon back on; failure destroys it, with `ET_HUK`.

### Left out

- Fame for a forged weapon refined to +10 (`pc_addfame`), achievements, and the hatched pet eggs the lists skip: those systems do not exist yet. Broken items do not save yet (server/README.md).

## Tests

- `server/zone`: `TestArrowCrafting` (both modes), `TestUpgradeWeapon`, `TestUpgradeWeaponLimit`, `TestWeaponRepair`, `TestSkillPickWhileTrading`. `skill_casting.json` already covers the three skills' cast times and costs.
- `server/skill`: `TestCrafting`. `internal/reference`: `TestExtracts` reads `crafting.json`.
- `client`: `skillItems.test.ts`.
