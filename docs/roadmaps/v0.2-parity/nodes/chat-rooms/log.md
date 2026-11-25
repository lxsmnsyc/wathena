# chat-rooms log

- Started 2026-11-25 on feat/chat-rooms. Nothing existed: no server/chatroom, no room commands converted (waitingroom 47 renewal / 38 pre-renewal rows blocked, enablewaitingroomevent 2/2), nochat and no_chat refused.
- Read rAthena chat.cpp and script.cpp (fork): NPC users do not count the NPC; join test is users+1 >= limit; trigger bit 0x80 is the disable flag.
- server/chatroom, protocol (events 128-132, command 35, CHAT_CHANNEL_ROOM 9), zone/chatroom.go, modscript/chatroom.go, prelude, spec, gateway, rules, staff catalog (chat.no_kick), permission chat_rooms.manage done; zone room tests pass. chat_warpportal dropped: no Warp Portal unit to consume it (mod test needs a behavior test per rule).
- converter (npcroom.go, R helpers), reports regenerated: waitingroom 47 -> 35 converted renewal (14 left are waitingroom2bg scripts), pre-renewal 38 -> 26; TestRunConvertedNPCs passes; lua-api docs regenerated.
- client chatroom.ts, wiring in main.ts, online.ts, connection.ts; vitest, check, lint, format, build pass. DOM tests not possible: client has no DOM test env, so window code is covered by the pure-model tests and tsc.
- e2e TestNPCChatRoomWarps passes; docs: decision 0080, review decision 69, mods/README, convert README.
