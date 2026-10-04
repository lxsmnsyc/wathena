# Guild Hall

A sample mod for guilds, proposal 0002, review decision 28 (mods/README.md, "Guilds").

- `scripts/main.lua` listens to local chat. "!guild" makes a guild with `guilds.create`, with the player as its master. It asks no Emperium, so the mod decides the cost. "!notice <text>" sets the guild notice for the master with `guild:set_notice`. "!guild size" reads `rules.limits.guild_members`.
- The `guild_join` and `guild_leave` events whisper to the player who joined or left, with the guild's name, the player's position, the master and the reason.
- The `guild_change` event tells the members on the map about a new level, notice, master or emblem.

`guilds.create` and the guild methods that change a guild need `guild.manage`, which the server owner approves.

Run `wactl mod test mods/examples/guild-hall`.
