# 0075: Damage taken with BF_WEAPON, absent monsters, regen intervals and autospell level 0

- Status: built. Details to review.
- Date: 2026-11-25.
- Code: [server/bonus](../../server/bonus) (`check.go`), [tools/convert](../../tools/convert) (`bonusmap.go`, `render.go`).
- Docs: [proposal 0002a](../proposals/0002a-bonus-names.md) ("Damage and damage taken"), docs/generated (item script and random option reports).
- Finishes the bonus-damage-taken node (docs/roadmaps/v0.2-parity/nodes/bonus-damage-taken/brief.md).

## Context

Five kinds of item bonus still failed to convert, although rAthena accepts all of them.

## Decisions

- `bonus3 bSubRace` and `bonus3 bSubEle` with `BF_WEAPON` and no range or attack type fill in every range and attack type (`pc_bonus_subrace`, `pc_bonus_subele`). The converter writes `kind = "physical"` alone, so `DamageGroup` takes `kind = "physical"` with `from = { race }` or `element` as the list sum (`subrace3`, `subele2`). The server already read the list back with `bfFlag`. The 10 `RACE_WEAPON_TOLERACE_*` random options and the three `bSubEle` items now convert.
- A monster ID that the mode's mob_db lacks converts to nothing for `bAddDamageClass`, `bAddMagicDamageClass`, `bAddDefMonster` and `bAddMDefMonster`. Pre-renewal's mob_db has 1900, 2048 and 2050 commented out, and rAthena's `pc_bonus2` stores the ID without a check, so no monster matches and the bonus never applies.
- The interval of `hp_gain_over_time`, `sp_gain_over_time`, `hp_loss_over_time` and `sp_loss_over_time` may be a function, as the Sealed cards' interval depends on the refine. The runtime already runs every computed argument.
- An autospell level that is 0 for sure converts to level 1. rAthena casts `it.lv ? it.lv : 1`. Falconer Flute reads an unset `.@i`.

## Left

- Nothing in this node.
