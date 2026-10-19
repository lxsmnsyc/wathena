# 0053: The Skill Command

- Status: proposed.
- Date: 2026-10-19.
- Code: [server/modscript](../../server/modscript) (`jobs.go`, `runtime.go`, `prelude.lua`, `luaapi/spec.go`, `giveskill_test.go`), [server/mod](../../server/mod) (`permission.go`), [server/zone](../../server/zone) (`giveskill.go`, `skills.go`, `stats.go`, `statuses.go`, `equipment.go`, `zone.go`, `giveskill_test.go`), [tools/convert](../../tools/convert) (`npcexpr.go`, `npc_skill_test.go`), [e2e](../../e2e) (`kro_test.go`).
- Docs: [mods/README.md](../../mods/README.md) ("Permissions", "Job changes and resets"), [tools/convert/README.md](../../tools/convert/README.md), [server/README.md](../../server/README.md) ("Job changes"), [docs/generated/classic-coverage.md](../generated/classic-coverage.md), the NPC coverage reports and the API reference in [docs/generated](../generated).
- Phase 8 step 6 (strategy section 15, "Phase 8"), review decision 57 of [proposal 0002](../proposals/0002-lua-api.md), after [decision 0052](0052-classic-mode-part-b.md).
- Lua API: `player:give_skill(skill, level, kind)` and the permission `player.skills`, as decision 57 approved. No other change. Protocol: none. Settings: none.

## Context

Decision 0052 left the renewal first job changers out: their scripts call rAthena's `skill` command, which grants quest skills. Decision 57 approved `player:give_skill` for it. This record also has the checks that commit aee8533 deferred.

## Decision

### player:give_skill

`player:give_skill(skill, level, kind)` ports `pc_skill` for `BUILDIN(skill)`:

- `kind` is `"permanent"`, `"temporary"` or `"temporary_add"`, rAthena's `ADDSKILL_PERMANENT`, `ADDSKILL_TEMP` and `ADDSKILL_TEMP_ADDLEVEL`. The kind is required. rAthena's default, `ADDSKILL_TEMP`, is the converter's.
- `permanent` sets the learned level in `p.char.Skills`, which is saved. Level 0 removes the skill. It skips the skill tree and the skill points, as rAthena does.
  - The client gets the skill list again, as `clif_addskill` and `clif_deleteskill`.
  - A passive skill computes the stats again, as `pc_skill` calls `status_calc_pc` when `skill_get_inf` is 0.
  - A permanent level replaces a temporary one, as the flag goes back to `SKILL_FLAG_PERMANENT`.
- `temporary` sets the skill's level to the higher of its level and the given one. `temporary_add` adds the given level to it, up to 13 (`MAX_SKILL_LEVEL`). The current level counts learned, item and temporary levels, as `sd->status.skill[idx].lv` does.
  - The levels are kept in the player's `tempSkills`, and are not saved.
  - `recalc` clears them, as `status_calc_pc` starts with `pc_calc_skilltree`. That is a change of equipment, a level up, a job change, a passive skill, a map change and the other full recalculations.
  - A status's own recalculation keeps them, as rAthena's `status_calc_bl` does not recalculate skills. Only a status with `SCB_BASE` clears them.
  - The client gets nothing, and the stats are not computed again, as `pc_skill` sends nothing for these kinds. `skillLevel` reads them, so casting uses them. The client's skill list does not show them, as it does not show item skills.
- It returns false for a skill the skill data does not have, a guild skill, and a level above 13, where `pc_skill` logs an error and returns false. A negative level, a skill ID of 0 or less, and another kind raise an error.

### The permission

No existing permission fits. `player.change_job` changes a job, and `player.status` starts statuses. So `player:give_skill` needs a new permission, `player.skills`, which needs the owner's approval and is flagged: it gives skills outside the skill tree without skill points, which changes the game for everyone. Core, a built-in mod, holds every permission, so converted NPCs need no config and log no warning.

### The converter

`skill` and `addtoskill` convert to `player:give_skill`:

- `SKILL_PERM` (0) is `"permanent"`, `SKILL_TEMP` (1) `"temporary"`, and `SKILL_TEMPLEVEL` (2) `"temporary_add"`. Without a flag, `skill` is `"temporary"` and `addtoskill` `"temporary_add"`.
- A skill by name or by a fixed ID converts. A computed skill or flag does not.
- `SKILL_PERM_GRANT` (3) does not convert: decision 57 has three kinds. One renewal script uses it.

Coverage, from the reports, before and after:

| Mode | NPCs | Before | After |
|---|---|---:|---:|
| renewal | NPC lines placed, duplicates too, of 15,337 | 4,957 | 4,982 |
| renewal | Script NPCs placed, of 7,067 | 2,915 | 2,938 |
| renewal | Job change NPCs placed, of 710 | 259 | 264 |
| renewal | Scripts with `skill`, of 31: placed | 0 | 23 |
| pre-renewal | NPC lines placed, duplicates too, of 8,390 | 3,065 | 3,091 |
| pre-renewal | Script NPCs placed, of 3,754 | 1,882 | 1,906 |
| pre-renewal | Job change NPCs placed, of 315 | 128 | 133 |
| pre-renewal | Scripts with `skill`, of 32: placed | 0 | 24 |

The renewal first job changers of `npc/re/jobs/1-1`:

| NPC | Map | Before | After |
|---|---|---|---|
| Swordman#swd | izlude_in | not placed: skill | placed, partly |
| Mage Guildsman#mage | geffen_in | not placed: skill | placed, partly |
| Archer Guildsman#archer | payon_in02 | not placed: skill | placed, partly |
| Cleric#aco | prt_church | not placed: skill | placed, partly |
| Merchant#mer | alberta_in | not placed: skill | placed, partly |
| Thief Guildsman#thief | moc_prydb1 | placed, partly | placed, partly |
| Thief Guide#thief | moc_prydb1 | not placed: skill | not placed: jobname |

- "Partly" is `Job_Change`'s transcendent and baby branch, as in decision 0052. A Novice's change converts fully.
- The Thief Guide gives the Thief test and sets `q_job_thief`, so the Thief change is still not reachable. It now stops at `jobname`.

`TestConvertedFirstJobChange` runs the converted Swordman of Izlude: a Novice at job level 10 with Basic Skill 9 becomes a Swordman through `Job_Change`, and a High Novice who was a Lord Knight becomes a High Swordman with Moving HP Recovery, Fatal Blow and Auto Berserk, and loses Trick Dead.

## Consequences

- Five more first job changers run in classic worlds. The Job Master of Prontera stays for the others.
- Mods that need quest skills can give them, with a flagged permission the owner approves.
- A temporary level from an NPC is gone at the next full recalculation, as in rAthena, and the client does not list it.

## The deferred checks

Commit aee8533 left these checks for later. They ran on 2026-10-19 on the Apple M2 with 8 GB of RAM, with about 20 GB of macOS swap in use, against PostgreSQL 18 on tmpfs:

| Check | Result |
|---|---|
| `scripts/check.sh quick`, first pass | vet and lint passed. unit failed in 6 packages: `server/account`, `server/admin`, `server/adminworld` and `server/store` on migrations that hit their context deadline, and `tools/convert` and `tools/modkit` on the prelude's wall-clock limit. All 6 passed when run again with `go test -p 1`. |
| `scripts/check.sh quick`, second pass | After the kRO checks, with less load: vet and lint passed. unit failed only in `TestRunConvertedNPCs/pre-renewal`, on the prelude's wall-clock limit. `tools/convert` passed alone, and under `-race`. |
| `go test -race -timeout 45m ./server/zone` | Passed in 347 s. |
| `go test -race ./tools/convert ./reference/rathena` | Both passed, in 164 s and 4 s. |
| `go test -race ./e2e` with PostgreSQL | Failed in 3 of its tests, in 64 minutes. `TestMuteBlocksChat` hit a migration deadline. `TestDuplicationRestoresWithDatabase` and `TestPhase5ExitCheck` timed out at start: the server converted the scripts in 1m51s and 2m42s, against about 1.6 s on an idle machine. Run alone with `-race`, `TestMuteBlocksChat` passed, and the other two failed on answers later than 3 s, with a tick p99 of 671 ms. Run without `-race`, `TestPhase5ExitCheck` passed, and so did `TestDuplicationRestoresWithDatabase` twice. One run without `-race` failed its audit during the moves: Jellopy (909) counted 241 against 240 at the start, while a bot's login timed out. |
| The full kRO asset build | Completed in 56m46s with `--fast`: 112,792 assets, 988 maps, 2.8 GB on disk. Two files were rejected: an effect with a bad frame count and a PNG with a bad checksum. Free disk fell from 11 GB to 5.6 GB as swap grew. |
| `TestClassicKRO` on the full build | The first run failed: the bot's `!zeny` got no answer, and 63 zones lost their Lua state to the prelude's wall-clock limit while 988 maps started. The second run passed in 365 s, with the Swordman step below. |
| The `kro` browser check on the full build | Passed in Chromium: `map/prontera` loaded 325 files, 8.0 MB sent and 13.2 MB decoded, in 13.2 s. Three walks, the Man from Morocc's dialog, and no page or server errors. |

`TestClassicKRO` has a new step. A second Novice starts in Prontera, learns Basic Skill 9, walks through prt_fild08 and Izlude into `izlude_in`, and talks to the converted Swordman. On the full build it took the warps prontera 156,22, prt_fild08 371,212 and izlude 52,172, and became a Swordman at job level 1.

The build used `--fast`, a low Brotli level, since the two builds of decision 0052 stopped at the 3 GB floor. The files are the same, with weaker compression. The scratch build was deleted after the checks.

## Review items

- The audit failure of `TestDuplicationRestoresWithDatabase` above: one Jellopy more than at the start, in a full audit during the moves, under heavy load. It did not repeat in two more runs. It needs a look before the release.
- The prelude's wall-clock limit ends zones' Lua states when the machine stalls, as strategy's known issues say. On a loaded machine with all 988 maps it hit 63 zones.

- `SKILL_PERM_GRANT` (`ADDSKILL_PERMANENT_GRANTED`), a permanent skill that survives job changes and resets. A fourth kind is a Lua API addition.
- `jobname`, which blocks the Thief Guide and so the Thief job change.
- Temporary levels are not in the client's skill list, and item skills are not either. rAthena shows item skills after `status_calc_pc`.
- `player:get_skill_level` reads learned levels only. rAthena's `getskilllv` reads `pc_checkskill`, which counts item and temporary levels.
