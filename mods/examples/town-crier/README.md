# Town Crier

A sample mod for chat, proposal 0002, review decision 26 (mods/README.md, "Chat").

- `scripts/main.lua` makes the `#events` channel with `channels.create`. It only takes players the mod adds.
- A chat hook stops lines with links. Hooks only compute, so it cannot tell the speaker. It turns the word "lol" into "(laughs)".
- A chat event, narrowed to local chat, answers "!events": the mod adds the player to `#events`, whispers a welcome, and says a line in the channel.
- A `restart_planned` event tells `#events` when staff plan a restart, or cancel one. Its test plans a restart with `world:plan_restart` (decision 0036).
- `data/chat_filter.yml` adds "grief" to the server's chat filter, which replaces it before the hook runs.

The hook needs `rules.override`, which the server owner approves.

Run `wactl mod test mods/examples/town-crier`.
