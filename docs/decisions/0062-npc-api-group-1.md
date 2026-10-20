# 0062: NPC API Group 1: Lookups, Visibility, Events and Variables

- Status: built. Details to review.
- Date: 2026-10-20.
- Code: [server/modscript](../../server/modscript) (`npcworld.go`, `npcs.go`, `runtime.go`, `host.go`, `variables.go`, `party.go`, `prelude.lua`, `luaapi/spec.go`), [server/zone](../../server/zone) (`npcworld.go`, `npcs.go`, `scripts.go`, `touch.go`, `zone.go`, `monster.go`, `partyscripts.go`), [server/world](../../server/world) (`world.go`), [server/variables](../../server/variables) (`temporary.go`, `server.go`), [server/mod](../../server/mod) (`npcs.go`, `permission.go`), [server/gamedata](../../server/gamedata) (`mods.go`), [server/social](../../server/social) (`party.go`), [server/cmd/waserver](../../server/cmd/waserver) (`main.go`), [tools/convert](../../tools/convert) (`npcworld.go`, `npcscript.go`, `npclua.go`, `npcexpr.go`, `npc.go`, `npcparty.go`, `rathena.lua`), [tools/modkit](../../tools/modkit) (`test.go`, `validate.go`).
- Docs: [mods/README.md](../../mods/README.md), [tools/convert/README.md](../../tools/convert/README.md), [server/README.md](../../server/README.md), [lua-api.md](../generated/lua-api.md).
- Builds review decision 58 of [proposal 0002](../proposals/0002-lua-api.md), areas B1 to B4 of [proposal 0002b](../proposals/0002b-npc-api.md). Areas B5 to B10 come later.
- Lua API: adds to API 1.0. Protocol: none. Settings: none. Data: `data/npcs.yml` takes `state`, `sprite: none` and records without a map. Permission: `npcs.control`.
- Number: 0055 to 0061 were taken, so this record is 0062.

## Context

- rAthena NPCs call each other: `donpcevent`, `enablenpc` and its aliases, `getvariableofnpc`, and floating NPCs without a map. The converter could not map them, and they blocked most scripts that did not convert (proposal 0002b).
- wAthena runs one Lua state per zone. An NPC of another map lives in another zone, so calls between NPCs cross zones.
- Review decision 58 approved the B batch, and decided:
  - An NPC event goes to the target NPC's zone as a message and runs after the current step.
  - NPC variables live in the world service, as server variables do.
  - `$@` variables are server-wide values that are not saved.
  - Core has no limit of 256 variables per mod.
  - `atcommand` and `setbattleflag` are refused.
  - `npc.full_name` fixes `strnpcinfo(0)`.

## Decision

### B1: lookups

- `world.get_npc(name)`: an NPC of any map, or one without a map, by record name. A plain name is the mod's record, then core's (as `world.get_shop`). Case counts, as `npc_name2id`. nil for none, or a removed NPC. `game.read`.
- New `wa.Npc` fields: `full_name` (strnpcinfo 0), `hidden_name` (2), `unique_name` (3), and `state`. `map` is "" for an NPC without a map, as `strnpcinfo(4)` gives for `nd->m < 0`.
- `full_name` is `name`, then `#` and the part after `#` of the record's name. A converted NPC whose rAthena name had a `::` unique name loses its `#` part, as its record has only the unique name (328 renewal lines).
- `maps.find_player(name)` and `maps.find_player_by_id(id)`: an online player of the context's map. By ID: the account ID, then the character ID, as `convertpcinfo`'s `map_id2sd` and `map_charid2sd`.
- `party:get_member_list()`: `{ name, character_id, account_id, online }` for every member, from the social registry.

### B2: visibility and state

- `npc:show()`, `npc:hide()`, `npc:set_hidden(hidden)`, `npc:set_cloaked(cloaked, player?)`, `npc:is_cloaked(player?)`, `npc:remove()`, and `npc.state`.
- The zone keeps rAthena's flags (`npc_enable_target`, npc.cpp 1036): `is_invisible` for disabled, `OPTION_HIDE`, `OPTION_CLOAK`, and the characters whose cloak is the opposite, as the player's `cloaked_npc` list. A change for everyone clears the list for the players on the NPC's map (`npc_cloaked_sub`).
- Disabled: no view, no click, no touch (npc.cpp 1897), no player events and no events with a player (npc.cpp 1767). Events without a player still run (`npc_event_do_sub`).
- Hidden: no view, no click (npc.cpp 2225). The touch area works.
- Cloaked for a player: no view and no touch for that player. Monsters do not touch a cloaked NPC (npc.cpp 2007).
- `show`, `set_hidden(false)` and an uncloak fire the touch event of the players standing in the area who are not in it yet (`npc_enable_sub`).
- `remove` is `unloadnpc`: the NPC leaves the map and the directory, its NPC variables go, and its context ends after the current step, so a script that removes its own NPC finishes its call.
- `npcs.yml`: `state: disabled | hidden | cloaked`, and `sprite: none` for rAthena's view -1.
- A mod's own NPCs need no permission. Another mod's need `npcs.control`, which needs the owner's approval.

### B3: events between NPCs

- `events.on("npc_event", handler, { name = "OnEnable" })` in NPC contexts. The label matches without case. The handler gets `{ name, player, conversation }`.
- `npc:fire_event(name)` is `donpcevent`. `npc:fire_event(name, { player = p })` is `doevent`: the handlers run in a conversation with the player, in the player's event queue while it talks, as touches do. The player must be on the NPC's map.
- `world.fire_npc_event(name)` is `donpcevent "::OnX"`: every NPC with the label, duplicates too (`npc_event_doall_sub`). Without `npcs.control` it reaches the mod's own NPCs.
- `login`, `logout`, `map_enter`, `base_level_up` and `job_level_up` now run in NPC contexts too. An NPC hears the players of its own map, and a disabled NPC hears none.
- `monster_touch`: a monster's step into an NPC's touch area, as `npc_touch_areanpc2`. Once per NPC until the monster leaves the areas or walks into another NPC's (unit.cpp 693).
- A record with `script` and no `map` or `place` is an NPC without a map. It runs in the start map's zone, as decision 16 does for world events, which starts its NPC scripts with the zone. It has no view, click or touch, and says nothing aloud.

### Calls across zones

- The world keeps an `NPCDirectory`: each NPC's zone, handle, names, cell, a copy of its view state, and the labels of its `npc_event` handlers. `world.New`, `Add`, `Swap` and `Remove` keep the zones in it. A record name that two zones have, such as an NPC placed at every place of a tag, finds the start map's first, then the others by map name.
- A handle of an NPC of another zone is a number from 2^41 of the calling zone. Its fields come from the directory. Its changes and events are messages. Its other methods raise "the NPC is on another map".
- Each zone has an inbox: a slice under a lock, and a wake channel. Posting never waits, so two zones never wait on each other. The zone's loop runs the inbox with its script events, in `runScripts`. An event sent by the same zone runs once the sending call returned, in the same pass.
- Order: messages from one zone run in the order sent. A control message and a later event from the same zone keep their order, so `hide` then `fire_event("OnReport")` reports `disabled`.
- Limits: at most 4 rounds of 4,096 messages per pass of `runScripts`, so an NPC that sends events to itself in a loop cannot hold its zone. An inbox holds at most 65,536 messages, and a full one drops a message with a warning.
- A message starts the NPC scripts of a zone that has not started them (the lazy start of decision 0056's context). A zone counts as started once its NPCs registered their labels.
- `fire_event` returns whether the NPC has a handler of the label: from the runtime in the same zone, and from the directory for another zone, true while that zone has not started. `fire_npc_event` counts its own zone's NPCs from the runtime and the others from the directory.

### B4: variables

- `variables.npc_integer`, `npc_boolean`, `npc_string` and `npc_table`, with `get(npc)`, `set(npc, value)` and `add(npc, amount)`. Keyed by the declaring mod, the NPC's full record name and the variable's name. Not saved.
- `variables.server_temporary_integer`, `_boolean`, `_string` and `_table`: one value for the world, not saved.
- Both live in `variables.Temporary`, which every zone shares, as `variables.Server`, under a mutex. `add` is one step under the lock. A runtime without one keeps its own.
- Core has no limit of 256 declarations per mod, and no limit of 256 values for its server, temporary server and NPC variables. Mods keep both. The byte limits stay for every mod.

### Refused

- `atcommand` and `setbattleflag` do not convert. The report names review decision 58.

### Converter

- Every label that starts with `On` is an `npc_event` handler, as rAthena's `ev_db` lists them, so `donpcevent` reaches `OnInit`, clock and timer labels too. `OnInit` runs through its function at the end of setup. An `OnInit` that waits or talks does not convert.
- The labels rAthena runs for the players of every map (`OnPCLoginEvent` and the others) do not convert: an NPC context hears only its own map.
- `OnTouchNPC` is a `monster_touch` handler too.
- `donpcevent`, `doevent`, `cmdothernpc`, the eight view commands, `isnpccloaked`, `unloadnpc`, `getvariableofnpc` and `set getvariableofnpc`, `strnpcinfo(0)` to `(4)`, `$@` variables and `setd`/`getd` of them, `getcharid(0|3, name)`, `convertpcinfo`, `getpartymember` types 1 and 2, and a `monster` whose label is another NPC's. tools/convert/README.md has each form.
- Before converting, the converter reads every script for the `.x` that `getvariableofnpc` reaches. Those become NPC variables in their NPC's own script.
- A script NPC without a map is placed. A `DISABLED`, `HIDDEN` or `CLOAKED` line gets `state`. View -1 is `sprite: none`.
- Assignments now convert a number into a string variable as its digits, and a string into a number variable as `conv_num`, which scripts newly placed needed.

## Coverage

Script NPC lines of `docs/generated/npc-scripts-<mode>.md`:

| Mode | Lines | Before | After |
|---|---:|---:|---:|
| renewal, not duplicates | 7,067 | 3,458 (48.9%) | 4,560 (64.5%) |
| renewal, with duplicates | 14,865 | 6,700 (45.1%) | 9,617 (64.7%) |
| pre-renewal, not duplicates | 3,754 | 2,061 (54.9%) | 2,827 (75.3%) |
| pre-renewal, with duplicates | 8,205 | 4,399 (53.6%) | 6,460 (78.7%) |

## Breaks

- Converted NPCs with a view of -1 and a touch area have `sprite: none` in place of `HIDDEN_NPC`. Both show nothing.
- `OnInit` is now a local function of the setup function, which setup calls last.
- An NPC context can listen to the player events, so a mod NPC that registered `login` before got an error and now gets the event.

## Alternatives

- **Remote player handles.** A floating NPC's `OnPCLoginEvent` needs the players of every map. A handle that acts in another zone needs every player call to become a message. Decision 58 did not decide it. Not built: the player events hear the NPC's own map, and the converter refuses those labels.
- **Run events at once.** rAthena's `donpcevent` runs its label before it returns. Decision 58 chose a message that runs after the current step, which is the same in every zone.
- **Start every zone's NPCs at boot.** It matches rAthena's `OnInit` at load, and gives exact counts. It makes a Lua state in every zone with NPCs. Not chosen: a message starts the zone instead.
- **Key NPC variables by the source of duplicates.** rAthena shares `.x` between a source and its duplicates. Converted duplicates already run their own setup with their own locals. Kept the same for NPC variables.

## Details to review

- The player events in NPC contexts hear only the NPC's map. rAthena's `OnPCLoginEvent` and the other labels hear every map, so the converter refuses them (one renewal script, `poring_war.txt`).
- `maps.find_player` finds only players of the context's map, for the same reason.
- `fire_event` and `fire_npc_event` give the world's knowledge, not what ran: an event to a zone that has not started counts as handled.
- Events between NPCs do not check a mod's CPU budget, as clicks and touches do not.
- A broadcast reaches every zone with script NPCs that has not started, which starts their Lua states. With core's NPCs, a floating NPC's `OnInit` that sends `::OnX` starts most towns at boot (decision 0056 measures state costs).
- A converted NPC that names an NPC that did not convert finds nothing, and its command does nothing, as rAthena does for a missing NPC.
- `npc:remove()` removes one NPC. `unloadnpc` of an NPC with duplicates, which rAthena removes together, does not convert.
- Not built from the B1 to B4 lists: `getnpcid` (its commands are B5), `mobcount` of another NPC's label (it needs the other NPC's spawn list), `awake`, `attachrid` and `sc_start2` on another character (instance and remote-player scripts), and `requestguildinfo`.
- `npc_event`'s `event.name` is in lower case.

## How to check it worked

- `go test -race ./server/world -run TestNPCsAcrossZones`: a floating NPC in the start zone sends 50 events, an NPC variable change, a `hide` and a report to an NPC of another zone that has not started, and `::OnAll` to every NPC. The events arrive in order, the report sees `disabled` and 150 hits, and the unstarted zones get the broadcast.
- `go test ./server/zone -run 'TestNPC|TestMonsterTouch'`: order within a zone, `doevent` in a conversation and in the event queue, the view states and per-player cloaks, removal, the floating NPC, permissions and `monster_touch`.
- `go test ./server/modscript -run 'TestCoreHasNoVariableLimit|TestServerTemporaryVariables|TestNPCEventOptions'` and `go test -race ./server/variables`.
- `go test ./tools/convert`: `TestNPCWorldConvert`, `TestRunNPCWorld`, and `TestRunConvertedNPCs`, which now fires every event label of every placed NPC.
