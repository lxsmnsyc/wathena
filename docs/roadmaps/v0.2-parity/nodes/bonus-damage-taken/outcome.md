# Outcome: bonus-damage-taken

spent: not measured (about one session, one active day)

## Built

- `server/bonus/check.go`: `DamageGroup` takes `kind = "physical"` alone with `from = { race }` or `element` as the `bSubRace (bonus3)` and `bSubEle (bonus3)` lists (rAthena's `BF_WEAPON` fills in every range and attack type). The status code already read them back through `bfFlag`. `checkArgs` allows a function for the `interval` of the regen bonuses (`isInterval`).
- `tools/convert/bonusmap.go`: `absentMonster` drops `bAddDamageClass`, `bAddMagicDamageClass`, `bAddDefMonster` and `bAddMDefMonster` for an ID the mode's mob_db lacks. `autoLevel` turns a static autospell level below 1 into 1.
- `tools/convert/render.go`: a non-static interval becomes a computed function in `luaBonus`.
- Decision 0075 (taken; the next free number is now 0076). Updated proposal 0002a (two table rows, rule 7) and decision 0014 ("Left").
- Regenerated docs/generated/item-scripts-renewal.md, item-scripts-pre-renewal.md, random-options-renewal.md and tools/convert/testdata/equipment_parity_*.md.

## Proof

- base: random-options-renewal.md 239 of 249 (10 `RACE_WEAPON_TOLERACE_*` failures). `bonus3 bSubEle` 2 partly (renewal) and 1 (pre-renewal). Unknown monster 1900, 2048, 2050 in pre-renewal. Regen interval and autospell level 0: 1 failure each.
- branch: random-options-renewal.md 249 of 249. `bonus3 bSubEle` 3 of 3 full (renewal), 1 of 1 (pre-renewal). `bAddDamageClass` 10 of 10 and `bAddDefMonster` 4 of 4 full in pre-renewal. `bHPRegenRate` and `bSPRegenRate` have no failure text (one item each stays partly for other parts). `bAutoSpell` 0 not converted. Equipment items converted fully: renewal 9993 to 9996, cards 1502 to 1503; pre-renewal 1875 to 1878 and 431 to 433, all with 0 mismatches.

## Deviations

- Touched `tools/convert/render.go`, `tools/convert/convert_test.go`, `tools/convert/randomopt_test.go`, `tools/convert/testdata/equipment_parity_*.md`, proposal 0002a and decision 0014, which are outside the brief's owned and shared paths. Each change is small and needed: the interval needs the renderer, the tests and parity files must follow the new counts, and the two docs named the old limits.
- The interval rule is an API change (a function in an argument other than the amount). The runtime already supported it, so only the checker changed; proposal 0002a rule 7 notes it.

## Stubs left

- None.

## Gotchas

- A static monster ID missing from the mode's mob_db is dropped without a warning, as in rAthena. A typo in a renewal-only script would also vanish.
- `bonus3 bSubRace` with `BF_WEAPON` and `bonus2 bSubRace` are separate sums in rAthena (`subrace3` and `subrace`); both lists add into one race fix at the same step in battle.cpp.
- Falconer_Flute reads an unset `.@i`; the converter writes level 1 as rAthena casts.

## Proposed learnings

- Run `go test ./tools/convert -run TestEquipmentParity -update` after a converter change that moves item counts | the parity report lists counts, and the test fails on stale files | tools/convert | yes

## Verification

- `go test ./server/bonus ./tools/convert ./server/status -p 1 -parallel 2`: 127 passed.
- `go test ./server/modscript -p 1 -parallel 2 -run 'Bonus|Item'`: ok.
- `go vet ./server/bonus ./tools/convert`: clean. `scripts/check.sh lint`: 0 issues.
- Reports regenerated with `go run ./tools/wactl convert items|random-options --mode <mode> --report docs/generated/<file>`.
- `scripts/check.sh quick` not run, by order.

## Size

- Estimated S, actual S.
