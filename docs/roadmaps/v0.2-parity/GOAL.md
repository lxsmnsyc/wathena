---
milestones: [v0.2.0, modding, v0.3.0, later]
merge:
  mode: maintainer
branches:
---

# wAthena roadmap: parity with rAthena, then modding

## Outcome

v0.2.0 is tagged with 1:1 parity with rAthena (strategy.md section 14.2a) for everything up to transcendent classes, plus instances, sieges, pets, homunculus, mercenaries and the small systems (chat rooms, marriage, achievements, navigation, stylist, bank, refine and enchant windows). Every in-scope NPC and item script converts and runs in both rulesets, except what a decision refuses on purpose. Classic mode is supported, not experimental. The modding ecosystem (strategy phase 9) follows, then v0.3.0 (third classes, battlegrounds, tier 3 systems).

## Acceptance

- `go test ./tools/convert` passes and the regenerated reports in docs/generated show every in-scope row converted or refused with a reason.
- Parity numbers in server/README.md match rAthena in both modes.
- Converted NPC scripts behave as in rAthena: the behavior parity report (npc-behavior-harness) shows no unexplained difference (maintainer, 2026-10-26).
- `scripts/check.sh all` passes on the tagged commit.

## Priorities

- must: needed to tag v0.2.0.
- should: improves v0.2.0 or starts the modding milestone; v0.2.0 can ship without it.
- could: worth doing when nothing must is ready.
- later: phase 10 items, parked.

## Constraints

- **Strictly sequential.** Run check-hygiene first: it stops the disk leak every later check run would add to. One node, one worker, one build at a time. No git worktrees: the worker runs in the main checkout on the branch the orchestrator names (maintainer, 2026-10-27). A build or check can use more than 20 GB of disk. Run `plan.py` with `--max-parallel 1`. Read-only research agents are fine.
- Keep at least 10 GB of disk free. Run `docker volume prune` after check runs.
- Changes to the protocol, the public Lua API, the map format, the seed spec or the database schema need a design proposal first (docs/proposals/README.md). Small API additions go in as amendments to proposal 0002.
- Commits are signed off (`git commit -s`) and use Conventional Commits. No CI (decision 0026); checks run locally.
- Golden and parity tests are the reference; no system ships without them.

## Non-goals

- Third, fourth and expanded classes, battlegrounds and tier 3 systems in v0.2.0 (they are v0.3.0).
- Raw SQL script commands, `atcommand` and `setbattleflag` (refused by decisions).
- A project-run server, published binaries or images (2026-10-20).

## Verification

- Fast check: `scripts/check.sh quick` (vet, unit, lint), before each commit.
- Full check: `scripts/check.sh` before a PR; `scripts/check.sh all` (adds compose and browsers) at the release. `e2e` (up to 90 minutes) and `economy` (10 minutes) on demand. Needs Docker for `postgres:18`.
- Reports: `go run ./tools/wactl convert <npcs|items|monsters|quests|random-options|classic> --mode <renewal|pre-renewal> --report docs/generated/<file>`. `go test ./tools/convert` fails on stale reports.

## Numbered resources

Nodes run one at a time, so each worker takes the next free number when it starts and notes it in its outcome:

- Decision records: docs/decisions/NNNN-*.md, next 0067.
- Review decisions (in proposal 0002): next 63.
- Design proposals: docs/proposals/NNNN-*.md, next 0007 (0003 to 0006 are written).
- Migrations: server/store/migrations/NNNNN_*.sql, next 00043.
- `schema.Version` is 1; `release-v0.2.0` raises it.

## Hot files

spec.go, prelude.lua, the generated Lua docs, proposal 0002, rathena.lua, npcexpr.go and the coverage reports are touched by most nodes. They are not split because nodes run in sequence: each node rebases on main, edits them, and regenerates the generated files before its PR.

## Sources

- docs/strategy.md: "Where work stands", sections 14.2a, 15 (phases 9 and 10), 16, 17.
- docs/proposals/0002-lua-api.md "Next steps" 1-45; docs/proposals/0002b-npc-api.md.
- docs/decisions 0040-0066; docs/security/review-2026-10.md; docs/releases/v0.1.0.md.
- docs/generated coverage reports at commit 38d3fb1.

## Coverage

Proposal 0002 next steps:

- 1, 3, 6-10, 20, 21, 23, 25-27 -> built (no open items).
- 2 (`wa.storage`, `wa.exports`) -> inbox: no script needs them yet.
- 4, 5 (`wa.testing` status, `respawn_milliseconds`) -> review-world-gen.
- 11 (dragon, Madogear, warg) -> third-classes.
- 12-16 (converter "needed for the rest") -> review-npc-api, then converter-commands; siege parts -> sieges-war.
- 17 (event rewards, map type) -> review-world-gen.
- 18 (expand read without permission) -> review-npc-api; voucher window -> ui-windows; `getbattleflag` -> dropped: settings, not rules (0002b B10).
- 19 (unused permission names) -> review-ops-security.
- 22 -> built (settled by step 27).
- 24 (sounds, map folders, mods' maps, map fields) -> mod-assets-gaps; skill icons and effects, monster sounds -> built in decision 0035.
- 28-33 -> review-world-gen.
- 34-37 -> review-ops-security, then security-fixes.
- 38 -> review-npc-api.
- 39 (long string compare, Lua CPU limit) -> review-npc-api; review decision 60 moved the string cost to lua-cost-accounting and refused the thread CPU limit.
- 40 -> converter-names, converter-commands, npc-api-timers; `OnInterIfInitOnce` -> review-npc-api.
- 41-45 -> review-npc-api, then npc-api-events and npc-api-timers; `maps.can_enter` -> sieges-war; HD and Enriched refine costs -> ui-windows; `active_transform` -> item-effects; warg -> third-classes.

Proposal 0002b:

- B1-B10 built (decisions 0062-0066); leftovers as in steps 41-45 above.
- Category C: instances -> proposal-instances, instances-core, instances-content; sieges -> proposal-sieges, sieges-*; battlegrounds -> battlegrounds; chat rooms, navigation, marriage, achievements, stylist, bank, enchant and reform windows -> proposal-small-systems, chat-rooms, marriage, navigation, achievements, ui-windows, item-windows (adoption -> tier3-systems, proposal 0006 decision 1); homunculus, mercenaries -> proposal-companions, companion-units, homunculus, mercenaries (pets -> pets; Homunculus S -> homunculus-s in v0.3.0, proposal 0005 decision 8); reputation -> tier3-systems; auctions -> dropped (strategy 11.3, replaced by vending search and mail).
- Category D (`query_sql`, `setbattleflag`, `atcommand`) -> dropped: refused by decisions.

strategy.md:

- "Open review items" -> review-npc-api, review-world-gen, review-ops-security (each line mapped in those briefs).
- "Next" 1 -> review-world-gen; "Next" 2 (phase 8) -> built: v0.1.0 tagged 2026-10-20.
- Known issues: Go and gosec, check.sh volumes -> check-hygiene; disk space -> Constraints; kRO client copy -> inbox; no CI -> dropped (decision 0026).
- Signing and SBOM (phase 8 step 1) -> signed-releases.
- Phase 9 items -> mod-tooling, signed-releases, api-reference-site, client-data-template, mod-index, modding-exit.
- Phase 10: 1 -> pre-renewal-parity; 2 -> pets, homunculus, sieges-* (and the rest of tier 2 -> tier3-systems); 3 -> third-classes; 4 -> extract-folders and the converter nodes; 5 -> distributed-mode; 6 -> client-plugin-api; 7 -> headless-client; 8 -> other-clients; 9 -> js-ts-mods.
- Section 17: JS/TS -> js-ts-mods; Lua API redesign -> dropped: API 1.0 shipped; one state per mod and crash recovery -> lua-state-per-mod.
- "Left:" lines of phases 2-7 that are not above (Safari and amd64 runs, live economy dashboards, guild trial dungeon, quest art, map cache on disk, touch input extras, Prometheus measurements) -> inbox.

Parity gaps (docs/generated, 2026-10-26):

- Missing folders -> extract-folders. Converter naming and parsing -> converter-names. Remaining commands -> converter-commands, npc-api-events, npc-api-timers.
- Item scripts -> item-effects, item-skill-use, bonus-damage-taken; `pet`, `mercenary_create` -> pets, mercenaries; `vip_status`, Laphine -> tier3-systems; `item_reform`, `item_enchant` -> ui-windows; `makerune` -> third-classes.
- Window and crafting skills -> window-skills. Monster gaps -> monster-gaps. Skill goldens of expanded classes -> expanded-class-goldens. Pre-renewal damage comparison -> pre-renewal-parity.
- Extracts made with functions outside the fork -> extract-sync (the fork has them all since 39de3053d; the README is stale).
- Behavior parity (maintainer, 2026-10-26) -> npc-behavior-spike, npc-behavior-harness.

## Open questions

- The fork's `wathena-reference` branch (~/Documents/GitHub/rathena) is not on its remote; the local checkout is the only copy. Push it? (maintainer's call; extract-sync reports it)

## Gates

- Every review and proposal node is gated: the maintainer settles it before dependents start.
- security-review-v0.2 and release-v0.2.0 are gated.
- Every PR waits for the maintainer to merge (`merge.mode: maintainer`).
