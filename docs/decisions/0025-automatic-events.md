# 0025: Automatic Events and the Phase 5 Exit Check

- Status: proposed.
- Date: 2026-10-05.
- Code: [server/worldevent](../../server/worldevent) (`typed.go`, `runs.go`, `field.go`, `core/data/events.yml`), [server/zone](../../server/zone) (`autoevents.go`), [server/world](../../server/world) (`events.go`), [server/modscript](../../server/modscript) (`worldevents.go`), [server/gamedata](../../server/gamedata) (`world.go`), [server/store](../../server/store) (`events.go`, migration 00021), [server/transfer](../../server/transfer), [server/mail](../../server/mail), [tools/modkit](../../tools/modkit), [client](../../client) (`worldEvents.ts`), [e2e](../../e2e) (`events_test.go`, `exit.go`, `exit_test.go`), [mods/examples/halloween](../../mods/examples/halloween).
- Phase 5, step 8, the last step of phase 5. Builds on strategy section 8.8, proposal 0002, review decisions 16 and 31, and decisions [0017](0017-e2e-and-transfers.md), [0022](0022-test-speed.md), [0023](0023-storage-cart-trade.md) and [0024](0024-mail-vending-shops.md).

## Decision

### Typed events

- An event of `data/events.yml` with `type` is a typed event: `boss`, `invasion` or `treasure_hunt`. The type's fields are fields of the record, beside `trigger` and `duration_minutes`. A typed event's `script` is optional. An event without `type` is a script event, as before.
- `server/worldevent` reads the fields and checks their forms. `server/gamedata` checks the names when the mods load: monsters with stats, items and currencies, NPC sprites. A plain name is the event's mod's record first. So `wactl mod validate` reports a missing monster or item.
- Each kind of reward needs the permission a script needs to give it: `player.give_items`, `player.give_zeny`, `player.give_exp`, and `currencies.modify` for another mod's currency.
- The maps an event names must run when the server starts: its `map`, an invasion's `town`, and a boss's or treasure hunt's `maps`. A hint NPC's map that does not run is skipped with a warning.
- Map names follow `--map`: a plain name is `map/<name>`, and a name with a slash stays as it is, so the placeholder maps work.

### Level bands

- `level_band: [lowest, highest]` picks the fields whose level falls in the band. A field is a map whose ID has `_fild`, as rAthena names its fields. rAthena has no flag for fields.
- A field's level is the middle level of its spawns, weighted by their amounts, without bosses and MVPs, from the mode's recorded spawns (`gamedata.FieldLevels`). A map with a mix of monsters lands at the level most of its monsters have.
- `maps` lists maps of any kind instead, and is the only way to name a town, a dungeon or a mod's map.

### One event manager

- `worldevent.Runs` runs every event: it is the scheduler's runner and what the zones report to. A script event's run is its script, as before. A typed run asks the zones, through `worldevent.Field`, to place monsters and NPCs, and the zones send it notes.
- The scheduler's goroutine calls the manager: starts, ends, and `Tick` once a second, which reads the notes, starts invasion waves and ends runs that are won. Zones only append notes, read hints and queue requests under a lock, and the manager never holds that lock while it waits on a zone, so a zone and the manager never wait on each other.
- A run's start announcements come after its start notice, and its end announcements before its end notice, so a player reads them in order.
- `wactl mod test` uses the same manager with a field of its one map.

### Boss

- The run picks a map of its band or `maps`, and one of `monsters`, from the world's secret stream `events`. The boss lands on a random walkable cell, as `spawning.monsters` places monsters.
- With `announce`, true by default, the world hears when the boss appears, who killed it and the MVP, and when it leaves. rAthena's `boss_monster` spawn type tells players a boss appeared (`BOSS_INFO_ALIVE_WITHMSG`). The MVP is `get_mvp_player`'s: the living player on the map with the most damage dealt and taken. An MVP boss also gives rAthena's MVP exp and prize, as any MVP does.
- The run is won when the boss dies, and lost when it leaves.

### Invasion

- Invaders enter at walkable cells at the town's edges: from a random cell of a random side, the first walkable cell toward the middle. On a town such as Prontera that is a gate or the foot of a wall.
- Invaders are aggressive and can move, whatever their record says: their status gets rAthena's `MD_AGGRESSIVE`, `MD_CANMOVE` and `MD_CANATTACK`. They find and chase players with rAthena's monster AI. With no target, they walk toward the nearest player on the map, up to 12 cells at a time, in both the hard and the lazy AI. A zone without players sleeps, so invaders wait for players.
- A wave with `start: cleared` starts once every invader so far died. A wave with a time starts that long after the wave before started, or after the run started for the first wave.
- The raid is won when the last wave has started and every invader died. The manager counts the living invaders through the field each second. It is lost when the time runs out. Each wave, the win and the loss are announced.

### Treasure hunt

- Each chest lands on a random walkable cell of a random map of the band or `maps`. A chest is a monster: its record's drops are the loot, and the player who kills it opens it.
- Hint NPCs are NPCs the run places and removes. A click shows what the manager answers, with a Close button, and no Lua: how many chests are still there, and for up to three of them, nearest first, the direction and distance from the NPC on its own map, or the map and the cell rounded to 10 on another map. Directions treat rAthena's y as north.
- The hunt is won when every chest is opened.

### Rewards

- `rewards` is `{ to, items, zeny, exp, currency, min_share }`. `to` is `killer`, `participants` or `everyone_on_map`, and each player gets the whole reward.
- A boss's or a chest's rewards come at its death, from its damage log: `participants` are the players on the map whose damage is at least `min_share` percent, 1 by default, of the damage logged on the monster, as `mob_log_damage` keeps it. An invasion's come when it is won, from the run's tally of damage to its invaders on the town, and its `killer` is the player who killed the last invader.
- Only players on the map get rewards, as rAthena's `mob_dead` gives exp only to players on the map.
- Items, zeny and currencies move through transfers with the action `event` and the end `event`, so the item and zeny logs show them. Items that do not fit, and zeny past MAX_ZENY, are mailed to the player, from the end `event`. Exp goes through `gainExp`, so a guild takes its tax.

### Never twice, and restarts

- Each reward has a unit of progress: `boss`, `chest:<n>` or `result`. The zone writes the unit's row of `world_event_progress` in the same commit as the characters the reward changed, so after a crash the reward and its row are both saved or both lost.
- The manager writes the units it decides: `wave:<n>` when a wave starts, and `end` when a run ends before its time.
- A typed run that a stop cut short goes on after the restart while its window is open: the scheduler finds its run in `world_event_runs` and its rows. A boss without a `boss` row comes back, chests with a `chest:<n>` row stay opened, and an invasion starts again at its last wave. A run with an `end` row does not go on. A newer run whose window is open wins over it, since an event has one run at a time.
- This differs from script events on purpose: a script event's run that a stop cut short still does not start again, since its script's state is not saved. A typed run's state is the units, so it can go on. A player who lost a boss to a restart gets it back, and nobody gets a reward twice.
- Without a database, runs and units are in memory, as before.

### Lua

- `context:get_event()` returns the run in a typed event's script, and nil elsewhere. `run:get_monsters()` and `run:get_participants()` read the context's map and need `game.read`. `run:finish("won" | "lost")` sends a note: the run ends within a second, and an invasion that finishes won gives its rewards.
- `world_event_start` and `world_event_end` run in every zone, with `{ name, type, result }`. `type` is `"script"` for an event without one. `result` is `"won"`, `"lost"` or `"stopped"`, and nil at the start and for a script event that ran its time.
- `world_events.start(name)` and `world_events.stop(name)` need `events.control`. They queue a request that the scheduler takes within a second, and return false when the event does not exist, runs already, or does not run. A started run lasts its duration, and its scheduled time is the time it started.

### Core's samples

- `core:Field_Boss` (a Prontera field boss on a field of levels 1 to 30), `core:Prontera_Invasion` (three waves) and `core:Treasure_Hunt` (five `TREASURE_BOX1` chests, a guide in Prontera). They are core's `data/events.yml`, embedded in `server/worldevent`.
- Core's events are off unless the config's `mods.events.<name>.enabled` is true. A mod's events are on unless `disabled` is true.

### Client

- Every player gets `WorldEventNotice` at each start and end, and `WorldEventList` at login when runs last.
- The client shows a banner for each start and end, a notice in the chat log, and a small list of the runs with their map and time left.
- Announcements and the server's own lines now show in the chat log. The client had `ChatLog.announce` and `ChatLog.server`, but nothing called them.

## The phase 5 exit check

- `TestPhase5ExitCheck` runs 24 bots in 5 groups on both maps at once, with PostgreSQL: trades, storage and cart moves, mails, vending, buying stores, the Exchange with an item cost and with a currency cost, and drops and pick-ups, with connections closed in the middle of moves. Half way, the server stops and starts again, and every bot logs in again.
- Item, zeny, currency and serial totals hold at every audit, the saved totals equal the start at the restart and at the end, and every serial has a history. Tokens given minus tokens spent equal tokens held, and the tokens spent match the potions bought.
- It runs 40 seconds of moves in the default `test` step, and 30 minutes in `scripts/check.sh e2e`, which prints its numbers.
- The 30-minute run on 2026-10-05, with `-race` and PostgreSQL 18:
  - 14,416 trades completed of 30,194 requests.
  - 10,370 storage openings, 4,426 puts and 4,352 takes, 11,776 cart puts and 11,418 cart takes, 4,282 moves from cart to storage and 4,217 back.
  - 508 mails sent and 538 taken, 47 vending sales, 762 buying store sales.
  - 58,938 Exchange purchases with Jellopy and 77 with tokens.
  - 26,957 drops and 27,530 pick-ups.
  - 20,538 connections closed by the bots in the middle of moves, each logged in again, and a restart after 15 minutes: every bot was back 34.9 seconds after the stop.
  - 3,227 audits with every bot online and 379 with a bot away. Every total held: 720 Jellopy and 80,000 zeny, and of 1,200 tokens given, 775 were spent for 155 Red Potions and 425 held.
- It found a duplication: a mail sent to a mailbox not in memory stayed in the registry's recent list for 5 minutes, and a login within those minutes put the stale copy back in the mailbox after the mail was taken and deleted, so its items could be taken twice. A load now takes from the list only mails sent after it started, and removes its mails from the list. `TestDeletedMailStaysDeleted` covers it.
- The audit endpoint listed the shared mail registry from every zone at different moments, which could count a mail and the items it moved twice. It now lists the registry from one zone, and each zone's own sent mails.

## Phase 5 systems and their tests

| System | Scripted tests |
|---|---|
| Chat and channels | `TestLoginWalkAndChat`, `TestWhispers`, `TestChannels`, the chat tests of `server/chat` and `server/modscript`, and examples/town-crier. |
| Moderation | `TestBlocking`, `TestChatFilter`, `TestMute`, `TestReport`, `TestReportInTheDatabase`. |
| Friends and parties | `TestPartyInviteAndAccept`, `TestPartyInviteBlocked`, `TestPartyChat`, `TestPartyEvenExpShare`, `TestPartyItemPickUp`, `TestPartyLeaderLeaves`, `TestFriends`, `TestPartySkill`, `TestPartyAndFriendsInTheDatabase`, `TestDuplicationPartyPickUps`, the `party_share.json` parity, and examples/party-hall. |
| Guilds, guild storage and emblems | `TestGuildCreateAndInvite`, `TestGuildInviteBlocked`, `TestGuildPositions`, `TestGuildChat`, `TestGuildExpTax`, `TestGuildLevelAndSkill`, `TestGuildEmergencyCall`, `TestGuildEmergencyCallRefused`, `TestGuildEmblem`, `TestGuildStorage`, `TestGuildInTheDatabase`, `TestDuplicationGuildStorage` and its database variant, the `guild.json` parity, and examples/guild-hall. |
| Personal storage and cart | `TestStoragePutAndTake`, `TestExtraStorage`, `TestStorageInTheDatabase`, `TestCart`, `TestStoragePages`, `TestDuplicationStorageAndCart` and its database variant, and examples/bank. |
| Trade | `TestTrade`, `TestTradeInTheDatabase`, `TestTradeRefused`, `TestRestrictions`, `TestBoundItems`, `TestDuplicationTrades` and its database variant. |
| Mail | `TestMail`, `TestScriptMailToOfflineCharacter`, `TestMailInTheDatabase`, `TestMailExpiry`, the tests of `server/mail`, and the mail moves of `TestDuplicationMarket`. |
| Vending | `TestVending`, `TestVendingAutotrade`, `TestVendingAutotradeInTheDatabase`, `TestStoreSearch`, `TestDuplicationMarket` and its database variant. |
| Buying stores | `TestBuyingStore`, `TestDuplicationBuyingStores` and its database variant. |
| NPC shops and currencies | `TestShopAndScriptItems`, `TestShopCostsAndStock`, `TestShopStockInTheDatabase`, `TestCurrencies`, `TestCurrenciesInTheDatabase`, and examples/arena-market. |
| Automatic events | `TestWorldEventBoss`, `TestWorldEventInvasion`, `TestWorldEventTreasureHunt`, `TestWorldEventRestartInTheDatabase`, the tests of `server/worldevent` and `server/zone` (`autoevents_test.go`), and the seven event cases of examples/halloween. |
| All at once | `TestPhase5ExitCheck`. |

## Other choices

- Typed fields sit beside `type` in the record, not in a mapping under the type's name, since the type already names them.
- `announce` is true or false. A text with the boss's name and map would need placeholders, which no other field has.
- `exp` is `{ base, job }`. A plain number would leave job exp out.
- An event's display name is its name without the mod prefix, with spaces for underscores. Events have no `display_name` field, and the announcements name maps by their IDs, since the server has no map display names.
- The text of announcements and hints is English in Go. Locale keys for core's text come with the locale files of strategy section 10.
- The run handle has methods only, as the decision lists them. Its name and type come with `context:get_instance()` and the world events.
- `world_events.start` and `stop` return whether the request was taken, not whether the run started: a run starts on the scheduler's goroutine, within a second.
- A chest that dies to a monster or a skill with no player gives no `killer` reward, and still counts as opened.

## Left

- A map type in map data, so `level_band` can pick dungeons or mod maps, and a field that is not named `_fild` counts.
- Rewards to participants on other maps or offline, and a list of rewards per event, such as one for the killer and one for the participants.
- Guild sieges, rotating dungeons and world changes (strategy section 8.8), which need sieges and procedural mode.
- Locale text for the announcements and hints, and map display names in them.
- The scheduler's clock is the system clock, so `--dev`'s clock does not move event times. Mod tests drive the clock with `world:set_time` and `world:advance`.
- The exit check's restart stops the ground group first and picks up every ground item, since ground items are not saved.
