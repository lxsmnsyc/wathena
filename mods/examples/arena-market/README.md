# Arena Market

A sample mod for currencies, shops, mail and vending, proposal 0002, review decision 30 (mods/README.md, "Mail, vending, shops and currencies").

- `data/currencies.yml` adds Arena Tokens, a currency each character has, up to 1000. Its name has no prefix, so it is `example.arena:tokens`.
- `data/items.yml` adds the Arena Trophy.
- `data/npcs.yml` adds the Arena Master in Prontera and the Prize Desk, a shop that only scripts open. A Red Potion costs 20 zeny and 2 tokens. A Fly Wing costs 5 Jellopy and 1 token, and the desk has 3, which every player shares and which fill again each day.
- `scripts/npc/arena_master.lua` is the Arena Master. "Claim a match prize" gives 5 tokens with `player:give_currency`. "Send me a trophy" takes 10 tokens with `player:take_currency` and mails a trophy with `player:send_mail`. "The prize desk" opens the shop.
- `scripts/main.lua` adds a `vend` hook that refuses a vending sale of a trophy, with a message, and a `vend` event that gives the seller a token for each sale.

The mod changes its own currency without a permission. `player:send_mail` needs `mail.send`, and `hooks.on` needs `rules.override`, which the server owner approves.

Run `wactl mod test mods/examples/arena-market`.
