# Bank

A sample mod for storage, bound items and trades, proposal 0002, review decision 29 (mods/README.md, "Storage, cart and trade").

- `data/storages.yml` adds the vault, an extra storage of 50 slots that the characters of an account share. Its name has no prefix, so it is `example.bank:vault`.
- `data/items.yml` adds the Bank Card.
- `scripts/npc/banker.lua` is the banker in Prontera. "My storage" opens the account's storage with `player:open_storage()`. "The vault" opens the vault with `player:open_storage("vault")` for players who have a Bank Card, and charges 100 zeny once it opened. "A Bank Card" gives one card with `player:give_item(card, 1, { bound = "account" })`. A bound card cannot be traded, dropped or sold.
- `scripts/main.lua` adds a `trade` hook that refuses a trade when a player gives more than 100,000 zeny, with a message, and a `trade` event that whispers to both players what changed hands.

`player:give_item` needs `player.give_items`, and `hooks.on` needs `rules.override`, which the server owner approves.

Run `wactl mod test mods/examples/bank`.
