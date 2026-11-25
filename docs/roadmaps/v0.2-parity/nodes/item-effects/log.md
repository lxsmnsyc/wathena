# Log: item-effects

- Branch feat/item-effects from batch/v0.2-parity. Nothing of the brief existed yet (item-scripts reports at 99.0% equipment, 94.8% usable before; see outcome).
- NPC side already had `heal`, `getexp2`, `skilleffect` and `transform` (npcgroup3.go, npcpresent.go). The item converter (use.go `commandErr`) did not. Reused the same API calls.
- Decision: `showscript` maps to `player:say_aloud` (bubble plus name), because a packet of its own needs a protocol change. Recorded in decision 0078.
- Decision: `active_transform` and `transform`'s status become options of `player:transform` (`active`, `status`, `values`). The status lives in val2 of the transform status, so ending the transform ends it (status.cpp 13814). Added SCMonsterTransform and SCActiveMonsterTransform to statuschange/ids.go.
- Dead end: sc_start of the `_To_Throw` ammo. First converted it as an equip effect, which dropped the rows. rAthena skips ammo scripts of group THROWABLE when equipping (status.cpp 4047), so that was wrong. Now those items refuse with `throwable ammo` (17 in each mode). The rows of sc_start drop less than the brief hoped, on purpose.
- Decision: the actions of an autobonus's bonus script (sc_start, heal, specialeffect2...) run once at trigger, not inside the `timers.with_bonuses` callback, which reruns at recalculation.
- Decision: `rand` in a `bonus_script` rolls once at use. Done by rewriting `function() return random.integer(a, b) end` to the call itself in the timed bonus table.
- `getitempos` needed a field `position` on `wa.EquippedItem`. The equipment parity test host now gives each copy its real EQP_ bit; before that, Comp_S_Novice_Figure (accessory in the right slot) mismatched.
- Open doubt: showscript with SELF only reaches the player (`to = "self"`); any other target constant refuses.
- Open doubt: `zone.percentLoss` kills through `zap` (passive damage). It sends no damage number, as before.
- Not done (outside the brief): EF_ENCHANCE, UNLUCKY, Eff_Confuse, `Hp` in conditions, monster 21530.
