# Outcome: proposal-sieges

## Built

- docs/proposals/0004-sieges.md: design proposal 0004 (Problem, Change sections 1 to 14, Breaks, Alternatives, 19 questions for review).
- docs/roadmaps/v0.2-parity/contracts/sieges.md: Interface, Behavior, Stub and Contract tests filled from the proposal. Still `status: draft`, version 1, for the orchestrator to freeze after the maintainer accepts.
- Numbers taken: proposal 0004. The design names migration 00043 (`castles`) for sieges-castles to write; no decision or review decision number used.

## Deviations

- None from the brief. The contract stays draft until the maintainer accepts, as for proposal-instances.

## Stubs left

- None. sieges-war builds on sieges-castles' real registry.

## Gotchas

For sieges-castles:

- **Owned paths look wrong.** `client/src/world/castle*` is the world generator folder; the guild window and emblems are in `client/src/viewer/` (`guild.ts`, `emblems.ts`). The node also needs `server/social/**` (guild break hook), `server/mod/**` (`CastleKind`, permission `castles.manage`), `server/gamedata/**` (loading `data/castles.yml`), `server/cmd/waserver/**` (registry wiring, load before zones), `reference/rathena/extracts/**` plus the fork's `tools/wathena/extract.py` (`castles.json`, `map_flags.json`, `job_noenter.json`), and `server/admin/**`, `server/adminworld/**`, `server/staff/**`, `schema/proto/wathena/admin/**` if it builds the owner endpoint.
- Migration 00043 is free only if instances-core still needs none (proposal 0003 adds no migration). Check `server/store/migrations` after rebasing.
- `social.Registry.guildBreak` (server/social/guild.go:655) holds `r.mu`; the castle hook must queue `OnGuildBreak` and the owner change, not call into zones under that lock.
- `startNPCs` is lazy (server/zone/npcs.go:84). Castle zones must start through the `OnAgitInit` broadcast at boot, after castles load, or `OnClock0001` treasure never runs.
- NPC badges: `EntityAppear.guild` is filled only for players today (`updateBadge`, server/zone/guild.go:397). Flags and guardians need the same path, and emblem changes must reach NPCs and guardians showing the guild.

For sieges-war:

- **Too big for one L node.** It holds the GvG rules (section 5, the largest blast radius in the design), the siege, guardians, the Emperium, five new calls, the converter rows, the admin endpoints, the e2e test and the performance scenario. See "Proposed split" below.
- There is no player versus player code at all: `isEnemy` (server/zone/query.go:183) returns false for two players. Skill target checks, splash, traps and statuses all go through it; read every caller.
- `Zone.GvG` (server/zone/zone.go:574) has readers in skills_guild.go only, but comments mark GvG rules waiting in skills_priest.go:87 and 678, skills_bard.go:812, skills_knight.go:325, skills_npc_buff.go:334, skills_assassin.go:116, skillunits.go:555, cast.go:414, status/changes.go:361, statuschange end.go:293, devotion.go:43, setup.go:56 and 596, mob/stats.go:44, battle/weapon.go:871 and 939, battle/config.go:35, social/guildrelations.go:18, rules/limits.go:100.
- Emergency Call (skills_guild.go:46) tests bit 1 always; it must test bit 2 during any siege.
- `skill.Def.NoCast` is loaded and never checked; `status.Equipment.MapType` is fixed to `item.MapNormal` (server/status/equip.go:250). Turning both on changes normal maps too (flag 1); list what changes in the decision record.
- `reviveMs` is 3000 with automatic respawn (combat.go:19); GvG ground uses 1000.
- The battle package has no map context; pass a small value (flags and rates), not the zone.
- `maps.set_flag` has no permission today; the GvG flags must need `maps.control`.
- `npc:wake()` depends on npc-api-timers' `timers.wait`.
- `jobcanentermap` restrictions only hit third and later classes (v0.3.0), so the v0.2.0 effect is mostly the expanded classes; still wire the check into warps, Warp Portal, Emergency Call and `party:warp`.
- Golden tests need a new `record_gvg` in the fork's recorder; the fork's `wathena-reference` branch is local only (GOAL open question).
- Performance: Crowd is already over budget at 250 in view (decision 0050) and decision 0055 is not built. Measure the `siege` scenario early; it may pull 0055 in before v0.2.0.

For sieges-content:

- The guild folders lean on `duplicate` (1,000), `strnpcinfo(2)` and `(4)` for the castle map, computed `donpcevent` names, `getd`/`setd` on `$agit_<code>`, NPC timers (SE barricades), `setunitdata UMOB_DMGIMMUNE`, `specialeffect`, `mapannounce` with colors and `bc_woe`. Labels such as `OnRecvCastlear01` and `OnRecvCastleAr01` must match without regard to case.
- Renewal also loads `npc/re/guild` (`invest_main.txt`, `mission_main.txt`) and `npc/re/quests/woe_te`, which are not in the guild folders the brief names; include them or they stay unconverted. `npc/re/warps/guildcastles.txt` holds the TE entry warp with `jobcanentermap` and `getguildalliance`.
- extract-folders must add `guild`, `guild2`, `re/guild`, `re/guild3` and the mapflag lines.

Proposed split (question 19 of the proposal):

- **gvg-rules** (new, L, after sieges-castles or in parallel by contract): section 5 whole: flags and predicates, player targeting on `vs` maps, damage rates, Emperium hit rules, nocast, noequip, `restricted` zones, revival, knockback, movement skills, respawn, guild action blocks, Emergency Call bits, ruleset rules, `record_gvg` goldens. Owns `server/battle/gvg*.go` and the zone predicates; shares `server/zone/**`, `server/battle/**`, `server/status/**`, `server/statuschange/**`, `server/social/**`.
- **sieges-war** (M to L, after gvg-rules, sieges-castles and npc-api-timers): siege state and events, `OnAgitInit` at boot, guardians and the Emperium's castle data and stats, `maps.respawn_guild`, `guild:warp`, `maps.can_enter`, `guilds.relation`, `npc:wake()`, the converter rows, admin endpoints and GM commands, the e2e bot siege, the `siege` performance scenario.
- **sieges-castles** stays M, with the client castle window, flag emblems and `MapRules` handling in `client/src/viewer/` (or move all client work into a small `sieges-ui` node after sieges-war).
- **sieges-content** stays M as an outline node, with the extra renewal folders above.

## Proposed learnings

- rAthena's castle labels are NPC events sent to every NPC; keep them as labels in converted scripts | the zone start rule and case-insensitive match depend on it | tools/convert, server/zone | yes
- GvG rules read zone fields (`gvg`, `gvgGround`, `vs`, `mapType`), never flags in hot paths | ticks stay within budget | server/zone, server/battle | yes

## Verification

- Design only. No builds, tests or `scripts/check.sh` runs, as the node asks.
- Checked the proposal and contract for em-dashes: none.

## Size

- Estimated M, actual M to L: the battle rules alone needed a full survey of rAthena's GvG checks.
