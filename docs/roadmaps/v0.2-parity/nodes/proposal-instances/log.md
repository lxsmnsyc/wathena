# Log: proposal-instances

## 2026-10-27

- Branch renamed to `docs/proposal-instances`, reset to a953ea2. Took proposal number 0003. No decision, review decision or migration number used.
- Design only: no builds, tests or check runs.
- Read: GOAL.md, the brief, the contract draft, proposals README, 0002 Change and review decisions 13 to 18, 46, 47, 58 to 60, 0002b category C.
- wAthena code read: server/world/world.go (Add, Swap, Remove, Retire), server/cmd/waserver/main.go (`newZone` factory, map list), worldchange.go (rotation's Apply and Done: Remove, Evacuate, Retire after 30 s), server/zone/zone.go (Run, sleep, Tick 50 ms), worldchange.go (Evacuate, Relocations), maps.go (mapShared, mapZone, map flags: only nomemo built), npcs.go (startNPCs is lazy), npcworld.go (NPCDirectory keeps the first record name), scripts.go (zone.NPC, Lua state per zone), modscript/variables.go (instance scope refused, Temporary store), mod/overlay.go (Kind, local names cannot hold a colon), mod/permission.go, staff/staff.go, rules/limits.go, social (party, guild), luaapi/spec.go, admin.proto (WorldView, ZoneView), session.proto and event.proto (MapChange.map is the asset name), migration 00001 (`characters.map` text up to 64), converter npc.go:553 and npclua.go (duplicates share `.x` through `NpcSource`).
- rAthena read: instance.cpp/.hpp, map_addinstancemap and map_delinstancemap, npc_duplicate4instance, npc_instanceinit, pc.cpp reconnect (`pc_lastpoint_special`), party.cpp and guild.cpp hooks, conf/battle/instance.conf and feature.conf, script.cpp instance buildins, instance_db.yml (78 renewal, 4 pre-renewal records).
- Sampled scripts: Faceworm's Nest in full structure, counts over all of npc/re/instances and npc/instances (72,439 lines). Every `instance_create` uses party mode; `OnInstanceDestroy`, `getinstancevar` and `setinstancevar` are unused; `instance_live_info` only with ILI_NAME.
- Map flags on base maps (npc/re/mapflag, npc/mapflag): noteleport 115, nowarpto, nomemo, nobranch, restricted (mostly 6) 114 each, partylock and monster_noteleport 113.
- Key design choices: one zone per instance map; base maps do not run; registry goroutine builds one instance at a time; ID counter in memory from 1; no saving; copies named `<record>:<id>`; `.x` shared with the base record as rAthena; `'x` in the registry with an explicit instance argument.
- Not measured: Lua state cost per zone (decision 0056 not built). The proposal makes instances-core measure it and set `max_running` from it.
- Wrote docs/proposals/0003-instances.md and filled contracts/instances.md (still draft, v1).
