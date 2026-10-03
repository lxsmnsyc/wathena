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
- The mod in [testdata/mods/fixture](testdata/mods/fixture) places Dummies, which stand still and always drop a Jellopy and a Test Charm with a roll, a shop, and NPCs that give and take items and zeny, change jobs, warp and mute. It adds the word "badword" to the chat filter. It uses only the 1.0 Lua API.
- The lower chat limits of new characters are off, since every test character is new.
- `Restart` stops the server and starts it again with the same options, addresses and database. Players join again.
- `AdvanceClock` moves the server clock with `--dev`'s `POST /debug/wathena/clock`, and `Reports` reads `GET /debug/wathena/reports`.
- Players are `--dev` characters. Without a database they stay in memory while the server runs, so a player who logs in again gets its character back. They start as Swordmen at base level 50 and job level 40, with 10 Red Potions and 20 Jellopy, unless `Options` says otherwise. `Options.StartJob` `4:50:40` makes them Acolytes.
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
| `TestWhispers` | Two players whisper on one map, then across maps. A whisper to a name that is not online says so. |
| `TestChannels` | Two players on different maps join `#trade`, talk there, list the members and the channels. A line in a channel the player is not in is refused. |
| `TestBlocking` | A player blocks another: its whispers are refused and its local chat is hidden, while a third player still hears it. Unblocking shows it again. |
| `TestChatFilter` | The fixture's filter word turns into asterisks in local chat and in a whisper. |
| `TestMute` | The Warden mutes a player for a minute. Its lines and whispers are refused, then work again after the server clock moves 61 seconds. |
| `TestReport` | A player reports a line. The report keeps the line and the other chat the reporter saw, and not a whisper it did not see. |
| `TestReportInTheDatabase` | The same with PostgreSQL: the report, `report_lines` and `chat_log`. |
| `TestTransfersInTheDatabase` | The shop and loot scenarios with PostgreSQL: one item log line for each move while the player plays, the zeny log, and the saved character. |
| `TestDisconnectKeepsItems` | A player's connection closes in the middle of a drop, while another player picks up its earlier drop. It logs in again with what the server held for it, and nothing is lost or held twice. |
| `TestDisconnectKeepsItemsInTheDatabase` | The same with PostgreSQL, then the saved inventories. |
| `TestPartyInviteAndAccept` | `Welcome` holds the limits. A leader makes a party and invites a player, who accepts. Both see the members in slot order, and the leader sees the member's walk in the party window. Invites to a name that is not online, to a member, and from a member who is not the leader say why. |
| `TestPartyInviteBlocked` | A player who blocks the leader refuses its invite at once and sees no invite. |
| `TestPartyChat` | A party line reaches a member on the other map, and not a player outside the party on the speaker's map. A player without a party cannot say a party line. |
| `TestPartyEvenExpShare` | A solo kill of a Dummy gives 10 base and 10 job exp. With even exp share, a kill gives each of two members 5 of each. Both are at the Dummy's level, so renewal's level penalty is 100%. |
| `TestPartyItemPickUp` | With shared pick-up, a member who did not fight takes a fresh drop at once. Without it, the server refuses with `PICK_UP_FAILURE_NOT_YOURS`. Each charm Alice picks up shows to Bob. With even item share, each pick-up goes to one member on the map, and a charm shows only to the member who did not get it, as `clif_party_show_picker` sends it. A Jellopy never shows. The totals stay right. |
| `TestPartyLeaderLeaves` | The leader leaves. Every other member sees `PARTY_OUTCOME_DISBANDED` and a party with ID 0, and the name is free again. |
| `TestFriends` | A friend request that is accepted puts each player on the other's list, online. The friend logs out and in again, and the other player sees both changes. |
| `TestPartySkill` | An Acolyte in a party casts Angelus. A member near it gets the status icon, and a player near it outside the party does not. |
| `TestPartyAndFriendsInTheDatabase` | With PostgreSQL, a party and a friendship come back after `Restart`, and their rows are saved. |
| `TestDuplicationDropsAndPickUps` | The duplication test below, in memory. |
| `TestDuplicationWithDatabase` | The same, with PostgreSQL, then the database checks. |
| `TestDuplicationPartyPickUps` | The same, in memory, with the bots in parties of 4 with even item share and shared pick-up. |

## Duplication tests

`DupTest` runs many bots that make `Move`s at the same time, such as `DropMove` and `PickUpMove`, while the test asks the server for every item it holds (`GET /debug/wathena/items`, with `--dev` only). After every audit and at the end:

- the amount of each item, in inventories and on the ground together, is what it was at the start,
- all zeny together is what it was at the start,
- no serial is held twice,
- each bot's inventory, from the server's messages, is what the server holds for it.

Disconnects are part of the test:

- `DisconnectMove` sends a drop or a pick-up and closes the connection at once. The move may happen or not. Moves have weights, and the test makes about one move in 60 a disconnect.
- A bot whose connection closes, by its own move or by the server, logs in again with the same name and goes on. Connections the server closed are in the test's log, with why.
- While a bot is away, an audit only checks that no total is above the start and that no serial is held twice. The last audit waits until every bot is back.
- A move sent just before a connection closed can happen without its bot seeing it, so the counts of drops and pick-ups seen may differ by one for each login again.

`DupTest.Setup` runs once every bot is online, before the start audit. `PartySetup` puts the bots in parties there. With even item share, a pick-up gives the item to a random member on the map, so the test also checks that each bot's items that came in differ from its own pick-ups, which it sees in its own `ItemTaken`. Without sharing they differ only for a connection that closed in the middle of a pick-up.

With a database, `CheckDatabase` stops the server and checks the saved inventories against the start, one item log line for each drop and pick-up the bots saw (and at most one more for each login again), and a history for every serial. Later steps add moves for trade, storage, mail and vending.

A failure that shows a total below the start with a closed connection means a character left the world while the test still counted on it. See decision 0017, "A missing total in the duplication test". `botclient` writes with its own timeout for this reason: the WebSocket library closes the connection when a write's context ends during the write.

## Running every check

`scripts/check.sh` runs the checks of `.github/workflows/ci.yml` on this machine, and its `test` step runs these tests (server/README.md, "Local checks").
