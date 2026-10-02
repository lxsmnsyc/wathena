# 0011: Skills Run in Go, with Mod Skills in Lua

- Status: proposed.
- Date: 2026-10-02.
- Code: [server/skill](../../server/skill), [server/battle](../../server/battle) (`skill.go`, `magic.go`, `prerenewal.go`), [server/zone](../../server/zone) (`cast.go`, `skillattack.go`, `skillunits.go`, `skillimpls.go`, `skilldispatch.go`, `skillhooks.go`, `skillscripts.go`, `query.go`), [server/modscript](../../server/modscript) (`skills.go`), [server/store](../../server/store) (migration 00011), [mods/examples/chain-lightning](../../mods/examples/chain-lightning).
- Builds on [proposal 0002](../proposals/0002-lua-api.md), review decisions 18 and 20, [decision 0010](0010-status-effects.md), and strategy sections 5.3 and 6.8.

## Decision

- Skills run in Go. The package `skill` holds the skill data, the skill trees, the cast time, delay, cooldown and requirement formulas, and each skill's damage formulas. The zone holds the cast: it ports `unit_skilluse_id2`, `unit_skilluse_pos2`, `skill_castend_id`, `skill_castend_pos`, `skill_attack`, `skill_blown`, the ground units of `skill_unitsetting` and `skill_unit_timer`, and each first class skill's `SkillImpl` class. Each function names the rAthena function it ports.
- The data comes from rAthena's `skill_db.yml` and `skill_tree.yml`, through two new extracts per mode, `skills.json` and `skill_trees.json`. Values per level are arrays of 13. A mod's `data/skills.yml` and `data/skill_trees.yml` change them and add skills, with IDs from the mod range.
- Skill damage goes through the same `battle` code as normal attacks, in both modes. A skill's own formulas are a Go type with rAthena's `SkillImpl` methods: `SkillRatio` (`calculateSkillRatio`) and `HitRate` (`modifyHitRate`). Shared shapes are types too: a weapon hit, a splash around a target (`SkillImplRecursiveDamageSplash`), a status on the target (`StatusSkillImpl`), and a ground unit.
- The zone advances casts each tick. A cast keeps rAthena's timing: the cast bar, the act delay of `SECURITY_CASTTIME`, the after-cast delay, cooldowns, and the walk delay. Only hits of at least 1 damage stop a cast, and only when the skill's `CastCancel` allows it and the player has no `bNoCastCancel`.
- Ground units are entities. Viewers see them as `ENTITY_KIND_SKILL_UNIT`. A timer every 100 ms runs them, as `skill_unit_timer`.
- Entity queries (decision 18) are one function, `findEntities`, which skills and scripts share. Its areas follow rAthena's `map_foreachinrange`, `map_foreachinarea` and `map_foreachinpath`. Ties sort by entity ID, which is the order the zone added the entities in. `"random"` shuffles with the zone's secret stream.
- Learned skills are saved in `character_skills` (migration 00011), as rAthena's `skill` table.

## Mod skills

- A mod's `data/skills.yml` adds skills, with IDs from 20,000, and overlays core's skills by name. `data/skill_trees.yml` adds them to jobs. Setting `script` on a core skill replaces its behavior and needs `rules.override`.
- A skill with a script runs its `cast` event in a context of kind `"skill"` when the skill takes effect, instead of core's behavior. The cast object keeps its fields, so timers can use it later. `cast:deal_damage` runs `skill_attack` with the script's percent as the skill's ratio.
- A cast method runs inside a script's call, where Lua cannot run again. What a hit makes Lua do waits until the call returns: the death and its drop and exp hooks, item damage handlers, and `skill_used`. `cast:run_default` waits the same way.
- A learned passive skill with a script runs it in a context per player and skill. It starts again when the level changes, and ends when the player leaves the zone.
- The hooks `skill_damage`, `cast_time`, `skill_cost` and `cast_allowed` wrap rAthena's values. Without hooks the results are rAthena's.

## Why Go for core skills

| | Core skills in Lua | Core skills in Go |
|---|---|---|
| Matches rAthena | A second port of `battle.cpp` order in another language. | One port, next to the normal attack code it shares. |
| Parity tests | Need a Lua state per case. | Run against `skill_damage.json` and `skill_casting.json` directly. |
| Mods | Could change any skill. | Mods add skills with a script, and replace a core skill's behavior with `script` and `rules.override` (decision 20). |

## Other choices

- A player who casts on a target out of range walks into range first, and the skill is cast when the walk gets there. rAthena's client asks for this itself, with `stepaction`. The server does it, so the web client needs no path code for it.
- Skills that need a window the client does not have yet fail before they start, so they cost nothing: Warp Portal, Item Appraisal, Vending, Arrow Crafting, Change Cart, Decorate Cart, Open Buying Store and Call Baby.
- Teleport opens no menu: both levels warp to a random cell, as a Fly Wing does. Level 2's choice of the save point waits for the menu.
- Party skills reach the caster alone, as rAthena does for a player without a party, since parties do not exist yet.
- Hits that come from a ground unit name the unit as the source of the `Damage` event, or the caster when the viewer does not see the unit.

## Left

- Monster skills, which come with monster AI in step 5.
- The menus of Teleport and Warp Portal, memo points, carts, vending, Item Appraisal, Arrow Crafting and buying stores.
- Water cells for Aqua Benedicta, which the map data does not have yet.
- Hit stun (`damage_walk_delay`), which the zone does not have for normal attacks either.
- Trick Dead's dead look, which the protocol does not carry yet.
