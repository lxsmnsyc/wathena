# 0080: Chat rooms: NPC waiting rooms, player rooms, the room channel and the window

- Status: built.
- Date: 2026-11-25.
- Code: [server/chatroom](../../server/chatroom) (`room.go`), [server/zone](../../server/zone) (`chatroom.go`, `zone.go`, `maps.go`, `social.go`), [server/modscript](../../server/modscript) (`chatroom.go`, `prelude.lua`, `luaapi/spec.go`), [server/gateway](../../server/gateway), [server/rules](../../server/rules) (`limits.go`), [server/staff](../../server/staff), [tools/convert](../../tools/convert) (`npcroom.go`, `rathena.lua`), [schema/proto](../../schema/proto/wathena/v1), [client/src/viewer](../../client/src/viewer) (`chatroom.ts`), [e2e](../../e2e) (`chatroom_test.go`).
- Docs: [proposal 0006](../proposals/0006-small-systems.md) section 2, [lua-api.md](../generated/lua-api.md), [npc-scripts-renewal.md](../generated/npc-scripts-renewal.md), [npc-scripts-pre-renewal.md](../generated/npc-scripts-pre-renewal.md), [classic-coverage.md](../generated/classic-coverage.md), [mods/README.md](../../mods/README.md).
- Builds the node chat-rooms of docs/roadmaps/v0.2-parity and the chat room part of the small-systems contract. Review decision 69 of [proposal 0002](../proposals/0002-lua-api.md) records the API additions.
- Lua API: adds to API 1.0. Protocol: adds events 128 to 132 and command 35, and the channel `CHAT_CHANNEL_ROOM`; the version stays 1. Settings: none. Data: none. Store: none.

## Context

47 renewal and 38 pre-renewal script NPCs stopped on `waitingroom`, and `nochat` and `no_chat` were refused with "chat rooms do not exist". rAthena's arenas, the job quests' test rooms and Poring War all use waiting rooms. Proposal 0006 section 2 designed the feature and the maintainer accepted it on 2026-10-27.

## Decision

### The room state

- `server/chatroom.Room` is pure state, with no player or NPC: the members in join order, the kick list of a player room, the limit (cut to 20), the trigger (cut to 127) with its disabled flag, the event, the fee and the level range. `Join` checks in rAthena's order (`chat_joinchat`): busy, in a room or full; password; level; zeny; kick list. An NPC room counts the NPC, so it takes `limit - 1` players, while `Full` (`getwaitingroomstate(32)`) tests `users >= limit`. A disabled trigger never counts as reached (rAthena's bit 0x80, which no user count reaches).
- `Leave` makes the first remaining member of a player room its owner, and tells the zone to delete a room that is empty. An NPC room stays. A kicked player cannot come back to a player room. Handing over swaps the two places.

### The zone

- `server/zone/chatroom.go` keeps `z.rooms`. A player holds its room in `p.room`. The fee of an NPC room is only checked on a join and taken by `warp_members`, as rAthena.
- A member who walks, warps (`depart`, `teleport`), dies or logs out (`leaveMap`) leaves. Walking is decision 10 of proposal 0006's "Decisions in review": the member leaves first and the walk goes on. While in a room the command's use, skill, attack, NPC click, pick up, trade, shop buy and sell, vending and buying store fields are dropped before they run.
- A trigger runs the event on the NPC with no player, after the step (`postNPC`), as `npc_event_do`. The event is `"<NPC's record name>::<label>"`; the room's own NPC is found without the world's directory, others through it. `enable_event` runs the check at once.
- `warp_members(map, x, y, n)` takes the first `n` players in join order (the trigger by default). Each pays the fee and leaves the room, and the loop stops at the first that cannot pay. A map that does not run stops it before anyone pays. `"Random"` is a random cell of the room's map and `"SavePoint"` each player's save point. The map flag `noteleport` of rAthena's save point case does not exist in wAthena, so it is not checked.
- Players near a room get `ChatRoomShow`, sent again when the room changes (each room has a version). Members get `ChatRoomJoined`, `ChatRoomMember` and `ChatRoomResult` at once. A room that goes sends `ChatRoomClear`.
- Creating a player room needs: no room, no store, a map without `nochat`, a cell without `no_chat`, Basic Skill level 4 (`basic_skill_check`), no mute, no NPC within `min_npc_vendchat_distance`, a title of 1 to 36 characters, a password of at most 8 and a limit of 1 to 20. The rules `chat_room_max_users` (20) and `chat_room_title` (36) set the two ceilings. The staff permissions `chat.no_kick` (admin and game_master) stops a kick, as `PC_PERM_NO_CHAT_KICK`, and `chat.join_all` (the same roles) joins a locked room without its password, as `PC_PERM_JOIN_ALL_CHAT`. Dead players may create and join rooms, and `warp_members` warps them, as rAthena (a player leaves its room when it dies, so only a join or a create while dead meets this).
- `nochat` is a map flag that `maps.set_flag` accepts, and `no_chat` a cell kind of `maps.set_cells` and `maps.check_cell`, kept as a cell mark as `no_vending`. The converter stops refusing both.
- A line sent on `CHAT_CHANNEL_ROOM` goes to the members of the speaker's room, except those who block the speaker. It passes the mute, the chat filter and the chat hook, and the `chat` event runs with the channel `"room"`, which counts as private. It is not written to the chat log: the log's kind column has no room kind, and a new kind would need a migration. This means a room's lines cannot be reported.

### Lua and the converter

- `npc:open_chat_room`, `npc:close_chat_room`, `npc:chat_room`, `player.chat_room`, `player.in_chat_room`, `wa.ChatRoom` (fields, `members`, `enable_event`, `disable_event`, `kick`, `kick_all`, `warp_members`), `chat_room_join` and `chat_room_leave`, as proposal 0006 section 12. The permission `chat_rooms.manage` needs approval and is not flagged. A second `open_chat_room` on an NPC with a room logs a warning and returns nil, as rAthena logs and ignores it. `event` takes a label as a string; a function is not taken.
- `chat_room_leave` runs after the step that caused the leave. For a player who warped or logged out, `player` is nil and `name` holds the name.
- The converter turns `waitingroom` into `npc:open_chat_room{...}` and the other commands into helpers of `rathena.lua` that act on the NPC itself or a named one. A limit of 0, which rAthena's PvP squares use, makes a bubble nobody can join. `warpwaitingpc` writes the account IDs to `$@warpwaitingpc[]` and the count to `$@warpwaitingpcnum`. Renewal: `waitingroom` 47 rows stopped, now 35 convert and the other 14 stop on `waitingroom2bg`, which waits for battlegrounds. Pre-renewal: 38 stopped, 26 convert and 14 stop on `waitingroom2bg`. `enablewaitingroomevent` (2 rows stopped) converts, with `disablewaitingroomevent` (25 rows), `warpwaitingpc` (26) and `getwaitingroomstate` (1). Renewal places 6,987 script NPCs of 9,219, up from 6,952, and pre-renewal 3,702 of 4,342, up from 3,676. Of the job change NPCs, renewal places 696 of 710, up from 686, and pre-renewal all 315, up from 308. Classic mode places 13,767 of 19,892 lines, up from 13,721.

### The client

- `chatroom.ts` has the window: the rooms near the player with a join button (and a password field for a locked player room), a form to make a room, and for a member the room with its members, the owner's mark, the room's chat, Leave, and for the owner Change, Hand over and Kick. `RoomBook` holds the data and is tested without a screen. The client has no bubble over heads yet, so the list stands in for it.
- The window's button is added beside the guild button, and the events come through `online.ts` and `connection.ts`.

## Consequences

- `chat_warpportal` is not built: wAthena has no Warp Portal skill unit that a room member could stand on, and a rule needs a behavior test. It comes with the Warp Portal.
- `room.password` reads as empty for a mod without `chat_rooms.manage`.
- A room's chat is not logged and cannot be reported (above).
- `getwaitingroomstate` and the room commands of an NPC on another map answer -1 or do nothing, except `delwaitingroom`, which reaches the NPC's zone after the step.
- `getwaitingroomusers` is not converted, and `waitingroomkick` and `checkchatting` are converted but no shipped script uses them.

## Tests

- `server/chatroom`: the join checks in order, the NPC count, the limit cuts, owner changes, kicks and the kick list, the trigger flags.
- `server/zone`: an NPC room's checks, the trigger with the event and the warp with the fee, enable and disable, kicks, `delwaitingroom` and the removal of an NPC, a player room's create checks (Basic Skill, values, `no_chat` cell, `nochat` map), joins, password, chat, change, hand over, kick and kick list, walking and dying, logout and teleport, and the two rules.
- `tools/convert`: the converted text of every command and its refusals, and a run of the commands against the real runtime with the variables of `warpwaitingpc`; `TestRunConvertedNPCs` starts every converted NPC of both modes with a host that has rooms; the reports are regenerated.
- `e2e`: the Arena of the fixture, an NPC room of 4 with a trigger of 2 and a fee of 10 zeny: two bots join, speak, and are warped to the field with the fee taken.
- `client`: the room texts, the book and the notices.
