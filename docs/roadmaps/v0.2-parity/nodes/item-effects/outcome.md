# Outcome: item-effects

## Built

Nothing of the brief existed before; every row below was refused. Branch `feat/item-effects`, decision record docs/decisions/0078-item-effects.md, review decision 67 in proposal 0002 (API additions). Migration number not used (next is still 00044).

Converter (tools/convert):
- `use.go`, `usestatus.go`: `heal`, `getexp`, `getexp2`, `skilleffect`, `showscript`, `setfont`, `transform` (with its status and values) and `active_transform` convert in use scripts, unequip scripts and autobonus effect scripts. `bonus_script` rolls `rand` once when the item is used.
- `equipitem.go`: `sc_start`, `sc_start2`, `sc_start4` and `sc_end` convert in equip scripts. The actions of an autobonus's bonus script (statuses, `heal`, `specialeffect2`, `showscript`) run once at trigger, before the effect script.
- `expr.go`: `getitempos()` reads `equipped.position`.
- `item.go`: items of the group `THROWABLE` do not convert (rAthena skips their script on equip): feature `throwable ammo`.

Server and API:
- `player:transform(monster, ms, { active, status, values })`, `player:set_font(font)`, `wa.EquippedItem.position`. Statuses of a transform end with it (`statuschange/end.go`, ids `SCMonsterTransform`, `SCActiveMonsterTransform`).
- `player:heal_percent` with -100 or less kills (E4, review decision 60): `server/zone/inventory.go` `percentLoss`. Docs at spec.go (heal_percent), server/README.md, tools/convert/README.md updated.

Report counts (docs/generated/item-scripts-*.md, regenerated):
- renewal equipment: full 13833 to 13966 (98.1% to 99.0%), not 166 to 120. `heal` 57 partly and 3 not to 58 full and 2 partly; `sc_start` 21 not to 0 not (13 scripts moved to `throwable ammo`); `active_transform` 25 partly and 8 not to 34 full; `showscript` 12 full; `getitempos` row gone.
- renewal usable: full 4947 to 5005; `transform` 17 of 18 full (monster 21530 is unknown); `showscript`, `skilleffect`, `getexp2`, `setfont`, `heal` all full; `bonus_script` 16 of 17 full.
- renewal delayed: full 16 to 17.
- pre-renewal: `heal` 20 full, `sc_end` 12 full, `autobonus` 35 full, `autobonus3` 2 full, `setfont`, `skilleffect`, `getexp2`, `heal` (usable) full.
- New row `throwable ammo`: 17 items not converted in each mode.
- Equipment parity stays green with more records compared: 10,046 of 10,046 equipment and 1,539 of 1,539 cards match in renewal (before 9,996 and 1,503).

## Proof

Not a bug-fix node, except E4.

- base: `TestHealPercentTakes` expected HP 1 at -100.
- branch: the same test now expects the player dead at -100, and 1 HP at -99; passes.

## Deviations

- Files outside the brief's owned and shared paths, kept minimal: `server/zone/playeractions.go` (`Transform`, `SetFont`), `server/zone/inventory.go` (`percentLoss`), `server/zone/equipment.go` (`position`), `server/zone/zone.go` (field `font`), `server/zone/npcs_test.go`, new `server/zone/itemeffects_test.go`, `server/modscript/playerapi.go` (Host interface: `Transform` takes `TransformOptions`, new `SetFont`; this changes `PlayerActionHost` for any other implementer), `tools/convert/README.md`, `server/README.md`, test files of tools/convert. item-skill-use shares server/zone/** and server/modscript/**; no overlap in code seen.
- `showscript` becomes `player:say_aloud`, which adds the player's name before the text; rAthena's packet has none. A protocol change was out of scope.
- Autobonus bonus-script actions run once at trigger; rAthena reruns them at each recalculation.
- `rand` in a bonus_script rolls once; rAthena rolls at each recalculation.
- Throwable ammo no longer runs `sc_start` on equip: rAthena does not either. The report shows them as `throwable ammo`, so the `sc_start` row drops by less than the brief's 21 and 14: 13 renewal and 14 pre-renewal scripts of that row are throwable ammo and stay refused for a lack of throw skills.
- The brief says 10 NPC files use `percentheal -100`; review E4 names 8 (plus the Poison Bottle), so the brief's number is wrong.
- Review decision 56 text at spec.go and READMEs changed as the brief says. Proposal 0002's older lines (1125, 1653) still read "never below 1"; review decision 60 and 67 supersede them.

## Stubs left

- `player:set_font` stores the font and shows nothing. The client has no fonts.
- `showscript` has no packet of its own.

## Gotchas

- `testHost` of tools/convert embeds a nil `modscript.PresentHost` and answers only `SayAloud` and `ShowSkillEffect`. A script that calls another presentation method in those tests panics on the nil embed.
- `testHost.positions` gives each equipped copy's EQP_ bit in the equipment parity test; copies without one read as the left accessory (0x80).
- `Transform` (Host) now takes `modscript.TransformOptions`.
- Throwable ammo detection reads item group `THROWABLE` from item_groups.json; a mod's ammo is not covered.
- A bonus script's `rand` rewrite is a regexp on the generated Lua (`rolledAmount` in usestatus.go); other computed amounts still refuse.

## Proposed learnings

- Check rAthena's status_calc_pc_ before converting a script of an Ammo item: ammo of group THROWABLE never runs on equip | rAthena skips it (status.cpp 4047) | tools/convert | no

## Verification

- `go test ./tools/convert -p 1 -parallel 2` (whole package, includes TestGolden, TestEquipmentParity, TestRunConvertedScripts, report tests): ok.
- `go test ./server/modscript/... ./server/statuschange ./server/item -p 1 -parallel 2`: ok.
- `go test ./server/zone -run 'Transform|Heal|Equip|Status|Poison|ItemScript|Kafra' -p 1 -parallel 2`: ok.
- `go vet ./server/... ./tools/convert`, `golangci-lint run ./server/... ./tools/convert/...`: clean.
- Generated files rewritten: `go test ./server/modscript/luaapi -run TestGeneratedFiles -update`; `go run ./tools/wactl convert items --mode <mode> --report docs/generated/item-scripts-<mode>.md` for both modes; `-update` for TestGolden and TestEquipmentParity.
- Not run: `scripts/check.sh quick queries` (the orchestrator runs it once).

## Size

- Estimated M, actual M. The API pieces (transform options, position, font) and the throwable-ammo finding added work the brief did not list.
- Risk estimated low, measured medium. The `Host` interface change and the kill at -100 touch live zone code, and the throwable rule changes what 17 items do per mode.
