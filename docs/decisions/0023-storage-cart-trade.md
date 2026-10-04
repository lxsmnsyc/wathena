# 0023: Storage, Cart, Trade and Limit Ranges

- Status: proposed.
- Date: 2026-10-04.
- Code: [server/storage](../../server/storage), [server/zone](../../server/zone) (`storage.go`, `storagescripts.go`, `trade.go`, `bound.go`, `pages.go`), [server/item](../../server/item) (`restrict.go`, `inventory.go`), [server/transfer](../../server/transfer), [server/rules](../../server/rules) (`limits.go`), [server/mod](../../server/mod) (`ruleset.go`, `storages.go`, `items.go`), [server/gamedata](../../server/gamedata) (`storages.go`), [server/store](../../server/store) (`storage.go`, `commit.go`, migration 00019), [server/modscript](../../server/modscript) (`storage.go`, `trade.go`), [server/gateway](../../server/gateway), [tools/convert](../../tools/convert), [client](../../client) (`storage.ts`, `trade.ts`), [e2e](../../e2e), [mods/examples/bank](../../mods/examples/bank).
- Phase 5, step 6. Builds on strategy sections 3.5, 5.4 and 6.8, proposal 0002, review decision 29, and decisions [0017](0017-e2e-and-transfers.md), [0020](0020-parties-and-friends.md), [0021](0021-guilds.md) and [0022](0022-test-speed.md).

## Decision

### Storages in one registry

- The package `storage` keeps the storages of the characters a process has online: an account's storage, the extra storages of `data/storages.yml`, and each character's cart. Every zone of the process shares one `Registry`, as the chat hub and the social registry.
- A storage loads when the first character that uses it logs in, on the goroutine that loads the character, never on a zone's. It stays while a character that uses it is online. Its zone lets it go after the logout's last commit is queued, and a later load waits until that commit is written (`Store.LoadStorage`).
- Two characters of one account can be online at once in wAthena, which rAthena's account login prevents. They share one storage object, so neither sees a stale copy, and only one has it open at a time, as `storage_flag`. The second gets `IN_USE`.
- Without a database, storages stay in memory while the server runs, as `--dev` characters do. A character in memory is its own account, by name.
- The cart is a storage of its character, so it moves through the same code, saves the same way, and the audit of the duplication tests sees it.

### Moves

They follow `storage.cpp` and `pc.cpp`'s cart functions:

- Every move goes through `server/transfer`: put and take, cart put and take, cart to storage, storage to cart, a script's give and take, and a bound item that goes away with its bond. The zone commits the storage's slots as a diff with the characters the move changed, in one transaction (decision 0017).
- One storage window is open at a time, personal or guild. A script opens it (`player:open_storage`). The cart is open while the player has one: the list comes at login and when a cart comes, and a close when it goes. Its items stay, as `pc_setcart`.
- A full storage is one whose filled slots reach its size, as `storage_additem`. A storage over its size after a limit went down keeps its items and takes no new stack.
- The cart's weight limit is the rule `max_cart_weight` (8,000 tenths). The speed penalty of a cart was already in the stats (decision 0016). The cart's weight now reaches the battle status, where Cart Revolution and Cart Termination read it; before, it was always 0.
- A trading player cannot put items into a storage or a cart, drop, use items or talk to NPCs, as `pc_istrading`.
- Equipped items cannot go in, as before.

### Restrictions and bound items

- Item records have `restrictions`, a list of `no_drop`, `no_trade`, `no_sell`, `no_cart`, `no_storage`, `no_guild_storage` and `no_mail`. Core's come from rAthena's `Trade` flags through a new extract, `item_trade.json` (rAthena fork commit `4214647ba`). The older `no_drop` and `no_sell` fields stay in step with the list.
- `itemdb_isrestricted` checks the item and each card in it. wAthena does the same.
- rAthena's `Override` lets group levels ignore the flags. Players have no group levels yet, so no one does. `trade_partner` and `no_auction` need marriage and auctions.
- A copy's `bound` is rAthena's `bound_type`: account, guild, party or character. It is part of what stacks, so bound and unbound copies never stack, as `pc_additem`. Migration 00019 saves it in every item table and in the item log.
- The rules, as rAthena's for a player without the bounded item permission:
  - Storage and cart take account-bound copies. Guild storage takes guild-bound copies.
  - A trade takes a guild-bound copy to a guildmate only.
  - Drops, sales and mail take no bound copy.
  - A player who leaves its party loses its party-bound items. A player who leaves its guild puts its guild-bound items into the guild storage. A guild that breaks up takes them with it. Either ends the player's trade.

### Trade

It follows `trade.cpp`:

- A request needs the other player on the map, within 2 cells, and not busy with a trade, an NPC, a shop or an open storage. Trading needs Basic Skill level 1 with the rule `basic_skill_check`, as `clif_parse_TradeRequest`.
- A player who blocks the requester refuses at once and never sees the request, as party and guild invites (decision 0018). rAthena does not check its ignore list here.
- Each side offers up to 10 items and an amount of zeny, then locks. Offers are checked as they come: the restrictions, bound copies, equipped items, the partner's free slots with `trade_count_stackable`, and the partner's weight. Zeny the player lacks, or that would take the partner past the cap, ends the trade, as `trade_tradeaddzeny`.
- Once both locked, each confirms. The `trade` hook runs before the trade completes and may refuse it with a message. Then every item and zeny moves in one transfer, checked on copies of both inventories first, as `trade_check`. The slots must still hold the offered serials. The commit holds both characters and every log line.
- A map change, a logout or a new request ends the trade. Moving does not, as rAthena.
- The `trade` event runs after a trade completes.

### Pages

- A server message is at most 64 KiB, and a zone sent one frame per player per tick. Lists that grow with a limit now go out in pages: the storage and cart lists (`StorageOpen` and `StorageItems`) and the inventory list (`InventoryUpdate.more`).
- A page holds at most 100 slots and 32 KiB. The first goes in the tick's frame, and the rest in frames of their own, 16 per tick, so a long list never fills a connection's buffer of 64 messages, which would drop the player.
- While pages wait, storage moves do nothing, so every page comes before any change to its list.

### Limit ranges and engine ceilings

- Review decision 29: each limit has a base, what a player or guild starts with, and a max, where expansions stop. A ruleset mod sets a number, the base, or `{ base, max }`. A max left out is the default max, or the base when the base is higher.
- The engine has a ceiling per limit. A base or a max above it fails the load, as does a base above the max.
- New limits: `inventory_slots` (base 100, max 200), `storage_slots` (600) and `cart_slots` (100). `guild_storage` is now rAthena's 600, and `guild_members` has the max 76, rAthena's `MAX_GUILD`, where Guild Extension stops.
- New rules: `max_cart_weight` (8,000) and `basic_skill_check` (yes).
- An inventory holds its usable slots, so memory follows the base and not the max. It grows when an expansion raises its size. Lowering a limit deletes nothing.
- The client gets every limit's base and max in `Welcome.limits.ranges`. Lua's `rules.limits.<name>` is the base, as before.
- Migration 00019 lets the tables hold every value up to the ceilings.

### Lua API

Review decision 29, as built:

- `player:open_storage(name)` and `player:open_guild_storage()`, which replaced the core-only `wa.internal` call of step 5.
- `count_item`, `give_item` and `take_item` take `from` or `to`: `"storage"`, `"cart"` or a storage's name. `give_item` takes `bound`. `guild:count_item(item)`.
- `data/storages.yml` records: `display_name`, `slots` and `owner`.
- The `trade` event and hook.
- `wa.testing` gained `world:trade(a, b, offer_a, offer_b)`, provisional.

## Engine ceilings

Measured on 2026-10-04 on an Apple M2 with 8 cores, with PostgreSQL 18 in Docker on the same machine. `go test ./server/zone -run '^$' -bench Ceiling` and `WATHENA_CEILING_BENCH=1 go test ./server/store -run TestCeilingSaves -v` give them again.

Lists: the paged list of a full holder of items with 4 cards, refine 10 and a random option, as `StorageOpen` and `StorageItems` frames (`BenchmarkCeilingLists`).

| List | Build time | Pages | Largest frame | All frames |
|---|---|---|---|---|
| Inventory of 200 | 0.06 ms | 2 | 4,890 B | 9.7 KB |
| Inventory of 1,000 | 0.30 ms | 10 | 4,918 B | 49 KB |
| Storage of 600 | 0.19 ms | 6 | 4,918 B | 29 KB |
| Storage of 10,000 | 2.9 ms | 100 | 5,018 B | 495 KB |
| Storage of 10,000, 16 cards each | 3.0 ms | 100 | 7,418 B | 735 KB |
| Cart of 100 | 0.03 ms | 1 | 4,814 B | 4.8 KB |
| Cart of 1,000 | 0.29 ms | 10 | 4,918 B | 49 KB |

Whole lists in one message (`TestCeilingMessageSizes`): a friend list of 1,000 names of 24 characters is 30,005 bytes, a block list of 1,000 is 26,007 bytes, and an item with 16 cards 73 bytes. Each fits the 64 KiB limit.

Tick work: a move into a nearly full holder and back, two transfers, which check on a copy of the holder (`BenchmarkCeilingItemMoves`); the copy of a character with a full inventory, as each autosave and commit makes (`BenchmarkCeilingSnapshot`); and a zone tick with every member of one party or guild on the map, a quarter of them walking each tick (`BenchmarkCeilingPartyTick` and `BenchmarkCeilingGuildTick`).

| Work | At rAthena's value | At the starting ceiling |
|---|---|---|
| Move into an inventory and back | 0.014 ms (200) | 0.055 ms (1,000) |
| Move into a storage and back | 0.052 ms (600) | 0.44 ms (10,000) |
| Move into a cart and back | 0.010 ms (100) | 0.062 ms (1,000) |
| Copy of a character | 0.022 ms (200 items) | 0.11 ms (1,000 items) |
| Tick with one party on the map | 0.010 ms (12) | 0.59 ms (256), 0.45 ms without the party |
| Tick with one guild on the map | 0.060 ms (76) | 1.85 ms (512), 1.81 ms without the guild |

Saves with PostgreSQL (`TestCeilingSaves`), each the mean of 20 rounds after a first full write:

| Save | At rAthena's value | At the starting ceiling |
|---|---|---|
| Character, first save with its items and block list | 38 ms (200 and 200) | 48 ms (1,000 and 1,000) |
| Character, a save that changes one slot | 1.9 ms | 1.9 ms |
| Character, load | 16 ms | 13 ms |
| Storage, first commit | 26 ms (600) | 189 ms (10,000) |
| Storage, a commit that changes one slot | 4.3 ms | 4.9 ms |
| Storage, load | 4.2 ms | 31 ms |
| Cart, first commit / one slot / load | 3.8 / 1.7 / 0.9 ms (100) | 21 / 2.0 / 3.1 ms (1,000) |
| Party, made with every member | 14 ms (12) | 214 ms (256) |
| Guild, made with every member | 97 ms (76) | 1,187 ms (512) |
| Every party, or every guild, loaded at start | 3.9 ms | 29 ms |

Chosen ceilings: every starting value holds, so each ceiling is its starting value.

- Inventory 1,000, storage 10,000, cart 1,000, party 256, guild 512, friends 1,000 and block list 1,000. Card slots keep their ceiling of 16, which the database and item data already had.
- Every frame stays under 8 KB, since lists go out in pages. A storage of 10,000 takes 7 ticks to list, at 16 pages a tick.
- A save writes only the slots that changed, so its time hardly grows with the limit. A first write and a load grow with it, and both happen once per login or creation.
- The largest tick cost is a guild of 512 members on one map, 1.85 ms, of which the guild itself is 0.04 ms: the rest is the players. The tick budget of strategy section 3.8 is 10 ms.
- A storage move at 10,000 slots costs 0.22 ms, since a transfer copies the holder for its checks. Storage moves are rare next to ticks. Checking a move without copying the holder is left for later.
- A guild of 512 takes 1.2 s to make in one write. Only `guilds.create` with many members and a load at start write a guild whole.

## Parity

No new recording. The rules come from reading rAthena's `storage.cpp`, `pc.cpp`, `trade.cpp`, `itemdb.cpp` and `clif.cpp` at the pinned commit, and each is a zone test. The restrictions come from a new extract, `item_trade.json`, of rAthena fork commit `4214647ba`: 7,387 items in renewal and 1,617 in pre-renewal set at least one flag. `TestItemRestrictions` checks that they reach the item data. The extract leaves the `Override` group level, which is 100 for every item in both modes.

## A bug from step 8 of phase 4

With a database, a mod with `data/storages.yml` failed to start: the `mod_ids` table's check took only the kinds of migration 00005 (item, monster, skill, job, quest and achievement). The same check refused the IDs of mod companions and mounts since decision 0016, so a mod with `data/companions.yml` or `data/mounts.yml` records failed to start with a database too. The tests of that step ran without one. Migration 00019 adds every kind of `mod.IDRanges`, and `TestModIDKinds` reserves an ID of each, so a new kind without a migration fails a test.

A second bug showed with the PostgreSQL e2e tests: `Store.DevCharacter` saved a `--dev` character's row and inventory, but not its skills or block list. With a database, a `--start-job` character lost the Basic Skill and the skills its job level gives. It now writes them in the same transaction (`TestDevCharacterSkills`).

## Other choices

- Storage keys name the owner: the account for its storage and its account-owned records, the character for its cart and its own records. Item log lines name an extra storage in `from_storage` and `to_storage`.
- The game role cannot lock account rows, so a commit that writes an account's storage takes an advisory transaction lock on the account.
- The serial primary keys of `inventory`, `guild_storage` and `storage_items` are checked at the end of each transaction. A commit writes each character and storage in turn, so a whole stack that changed hands is in two rows for a moment.
- `count_item` with `from` returns nil when the storage cannot be read, where rAthena returns -1: no cart, or a storage window open.
- `player:open_guild_storage()` returns true, or false and why, so the converter can give back rAthena's `GSTORAGE_` values.
- A script's `give_item` with `to` follows the place's restrictions and bound rules, as a player's move.

## Left

- Expanding a limit from scripts, such as rAthena's `expandinventory`, and reading a limit's max or a rule from Lua: new API for a later review.
- Rental items in storage and trade, the trade partner and auction restrictions, and group levels that override restrictions.
- The bound items of a player who is offline when it leaves its party or guild, as rAthena's check at login.
- Pushcart rental at the Kafras (`F_KafCart`), the Niflheim branch of `F_KafStor`, and storage passwords (`#kafra_code`, which no script sets yet).
- Get-only and put-only extra storages (`openstorage2` modes), `countbound`, and the other script commands that the converter leaves out (proposal 0002, next step 15).
- Checking a storage move without copying the holder, for storages near the ceiling.
- Mail and vending, which reuse the holders and the restrictions (step 7).
