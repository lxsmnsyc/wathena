# Outcome: proposal-companions

## Built

- docs/proposals/0005-companions.md: design proposal 0005 (Problem, Change sections 1 to 17, Breaks, Alternatives, 18 questions for review).
- docs/roadmaps/v0.2-parity/contracts/companions.md: Interface, Behavior, Stub and Contract tests filled from the proposal. Still `status: draft`, version 1, for the orchestrator to freeze after the maintainer accepts.
- Numbers: proposal 0005. Migrations named 00044 (`character_followers`), 00045 (`pets`), 00046 (`character_homunculi`), 00047 (`character_mercenary_guilds`), after 0004's 00043; numbers are taken in merge order. No decision or review decision number used. Protocol field numbers start at 121 for events and move with merge order.
- Accepted: the maintainer accepted the recommendations on 2026-10-27 except decisions 8 and 13. Homunculus S needs the Geneticist (a third class, not built), so homunculus-s is a v0.3.0 outline that depends on third-classes, and `homunculus_s_max_level` is decided there. The questions became "Decisions in review"; the contract is frozen at version 1.
- No builds, tests or Docker. Read-only greps and reads of the repo and of the rAthena fork (branch wathena-reference, untouched).

## Deviations

- None from the brief. The proposal proposes a split (homunculus-s) and path changes for the briefs; the orchestrator owns the briefs.

## Stubs left

- None. companion-units builds the real base; its tests register a test kind that is not shipped.

## Gotchas for each downstream node

For companion-units:

- **Name clash.** `server/companion` and `server/zone/companions.go` already use "companion" for falcon and mount records (option bits). The new units are "followers". Do not rename the old code.
- **Owned paths look short.** Besides `server/companion/**` it needs `server/character/**`, `server/progression/**` (damage log flag), `server/battle/**` and `server/mob/**` (`CalcMisc` moves out of `mob/stats.go`; `mob.calcMisc` is monster-only), `server/store/*.go`, `server/mod/**`, `server/gamedata/**`, `server/rules/**`, `server/admin/**`, `server/staff/**`, `schema/proto/wathena/admin/**`, `client/src/viewer/follower*`, `tools/wabots/**`, `tools/botclient/**`.
- **Walk state extraction.** `walkToBL`, `mobUpdateChase`, `mobStepped`, `stopWalking`, `unitAttack` and `unitUnattackable` use `e.monster` fields. `mobNextCell` also runs monster skills: keep that part monster-only. Embedding keeps `m.chaseTo` valid. `mob_ai.json` goldens must not change.
- **Players move between zones by handoff** (server/zone/warp.go, `depart` and `arrive`). Put the follower in `character.Character` and make it in `arrive` or `join`; do not give it a handoff of its own. `removeMap` for the follower in `depart`.
- **Entity set side effects.** `entitySet.add` counts monsters and `mobs` only when `e.monster != nil`; keep followers out. `isEnemy` (query.go:183) has special cases for summons; add followers before them. `queryKind` must leave pets out of enemy queries.
- **Damage log and exp.** `progression.KillWith` returns shares by char ID. A homunculus's exp is 10 percent of the owner's share in renewal (the owner must have taken part) and the full share only for its own damage entry in pre-renewal.
- **No player SP for followers.** Skill dispatch reads `p.sp` for players. A follower's SP is `follower.sp`; cooldowns need their own map.
- **Performance.** Crowd is already over budget at 250 in view (decision 0050) and decision 0055 is not built. Measure the `companions` scenario early.
- **The base's e2e needs a saved kind.** 00044 `character_followers` is for that; mercenaries use it afterwards.

For pets:

- **Egg and pet are one thing.** `item.Extra` gets `PetID`; four item tables get `pet_id`. Eggs the server made before (plain drops) need repair at load. loot.go:213 and the shop path (npc.cpp 2914 in rAthena) must call `pet_create_egg`, not add a plain item.
- **Bonus `Script` runs with no equip check**; `pet_equip_required` only gates `SupportScript`. All 93 renewal and 48 pre-renewal scripts are `if / else if` tier chains. 9 use `petautobonus` (item `autobonus` machinery).
- **Equipment scripts read pets.** 12 renewal items use `getpetinfo(PETINFO_EGGID)` and `(PETINFO_INTIMATE)`. Changing a pet's intimacy or class must recalculate the owner. The converter forbids `rand` in equip contexts for the same reason; allow `player.pet` reads.
- `getpetinfo` returns 0 and "null" without a pet. Pets differ per mode (107 renewal, 57 pre-renewal); the shared `db/pet_db.yml` has no rows.
- The hatched egg's `attribute` is 1; hatched eggs cannot be traded or mailed.
- The map flag `nopetcapture` is new for `maps.set_flag`; add its line to `map_flags.json` (the sieges extract).
- Pet sprites: the GRF importer does not name them yet; the client draws the placeholder.

For homunculus:

- **No server AI in rAthena.** The kRO client runs it. No recording shows when a homunculus acts, so only its effects have goldens. Write each skill's use rule in the decision record.
- **Extracts differ per mode.** `db/homunculus_db.yml` has no rows; use `db/re` (13 entries: 8 base classes plus 5 S) and `db/pre-re` (8). An evolved class shares its base's record (`evo_class`). `exp_homun.yml` also differs per mode.
- **The recorder needs hooks** that build `homun_data` without the char server (`hom_alloc`, `hom_recv_data` go through `intif`). `extract-sync` must be done; the fork's branch is local only (GOAL open question).
- **Growth is random.** Record with `rnd_force_min` and `rnd_force_max`; stats are truncated to tenths (`growth - growth % 10`).
- **The flag `homunculus` sites**: stat weight (`level / 10` added to six stat values), ASPD, DEF and MDEF, `SC_HOMUN_TIME` (30 min, ends with vaporize), the exp share, the db files. Pre-renewal Call Homunculus costs one item and has no time limit.
- `AM_CALLHOMUN` consumes items only when a unit is created (pre-renewal) or per recall rules (renewal, `skill_area_temp`); read skill.cpp 9880 to 9905.
- Intimacy is 0 to 100,000 in the unit and 0 to 1,000 in the client's window. The db's `RequiredIntimacy` (0 to 1000) is multiplied by 100 at load (homunculus.cpp 1926), so the record's `intimacy` field in `data/homunculi.yml` is the db's 0 to 1000 value.
- `HVAN_EXPLOSION` kills the homunculus; its default use rule is never.
- Homunculus S, mutation, `morphembryo`, `hommutate`, `homshuffle` and the 42 `MH_*` skills are for `homunculus-s`, if the maintainer accepts the split.

For mercenaries:

- **Its brief lacks the skills.** 41 skills (`MS_*`, `ML_*`, `MA_*`, `MER_*`) need `server/skill/merc*.go`, goldens and the recorder's `follower_skill`. Add owned paths or split a `mercenary-skills` node.
- **Data**: `db/mercenary_db.yml` has 44 shared records; `db/re` adds 20; `db/pre-re` has none. Class IDs 6017 to 6046 are the guild mercenaries; other IDs serve events and instances.
- **Faith and calls** belong to the character, by guild (archer, spear, sword), not to the unit. Faith caps at 32,767.
- **Contract time** only runs while the owner is online. `mercenary_create` is a no-op when the player has one.
- The renewal copy `npc/re/other/mercenary_rent.txt` converts 0 of 10 NPCs today; the plain copy 4 of 10. Both use `getd` on guild faith arrays and `mercenary_get_faith`, `mercenary_set_faith`.
- A mercenary cannot attack the Emperium (battle.cpp 8063).

## Proposed learnings

- A unit that scripts count must not share the monster type: keep followers off `entity.monster | scripts count and kill monsters | server/zone | yes
- rAthena has no server AI for homunculi and mercenaries: write wAthena's rules and test them as rules, golden only the effects | the kRO client runs that AI | server/zone, server/skill | yes

## Verification

- Design only. No builds, tests or `scripts/check.sh` runs, as the node asks.
- Checked the proposal, contract, log and outcome for em-dashes: none.

## Size

- Estimated M, actual M to L: the three systems needed a full read of pet.cpp, homunculus.cpp and mercenary.cpp and of the zone's AI coupling.
