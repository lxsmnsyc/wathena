# Outcome: chat-rooms

## Built

Nothing of the brief existed before: no `server/chatroom`, no room command converted (`waitingroom` stopped 47 renewal and 38 pre-renewal rows, `enablewaitingroomevent` 2 each), and `nochat` and `no_chat` were refused. Branch `feat/chat-rooms`, decision record docs/decisions/0080-chat-rooms.md, review decision 69 in proposal 0002. No migration (rooms are not saved; next is still 00044). Spent: one session, about one active day.

Numbers used: decision 0080, review decision 69, event 128 to 132 (`ChatRoomShow`, `ChatRoomClear`, `ChatRoomJoined`, `ChatRoomMember`, `ChatRoomResult`; proposal 0006's table, 138 is navigation's), command 35 (`ChatRoomCommand`; 34 is navigation's), channel `CHAT_CHANNEL_ROOM` 9. Numbers move with merge order, as the contract says.

Server:
- `server/chatroom/room.go`: pure `Room` state: members in join order, the kick list, `Join` with rAthena's check order (`Full`, `WrongPassword`, `LevelLow`, `LevelHigh`, `NoZeny`, `Kicked`), the NPC counted as a user (limit - 1 players, while `Full()` is `users >= limit`), `Leave` (owner passes to the first member, empty player room deleted, NPC room stays), `Kick`, `Transfer`, `Change`, trigger and its disabled flag (`ShouldRun`), `NextID`.
- `server/zone/chatroom.go`: `z.rooms`, `p.room`, `roomEvents` (a `ChatRoomShow` per room in view, sent again when its version changes, `ChatRoomClear` when it goes), commands (`chatRoomCommand`: create, join, leave, change, owner, kick), the trigger (`roomTrigger`, the event goes to the NPC with no player through `postNPC`), `roomGuard` (walk leaves; items, skills, attack, NPC click, pick up, trade, shops and stores are dropped), room chat (`chatRoom` kind in `social.go`, not logged), and the script host (`OpenChatRoom`, `CloseChatRoom`, `NPCChatRoom`, `ChatRoomField`, `ChatRoomMembers`, `ChatRoomEvent`, `ChatRoomKick`, `ChatRoomKickAll`, `ChatRoomWarp`, `OpenPlayerRoom`). Hooks: `leaveMap`, `depart`, `teleport`, `die`, `removeNPC`, `handle`, `update`, `scripts.go` (`chat_room` and `in_chat_room` player fields), `maps.go` (`nochat` flag accepted, `no_chat` cell mark, refusals removed), `headless.go` (`OpenChatRoom` for tests).
- `server/modscript/chatroom.go` (`ChatRoomHost`, `RoomSpec`, `ChatRoomEvent`), `prelude.lua` (`npc:open_chat_room`, `close_chat_room`, `chat_room`, `wa.ChatRoom`, `world:open_chat_room`, the event object; `check` moved to the top of the file), `luaapi/spec.go`, `runtime.go` (host functions, events `chat_room_join` and `chat_room_leave`), `chat.go` (channel `"room"` is private), generated Lua docs regenerated.
- `server/mod/permission.go`: `chat_rooms.manage` (approval, not flagged). `server/rules/limits.go`: `chat_room_max_users` (20), `chat_room_title` (36). `server/staff/staff.go`: permission `chat.no_kick` and `chat.join_all` (admin: `chat.*`, game_master: both). Gateway: the command case (sizes checked), the room channel, rate kind `kindSocial`.
- Protocol: `ChatRoomCommand` (create, join, leave, change, owner, kick), the five events, `CHAT_CHANNEL_ROOM`; Go and TS code regenerated with `buf generate`.

Converter (`tools/convert/npcroom.go`, `rathena.lua`): `waitingroom`, `delwaitingroom`, `kickwaitingroomall`, `waitingroomkick`, `enablewaitingroomevent`, `disablewaitingroomevent`, `warpwaitingpc`, `getwaitingroomstate`, `checkchatting`. Reports regenerated:
- renewal `waitingroom` rows: 47 stopped before; now 49 rows, 35 convert, 14 stop on `waitingroom2bg` (battlegrounds, v0.3.0). `enablewaitingroomevent` 2 stopped, now 25 convert; `disablewaitingroomevent` 25 convert; `warpwaitingpc` 26; `getwaitingroomstate` 1. Script NPCs placed 6,952 to 6,987 of 9,219.
- pre-renewal `waitingroom`: 38 stopped before; now 40 rows, 26 convert, 14 stop on `waitingroom2bg`. `enablewaitingroomevent` 22, `warpwaitingpc` 22. Script NPCs placed 3,676 to 3,702 of 4,342. All 315 job change NPCs are placed (308 before).
- classic-coverage: 13,721 to 13,767 of 19,892 lines placed.

Client (`client/src/viewer/chatroom.ts`, wired in `main.ts`, `online.ts`, `net/connection.ts`, `viewer.css`): the Rooms window lists rooms near the player with Join (and a password field for a locked player room), makes a room, and for a member shows members with the owner's mark, the room's chat, Leave, and for the owner Change, Hand over and Kick. `RoomBook` and the text helpers are tested without a screen.

Docs: decision 0080, proposal 0002 review decision 69, mods/README.md ("Chat rooms" and the permission), tools/convert/README.md.

## Proof

Not a bug-fix node.

## Deviations

- Files outside the brief's owned and shared paths, kept minimal: `server/mod/permission.go`, `server/mod/ruleset_limits_test.go` (two names in `boundTests`), `server/staff/staff.go` (the `chat.no_kick` permission), `server/gateway/gateway.go` and `limits.go` (the command and channel cases), `server/modscript/runtime_test.go` (the event list in an expected error), `server/zone/headless.go`, `tools/modkit/test.go` and `modkit_test.go` (`world:open_chat_room`), `tools/botclient/chatroom.go` (new: the join and chat commands of a bot), `e2e/chatroom_test.go`, `e2e/server.go` (one approval), `e2e/testdata/mods/fixture` (the Arena NPC and its script, the permission), `mods/README.md`, `tools/convert/README.md`, `tools/convert/npc_runtime_test.go` (a field), new `tools/convert/npc_room_test.go`. The brief asks for an e2e test, which needs the bot and the fixture. `server/zone/**`, `server/modscript/**`, `server/rules/**`, `server/chat/**` (unused), `schema/**`, `client/src/net/**` and `tools/convert/npc*.go` are in the shared list.
- `chat_warpportal` of the contract is not built: wAthena has no Warp Portal unit to consume it, and `TestEveryLimitAndRuleTested` demands a behavior test for each rule. Add it with the Warp Portal skill. The other two rules (`chat_room_max_users`, `chat_room_title`) are built.
- A room's chat is not written to the chat log, so it cannot be reported: the log's `kind` column is a check constraint without a room kind, and the brief expects no migration.
- No room bubble over heads: the client has no place for it. The window's list of nearby rooms stands in for it. The server sends the `ChatRoomShow` data a bubble needs.
- `event` of `open_chat_room` takes a label as a string, not a function (proposal 0006 says "or a function").
- `getwaitingroomusers` is not converted (no script uses it).
- Client window code has no DOM test: the client's tests run without a DOM. The model (`RoomBook`) and the texts are tested; `tsc` covers the window.
- No change request: the contract was right, except `chat_warpportal` above.

- After review: dead players may create and join rooms and are warped by `warp_members`, as rAthena; no refusal for an open trade (rAthena has none); `room.password` is empty for mods without `chat_rooms.manage`; `roomEvents` uses a sorted ID slice kept per zone.

## Stubs left

- None. `z.achieve` is not touched. The contract's trigger site "chat room" (`AG_CHATTING_*`) is for the achievements node: join and create are in `joinPlayer` and `createPlayerRoom` of `server/zone/chatroom.go`.

## Gotchas

- The NPC counts: `limit` 4 takes 3 players. `getwaitingroomstate(32)` is `users >= limit`, so a room that refuses joins is not "full" until a player room would be.
- `enablewaitingroomevent` runs the trigger at once, and a disabled trigger sets the flag only; the converted `warpwaitingpc` warps `trigger` players by default.
- The event of a trigger runs after the step, in the NPC's zone; the leaves of a warp run after the warp. `chat_room_leave` for a player who warped has `player` nil and `name` set.
- `warp_members` leaves the room, pays and sets the player's warp; the warp itself runs at the end of the step (`z.later`). A map that does not run stops the loop before anyone pays. `noteleport` is not checked for `SavePoint`: the flag does not exist.
- A converted event name is a record name: `"core:Lv50 Waiting Room::OnStartArena"`. A name of the room's own NPC is found without the world directory.
- A walk command from a member makes it leave and the walk goes on, so the first step of a player room's owner closes the room if nobody else is in.
- Reads and `kick`/`enable` of an NPC on another map do nothing or answer -1: only `delwaitingroom` reaches another zone.
- Room IDs are per zone. A player's `chat_room` is nil on another map.
- Any test host that runs converted NPCs needs `modscript.ChatRoomHost` now (`npc_room_test.go` gives `npcHost` one).
- Most arenas stay unplaced where their maps are PvP maps: PvP is not ported. Their `waitingroom` call converts.

## Proposed learnings

- A chat or channel kind that a mod can hear must be listed as a private chat kind in `modscript.IsPrivateChat` | a room's lines would reach every mod with `events.on("chat")` otherwise | server/modscript | yes
- A new rule needs a behavior test named in `boundTests`, so a rule with no consumer cannot land | `chat_warpportal` had none | server/rules, server/mod | yes
- A script event that names the player of a leave or a warp must not hand out the player's handle when the player may be gone by the time it runs | a handle of a player that left the map raises on every read | server/modscript | no

## Verification

- `go build ./...`, `go vet` on server/zone, server/modscript/..., server/chatroom, tools/convert, tools/modkit, tools/botclient, e2e, server/gateway: clean. `gofmt -l`: clean.
- `go test -p 1 -parallel 2 -count=1`: ./server/chatroom ./server/rules ./server/staff ./server/mod ./server/gateway ./server/modscript/... ./schema/... ./server/constants ./server/gamedata ./server/cmd/... (632 passed); ./server/zone with `-run 'Room|Navi|Warp|Handoff|Arrival|Chat|Social|Persist|Map|Cell|Flag|Hidden|Guild|Party|Mail|Quest|Vending|Trade'` (118) and `-run 'NPC|Script|API|Companion|Event|Timer|World|Present|Touch|Rule|Limits|Gm|Staff|Death|Die|Join|Logout|Sanction|Mute'` (159); ./tools/convert with `-run 'TestRunConvertedNPCs|TestNPCReportsCurrent|TestClassicNotesCurrent'` and with `-run 'NPC|Npc|Shop|Present|Group3|Quest|Report|Golden|Names|Events|Timers|World|Touch|Rules|Chat|Party|Companion|Players|Storage|Skill|Decision|Navi|WaitingRoom'` (46), including `TestWaitingRoomConverts` and `TestRunWaitingRoom`; ./tools/modkit with `-run 'TestWorldMethods|TestNewModPasses|TestValidate'` (a new case for `world:open_chat_room`).
- e2e: `go test -count=1 -run TestNPCChatRoomWarps ./e2e` passes (no database needed): two bots join the fixture's Arena, one speaks in the room, the second join warps both to the field and takes 10 zeny from each.
- `golangci-lint run` on the touched packages: no issues of this change; one pre-existing `errcheck` at `e2e/server.go` (`<-exited`).
- Client: `pnpm check`, `pnpm lint` (warnings only, none new), `pnpm format:check`, `pnpm test` (632 passed), `pnpm build`.
- buf lint and generate: clean, generated files committed. `TestGeneratedFiles` passes after `-update`.
- Not run: `scripts/check.sh quick queries` (the orchestrator runs it), the full zone, tools/convert and tools/modkit suites, security, fuzz, browsers.

## Size

- Estimated M, actual L: the room had to reach the converter, the Lua API, the protocol, the gateway, the zone, the client, the test world and the e2e fixture.
- Risk estimated low, measured medium. The new code runs on a room call, but the guard in `handle` and the hooks in `leaveMap`, `depart`, `teleport` and `die` run for every command and move. They return at once for a player without a room.
