# 0024: Mail, Vending, Buying Stores, Shops and Currencies

- Status: proposed.
- Date: 2026-10-04.
- Code: [server/currency](../../server/currency), [server/shop](../../server/shop), [server/mail](../../server/mail), [server/market](../../server/market), [server/zone](../../server/zone) (`shops.go`, `currency.go`, `mail.go`, `vending.go`, `marketscripts.go`), [server/transfer](../../server/transfer), [server/rules](../../server/rules) (`limits.go`), [server/mod](../../server/mod) (`npcs.go`, `currencies.go`), [server/gamedata](../../server/gamedata) (`currencies.go`, `mods.go`), [server/store](../../server/store) (`market.go`, migration 00020), [server/modscript](../../server/modscript) (`market.go`), [server/cmd/waserver](../../server/cmd/waserver) (`market.go`), [tools/convert](../../tools/convert), [client](../../client) (`mail.ts`, `vending.ts`, `buyingStore.ts`, `storeSearch.ts`, `shop.ts`), [e2e](../../e2e), [mods/examples/arena-market](../../mods/examples/arena-market).
- Phase 5, step 7. Builds on strategy sections 3.5, 5.4 and 6.8, proposal 0002, review decision 30, and decisions [0017](0017-e2e-and-transfers.md), [0022](0022-test-speed.md) and [0023](0023-storage-cart-trade.md).

## Decision

### Currencies

- `data/currencies.yml` records have `display_name` (1 to 24 characters, the name without the prefix by default), `owner` (`account` by default, or `character`) and `max` (1 to 2,147,483,647, the default). Core's records are `cash` (ID 1, rAthena's `#CASHPOINTS`) and `kafra` (ID 2, `#KAFRAPOINTS`), on the account, up to MAX_ZENY as `pc_getcash` caps them. Mod records get IDs from 10,000.
- A character's own currencies are in its record and save with it. An account's currencies are with the account's storage in server/storage, which the account's characters share, so two characters online at once never see different balances. A move holds the storage's lock.
- Every change goes through server/transfer: a `CurrencyWallet` moves as zeny does, with its own limit, and its log line in `zeny_log` names the currency. A give over the max is lost, as `pc_getcash`.
- The client gets `CurrencyUpdate` at login and after each change.

### One shop type

- A record's `shop` is the old list, or a table with `items`, `buys_from_players` and `discount`. Each item has a `price` in zeny, or a `cost` that mixes `zeny`, `items` and `currency`, at most 5 items and currencies, as the 5 required items of a barter. Without either, the price is the item's buy price.
- `stock` limits an item for every player, as a marketshop's or a barter's stock. `restock` fills it again: `daily` and `weekly` at the start of the day or of Monday on the server's clock, or a time such as `6h` after the last fill.
- One registry of shops serves every zone. A zone holds a shop's lock through a purchase, so two players never buy the same stock. The stock saves in the purchase's commit (`shop_stock`) and loads at start. A row of an item a shop no longer limits is left out, as `npc_market_checkall`.
- A purchase checks as `npc_buylist` and `npc_barter_purchase` do: an item the shop does not sell, the stock, a stack over its limit, the zeny, the items, the currencies, the weight and the free slots. Then the zeny, the cost items, the currencies and the new items move in one transfer: the costs leave first, so a stack a cost empties frees its slot.
- Discount and Compulsion Discount lower the zeny part only, as `discount_item_point_shop` 0. Items and currencies are never discounted.
- A cost takes unequipped copies in slot order, and leaves out refined copies and copies with cards, so a cost never takes a player's upgraded gear. rAthena's barters check a refine level instead.
- A shop that does not buy from players has no sell tab, and refuses a sale.
- The events `shop_buy` and `shop_sell` run after a purchase or a sale.

### Mail

It follows rAthena's `mail.cpp`, `int_mail.cpp` and the RODEX parts of `clif.cpp`:

- One mail registry per process. A mailbox loads at login, stays while its character is online, and goes after the logout's last commit. A load waits for the commits that hold mail of its character, as storages do.
- A mail holds a title (39 bytes), a body (499 bytes), zeny and up to `mail_items` items. Its items are a transfer box of kind `mail`, so each move of an attachment is logged.
- Sending checks each attachment as `mail_setitem`: `no_mail`, a bound copy, an equipped copy, the count and `mail_attachment_weight`. The sender pays the zeny, `mail_zeny_fee` percent of it and `mail_attachment_price` per item, to the end `fee`. `mail_delay` and `mail_daily_count` apply, and the count saves with the character (`mail_day` and `mail_sent`), as `SC_DAILYSENDMAILCNT` is saved.
- The recipient is found by name among the mailboxes in memory, or in the database on another goroutine, after which the send finishes on the zone's goroutine. A full mailbox (`mailbox`) refuses the mail. RODEX lets a player mail itself, and so does wAthena.
- A sent mail reaches its recipient's mailbox only after the sender's commit is queued, so the recipient's later commits come after it. Each mail has its own lock, which a take and the expiry both hold.
- Taking moves the zeny, the items, or both, in one transfer, as `mail_getattachment`. A mail with attachments cannot be deleted.
- Expiry runs once a minute, as `mail_return_timer` and `mail_delete_timer`. A mail older than `mail_return_days` goes back to its sender with "RE:" before its title, and its items and zeny move with it. A mail that came back, or a script's mail, which has no sender (rAthena's `send_id` 0), is deleted after `mail_delete_days`, with its items. With `mail_return_empty` off, an empty mail is deleted instead of returned.
- `player:send_mail` makes the items and zeny for the mail. Script mail skips the mail hook, as `player:whisper` skips the chat hooks. The mail event runs after every mail.

### Vending

It follows `vending.cpp`:

- The Vending skill opens the setup window with 2 lines and 1 per level, up to 12, and needs a cart. An NPC within `min_npc_vendchat_distance` refuses the skill and the shop.
- A shop sells cart slots. A line whose copy is unidentified, broken, bound or `no_trade` fails the whole shop, as `vending_openvending`. A price above `vending_max_value` is cut to it. With `vending_over_max` off, a shop whose sales would take the seller past MAX_ZENY does not open.
- A purchase checks as `vending_purchasereq`: the shop's ID, the range of 14 cells, the zeny of both, the weight, the cart's amounts and the free slots. The vend hook may refuse it. Then the buyer's zeny, the tax and the cart's items move in one transfer: the seller gets the price less `vending_tax` hundredths of a percent when the purchase is at least `vending_tax_min`, as `vending_calc_tax`, and the tax goes to the end `fee`.
- The commit holds both characters, the cart and the store's row. A shop that sold out closes.
- A vendor or a buying store owner cannot walk. A map change or a logout closes the store.

### Buying stores

They follow `buyingstore.cpp`:

- `player:open_buying_store` opens the setup with up to 5 slots. A store buys items of rAthena's `BuyingStore` flag that the owner holds one of already, at 1 to 99,990,000 zeny each, up to 9,999 with what it holds, and up to a zeny limit of its zeny. A store whose items would take the owner to 90% weight does not open.
- A sale checks as `buyingstore_trade`: the seller's slots, bound, `no_trade` and carded copies, the amounts, the owner's weight and the zeny limit. The buying_store_sale hook may refuse it. Then the items and the zeny move in one transfer.
- The store closes once it bought every line, or spent its limit.

### Autotrade

- `player:autotrade()` closes the connection with `DISCONNECT_REASON_AUTOTRADE` and keeps the player and its store. The player's messages go nowhere, and the gateway's Leave saves it and keeps it in the zone.
- The store's row has `autotrade` set. At start, with a database and `autotrade_restore` on, each autotrade store's owner logs in without a connection and opens the store again, as `do_init_vending_autotrade`. A vending line names its cart stack by serial, and a line whose stack is gone is left out. A store with no line left does not open, and its owner leaves.
- The autotrader leaves when its store closes or sells out. A login of the character ends the autotrade first, and waits until its last save went out.

### Store search

It follows `searchstore.cpp`:

- `player:search_stores({ uses, effect })` opens the window with `uses` searches. `effect` is `"show"`, which shows a result's place, or `"open"`, which opens the store, as `SEARCHSTORE_EFFECT_REMOTE`.
- The world's market registry holds a copy of every open store, which each zone updates after a change. A search reads every map, as rAthena's search without a map. It finds up to `searchstore_maxresults` lines, with `searchstore_querydelay` seconds between searches.
- Results filter by items, cards, price, and the ranges of rolls and random options (strategy section 5.4). An unidentified copy matches no roll range.
- A store opened from a search allows a purchase or a sale without the range check, as `searchstore_queryremote`, only on the searcher's map.

### Rules and limits

- New limits: `mail_items` (base 5, ceiling 20) and `mailbox` (base 100, ceiling 1,000).
- New rules with rAthena's values: `vending_tax` (500), `vending_tax_min` (100,000,000), `vending_max_value` (1,000,000,000), `vending_over_max` (yes), `min_npc_vendchat_distance` (3), `autotrade_restore` (yes, feature.autotrade), `searchstore_querydelay` (10), `searchstore_maxresults` (30), `mail_zeny_fee` (2), `mail_attachment_price` (2,500), `mail_attachment_weight` (2,000), `mail_daily_count` (100), `mail_delay` (1,000), `mail_return_days` (15), `mail_delete_days` (15) and `mail_return_empty` (yes). The mail days are char_athena.conf's values, not the code's defaults of 14.
- Each has a behavior test (`TestEveryLimitAndRuleTested`).

### Persistence

Migration 00020:

- `mod_ids` takes the kind `currency`.
- `character_currencies` and `account_currencies`, and `zeny_log.currency`.
- `shop_stock`.
- `mails` and `mail_items`. The serial key of `mail_items` is checked at the end of each transaction, as the other item tables.
- `stores` and `store_items`, for autotrade. At start, the stores that were not on autotrade are deleted.
- `characters.mail_day` and `mail_sent`.

Every new row saves in the commit of the transfers that changed it, with the characters and the log lines, in one transaction (decision 0017). Mail expiry commits its own writes and log lines the same way.

### Lua API

Review decision 30, as built:

- `player:get_currency`, `give_currency` and `take_currency`, with `currencies.modify` for another mod's currency, core's too.
- `player:send_mail(spec)`, with `mail.send`, and `player:open_mail()`.
- `player:autotrade()`, `player:search_stores(spec)` and `player:open_buying_store(spec)`.
- `world.get_shop(name)`, which `world:get_shop(name)` also calls, with `add_item`, `remove_item` and `set_stock`, which need `shops.modify`.
- The events `shop_buy`, `shop_sell`, `vend`, `buying_store_sale` and `mail`, and the hooks `vend`, `buying_store_sale` and `mail`, which need `rules.override`.
- The limits `mail_items` and `mailbox` in `rules.limits`.
- `wa.testing` gained `world:buy`, `world:get_mails` and `world:vend`, provisional.

### Converter

- A new extract, `shops.json` (rAthena fork commit `3e45adbd3`), holds the barter shops of `npc/barters.yml` with their imports (114 in renewal, none in pre-renewal), the items of the `BuyingStore` flag (3,551 in renewal, 1,520 in pre-renewal), and the vending, store search and mail settings of rAthena's conf files. `TestMarketRulesMatchRathena` checks the rules' defaults against them.
- `shop` lines keep the list form. itemshop, pointshop, cashshop, marketshop and barter lines become the table form without a sell tab: an itemshop's cost is its item, a pointshop's its variable as a currency (`#CASHPOINTS` is `cash`, `#KAFRAPOINTS` is `kafra`, and another variable a new core currency from ID 3), a cashshop's `cash`, a marketshop's zeny with its stock, and a barter's zeny and items with its stock.
- In renewal the 45 marketshops now convert, and 103 of 114 barters fully and 6 partly. 62 barter items take refined items and 5 repeat an item, which the table form cannot hold, and 1 takes 6 kinds of items. Script duplicates placed go from 889 to 913. One script NPC fewer converts, since `callshop` now refuses a shop that did not convert.
- The commands `mail`, `openmail`, `searchstores`, `buyingstore`, `npcshopadditem`, `npcshopdelitem` and `npcshopupdate` convert, and so do reads and writes of currency variables. proposal 0002, next step 16, lists what does not.

### Tests

- Zone tests for each rule and limit, package tests for the shop, mail, market, currency and store packages, and Lua tests for the calls and their permissions.
- e2e scenarios for shops, currencies, mail, expiry, vending, autotrade, store search and buying stores, with PostgreSQL variants.
- Duplication tests close connections in the middle of vending sales, buying store sales, shop purchases and mail sends and takes, with and without PostgreSQL. The audit counts the items and zeny of mails, those still on their way to a mailbox too. Totals and serials held.
- The duplication test found that a mail's items showed to the player as inventory changes. `Zone.apply` now tells a player only the changes of its own inventory.

## Parity

The rules come from reading rAthena's `mail.cpp`, `int_mail.cpp`, `clif.cpp`, `vending.cpp`, `buyingstore.cpp`, `searchstore.cpp` and `npc.cpp` at the pinned commit, and each is a zone or package test. The barter shops, the `BuyingStore` flags and the settings come from a new extract, `shops.json`, of the rAthena fork (see the converter section). No new recording: a sale or a mail has no random part to compare.

## Other choices

- A sent mail stays in a short list for 5 minutes when its recipient's mailbox was not in memory, so a login that loaded just before the mail's commit still sees it.
- The store search opens a store only on the searcher's map. A purchase across zones would move two players' items on two goroutines, which transfers do not do.
- `player:send_mail` returns nothing. A mail to a character who is not online is found in the database after the call returns, so the call cannot tell whether the name exists. A name without a character is logged.
- The `BuyingStore` flag has no field in `data/items.yml` yet, so mod items cannot be bought by buying stores.
- `world:get_shop` changes hold for the world until the server stops. A stock that `set_stock` sets saves; an item `add_item` adds does not, as `npcshopadditem` for a normal shop.
- Script mail goes through the transfer log as items made from the end `script`, so the duplication audit counts them.

## Left

- Cash purchases with real money, auctions, a stock limit per player and costs priced by a Lua function (review decision 30, "Later").
- Opening a store from a search across maps.
- Returning a mail by hand, which RODEX clients after 2020 do not send, and rAthena does not support for them.
- Expanding `mail_items` and `mailbox` from scripts, and a field for the buying store flag in `data/items.yml`.
- Store search for one map (`searchstores`'s map argument), and the remote open's coordinates marker on the minimap.
- Autotrade's `at_timeout`, `at_mapflag` and sitting autotraders: wAthena has no sitting yet.
- The fork's `parse_shop` needs a comma after a shop line's first item, where rAthena's `sscanf` does not, so the `Pet_Items` shop is missing from `npcs.json`. A fix needs the full extract run again.
