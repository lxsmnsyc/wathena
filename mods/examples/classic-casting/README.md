# Classic Casting

A sample ruleset mod (strategy section 3.6, decision 0019, mods/README.md, "Ruleset mods"). It extends `core.ruleset.renewal` and sets four flags to pre-renewal's values:

- `cast_time: pre_renewal`: cast times depend on DEX only, as rAthena's `skill_castfix` without `RENEWAL_CAST`. At 150 DEX every spell casts at once, and there is no fixed cast time.
- `element_table: pre_renewal`: damage uses pre-renewal's `attr_fix.yml` and its status bonuses. Fire on a water level 3 target deals nothing, where renewal's table deals 70%.
- `skill_cooldown: off`: skills keep pre-renewal's cooldowns. Angelus has none, where renewal's has 30 seconds.
- `skill_rebalance.HW_MAGICPOWER: pre_renewal`: Mystical Amplification raises MATK for the next spell only.

Everything else is renewal's: the items, monsters, classes, stats and damage formulas.

Run `wactl mod test mods/examples/classic-casting`. A case reads the flags with `rules.is` (review decision 32). The first case has a Venom Bug, whose MATK is always 340, cast Fire Bolt at a Knight in a water armor and at one in an earth armor: with pre-renewal's table the water armor takes a third of the earth armor's damage, where renewal's would give three fifths. `TestClassicCasting` in `tools/modkit` checks in Go what a test cannot read: the rules the world gets, a Mage's instant Fire Bolt at 150 DEX against renewal's cast time, Fire Bolt doing nothing to a water 3 Strouf, and the cooldowns.
