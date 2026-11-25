# Outcome: navigation

## Built

Nothing of the brief existed before: no `server/navi`, no `<NAVI>` token, and 82 renewal `navigateto` rows were refused. Branch `feat/navigation`, decision record docs/decisions/0079-client-navigation.md, review decision 68 in proposal 0002 (API additions). Migration number not used (next is still 00044). Spent: one session, about one active day.

Server:
- `server/navi/classic_test.go` (package `navi_test`): routes over the real warps of both modes (`world.LoadWarps`): prontera to yuno (at least 2 legs, each leg's warp leads to the next map), prontera to prt_fild08 (1 leg), and an unreachable map.
- `server/navi/navi.go`: `Graph` (`Add`, `Route`, `Len`), `Edge`, `Step`, `Service` (`Warp`, `Airship`, `Kafra`, `Scroll`), `Services` (`AllowAirship`, `AllowKafra`, `AllowScroll`), `FromFlag` (rAthena's `NAV_*` decimal digits: ones airship, tens scroll, hundreds Kafra), `Asset`, `FromRecord`. Breadth first search, fewest map changes, edges in the order added.
- `server/zone/navi.go`: per-zone atomic copy of the visible, non-self warps (`setNaviEdges`, called from `SetWarps`), `naviGraph` (all zones through the optional `Zones()` of the world, plus `Zone.NaviExtra`), `Session.Navi`, `naviCommand` (one request a second, counted on the zone clock, a map that does not run is ignored, cancel), `sendNavi`, `scriptHost.Navigate`. The player's target (`Player.navi`) moves in the warp `handoff`; the new zone sends a route and drops the target on the target's map. Hooks in `zone.go` (command field, `Player` fields, join) and `warp.go` (`handoff.navi`, `SetWarps`).
- `server/modscript/navi.go`: `NaviHost`, `NaviTarget`, `player_navigate`. `prelude.lua`: `player:navigate_to(map, { x, y, monster, services, hide_window })`, defaults 101 and hidden window true. `luaapi/spec.go`: function, `wa.NavigateOptions`, constants module `wa.navigation` (`server/constants`: prefix `NAV_`). Generated docs regenerated.
- `server/mod/navi.go` (`NaviKind`, `data/navi.yml`), `server/gamedata/navi.go` (`modNavi`, `Mods.Navi`), `server/cmd/waserver/main.go` sets `z.NaviExtra`. Core ships no `data/navi.yml`.
- Protocol: `NaviRoute` event 138, `NaviCommand` command 34, `NaviStep`; gateway case (`MaxNaviMap` 64, `MaxNaviCell` 65535 (uint16), services at most 255, bad values are acknowledged and dropped) and rate kind `kindNpc`. Go and TS generated code regenerated with buf v1.73.0. Numbers are the next free; the contract says they move with merge order.

Converter:
- `tools/convert/npcnavi.go`: `navigateto` in both modes. Optional arguments stay out of the table; the flag becomes `navigation.<name>` (or the number); a seventh argument (a character ID) is refused, as are a non-text map and a wrong argument count. Hooked in `npcexpr.go` and `reserved` in `npclua.go`. `F_Navi` was converted whole already; its markup now renders.
- Reports regenerated (npc-scripts-renewal, classic-coverage; pre-renewal unchanged): `navigateto` 82 rows, 78 now convert, 4 stop on other commands; renewal placed script NPCs 6874 to 6952 (not placed 2345 to 2267); classic-coverage placed 13634 to 13721 of 19892.

Client (`client/src/viewer`):
- `navi.ts`: `parseNaviInfo`, `naviMaps`, `naviNext`, `naviAngle`, `naviSummary`, `NaviWindow` (summary, maps of the route, an SVG arrow turned toward the next warp, Cancel; compact when `hide_window`; closes 5 s after arrival).
- `dialog.ts`: the `<NAVI>name<INFO>map,x,y,services,monster,hide</INFO></NAVI>` token, a button link; an unreadable link shows its name. `DialogActions.navi`.
- `net/connection.ts` (`navi`, `naviCancel`), `online.ts` (`onNaviRoute`), `main.ts` (window, 500 ms arrow timer), `viewer.css`.

Docs: mods/README.md (`navigate_to` row, a `data/navi.yml` section), tools/convert/README.md, proposal 0002 review decision 68, decision 0079.

## Proof

Not a bug-fix node.

## Deviations

- Files outside the brief's owned and shared paths, kept minimal: `server/gateway/gateway.go` and `limits.go` (the command case), `server/gamedata/mods.go`, new `server/gamedata/navi.go` and test, new `server/mod/navi.go`, `server/constants/constants.go` (the `navigation` module), `server/cmd/waserver/main.go` (one line), new `tools/convert/npcnavi.go`, `npc_navi_test.go`, a field in `npc_runtime_test.go`, `npclua.go` (reserved name), `client/src/viewer/main.ts`, `viewer.css`, `online.ts`, `mods/README.md`, `tools/convert/README.md`. `server/zone/**`, `server/modscript/**`, `schema/**`, `client/src/net/**` and `dialog.ts` are in the brief's shared list.
- The contract says "`data/navi.yml` optional"; it is built as a mod kind (`mod.NaviKind`) with no record naming rule beyond `LocalNames`. The contract did not name the file's fields; they are `from`, `to`, `x`, `y`, `to_x`, `to_y`, `service`.
- The client has no minimap, so the "arrow on the minimap and marker on the target" of proposal 0006 is an arrow in the navigation window, and the target is named in its summary. The minimap marker comes with a minimap.
- Event 138 as proposal 0006 section 14 says (116 is InstanceEnded of proposal 0003). Command 34 is the next free; whoever merges later renumbers it.
- Warps hidden (`Hidden`) are not edges: a hidden warp is a secret passage.
- No change request: the contract was right.

## Stubs left

- None. `z.achieve` is not touched.

## Gotchas

- Route legs by map changes, not by walking distance: an arrival cell is not measured against the next warp's cell.
- The graph is rebuilt from all zones for each request. About 4,000 edges for 988 maps, at most one request a second for each player; cache it if scripts call `navigate_to` in a loop.
- The `<INFO>` field order (services, monster, hide) is proposal 0006's reading; the fork has no kRO client to check it against. Only `parseNaviInfo` changes if a capture says otherwise. The guides' `0,000,0` means services 0, monster 0, window shown.
- A guide's link carries services 0, so a click routes over warps only. Scripts calling `navigateto` without a flag get `NAV_KAFRA_AND_AIRSHIP`, which routes over warps too until a mod adds `data/navi.yml` trips.
- `TestRunConvertedNPCs` runs every converted NPC against `npcHost`, so any host used for it must implement `modscript.NaviHost` (`npc_navi_test.go` does it for `npcHost`).
- Procedural worlds: `navigate_to` returns false because no map has a classic name. A mod's own map names work.
- Cells of `NaviRoute` steps are the warp's centre cell, not its touch area's edge.

## Proposed learnings

- A new `player:` call that needs the zone goes in a small host interface of its own (`NaviHost`, as `MarketHost`), and every test host that runs converted NPCs must implement it | the Lua runtime raises "does not exist here" when the host lacks the interface, and converted scripts then fail in `TestRunConvertedNPCs` | tools/convert, server/modscript | yes

## Verification

- `go vet` on server/navi, mod, gamedata, constants, gateway, modscript, zone, cmd and tools/convert: clean.
- `go test -p 1 -parallel 2 -count=1`: ./server/navi ./server/mod ./server/constants ./server/gateway ./server/modscript/... ./schema/... (532 passed), ./server/gamedata ./server/world ./server/cmd/... (96 passed), ./server/zone with `-run 'Navi|Warp|Handoff|Arrival|CommandsGoTo|ChatAndView|WalkOnto'` (22 passed) and `-run 'NPC|Script|API|Persist|Social|Party|Mail|Quest|Hidden|Guild'` (148 passed), ./tools/convert with `-run 'TestRunConvertedNPCs'` (3 passed), `-run 'NPC|Npc|Shop|Present|Group3|Quest|Report|Golden|Names|Events|Timers|World|Touch|Rules|Chat|Party|Companion|Players|Storage|Skill|Decision'` (42 passed, includes the stale-report tests) and the new `TestNavigatetoConverts`, `TestRunNavigateto`.
- `golangci-lint run` on the touched packages: no issues (v2.15.0 installed; check.sh pins 2.14.0).
- Client: `pnpm check`, `pnpm lint` (warnings only, none new), `pnpm format:check`, `pnpm test` (623 passed), `pnpm build`.
- buf lint and generate (v1.73.0): clean, generated files committed. `TestGeneratedFiles` (Lua API docs) passes after `-update`.
- Not run: `scripts/check.sh quick queries` (the orchestrator runs it), the full zone, tools/convert and tools/modkit suites, security, fuzz, browsers.

## Size

- Estimated M, actual M. The graph and converter were small; the protocol, gateway, handoff, mod kind, generated docs and client took the rest.
- Risk estimated low, measured low. The new code runs only on a request or a script call; the shared hooks are a field in the command, `Player` and `handoff` structs, and one line in each of `SetWarps` and join.
