# Outcome: window-skills (partial: step 1, part A)

Partial outcome. Step 1 part B, step 2 and step 3 remain (log.md, "Next session").

## Built

- Arrow Crafting, Weapon Repair and Upgrade Weapon cast and work in both modes (server/zone/skillitems.go: `makingArrow`, `weaponRepair`, `upgradeWeapon`, `arrowCreate`, `repairWeapon`, `weaponRefine`, `skillItemPick`). Before this, all three failed before they started.
- Protocol: event `SkillItemList` with `SkillListItem`, command `SkillItemPick` (schema/proto, generated Go and TypeScript). Proposal 0001 "Breaks" amended.
- Data: `reference/rathena/extracts/<mode>/crafting.json` (create_arrow_db.yml and produce_db.txt), from fork commit `7885f522f`. Loaded into `skill.Data.Crafting`, read through `skill.DB.Crafting()`, `Crafting.Arrow`.
- Client: `client/src/viewer/skillItems.ts` (`SkillItemsWindow`), opened from `onSkillItemList`, picks through `Connection.skillItemPick`.
- Decision 0073 (docs/decisions/0073-skill-item-lists.md). Numbers taken: decision 0073. No review decision, proposal or migration.
- Already there before: nothing of these three skills. Twilight Alchemy's 5 hardcoded produce recipes stay until part B.

## Deviations

- Edited outside owned and shared paths, minimally: server/zone (the skill behaviors live there, not in server/skill), server/gateway (command dispatch and rate kind), server/gamedata/skills.go (load crafting.json), reference/rathena (new extract, embed list, embed test count, README), internal/reference/reference_test.go (extract list), client/src/gen (generated from the proto), docs/decisions/0073, server/README.md (skill list), and the rAthena fork (`extract.py`, commit `7885f522f`, not pushed, like the fork's other commits).
- Step 1 split in two PRs: this one covers Arrow Crafting, Weapon Repair and Upgrade Weapon.

## Stubs left

- Prepare Potion and Create Elemental Converter still fail before they start; `cooking` and `produce` are still refused by the converter. Part B of step 1.

## Gotchas

- Golden zone job tables have no refine table; tests that refine copy them and call `SetRefine` (`withRefine` in skillitems_test.go).
- `SkillItemPick` names a line of the server's own list; part B and step 2 can reuse `Player.skillMenu`.
- Broken items do not save yet, so repair matters only within a session.

## Proposed learnings

- A window skill keeps the offered list on the player and the pick names a line | the client cannot pick what was not offered, and rAthena's checks still run | server/zone | yes

## Verification

- `go test -p 1 -parallel 2 ./server/skill ./server/gamedata ./server/gateway ./internal/reference ./schema ./reference/rathena`: ok.
- `go test -p 1 -parallel 2 ./server/zone -run 'Skill|Smith|Arrow|Upgrade|Repair|Alchemist|Twilight|Homunculus|Refine|Vending|Trade|Quest|Memo|Blacksmith|Maximize|Golden'`: ok.
- client: `tsc --noEmit`, `vitest run src/viewer/skillItems.test.ts`, oxlint on the touched files: ok.
- `scripts/check.sh lint`: passed. The full check is left to the orchestrator.

## Size

- Estimated L, actual L for the node; this part is M. The data extract and the generic protocol took most of it.

spent: 1 active day (2026-11-06), tokens not measured by the worker.
