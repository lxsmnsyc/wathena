# e2e

End-to-end tests (strategy section 12.1, [decision 0017](../docs/decisions/0017-e2e-and-transfers.md)). Each test starts a real `waserver` in its own process and plays it with bots from [tools/botclient](../tools/botclient). Every check is on what the server sent to a bot, or on what the server saved.

```text
go test ./e2e/
go test -race ./e2e/
WATHENA_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable' go test ./e2e/
```

## The server

- `Start` builds `waserver` and an asset build once per test run, then starts a server for the test and stops it when the test ends.
- The world has two maps: `placeholder/map`, where players start, and `e2e_field`, a copy of it for warps.
- The mod in [testdata/mods/fixture](testdata/mods/fixture) places Dummies, which stand still and always drop a Jellopy and a Test Charm with a roll, a shop, and NPCs that give and take items and zeny, change jobs and warp. It uses only the 1.0 Lua API.
- Players are `--dev` characters. They start as Swordmen at base level 50 and job level 40, with 10 Red Potions and 20 Jellopy, unless `Options` says otherwise.
- `--seed` fixes the random streams, so the same test makes the same draws.
- With `Options.Database`, the server gets a new PostgreSQL database and connects as the game role. Without `WATHENA_TEST_DATABASE_URL`, those tests skip.
- With `-race`, the servers of tests that set `Options.Race` are built with the race detector, and one of them runs at a time. Others run without it, since it makes the server start about ten times slower.
- A server's ERROR lines and data races fail the test that calls `CheckLogs`. A failed test prints the end of the server's log.
- `-short` skips the tests.

## Scenarios

A `Player` sends a command and waits for the events that show the result, up to 15 seconds. A step that fails or times out fails the test.

```go
srv := e2e.Start(t, e2e.Options{})
p := srv.Join(t, "Fighter")
dummy := p.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Dummy")
p.Kill(dummy)
p.PickUp(p.FindItem(e2e.Jellopy))
p.Talk("Quartermaster").Choose("Give")
```

| Test | What it plays |
|---|---|
| `TestLoginWalkAndChat` | Two players log in and see each other. One walks, and the other sees the walk. Both hear a chat line. |
| `TestCombatLootAndItems` | A player kills a Dummy, picks up both drops, the charm with its roll, and uses a Red Potion. |
| `TestSkillAndJobChange` | A Swordman learns Bash, casts it on a Dummy, and becomes a Knight at the Mentor. |
| `TestShopAndScriptItems` | A script gives items and zeny, a shop sells and buys, and a script takes items. |
| `TestWarp` | The Guide warps the player to the other map. |
| `TestTransfersInTheDatabase` | The shop and loot scenarios with PostgreSQL: one item log line for each move while the player plays, the zeny log, and the saved character. |
| `TestDuplicationDropsAndPickUps` | The duplication test below, in memory. |
| `TestDuplicationWithDatabase` | The same, with PostgreSQL, then the database checks. |

## Duplication tests

`DupTest` runs many bots that make `Move`s at the same time, such as `DropMove` and `PickUpMove`, while the test asks the server for every item it holds (`GET /debug/wathena/items`, with `--dev` only). After every audit and at the end:

- the amount of each item, in inventories and on the ground together, is what it was at the start,
- all zeny together is what it was at the start,
- no serial is held twice,
- each bot's inventory, from the server's messages, is what the server holds for it.

With a database, `CheckDatabase` stops the server and checks the saved inventories against the start, one item log line for each drop and pick-up the bots saw, and a history for every serial. Later steps add moves for trade, storage, mail and vending.

## Running every check

`scripts/check.sh` runs the checks of `.github/workflows/ci.yml` on this machine, and its `test` step runs these tests (server/README.md, "Local checks").
