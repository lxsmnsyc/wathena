# 0018: Chat and Moderation

- Status: proposed.
- Date: 2026-10-03.
- Code: [server/chat](../../server/chat), [server/zone](../../server/zone) (`social.go`), [server/gateway](../../server/gateway), [server/modscript](../../server/modscript) (`chat.go`), [server/store](../../server/store) (`chat.go`, migration 00016), [server/cmd/waserver](../../server/cmd/waserver) (`moderation.go`), [tools/convert](../../tools/convert) (`npcexpr.go`), [client](../../client) (`chatTabs.ts`, `chatCommands.ts`, `chatNotices.ts`, `blockList.ts`), [e2e](../../e2e) (`chat_test.go`), [mods/examples/town-crier](../../mods/examples/town-crier).
- Phase 5, step 2. Builds on strategy sections 6.8, 7.10 and 11.3, and proposal 0002, review decision 26.

## Decision

### One hub per world

- The package `chat` has a hub that every zone of the process shares. It knows the online players by name, with their block lists, and runs the channels.
- A zone hands the hub a player when it logs in, moves it when it changes maps, and takes it out when it logs out. A newer login of the same name replaces the older one.
- The hub hands lines to the zone that holds a player through the player's session. The hand-off never blocks: a zone that is far behind drops the line, as it drops other messages.
- A whisper is decided in the sender's zone, with the hub locked: the target is online or not, and blocks the sender or not. The sender gets the answer in the same frame. This matches rAthena's `clif_wis_end` codes: sent, not online and ignored.
- Several processes need the hub to go through PostgreSQL or another bus. That is left to the multi-process deployment.

### Lines

Every line a player says goes through these steps in the sender's zone:

1. The gateway's rate limit. The limits are server settings. A character made less than an hour ago has lower ones: 3 lines at once and 1 every 3 seconds, against 5 and 1 per second. A dropped line comes back as `ChatRefused` with `TOO_FAST`. rAthena drops it without a word, but the client cannot know a new character's limits.
2. The mute. A muted player's line comes back as `ChatRefused` with a `SanctionNotice`. rAthena drops it without a word.
3. The chat filter of `data/chat_filter.yml`: replace turns a word into asterisks, reject refuses the line and tells the speaker. Words match alone by default, so a short word does not hit longer words. `match: part` finds them inside words.
4. The `chat` hooks. A hook returns the text, or nil to stop the line. A stopped line is not shown or logged, and the speaker is not told, since hooks only compute.
5. Delivery, then the chat log, then the `chat` event.

Each line a player says gets an ID, a UUID of version 7, which the protocol sends with the line so a player can report it.

### Channels

- Core's channels are rAthena's defaults from `conf/channels.conf`: `#global`, `#support`, `#trade` and `#map`, with rAthena's aliases, colors, delay and leave options. Review decision 26 named `#main` as an example. rAthena has no `#main`, so wAthena keeps rAthena's names.
- `#map` is one channel per map, as `CHAN_TYPE_MAP`. A member hears the members on its own map, and leaves it on a map change, as `pc_setpos`.
- A whisper to `#name` speaks in the channel, and joins it first when anyone may join, as `clif_parse_WisMessage`.
- A mod's channel is one for the whole world. Each zone runs the mod's setup, so `channels.create` from the same mod in another zone, or after a reload, returns the same channel and adds an owner. The channel ends with its last owner, and its members leave it. Another mod's name, or core's, raises an error.
- A mod changes core's channel records by name, and makes its own channels with `channels.create`. Mod records of new channels are refused.

### Blocking

- The block list holds names, at most 20, as rAthena's `MAX_IGNORE_LIST`. Names compare without regard to case.
- rAthena keeps the list for the session. wAthena saves it with the character, as section 7.10 asks.
- A blocked name cannot whisper, and its local and channel lines are hidden. Trade and party invites check the list once they exist.
- Like rAthena's ignore list, any valid name can be blocked, even one that does not exist.

### Sanctions

- Mutes and jails have a reason, a start and an end on the server clock, and who gave them. A new one replaces one of its kind that has not ended. The character saves its active sanctions. The `sanctions` table keeps every one, with the time a sanction was lifted early, for the staff tools.
- A jail sends the player to the jail map, `sec_pri` at one of `pc_jail`'s two cells by default, and keeps where it was. A jailed player cannot use warp portals, script warps or wings, and comes back in jail after death. When the jail ends, the player goes back. A jail map that does not run leaves the player in place, still unable to warp.
- Sanctions end within a second of their time, while the zone runs.
- Bans are a staff tool, not Lua API, as review decision 26 says. There is no `unjail` call yet.

### Reports and the chat log

- Each player keeps the lines it saw in the last 5 minutes: local lines in range, whispers to and from it, and channel lines. A report takes them. A report of a line must name a line the player saw, and its speaker.
- A player may send 10 reports an hour.
- With a database, one worker writes lines and reports in batches, in the order they came, so a report's lines are in `chat_log` before the report. `report_lines` ties a report to them. Lines the queue dropped are left out of the report.
- The worker deletes lines older than `chat.log_days` every 12 hours, except lines of open reports. A closed report's lines then go with the others. This is the retention of section 7.10.
- Without a database, the last 10,000 lines and 1,000 reports stay in memory, and `--dev`'s `GET /debug/wathena/reports` shows them.

### Testing

- `--dev`'s `POST /debug/wathena/clock` moves the server clock forward, so the end-to-end tests end a mute without waiting.
- `wa.testing` gets `world:chat` and `world:get_chat`, so mod tests can say lines and read what players got. They are provisional and wait for review.

## Why a hub in the process

| | One hub per process | A zone asks every zone |
|---|---|---|
| Whisper answer | At once, in the sender's zone. | Waits for the target's zone. |
| Block lists | One copy per online player, in the hub. | Only in the target's zone, so the answer is async. |
| Channels | One member list. | One list per zone, kept in step. |
| Several processes | Needs a bus later. | Needs a bus later too. |

## Other choices

- The chat event's `channel` is `"public"`, `"whisper"`, `"party"`, `"guild"` or the channel's name without its `#`. Those four words are not channel names. `to` is the whisper's target name, since the target may be on another map, where a handle cannot point.
- `channel:get_members()` returns names, since members may be on other maps.
- `player:whisper` does not pass the filter or the hooks, and blocks do not stop it: the mod chose the text. It is not logged.
- `channels.create` and the channel methods need no permission, `channels.get` and `get_members` need `game.read`, and `player:is_blocking` needs `game.read`.
- The protocol adds `CHAT_CHANNEL_NAMED` for channel lines, and keeps `CHAT_CHANNEL_WORLD` unused.
- The converter maps `channel_chat`, `channel_join`, `channel_kick` and `channel_create` in their forms that the API covers, for the attached player. It fixes `message`, which sent to the attached player whatever name it named. The converted NPCs use none of them, so the coverage reports did not change.

## Left

- Party and guild chat (steps 3 and 4).
- Reports of guild names and emblems, the staff queue in the admin tools, bans, and closing reports.
- Block everyone, as rAthena's `/exall`.
- The hub across processes.
- Players' own channels, as rAthena's `@channel create`, channel passwords, bans, kicks and options.
- Data export of a player's own chat lines (section 7.10, "Player rights").
