# Outcome: proposal-sieges

## Built

- docs/proposals/0004-sieges.md: design proposal 0004 (Problem, Change sections 1 to 14, Breaks, Alternatives, 19 questions for review).
- docs/roadmaps/v0.2-parity/contracts/sieges.md: Interface, Behavior, Stub and Contract tests filled from the proposal. Still `status: draft`, version 1, for the orchestrator to freeze after the maintainer accepts.
- Numbers taken: proposal 0004. The design names migration 00043 (`castles`) for sieges-castles to write; no decision or review decision number used.
- Accepted: the maintainer accepted every recommendation on 2026-10-27, including the split into gvg-rules, a smaller sieges-war and sieges-ui. The proposal's "Questions for review" became "Decisions in review", section 15 names who builds what, and the contract is frozen at version 1.

## Deviations

- None from the brief.

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

- **Too big for one L node.** It holds the GvG rules (section 5, the largest blast radius in the design), the siege, guardians, the Emperium, five new calls, the converter rows, the admin endpoints, the e2e test and the performance scenario. Accepted split: see "Nodes after acceptance" below; most of these gotchas go to gvg-rules.
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

## Nodes after acceptance

For the orchestrator's briefs. Every node also shares the hot files of GOAL.md: `server/modscript/luaapi/spec.go`, `server/modscript/prelude.lua`, `docs/generated/lua-api.md`, `docs/generated/lua/**`, `docs/proposals/0002-lua-api.md`, `tools/convert/rathena.lua`, `tools/convert/npcexpr.go`, `tools/convert/output.go`, `docs/generated/npc-scripts-*.md`, `docs/generated/classic-coverage.md`. Each writes its decision record under `docs/decisions/`.

### sieges-castles

- Goal: castle records (`data/castles.yml`, `mod.CastleKind`) converted from `castle_db.yml`, the extracts `castles.json`, `map_flags.json` and `job_noenter.json`, and the castle registry with migration 00043 and write-through saves. Guild break runs `OnGuildBreak` and clears the owner; `npc:set_guild` and guild badges on NPCs and monsters, with emblem updates. The castle half of `wa.castles` (reads, `set_owner`, `set`, `set_guardian`, `get_guardian`, `castle_change`, permission `castles.manage`) and the castle converter rows (`getcastledata`, `setcastledata`, `getcastlename`, `flagemblem`). Proposal 0004 sections 1 to 3, 7, part of 8 and 9.
- Owns: `server/castle/**`, `server/mod/castles.go`, `server/store/migrations/00043_castles.sql`, `server/store/queries/castles.sql`, `server/store/castles.go`.
- Shares: `server/social/**` (guild break hook), `server/gamedata/**`, `server/mod/**` (`permission.go`, jobs `no_enter` field), `server/cmd/waserver/**` (wiring, load before zones), `server/zone/**`, `server/modscript/**`, `server/store/db/**`, `schema/proto/wathena/v1/event.proto` (badge doc only), `schema/gen/**`, `tools/convert/npcguild.go`, `reference/rathena/extracts/**`, `reference/rathena/README.md`, and the fork's `~/Documents/GitHub/rathena/tools/wathena/extract.py` (outside the repo).
- Depends on: proposal-sieges (contract `sieges`: the frozen interface).
- Size: M.

### gvg-rules

- Goal: the GvG map flags and every battle rule they change, as proposal 0004 section 5: flags with effects and `maps.control`, the zone fields `gvg`, `gvgGround`, `vs` and `mapType` in place of `Zone.GvG`, player targeting on `vs` maps, damage rates and their exceptions, the Emperium's hit rules, flee penalty, `skill_nocast_db` and `item_noequip` on every map, `restricted` zones, revival, knockback, movement skills, cast cancel, 1 s respawn, guild action blocks, Emergency Call's siege bit, the ruleset rules, and `MapRules` on the server. Golden tests against a new `record_gvg` in the fork's recorder, both modes.
- Owns: `server/battle/gvg*.go`, `server/zone/gvg*.go`.
- Shares: `server/zone/**`, `server/battle/**`, `server/status/**`, `server/statuschange/**`, `server/skill/**`, `server/mob/**`, `server/social/**`, `server/rules/**`, `server/mod/**` (ruleset rules), `server/modscript/**`, `schema/proto/wathena/v1/event.proto`, `schema/gen/**`, `client/src/net/**`, `reference/rathena/extracts/**`, `reference/rathena/README.md`, the fork's recorder (`~/Documents/GitHub/rathena/tools/wathena/`, `src/map` recorder hooks).
- Depends on: sieges-castles (hard: castle records set `gvg_castle` and `gvg_te_castle`, and `map_flags.json` comes from its extract); proposal-sieges (contract `sieges`). The siege state input is a zone setter that sieges-war wires; gvg-rules tests set it directly.
- Size: L (high blast radius: every skill, status and target check).

### sieges-war

- Goal: the siege itself: siege state per edition, `OnAgitInit` at boot (castle zones start), `OnAgitStart*` and `OnAgitEnd*` as NPC events and the typed events, restart behavior; guardians (`spawn_guardian`, castle stats with Guard Up, ownership, installed bits) and the Emperium's castle data; `maps.respawn_guild`, `guild:warp`, `maps.can_enter`, `guilds.relation`, `npc:wake()`; the siege converter rows; admin endpoints, staff permissions `world.sieges` and `world.castles`, audit rows and GM commands; the e2e bot siege; the `siege` wabots scenario with its measurements. Sections 4, 6, rest of 8 and 9, 11, 13, 14's e2e.
- Owns: `server/castle/siege*.go`, `server/castle/guardian*.go`, `tools/convert/npcsiege*.go`, `e2e/siege_test.go`.
- Shares: `server/castle/**`, `server/zone/**`, `server/world/**`, `server/modscript/**`, `server/mod/**`, `server/gamedata/**`, `server/mob/**`, `server/admin/**`, `server/adminworld/**`, `server/staff/**`, `server/cmd/waserver/**`, `schema/proto/wathena/admin/**`, `schema/gen/**`, `tools/convert/npcguild.go`, `tools/convert/npcgroup3.go`, `tools/wabots/**`, `tools/botclient/**`, `e2e/**`, `docs/generated/**`.
- Depends on: gvg-rules (hard: guardians, the Emperium and the e2e fight need the battle rules); sieges-castles (hard: ownership and guardian bits live in the registry); npc-api-timers (hard: `npc:wake()` ends `timers.wait`).
- Size: L (M without the performance scenario; the measurement may pull decision 0055 in).

### sieges-ui

- Goal: what players and staff see: `GuildCastles`, `CastleInfo`, `CastleInfoRequest`, `CastleTeleport` and its result on the server (cost from the siege state); the client's castle list in the guild window with the teleport button and the economy and defense view; emblems drawn on flag NPCs and guardians; the attack cursor and click to attack on enemy players while `MapRules.gvg` is set; the admin World screen's castles and sieges with start, end and owner forms.
- Owns: `client/src/viewer/castle*.ts`, `client/src/admin/castles*.ts`, `server/zone/castleteleport*.go`.
- Shares: `client/src/viewer/**` (`guild.ts`, `emblems.ts`, `playerMenu.ts`, input), `client/src/admin/**` (`world.ts`), `client/src/net/**`, `schema/proto/**`, `schema/gen/**`, `server/zone/**`, `server/castle/**`, `server/gateway/**`.
- Depends on: sieges-war (hard: siege state for the teleport cost and the admin endpoints the screen calls); gvg-rules (hard: `MapRules` and player targeting).
- Size: M.

### sieges-content

- Goal: rAthena's castle and siege scripts convert and run in both modes: `npc/guild`, `npc/guild2`, renewal `npc/re/guild3` and `npc/re/guild` (`invest_main.txt`, `mission_main.txt`), `npc/re/quests/woe_te`, and the castle warps (`npc/warps/guildcastles.txt` and the mode copies), including `linkflag`, `returnflag`, `f_flag_woe_te`, `f_invest_status`, the controllers and the SE stones and barricades. e2e of one FE, one SE and one TE castle from the controller's schedule on the dev clock to a capture and the next day's treasure.
- Owns: none of its own beyond its decision record; converted output goes through the shared converter files.
- Shares: `tools/convert/**`, `docs/generated/**`, `reference/rathena/extracts/**`, `e2e/**`, `server/zone/**` (fixes the scripts reveal).
- Depends on: sieges-war (hard: castle scripts run on the real siege rules, not a stub); extract-folders (hard: the guild folders and the mapflag lines must be in the extract); sieges-ui (soft: the e2e checks flag emblems through the client view only if it is built).
- Size: M (outline node; 1,267 renewal script NPC lines).

## Proposed learnings

- rAthena's castle labels are NPC events sent to every NPC; keep them as labels in converted scripts | the zone start rule and case-insensitive match depend on it | tools/convert, server/zone | yes
- GvG rules read zone fields (`gvg`, `gvgGround`, `vs`, `mapType`), never flags in hot paths | ticks stay within budget | server/zone, server/battle | yes

## Verification

- Design only. No builds, tests or `scripts/check.sh` runs, as the node asks.
- Checked the proposal and contract for em-dashes: none.

## Size

- Estimated M, actual M to L: the battle rules alone needed a full survey of rAthena's GvG checks.
