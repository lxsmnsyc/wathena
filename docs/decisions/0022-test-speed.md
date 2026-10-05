# 0022: Test Speed

- Status: proposed.
- Date: 2026-10-04.
- Code: [internal/testrace](../../internal/testrace), [scripts/check.sh](../../scripts/check.sh), [scripts/fuzz.sh](../../scripts/fuzz.sh), `ci.yml` (removed by [decision 0026](0026-local-checks.md)), the tests of [server/zone](../../server/zone) (`ruleset_test.go`, `skills_test.go`, `inventory_test.go`), [tools/modkit](../../tools/modkit) and the packages listed below.
- Builds on [decision 0017](0017-e2e-and-transfers.md), "Local checks". It changes no game code and no assertion.

## Decision

### Two test passes

The race detector only finds races in code that runs goroutines. It makes code 5 to 20 times slower. Most of the slow tests compare whole reference recordings on one goroutine, so the race detector checked nothing in them.

- The first pass runs `go test` without the race detector on every package but `e2e`.
- The second pass runs `go test -race ./...`. The heavy tests that run on one goroutine skip with `testrace.SkipUnderRace`, which names the reason.
- The `test` step of `scripts/check.sh` and the `test` job of `ci.yml` run both passes, in that order. Each test runs in at least one pass.
- `e2e` runs in the second pass only, as it did before. In a first try it ran in both. Beside the busy first pass, `TestDuplicationWithDatabase` made 1 audit where it needs 5, and failed. Its servers wait on the CPU, and its bots were away during most audits.
- `internal/testrace` holds `Enabled`, a constant set by the `race` build tag, and `SkipUnderRace`. It replaces the `raceEnabled` constants of `server/zone` and `e2e`.
- The concurrency tests keep running under `-race`: the zone loop, the gateway, transfers, social, chat, the store, `e2e` and `botclient`.

The tests that skip under `-race`:

| Package | Tests | Why they need no race detector |
|---|---|---|
| `server/zone` | `TestMixedRulesetsZone`, `TestSkillStatusGolden`, `TestGuildSkillsMatchReference`, `TestMobSkillUseParity`, `TestMobAIParity`, `TestFullRecalculationEveryItem`, `TestViewsMatchFullScan` | They call the zone's step functions directly. The zone loop and its goroutines do not run. |
| `server/skill` | `TestSkillDamage`, `TestMixedRulesetsSkills`, and the 11 tests that load the recorded damage data: `TestShieldBaseDamage`, `TestShieldChainGolden`, `TestShieldChainHitBonus`, `TestSacrificeLuckyDodge`, `TestGrandCrossOnCaster`, `TestGospelDamage`, `TestDefenderRanged`, `TestSpiralPierceGolden`, `TestAutoCounterCritical`, `TestAutoBlitzBeatSplit`, `TestSpheresInDamage` | The package runs no goroutines. |
| `server/battle` | `TestMobDamageRolls`, `TestPreRenewalMobDamageRolls`, `TestPCDamageRolls`, `TestPreRenewalPCDamageRolls` | The package runs no goroutines. The PC tests run one parallel subtest per job only to use every CPU. |
| `server/progression` | `TestMixedRulesetsLevelPenalty`, `TestLevelPenaltyFlags`, `TestPartyExpShareMatchesReference`, `TestPartyExpShareFlag`, `TestGuildTaxOnKillsMatchesReference`, `TestKillExp`, `TestPenaltyGaps` | The package runs no goroutines. |
| `server/status` | `TestMixedRulesetsStatus`, `TestRecordedPlayers` | The package runs no goroutines. |
| `server/gamedata` | `TestRandomOptionParity`, `TestSampledMixedRulesets`, `TestCoreJobRecordsMatchRAthena` | The package runs no goroutines. |
| `server/cmd/waserver` | `TestLoadSampleMod` | Loading core and the mods runs on one goroutine. The server's goroutines start in `main`. |
| `tools/convert` | `TestRandomOptions`, `TestGolden`, `TestEquipmentParity`, `TestHealParity`, `TestRunConvertedScripts`, `TestRunConvertedNPCs`, `TestRunNPCEvents` | The package runs no goroutines. |
| `tools/modkit` | Every test but `TestMinisign` | The test worlds are headless zones on one goroutine. |

A test that starts goroutines, or runs the zone loop, must not call `SkipUnderRace`.

### Shared test data

- `TestMixedRulesetsZone` took 1,061 seconds under `-race`, 62% of `server/zone`. For each recorded case, `zoneRules` parsed `skills.json` and `job_stats.json` of the other mode again. The test now reads them once per test binary and makes the same 1,899 case parts. It takes 2 seconds without `-race` and 6.5 seconds with it, for two mixes per base. Before, `-race` ran one mix per base.
- `testSkills` in `server/zone` parses a mode's skills once. Each call returns new copies of the skills (`Def.Copy`) and the trees, so a test can change them.
- `server/zone` loads core once per mode for every test (`testCores`). `tools/convert`'s `TestRandomOptions` uses the package's shared core.
- `tools/modkit` already loaded its environment once. Each test loads its own mod over it, which merges all of core's items again. That load is what made each test slow under `-race`, and the tests now skip there.

### Fuzz tests

- `fuzz.sh` finds the fuzz tests in the test files that `go list` names. Before, it ran `go test -list` on all 64 packages, which compiled each one.
- It runs 4 fuzz tests at once (`FUZZ_JOBS`). Each gets `-parallel` of the CPU count divided by 4. Each test still fuzzes for 10 seconds.

## Results

Measured on 2026-10-04 on an Apple M2 with 8 cores, each run alone.

`go test -race -count=1 -json ./server/zone ./tools/modkit ./server/gamedata ./server/cmd/waserver`:

| Package | Before | After |
|---|---|---|
| `server/zone` | 1,348 s | 82 s |
| `tools/modkit` | 430 s | 2 s |
| `server/gamedata` | 124 s | 9 s |
| `server/cmd/waserver` | 71 s | 11 s |
| Wall time | 1,372 s | 85 s |

The same packages without `-race`, where every test runs, take about 45 seconds.

`scripts/check.sh` steps:

| Step | Before | After |
|---|---|---|
| `test` | 1,469 s | 433 s |
| `fuzz` | 411 s | 87 s |
| `mods` | 77 s | 70 s |
| `lua` | 61 s | 36 s |
| `loadtest` | 49 s | 48 s |
| `tinygo` | 35 s | 36 s |
| `vet`, `schema`, `queries`, `lint`, `security`, `client` | 45 s | 23 s |
| All steps | 2,152 s | 736 s |

- Both runs used `GOFLAGS=-count=1`, so no test result came from the cache. The build cache was warm in the second run, which explains most of the change in `vet`, `lint` and `lua`.
- In the `test` step, the slowest package of the first pass is `server/zone` at 35 seconds. `e2e` bounds the `-race` pass at 374 seconds, and `server/zone` takes 200 seconds there.

## Other choices

- `t.Parallel()` in `server/zone`. Its tests share package caches and package variables, and the package takes about 25 seconds without `-race`, so it is left as is.
- `t.Parallel()` in `tools/modkit`. It cut the package from 18 to 9 seconds without `-race`, and a run with the race detector found no race between the tests. In the full first pass, `TestDesperado` then failed: its Lua state could not be made. The likely cause is the 1 second wall-clock limit of a Lua call, with 11 Lua states made at once beside the other packages. So the tests run one at a time.
- Caching the core merge of `gamedata.LoadMods` for `tools/modkit`. It would change game code to speed up tests, and the tests skip under `-race` anyway.
