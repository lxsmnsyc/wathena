# Party Hall

A sample mod for parties, proposal 0002, review decision 27 (mods/README.md, "Parties and friends").

- `scripts/main.lua` listens to local chat. "!party" makes a party with `parties.create`, with the player as its leader, and turns on even exp share with `party:set_options`. "!party size" reads `rules.limits.party_members`.
- The `party_join` and `party_leave` events whisper to the player who joined or left, with the party's name, its leader and the reason.
- The `party_change` event tells the members on the map the party's exp share.

`parties.create` and the party methods that change a party need `party.manage`, which the server owner approves.

Run `wactl mod test mods/examples/party-hall`.
