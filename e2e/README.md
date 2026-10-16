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
- The fixture has three typed world events (decision 0025): `Boss` on the start map, `Raid`, an invasion of the start map in two waves of Raiders, and `Hunt`, two chests on `e2e_field` with a Treasure Hunter on the start map. Their condition never holds, so they start only from `!event start <name>` and stop with `!event stop <name>`, which call `wa.world_events`. The config approves `events.schedule` and `events.control` for the fixture.
- The fixture also has a currency, `e2e.fixture:tokens`, that each character owns, and the Exchange, a shop of the table form at 3,12 that does not buy from players:
  - An Apple costs 10 zeny, 2 Jellopy and 1 token, with a stock of 3 that fills again an hour after the last fill.
  - A Red Potion costs 5 tokens.
  - A Jellopy costs 1 Jellopy, so the duplication tests buy with an item cost and keep the totals.
- Chat commands of the fixture reach the market calls: `!currency <name> <n>` and `!pay <name> <n>` give and take a currency, `!mailbox` opens the mailbox, `!mail <to|self> <item> <amount> [<zeny>]` sends a mail from the "Postmaster", `!buyingstore <slots>` opens the buying store setup, `!search <uses> <show|open>` opens the store search, `!autotrade` puts the speaker on autotrade, and `!stock <item> <n>` sets a stock at the Exchange. The fixture's vend hook refuses Contraband, and its vend and buying_store_sale events tell both players what changed hands. The server's config approves `mail.send`, `currencies.modify` and `shops.modify` for the fixture.
- `Options.Procedural` runs a procedural world: `waserver` and `wactl` built with `-tags procedural`, and a world that `wactl world new --mode procedural --config` makes in the test's database, with the server's config and the Home Village sample beside the test mods. `Options.Config`'s `procedural.layout` shapes it. It needs `Options.Database`.
- The fixture's `!slayer` places a Slayer next to the speaker, a monster that attacks on sight and kills in one hit, so a test sees a death and the way back to the save point.
- The fixture also has the Training Dummy, a Dummy worth a million base and job exp, which `!trainer` places next to the speaker, for a level up in the browser checks' exit check. The Kafra opens the guild storage too, with "Guild storage". The Test Charm has a `description`.
- The fixture has art in `art/`: the Test Charm's icon, found by its name, and the Dummy's sprite, which its record names. `Start` copies `testdata/mods` to its temporary folder and builds each mod's art there with `wactl mod build-art`'s code, so nothing is written into the repository.
- [testdata/mods/marketrules](testdata/mods/marketrules) is a ruleset mod, `e2e.marketrules`: renewal without mail fees, for the mail duplication test.
- A player who says `!guild level <n>` in local chat gets the fixture to give its guild the exp that takes it to level n, with `guild:give_exp`. The tests reach the guild skills this way, without thousands of kills. The server's config approves `guild.manage` for the fixture.
- [testdata/mods/limitrules](testdata/mods/limitrules) is a ruleset mod, `e2e.limitrules`: renewal with `storage_slots` up to 700, `cart_slots` up to 150, and `block_list_refuses_trade: no`, for the limit and trade tests. The fixture's `!expand <limit> <amount>` calls `player:expand`, and the config approves `player.expand_limits` for the fixture.
- [testdata/mods/guildrules](testdata/mods/guildrules) is a ruleset mod, `e2e.guildrules`: renewal with `emergency_call: 15`, so Emergency Call works outside GvG maps. `Options.Ruleset` passes it as `--ruleset`. Other tests use the core renewal ruleset, and the server logs that the mod is installed but not used.
- The lower chat limits of new characters are off, since every test character is new.
- `Restart` stops the server and starts it again with the same options, addresses and database. Players join again.
- `AdvanceClock` moves the server clock with `--dev`'s `POST /debug/wathena/clock`, and `Reports` reads `GET /debug/wathena/reports`.
- The account deletion tests write the deletion request as the account page does, since `--dev` has no account pages, and run the deletion job with `--dev`'s `POST /debug/wathena/deletions` (decision 0032).
- `UploadEmblem` sets a guild's emblem with `POST /debug/wathena/emblem`, and `OpenGuildStorage` opens a player's guild storage with `POST /debug/wathena/guild-storage`, as an NPC would. Both are `--dev` only. `HTTP` is the game server's base URL, for `GET /emblems/<guild>/<version>.png`.
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
| `TestWorldLayout` | A procedural world that `wactl world new` made with the Home Village sample (decision 0041). A new character starts on the home map's start place, walks through its portal into the first town, saves at the teleport service, visits the second town with `@go`, teleports there for its price, dies to a Slayer, and comes back at its save point. Its entered maps are in the database. Needs PostgreSQL. |
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
| `TestGuildCreateAndInvite` | A player with an Emperium makes a guild, and the Emperium leaves its inventory. It invites a player, who accepts, and both see the two members with their positions. A player in sight sees the new member's badge in `EntityGuild`, and a player who joins later sees both badges, guild name and position name, in `EntityAppear`. |
| `TestGuildInviteBlocked` | A player without an Emperium cannot make a guild. A player who blocks the master refuses its invite at once and sees no invite. |
| `TestGuildPositions` | A Newbie's invite fails with `GUILD_OUTCOME_NO_PERMISSION`, and a member cannot change positions. The master lets position 1 invite and moves the member there. A player in sight sees the new position name, and the member's invite works. |
| `TestGuildChat` | A guild line reaches a member on the other map, and not a player outside the guild on the speaker's map. A player without a guild cannot say a guild line. |
| `TestGuildExpTax` | The master sets an exp tax of 50% on the Newbie position. A member kills a Dummy, worth 10 base exp: the guild's exp and the member's paid exp grow by 5, and the member keeps 5 base exp and all 10 job exp. |
| `TestGuildLevelAndSkill` | The fixture gives the guild a level. Both members see `GUILD_OUTCOME_LEVEL_UP` and a skill point. A skill whose requirement is missing is refused, a member cannot raise a skill, and the master raises Approval, which both see. |
| `TestGuildEmergencyCall` | With `e2e.guildrules`, the master of a guild at level 11 raises the skills up to Emergency Call and casts it. A member on the other map warps to a cell next to the master. |
| `TestGuildEmergencyCallRefused` | With the core ruleset, Emergency Call fails, since it works on GvG maps only. |
| `TestGuildEmblem` | A 24 x 24 BMP emblem reaches the members in `GuildInfo` and the players in sight in `EntityGuild`, with version 1. `GET /emblems/<guild>/1.png` returns a 24 x 24 PNG with `Access-Control-Allow-Origin: *`, and magenta is transparent. Another version is not found, and a 32 x 32 image is refused. |
| `TestGuildStorage` | The storage fails before Guild Storage is learned, and for a position without storage rights. The master opens it, puts Jellopy in and takes some out. Another member gets `STORAGE_OUTCOME_IN_USE` while it is open, then sees what is in it. The audit counts the stored items. |
| `TestGuildInTheDatabase` | With PostgreSQL, a guild's members, positions, notice, level, skills, emblem and storage come back after `Restart`, and their rows are saved. |
| `TestStoragePutAndTake` | `Welcome` holds the storage limits with their ranges. The fixture's Kafra opens the account's storage. A player puts Jellopy in and takes some out, a script counts and takes what it holds, and the storage shows the same items when it opens again. |
| `TestExtraStorage` | The Kafra opens the fixture's Vault, an extra storage of 50 slots. Its items stay apart from the account's storage, and a script gives items into it. |
| `TestStorageInTheDatabase` | With PostgreSQL, the account's storage, the Vault and the cart come back after a login and after `Restart`, and their rows and item log lines are saved. |
| `TestCart` | A Merchant learns Pushcart and gets a cart. It puts items in, takes them out, moves them between the cart and the storage, and the cart refuses what passes `max_cart_weight`. A player without a cart gets `STORAGE_OUTCOME_NO_CART`. |
| `TestStoragePages` | A script fills a storage with 250 charms that do not stack. The list comes in pages after `StorageOpen`, each in its own frame under 64 KiB, and lists every charm. |
| `TestTrade` | Two players trade items and zeny. Both inventories and zeny change, and the fixture's `trade` event tells both. |
| `TestTradeInTheDatabase` | The same with PostgreSQL: the item and zeny log lines have the action `trade` and one transaction, and the saved characters hold the result. |
| `TestTradeRefused` | The fixture's `trade` hook refuses a trade with Contraband, and both players get its message. A player who blocks the requester refuses at once and sees no request. A player 3 cells away is too far. |
| `TestTradeBlockListRule` | With `e2e.limitrules`, whose `block_list_refuses_trade` is no, a player who blocks the requester still gets the request. |
| `TestExpandLimits` | `!expand` raises the inventory's slots up to the max and lowers them to the base, and the player gets `PlayerLimits` after each change. The account's storage stays at its default max. |
| `TestExpandLimitsInTheDatabase` | With PostgreSQL and `e2e.limitrules`, the expanded inventory, storage and cart come back after a new login and after a restart, the storage window opens with the expanded size, and `characters.inventory_size` and `storage_sizes` hold the values. |
| `TestRestrictions` | Items with one restriction each show it in `InventoryAdd`. A `no_trade` item cannot be offered, a `no_storage` item cannot be stored by a player or a script, and a `no_drop` item cannot be dropped. An equipped item cannot be offered or stored. |
| `TestBoundItems` | Bound Jellopy do not stack with the others. An account-bound copy goes into storage. A character-bound copy cannot be stored, offered or dropped. A party-bound copy goes away when its owner leaves the party. |
| `TestShopCostsAndStock` | The Exchange shows its costs, stock and name. Purchases fail without the zeny, the tokens, the Jellopy or the stock. A purchase takes zeny, items and tokens together, with `INVENTORY_REMOVE_REASON_PAID`. The stock runs out, fills again after `AdvanceClock` passes an hour, and `shop:set_stock` sets it. |
| `TestShopStockInTheDatabase` | With PostgreSQL, the Exchange's stock is saved with the purchase and comes back after `Restart`. |
| `TestCurrencies` | A script gives tokens, which the character owns, and cash, which the account owns. `CurrencyUpdate` lists both with their display names, and comes again at the next login. |
| `TestCurrenciesInTheDatabase` | The same with PostgreSQL: the currencies come back after `Restart`, with their rows and zeny log lines. |
| `TestMail` | Alice mails Bob 3 Jellopy and 1,000 zeny and pays 2% of the zeny and 2,500 for the item. A mail within `mail_delay`, to a name nobody has, or with too little zeny is refused. Bob gets `MailNew`, reads the mail, cannot delete it while it holds items, takes it and deletes it. The audit counts the mail while it waits. `player:open_mail` opens the mailbox. |
| `TestScriptMailToOfflineCharacter` | A script mails Apples and zeny to Bob while he is offline. He finds the mail at his next login and takes it. |
| `TestMailInTheDatabase` | With PostgreSQL, a player's mail and a script's mail to an offline character are saved, and their recipients take them after `Restart`. |
| `TestMailExpiry` | After `AdvanceClock` passes `mail_return_days`, the expiry, which runs once a minute, sends Bob's untaken mail back to Alice as "RE:Hello", and Alice takes back her items and zeny. |
| `TestVending` | A Merchant learns Vending and opens a shop from her cart. Bob sees its title, browses it and buys. She sees `VendingSold`, and the zeny and items move. A purchase needs the zeny and the stock, the vend hook refuses Contraband, and a purchase of 100,000,000 zeny pays the 5% vending tax. |
| `TestVendingAutotrade` | The vendor goes on autotrade and her connection closes with `DISCONNECT_REASON_AUTOTRADE`. Bob still buys, and once the shop sells out she leaves. Her next login has the zeny. |
| `TestVendingAutotradeInTheDatabase` | With PostgreSQL, the autotrade shop is saved, opens again after `Restart`, and still sells. The vendor's next login ends the autotrade. |
| `TestStoreSearch` | A store search on the start map finds a shop on the other map, with its map and cell, and a click shows the place. A search too soon, without uses, or of buying stores with none open is refused or finds nothing, and a click that opens stores cannot open one on another map. |
| `TestBuyingStore` | A script opens the buying store setup, and Alice opens a store that buys Jellopy. Bob sees its title, browses it, finds it with a store search and sells to it. Alice sees `BuyingStoreBought`, and the zeny and items move. A sale of more than the store buys is refused, and the store closes once it bought everything. |
| `TestDuplicationDropsAndPickUps` | The duplication test below, in memory. |
| `TestDuplicationWithDatabase` | The same, with PostgreSQL, then the database checks. |
| `TestDuplicationPartyPickUps` | The same, in memory, with the bots in parties of 4 with even item share and shared pick-up. |
| `TestDuplicationGuildStorage` | The same with 8 bots in one guild, which also use the guild storage. |
| `TestDuplicationGuildStorageWithDatabase` | The same, with PostgreSQL, then the database checks with the saved guild storage. |
| `TestDuplicationStorageAndCart` | The drop and pick-up test with personal storage and cart moves, in memory. |
| `TestDuplicationStorageAndCartWithDatabase` | The same, with PostgreSQL, with the saved storages and carts. |
| `TestDuplicationTrades` | 8 bots trade Jellopy and zeny, and use their storages and carts, with connections closed in the middle of trades. |
| `TestDuplicationTradesWithDatabase` | The same, with PostgreSQL, then the saved inventories, storages, carts and zeny. |
| `TestDuplicationMarket` | 8 Merchants open vending shops and buy from each other, buy at the Exchange, and mail each other Jellopy and zeny, with connections closed in the middle of sales, purchases, mails and takes. |
| `TestDuplicationMarketWithDatabase` | The same, with PostgreSQL, then the saved inventories, carts, mails and zeny. |
| `TestDuplicationBuyingStores` | 8 bots open buying stores and sell to each other's, with connections closed in the middle of sales and with stores open. |
| `TestDuplicationBuyingStoresWithDatabase` | The same, with PostgreSQL, then the saved inventories and zeny. |
| `TestWorldEventBoss` | `!event start Boss` starts the fixture's boss event with `world_events.start`. Alice gets `WorldEventNotice` and the announcement, a second start is refused, and Bob, who logs in during the run, gets `WorldEventList`. Alice kills the boss: the announcement names her, the end notice says won, and she gets the 100 zeny of a participant while Bob gets none. A stop ends a second run with "stopped". |
| `TestWorldEventInvasion` | The Raid's two waves of Raiders enter at the map's edges and come to the players. Alice kills them, the second wave comes once the first died, and the raid is won: Bob, who did not fight, gets the 2 Apples of everyone on the map. |
| `TestWorldEventTreasureHunt` | The Hunt hides 2 chests on `e2e_field`. The Treasure Hunter on the start map tells their map and cells. Alice warps there and opens both: each gives 50 zeny and drops its Jellopy, and the hunt is won. |
| `TestWorldEventRestartInTheDatabase` | With PostgreSQL, a boss run that `Restart` cut short goes on after it, and Alice, back after the restart, gets it in `WorldEventList`. She kills the boss once. After another restart the boss stays dead, her zeny holds the reward once, `world_event_progress` has the rows `boss` and `end`, and the zeny log has one `event` line. |
| `TestAnnouncementsAndCancel` | `POST /debug/wathena/announce` reaches the player for the world and its own map, not for the other map. A planned restart fires the fixture's `restart_planned` handler with its time and reason, a cancel is announced and fires it again, and the old time passes without a restart. |
| `TestPlannedRestart` | With PostgreSQL, a restart planned 2 minutes ahead, with `AdvanceClock`, announces itself at 1 minute and at 30 seconds, refuses a new login with `DISCONNECT_REASON_RESTART` in its last minute, disconnects the player with its reason, and the server exits with status 75. After `Restart` the character comes back as saved. |
| `TestDeletionBlocksPlayUntilCancelled` | With PostgreSQL, a character whose account is set to be deleted cannot log in: the login gets `DISCONNECT_REASON_ACCOUNT_DELETION` with the date. The job does nothing before the date. After a cancel the character plays again. |
| `TestDeletionRunsAfterTheWait` | With PostgreSQL, a party leader's account is set to be deleted. After `AdvanceClock` moves 15 days, the job deletes the account, the character, its inventory and its chat lines. The party breaks up for the member who stays, and the item log keeps the character's ID. |
| `TestPhase5ExitCheck` | The phase 5 exit check below: 24 bots in 5 groups on both maps trade, store, mail, vend, sell to buying stores, buy at the Exchange and drop items at the same time, with a restart in the middle, with PostgreSQL. |
| `TestModArt` | The client data names the fixture's art for the Test Charm and the Dummy by the IDs the server gave, and the server serves both from `/assets/mods/e2e.fixture/0.1.0/` through the build's manifest and shards, with hashes that match (decision 0033). |

## Duplication tests

`DupTest` runs many bots that make `Move`s at the same time, such as `DropMove` and `PickUpMove`, while the test asks the server for every item it holds (`GET /debug/wathena/items`, with `--dev` only). After every audit and at the end:

- the amount of each item, in inventories, guild storages and on the ground together, is what it was at the start,
- all zeny together is what it was at the start,
- no serial is held twice,
- each bot's inventory, from the server's messages, is what the server holds for it.

Disconnects are part of the test:

- `DisconnectMove` sends a drop or a pick-up and closes the connection at once. The move may happen or not. Moves have weights, and the test makes about one move in 60 a disconnect.
- A bot whose connection closes, by its own move or by the server, logs in again with the same name and goes on. Connections the server closed are in the test's log, with why.
- While a bot is away, an audit only checks that no total is above the start and that no serial is held twice. The last audit waits until every bot is back.
- A move sent just before a connection closed can happen without its bot seeing it, so the counts of drops and pick-ups seen may differ by one for each login again.

`DupTest.Setup` runs once every bot is online, before the start audit. `PartySetup` puts the bots in parties there. With even item share, a pick-up gives the item to a random member on the map, so the test also checks that each bot's items that came in differ from its own pick-ups, which it sees in its own `ItemTaken`. Without sharing they differ only for a connection that closed in the middle of a pick-up.

Guild storage is part of the test too:

- The audit lists the guild storages as one more entry, with the map `guild_storage` and one player per guild, named `guild:<ID>`. The test counts their items in the totals, and not as players.
- The audit reads the guild storages on the zone of the first map with players, so the bots that use the storage stay on the start map.
- `GuildStorageSetup` puts every bot in one guild. The fixture takes it to level 21, the master raises the skills up to Guild Storage, and the Newbie position may use the storage. The first bot needs an Emperium.
- `GuildStorageMove` opens the bot's guild storage through the debug endpoint. Only one bot at a time has it open, and the others get `STORAGE_OUTCOME_IN_USE`. The bot puts in and takes out parts of Jellopy stacks one to four times, and closes it. One time in 8 it closes its connection instead, with the storage open, and logs in again.
- An item taken out of the storage enters the inventory, but it is not a pick-up, so the counts of items that came in leave it out.

Personal storage, carts and trades are part of the test too (phase 5 step 6):

- The audit lists each storage and cart in memory in the `guild_storage` entry, named `storage:<key>`. The test counts their items in the totals.
- `PersonalStorageMove` opens the bot's account storage through the fixture's `!storage`, and moves parts of Jellopy stacks one to four times between the inventory, the storage and the cart. One time in 8 it closes its connection instead, with the storage open. `CartMove` moves Jellopy between the inventory and the cart. `CartSetup` gives each bot Pushcart and a cart.
- `TradeMove` has the bots with odd numbers ask a bot with an even number to trade. Each side offers up to two parts of Jellopy stacks and sometimes zeny, locks and confirms. One time in 15 at each of three points, before the lock, after it and right after the confirm, a bot closes its connection instead and logs in again. The trade must happen whole or not at all.

Mail, vending, the Exchange and buying stores are part of the test too (phase 5 step 7):

- The audit lists each mail in memory in the `guild_storage` entry, named `mail:<ID>`, with its items and zeny. It includes a zone's sent mails that wait for its next commit to reach their mailboxes. The test counts them in the totals.
- The audit reads the mail registry on the same zone as the storages, the first map with players by name. So the bots that use mail, storages and carts stay on one map, as the bots that use the guild storage do.
- `MarketSetup` gives each Merchant a cart, zeny, Pushcart 3 and Vending. `VendMove` opens a shop of Jellopy from the cart, and closes it, or closes the connection with it open. `VendBuyMove` buys from a shop in view. `ExchangeMove` buys Jellopy that cost Jellopy. `MailMove` mails Jellopy and zeny to another bot, and `MailTakeMove` takes a mail. `BuyingStoreMove` opens a store that buys Jellopy, and `SellToStoreMove` sells to one. Each sends its sale, purchase, mail or take and closes the connection at once one time in 10.
- The mail test runs `e2e.marketrules`, which has no mail fees, so the zeny total holds.
- Items that a sale, a purchase or a take moved into an inventory are not pick-ups, so the counts of items that came in leave them out: the changes just before `VendingSold`, `VendingResult` BOUGHT, `BuyingStoreBought`, `ShopResult` BOUGHT and `MailResult` TAKEN.

The audit also lists each player's own currencies, such as the fixture's tokens, by full name. The test counts them in the totals, and `SavedTotals` reads them from `character_currencies`.

With a database, `CheckDatabase` stops the server and checks the saved inventories, guild storages, storages, carts and mails, and the zeny of characters and mails, against the start, one item log line for each drop and pick-up the bots saw (and at most one more for each login again), and a history for every serial.

A failure that shows a total below the start with a closed connection means a character left the world while the test still counted on it. See decision 0017, "A missing total in the duplication test". `botclient` writes with its own timeout for this reason: the WebSocket library closes the connection when a write's context ends during the write.

## The phase 5 exit check

`TestPhase5ExitCheck` is the exit check of phase 5 (docs/strategy.md, "Phase 5"): the duplication tests' moves, all at the same time, under load, with a restart. It needs PostgreSQL, and skips without `WATHENA_TEST_DATABASE_URL`.

What runs:

- 24 Merchants in 5 groups. On `e2e_field`, 6 open vending shops and buy from each other and mail each other, and 6 trade and use their storages and carts. On the start map, 4 buy at the Exchange, 4 open buying stores and sell to each other's, and 4 drop Jellopy and pick up each other's.
- The moves close their connections in the middle one time in 8 to 15, as in the duplication tests, and the bots log in again.
- The shoppers buy Jellopy for Jellopy, and Red Potions for 5 tokens each, at most 20 times each over the run. Only this move takes tokens, so the audits count each Red Potion as 5 tokens, and the tokens stay constant.
- After half the time, the bots that drop items stop and pick up every ground item, since ground items are not saved. Then the server stops while the other groups move. The saved totals must be the totals of the start. The server starts again, every bot logs in again and waits, and the first audit with every bot must equal what the database saved. Then the bots go on.
- At the end, `CheckDatabase` checks the saved totals against the last audit, the item log of drops and pick-ups, and a history for every serial. The zeny log must show the tokens given at the start, less the tokens the Exchange took, as the tokens the characters hold, and 5 tokens for each Red Potion the item log shows bought.
- The test logs a summary: the bots, maps and minutes, the moves by kind, the disconnects and logins again, the audits, the restart, and the totals at the start, at the restart, after it and at the end.

How long:

- By default the moves run for 40 seconds, the restart left out, so the `test` step stays short.
- `WATHENA_EXIT_MINUTES` sets the minutes of moves. The `e2e` step of `scripts/check.sh` sets it to 30.
- `WATHENA_EXIT_REPORT` names a file the test writes its numbers to. The `e2e` step prints that file, since `go test` without `-v` hides the log of a test that passed.

## Running every check

`scripts/check.sh` runs the project's checks on this machine, and its `test` step runs these tests (server/README.md, "Local checks").
