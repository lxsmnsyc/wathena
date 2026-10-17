# 0046: The Economy Simulator and the Phase 7 Exit Check

- Status: proposed.
- Date: 2026-10-18.
- Code: [server/economy](../../server/economy) (`config.go`, `player.go`, `sim.go`, `targets.go`, `core/data/economy.yml`, `core/data/economy_targets.yml`), [server/gamedata/economy.go](../../server/gamedata/economy.go), [server/spawns](../../server/spawns) (`select.go`, `plan.go`), [server/zone](../../server/zone) (`rolls.go`, `scripts.go`), [server/cmd/waserver](../../server/cmd/waserver) (`main.go`, `config.go`, `worldchange.go`), [tools/wactl](../../tools/wactl) (`sim.go`, `layout.go`, `main.go`), [e2e/economy_test.go](../../e2e/economy_test.go), [scripts/check.sh](../../scripts/check.sh), [client/scripts](../../client/scripts) (`browser-check.ts`, `exit-check.ts`, `quests-check.ts`).
- Phase 7 step 8 (strategy section 15, "Phase 7 in full"), strategy sections 8.11, "Rolled drops", and 8.13.
- Lua API: none. Protocol: none. Settings: `spawns.economy` and `procedural.drop_roll_levels`.

## Context

Section 8.13 asks for a simulator of each field's exp and zeny per hour, target rates per band, generator checks that keep every field within 30% of its band's target, and bot runs that agree with the simulator within 20%. Section 8.11 asks for better rolled drops in higher bands. The phase 7 exit criteria also ask for a test of the public seed, and for proof of the other criteria. The open questions were:

- What the "simple player model" of section 8.13 is, so that a real bot can play it.
- Where the targets come from.
- How the generator brings a field that misses back into range.
- How bot runs measure exp and zeny well enough to compare within 20%.

## Decision

### The model player

`server/economy/core/data/economy.yml` holds one model player per level, which the simulator fights with and the bot runs play:

- A Novice to level 10, a Swordman to level 49 and a Knight from level 50. The job level is the base level less the job's first level, plus 1, up to the job's cap.
- The status points of the level go to STR, VIT, DEX and AGI by the weights 5, 4, 3 and 2: each point goes to the stat with the lowest value per weight.
- The gear of a town's dealers, by level: from a Knife and a Cotton Shirt to a Zweihander and Plate Armor, refined to +4 from level 50 and to +7 from level 70. No cards or skills.
- A potion of the level, Red to White, for the supplies.

The model is a plain melee player. It is enough to compare fields and worlds with each other, which is what the checks do. It is weak at high levels: from about level 70 it spends far more on potions than it earns. The first bot runs found a model of weights 5, 3, 3 and 3 without refines dying 15 times in 12 minutes at level 77, so the VIT weight and the refines went up until the bots lived as the simulator assumes.

### The simulator

`server/economy` estimates a field's rates per hour for the model player of a level. The model of the hunt:

- The player meets monsters as their counts share the field. A line cannot give more kills than its monsters come back, count times an hour over its respawn, and the time a capped line leaves goes to the others.
- Each kill takes the walk to the next monster, the fight, an overhead of 2 seconds and 400 ms for each item that drops. The walk is the side of the square that holds one hunted monster on average, less the melee reach, at the player's speed. The overhead is the search for a monster in sight that the path search reaches, which the bot runs measured at about 2 seconds a kill; it was 300 ms before them.
- The fight is played 32 times with the server's battle formulas (`battle.AttackRules`), the player's battle status from `status.Tables.Calc` with its gear worn, and the monster's from `mob.Calc`, at the attack delays rounded up to the zone's tick. Draws come from a stream seeded by the monster and the level.
- Exp is the kill's exp after the renewal level penalty (`progression.Tables.Kill`). Zeny is each drop's rate after the level penalty and the player's LUK, times its NPC sell price. Supplies are the damage taken per hour over the potion's heal, at its dealer price, without natural regen.
- The player leaves alone monsters that teleport away or heal themselves in a fight (AL_TELEPORT, or AL_HEAL on itself, in a state with a target), since the battle formulas leave monster skills out, and monsters whose fights last over a minute on average. They do not count toward the density it hunts in.
- Everything the checks compare is an integer, so a world gives the same results on every platform.

`wactl sim economy --seed <text> [--config waserver.yml] [--floors]` makes a procedural world in memory, as `wactl world new` does without a database, and prints each field's kills, base and job exp, zeny and supplies per hour against its band's targets. It starts no server.

### Targets

Section 8.13 writes the targets as data. `server/economy/core/data/economy_targets.yml` holds one per band of 10 levels, 1 to 10 up to 91 to 99, the bands the layout's coverage check counts:

- A classic field is a renewal map whose name holds `_fild`, from the spawn extract. Its hunt level is the median level, by count, of its monsters that respawn within a minute, without bosses. 128 field maps give 13, 11, 6, 9, 9, 7, 8, 10, 17 and 4 fields to the ten bands.
- Each band's target is the median of each rate over its classic fields, for the model player of the band's middle level.
- A generated field's target is on the line between the two targets nearest its band's middle level.
- `TestTargets` derives them again and fails when the file differs. `go test -run TestTargets ./server/economy -update` writes it.

| Band | Model | Fields | Base exp/h | Job exp/h | Zeny/h | Supplies/h | Hours per level at the model's level |
|---|---|---|---|---|---|---|---|
| 1-10 | Novice 5 | 13 | 78,561 | 31,252 | 7,656 | 270 | 0.04 |
| 11-20 | Swordman 15 | 11 | 83,040 | 62,789 | 14,683 | 1,310 | 0.09 |
| 21-30 | Swordman 25 | 6 | 94,858 | 76,129 | 15,328 | 5,925 | 0.14 |
| 31-40 | Swordman 35 | 9 | 104,351 | 89,672 | 11,869 | 10,500 | 0.21 |
| 41-50 | Swordman 45 | 9 | 114,183 | 97,845 | 14,195 | 57,960 | 0.29 |
| 51-60 | Knight 55 | 7 | 124,809 | 109,572 | 11,197 | 97,380 | 0.40 |
| 61-70 | Knight 65 | 8 | 147,056 | 144,164 | 13,710 | 112,860 | 0.47 |
| 71-80 | Knight 75 | 10 | 186,577 | 168,952 | 18,209 | 616,800 | 0.77 |
| 81-90 | Knight 85 | 17 | 182,689 | 165,672 | 7,988 | 854,400 | 1.8 |
| 91-99 | Knight 95 | 4 | 126,667 | 120,060 | 6,791 | 1,342,800 | 7.2 |

The leveling time follows the ruleset's exp curve, as section 8.13 asks: it grows with the level, slowly to level 70 and steeply after. The model levels faster than section 8.13's example of an hour per level at level 30, since renewal's exp table is lower there than pre-renewal's.

### Generator checks

`wactl world new` checks every field (section 8.13, step 5). `gamedata.EconomyRater` rates a field's list: each of base exp and zeny per hour more than 30% from the target adds the percent beyond it, and a median monster level more than 10 levels from the band adds 10 for each level beyond. `spawns.SelectChecked` then adjusts a field that misses:

1. `spawns.Balance` makes up to 16 moves, each the one that lowers the miss most: a quarter of a species' monsters to another species of its role, which keeps the role shares of section 8.11, or every species scaled by 7/8 or 9/8, which changes the density within half to twice the first total.
2. When that is not enough, the species are drawn again from the map's secret node's children `economy(1)` to `economy(31)`, each balanced. The draws shift the species' levels by 0, -4, 4, -8 and 8 in turn, and the second half takes monsters of every biome at one weight, for biomes with few monsters at a band. The map keeps its band and biome.
3. The first list that passes stays, or the one that misses least.

Dungeon floors are not checked: section 8.13 and the exit criteria name fields. `waserver` checks the fields of a changed region in the same way. `spawns.economy: false` turns the checks off.

The checks change new worlds' spawn lists, and so the quests drawn from them. The golden tests of earlier steps protect the selection without the checks: `TestGoldenSpawns` and `TestGoldenRotation` plan without a rater, and `TestGoldenContent` makes its worlds with `spawns.economy: false`, so their files did not change. New golden files protect the checked path: `testdata/golden-spawns-economy.json`, which `scripts/check.sh gen` checks on arm64 and on amd64 under Rosetta, and `tools/wactl/testdata/golden-content-economy.json`. No new generator version was needed: maps do not change, a made world keeps its stored spawns, and the setting chooses the selection.

Results in `TestEconomyChecks`, 24 default worlds of 30 fields:

- Without the checks, 367 of the 720 fields miss.
- With them, every field's base exp and zeny per hour is from 70% to 130% of its band's target, and its monsters' median level within 10 levels of its band, in every band from 1-10 to 90-99.
- In 100 default worlds of an earlier model, all 3,000 fields passed.

### Rolled drops by band

Section 8.11 says monsters in higher bands give better roll ranges. On a map of the atlas, each full `procedural.drop_roll_levels` levels of the band past level 1, 30 by default, gives each random roll of a monster's drop one more draw, and the best stays. A band from level 31 draws twice and one from level 91 four times. The roll's range does not change, so values stay valid, and the `item_roll` hooks see the drawn value. Classic worlds and other sources keep one draw. Core's items have no rolls, so this changes no NPC sale price, and the simulator leaves it out.

### Bot runs

`TestEconomyBots` in `e2e`, which `scripts/check.sh economy` runs with `WATHENA_ECONOMY_MINUTES`, 10 by default, plays section 8.13's bot runs:

- A default world that `wactl world new` makes, with PostgreSQL, run by `waserver`. For each of the bands from levels 1, 25, 49 and 73, the field whose band starts nearest, and 3 bots on it.
- Each bot is the model player of the field's band, written in the database: job, levels, stats, the gear worn, and 50 potions, few enough that the weight stays under half. It goes back to the model every segment, 30 seconds below level 30 and 2 minutes above, so it does not outgrow the model. Time between segments does not count.
- It hunts the nearest monster of the field's lines that are not bosses, by the walk to it on the field's cells within the server's path search, leaves alone the monsters the simulator leaves alone, drinks a potion below half HP, gives up a fight after a minute and a target it has not hit for 10 seconds, picks up the drops of its kills, and, with nothing in sight, walks on to a cell the path search reaches.
- The fields have 200 cells a side, which keeps the server small on a machine of 8 GB; the density per cell, which the hunt depends on, is a real world's.
- Exp counts across level ups, from the stats the server sends. Zeny is the NPC sell value of what it picks up.
- The check: base exp per hour within 20% of the simulator's, and zeny per hour within 20%, or, since drops are rolled, within two standard errors of the expected value of the kills the bots made, when that expected value is within 20%.

The results of the last run are below, under "Exit criteria and their proof".

The first runs found the parts of the model that the bots did not play, and the bots and the model changed to agree on each:

- A weight over 90% from 300 potions, which stops attacks. The bots carry 50.
- Targets out of the path search's reach, which wasted 10 seconds each. The bots pick by the walk on the cells.
- Monsters that heal or teleport, which the bots could not finish. Both leave them alone.
- A model too weak at level 77, which died 15 times in 12 minutes. The build gained VIT and the gear refines.
- Wander targets on cells no path reaches, which left bots standing. The bots wander to reachable cells.
- The search for the next monster, about 2 seconds a kill. The overhead went from 300 ms to 2 seconds.

The bot runs also found a server bug. A monster that cast Cloaking or Maximize Power kept the status past its 10 seconds: the status timer charged SP with `charge`, which lets any unit but a player go on, and the status's interval was 0, so the timer ran again at once, forever, with a `status_tick` event each time. The zone hung, its players' saves stopped, and `waserver` grew past 2 GB in seconds until macOS killed it. rAthena takes the SP with `status_damage`, which takes none from a monster, so the status ends at its first timer. The timer now ends it for every unit but a player (`TestMonsterCloakingEnds` in `server/statuschange`). The e2e harness now writes the server's exit status into its log, which showed `signal: killed`.

What still differs: monster skills other than heal and teleport, such as status attacks that stop the player's attacks, aggressive monsters that join a fight, deaths, and a search time from 1 to 9 seconds a kill by field, where the model has one overhead. Runs before the last fix missed by more than 20% at levels 49 and up; the bug below hung zones in several of them.

### The public seed test

A player knows the world's public branch, every node a client derives from it, and the maps and places the server sends. The test shows this predicts neither loot nor spawns:

- Spawns: `TestPublicSeed` in `server/spawns` plans a world's lists from the public branch, its secret child, the public key as a root, the layout node and a field's node, in place of the secret branch, with and without the economy checks. Each gives at most a quarter of the world's lists. Other secrets with the same maps give other lists, no map data names a monster, and no public node gives the rotation or season keys that the game role holds.
- Loot: `TestPublicSeedLoot` in `server/cmd/waserver`. The zones roll drops and rolled values from `runRoot`, a fresh secret at each start that no world node gives, or `--seed`'s text in development. Two starts differ. Drops of 2,000 Poring kills and 2,000 rolled values from each node the player can derive, and from the world's secret branch and root, each used as a zone's stream, match the server's no better than two independent streams do: about 760 of 2,000 kills drop the same items and 19 of 2,000 rolls are equal, by chance alike.

## Touch input

The user froze touch input as built in decision 0034: it stays and will not be extended.

- `CHECKS=touch,phone` runs the touch run of `game-check.ts` and the checks at 375 pixels wide. The default `check:browsers` run, and so `scripts/check.sh browsers`, leaves them out.
- The phase 6 exit check plays all three players on desktops with the mouse by default, and every tier 1 system it checked stays checked: Bob's steps run on a desktop, with a double click and a drag to the hotkey bar in place of the double tap and the long drag. With `touch` in `CHECKS`, Bob and Dora play on the Pixel 7 as before. The quests check's phone page runs only with `touch` too.
- The user's test on a real phone is no longer in "Where work stands".

## Exit criteria and their proof

### The same seed and generator version give the same cell grid on the server and in Chrome, Firefox and Safari, on amd64 and arm64

- `TestGolden`, `TestFieldV2`, `TestDungeonV3`, `TestTownV3` and the other golden tests of `shared/gen` (decisions 0038 to 0040), which `scripts/check.sh gen` runs in Go on darwin/arm64 and on darwin/amd64 under Rosetta, and `tinygo` in TinyGo native and wasmtime. `generator.test.ts` runs `gen.wasm` in Node, and the browsers check `gen` in Chromium, Firefox and WebKit, WMAP and places hashes included.
- What is proven: Go on arm64, Go on amd64 under Rosetta, TinyGo, Node, Chromium, Firefox and WebKit. Not proven: a real amd64 machine, and Safari itself rather than Playwright's WebKit. Chrome is proven through Chromium.

### A generated world with at least 5 towns, 20 fields and 5 dungeons is playable from level 1 to the transcendent level cap

- `TestLevelPath` in `tools/wactl` (new) walks the level path of three default new worlds, as `wactl world new` makes them: 5 towns, 30 fields and 6 dungeons; every map reachable from the home area; each field reachable through maps whose band starts no higher than its own; every level from 1 to 99, the Lord Knight's cap and so the transcendent cap, in the band of fitting fields, and 3 or more fitting fields per band, where fitting means a median monster level within 10 of the band and the economy targets met; a guild per class family with first and second job tests, the Swordman guild in a town of a band from level 20 or lower; the rebirth place in a town of the highest town band.
- `TestProceduralContent` (e2e, decision 0044): a new Novice takes a board quest and passes the Swordman guild's test to become a Swordman, with PostgreSQL. `tools/modkit`'s procedural content test plays a Swordman that passes the second test and becomes a Knight, a High Novice that changes job without a test, and the Valkyrie, which reborns a Knight of level 99. `TestRebirth` in `server/zone` checks the rebirth itself.
- What is proven: the parts of the path, each by a test. Not proven: one character played from level 1 to 99 and through rebirth to 99 again. No bot does that. The other class families' guilds may stand in a town of a higher band, such as levels 41 to 50, so a Novice who wants to be an Archer may have to cross harder fields first. This is a review item.

### A rotating dungeon changes on schedule without staff action

- `TestRotation` (e2e, decision 0045): the dungeon changes as the server clock moves, with PostgreSQL, also after a restart. The rotation package's timing, catch-up and daylight saving tests, and the browser check `rotation`.

### Every generated map is reachable from the home area, and no two towns in a world share a name

- `TestLayoutRules` (20 layouts), `TestLayoutRetries`, `FuzzLayout` and `TestCoverage` (300 default worlds) in `server/layout` check reachability; `TestTownNames` and `TestNaming` check unique names with the chat filter. `TestLevelPath` checks reachability again on made worlds. The e2e test `TestWorldLayout` plays one.

### Removing a rotating dungeon moves its players, save points and memo points to the nearest town

- `TestRotation` (e2e): the player inside goes to the nearest town, and an offline character's save and memo points there follow, in the database. The world change tests of `server/zone` and `server/cmd/waserver`.

### Every level band has at least 3 fields, and every generated field meets the spawn constraints in section 8.11

- `TestCoverage` (300 default worlds), `TestSpawnRules` (12 worlds with the settings), `FuzzSelect`, and `TestEconomyChecks`, which runs the spawn rules on 24 worlds with the economy checks. `TestLevelPath` counts 3 fitting fields per band. The e2e test `TestWorldSpawns` checks a running world's monsters against its stored lists.

### Every field is within 30% of its band's target exp and zeny rates in the simulator, and bot runs agree with the simulator within 20%

- The simulator: `TestEconomyChecks` and `TestLevelPath`, every field of their worlds within 30%. `TestTargets` keeps the targets derived from the classic fields.
- The bot runs: `scripts/check.sh economy`. The last run, 10 minutes of 3 bots on each field:

| Field | Band | Model | Bot minutes | Kills/h, bots and simulator | Base exp/h, bots and simulator | Zeny/h, bots and simulator | Deaths |
|---|---|---|---|---|---|---|---|
| gen:field:0:3 | 1-10 | Novice 5 | 34.7 | 314, 300 | 136,184, 118,153 (115%) | 8,410, 8,826 (95%) | 1 |
| gen:field:2:3 | 25-34 | Swordman 29 | 38.2 | 260, 203 | 140,798, 117,721 (119%) | 14,595, 12,740 (114%) | 0 |
| gen:field:4:2 | 49-58 | Knight 53 | 31.0 | 267, 242 | 125,612, 114,345 (109%) | 29,570, 15,255 (193%) | 1 |
| gen:field:3:0 | 73-82 | Knight 77 | 34.1 | 153, 138 | 170,963, 167,000 (102%) | 12,209, 11,595 (105%) | 0 |

- Base exp is within 20% on every field.
- Zeny is within 20% on three fields. On the band 49-58 field, the bots' drops were worth 193%: the expected value of the kills they made is 18,060 zeny an hour, 118% of the simulator's, and the run's 29,570 is within two standard errors of it (6,702 each), so the check passes it as the spread of rolled drops.
- Job exp is not compared. The Novice of band 1-10 reaches the Novice's cap of job level 10 within a segment, so its job exp stops.
- Earlier runs, before the fixes above, gave base exp from 47% to 119% of the simulator's, most of the misses at levels 49 and up.

### Players cannot predict loot or spawns from public seeds, tested with a known public seed

- `TestPublicSeed` in `server/spawns` and `TestPublicSeedLoot` in `server/cmd/waserver`, above.

## Tests and checks

- `server/economy`: `TestTargets`, `TestModel` (every level from 1 to 99 wears its gear and spends its points), `TestSimulate` and `TestTargetsAt`.
- `server/spawns`: `TestEconomyChecks`, `TestEconomyOff`, `TestBalance`, `TestGoldenEconomySpawns`, and `TestPublicSeed` with the economy checks and the rotation and season keys. `TestResolve` reads `spawns.economy`.
- `server/zone`: `TestDropRollsByBand`.
- `server/cmd/waserver`: `TestPublicSeedLoot`, and `TestSpawnsConfig` with `spawns.economy` and `procedural.drop_roll_levels`.
- `tools/wactl`: `TestLevelPath`, `TestGoldenEconomyContent`, and `TestGoldenContent` with the checks off.
- `server/statuschange`: `TestMonsterCloakingEnds`.
- e2e: `TestEconomyBots`, which skips without `WATHENA_ECONOMY_MINUTES`. `TestProceduralContent` waits up to 2 minutes for the Novice's board hunt, since a world with the checks may give the first town's board a hunt of Moss Porings, which a Novice of level 1 fights with its fists. The harness writes the server's exit status into its log.
- `scripts/check.sh`: the step `economy`, outside the default steps and `all`, and `TestGoldenEconomySpawns` in `gen`.

## Lua API

None. A `spawn_table` hook still changes the checked lists after the checks, and the checks do not run again on its result.

## Left

- The live economy dashboards of section 8.13, step 7: total zeny, zeny made and removed per day, exp per hour by band, the most farmed fields and roll spread.
- Settings for exp, drop and zeny rates and shop multipliers that the simulator reads (section 8.13, step 8). The simulator reads the loaded tables, so the exp and drop rates of the ruleset apply, but the targets assume the defaults.
- Monster skills other than heal and teleport, and aggressive monsters that join a fight, in the simulator.
- A model player with skills, cards and refines, which would make the high bands' supplies credible.
- Economy checks of dungeon floors, and of quest rewards and shop prices.
- A run on a real amd64 machine and in Safari.

## Review items

- Whether every class family's guild should stand in a town of the first bands, so every first job is reachable at job level 10.
- The model player: its build, gear and job path, and whether the targets should come from more than the medians of the classic fields, such as from the 4 fields of the band 91-99.
- Whether the economy checks should also run after the `spawn_table` hooks, and whether a mod may set its own targets.
- A Lua way to read a field's simulated rates, for mods that tune their spawns.
