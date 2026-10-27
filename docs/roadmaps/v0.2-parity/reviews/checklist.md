# Blindspot checklist (2026-10-26)

- **Data and migrations**: instances-core, sieges-castles, pets and the other companion nodes add tables. Migration numbers are taken in merge order (GOAL.md). Restores replay migrations, so each must be idempotent.
- **Compatibility**: saved worlds from v0.1.0 must load after every migration. Protocol changes raise `schema.Version` once, in release-v0.2.0. Lua API additions keep API 1.0 compatible (amendments only). Mods written against v0.1.0 keep working; the modding milestone checks outside mods.
- **Auth and permissions**: new Lua calls that act across zones or on other players need permission names, settled in review-npc-api and the proposals. Siege and instance staff controls need audit rows.
- **Failure modes**: instance timeouts, a siege across a restart, a remote call to a zone that has not started (review-npc-api item). Each proposal must state restart behavior.
- **Concurrency**: cross-zone calls (npc-api-events), instance zones that start and stop at run time (instances-core), siege ownership changes during a fight (sieges-war). Covered by the race checks in `scripts/check.sh test`.
- **Limits**: instance count per world and per party, memory per instance zone; the proposal sets them. Load tests at release.
- **Security**: security-fixes for known issues, security-review-v0.2 for the new surface, hour-long fuzz runs.
- **Observability**: instance and siege counts on the admin World screen; part of the proposals.
- **Performance**: tick budget of section 3.8; release-v0.2.0 reruns the load tests. Instance zones are new goroutines; proposal 0003 must give a budget.
- **Testing**: golden and parity tests per system; e2e for instances, sieges and companions; reports checked by `go test ./tools/convert`. Nodes run one at a time, so check runs never share a machine.
- **Developer experience**: generated Lua docs regenerate per node; proposals document new API.
- **Rollout**: classic mode moves from experimental to supported only in release-v0.2.0. New systems can be turned off by ruleset flag where rAthena has one (homunculus, sieges); proposals name the flags.
- **Cleanup**: `variables.instance_*` refusal goes away in instances-core. The pre-renewal warning goes in pre-renewal-parity. docs/strategy.md status sections are replaced by a link to this roadmap.
- **Coverage**: GOAL.md "Coverage" maps every step of proposal 0002, 0002b, strategy.md and the reports.
- **Dependencies**: no new libraries planned. extract-sync depends on the rAthena fork outside this repository.
