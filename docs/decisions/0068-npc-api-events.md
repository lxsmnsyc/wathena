# 0068: NPC API Events: Kill Events, Attached Players and Handles Across Maps

- Status: built. Details to review.
- Date: 2026-10-31.
- Code: [server/zone](../../server/zone) (`npcworld.go`, `maps.go`, `touch.go`, `npcs.go`, `npcpresent.go`), [server/modscript](../../server/modscript) (`npcworld.go`, `npcs.go`, `maps.go`, `present.go`, `runtime.go`, `prelude.lua`, `luaapi/spec.go`), [tools/convert](../../tools/convert) (`npcpresent.go`, `npcexpr.go`, `npcscript.go`, `npcworld.go`, `npclua.go`, `rathena.lua`).
- Docs: [lua-api.md](../generated/lua-api.md), [mods/README.md](../../mods/README.md) (`sprite: none`).
- Builds the npc-api-events items of review decision 60 of [proposal 0002](../proposals/0002-lua-api.md): A2, A5, A6, B4, B8, B9, B11, D6, E1, E5, and G15's `getunitdata` and `getunittype`.
- Lua API: adds to API 1.0. Protocol: none. Settings: none. Data: none. Store: none. Permission: none new.

## Context

- Decision 0066 built players and monsters across zones. Review decision 60 accepted the calls and rules converted scripts still needed, and gave these to npc-api-events.

## Decision

### Lua API

- `monster:show_emotion(emotion)` (B9): local, or on another map after the current step, as the other remote monster calls.
- `player:say_aloud(text, { to = "self" })` (B11): only the speaker hears it. A plain value, so a handle of another map takes it.
- `npc:remove_effect(effect, { to })` and `player:remove_effect(effect, { to })` (D6), with `show_effect`'s `to`.
- `monster_id` and `monster_game_id` on a monster label's `wa.NpcEvent` (B4), and nil on other events. They ride with the label's counts, so a label that waits for a talking player still has them.
- `npc:fire_touch(player)` (E1): runs the NPC's touch handlers for a player in its touch area, after the current step and waiting while the player talks. False outside the area; an NPC of another map raises an error.

### Rules

- A broadcast (`world.fire_npc_event`, `::OnX`) starts a zone that has not started only when one of its NPC sources names the label in quotes, without regard to case (A2). `npc:fire_event` to an NPC of such a zone returns the same test. The events still reach the zone either way.
- A player answers a dialog of an NPC with `sprite: none` from any distance, as `npc_checknear` for a class-less NPC (E5).

### Converter

- `killedrid` and `killedgid` read the label's event in the code of an event label; converted event functions take the event as a third argument. Elsewhere they are refused.
- `unitexists` and `getunittype` of the attached player, an NPC of `getnpcid`, or another game ID (`R.unit_exists`, `R.unit_type`, `R.npc_exists`).
- `getunitdata` of a monster fills `UMOB_LEVEL`, `UMOB_HP`, `UMOB_MAXHP`, `UMOB_X`, `UMOB_Y` and `UMOB_CLASS` (`R.unit_data`). Of the attached player or an NPC it is refused.
- `emotion` and `unittalk` (with `bc_self`) on another game ID; `removespecialeffect` with `SELF` and `ALL_SAMEMAP`.
- `OnInterIfInitOnce` runs as `OnInit` once per server start, guarded by `R.once`, a temporary server variable (A5).
- `unloadnpc` removes the NPC's duplicates too (A6), except the NPC that runs the script.
- `R.touch_one` gives an `OnTouch_` dialog to the next player already in the area through `npc:fire_touch` (E1).

## Breaks

- A broadcast no longer starts an unstarted zone whose sources do not name the label, and `fire_event` to such an NPC returns false. Labels built at run time, such as `"OnE" .. i`, no longer start zones.
- `PresentHost.RemoveEffect` takes a send target; `NPCHost` gains `FireTouch`; `FireNPCEventAway` takes the label's monster.
- Converted event functions take a third argument, and `R.touch_one` takes the NPC first.

## Alternatives

- **A player field that keeps the last kill**, as rAthena's `killedrid` param (B4, option 2). More state, and it lags across zones.
- **Start every zone at boot** (A2, option 3). Refused by decision 0062.

## Details to review

- `getunitdata` leaves the other `UMOB_` indexes as they were. Scripts read only the six it fills and `UMOB_TARGETID` (4 lines, in instances).
- `killedrid` inside a `callsub` function of a label is refused: the event is not in scope there.
- `unloadnpc` keeps the running NPC when it is one of the duplicates; rAthena would unload it too.
- `R.once`'s variable names are cut to 64 characters.

## How to check it worked

- `go test ./server/zone -run 'TestMonsterEmotion|TestSayAloudSelf|TestRemoveEffectTo|TestLabelMonster|TestAnswerNoSpriteFromAfar|TestFireTouchOutside|TestTouchDialogConverted'`.
- `go test ./server/world -run 'TestNPCsAcrossZones|TestBroadcastStartsNamingZones'`.
- `go test ./tools/convert -run 'TestDecision60Convert|TestUnloadDuplicates|TestPresentConvert|TestRunConvertedNPCs|TestNPCReportsCurrent'`.
