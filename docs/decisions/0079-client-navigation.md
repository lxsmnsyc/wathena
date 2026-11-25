# 0079: Client navigation: routes over warps, the NaviRoute message and the NAVI link

- Status: built.
- Date: 2026-11-25.
- Code: [server/navi](../../server/navi) (`navi.go`), [server/zone](../../server/zone) (`navi.go`, `zone.go`, `warp.go`), [server/modscript](../../server/modscript) (`navi.go`, `prelude.lua`, `luaapi/spec.go`), [server/gamedata](../../server/gamedata) (`navi.go`), [server/mod](../../server/mod) (`navi.go`), [server/gateway](../../server/gateway), [server/constants](../../server/constants), [tools/convert](../../tools/convert) (`npcnavi.go`), [schema/proto](../../schema/proto/wathena/v1), [client/src/viewer](../../client/src/viewer) (`navi.ts`, `dialog.ts`).
- Docs: [proposal 0006](../proposals/0006-small-systems.md) section 5, [lua-api.md](../generated/lua-api.md), [npc-scripts-renewal.md](../generated/npc-scripts-renewal.md), [classic-coverage.md](../generated/classic-coverage.md).
- Builds the node navigation of docs/roadmaps/v0.2-parity and the navigation part of the small-systems contract. Review decision 68 of [proposal 0002](../proposals/0002-lua-api.md) records the API additions.
- Lua API: adds to API 1.0. Protocol: adds `NaviRoute` (event 138) and `NaviCommand` (command 34); the version stays 1. Settings: none. Data: optional `data/navi.yml`. Store: none.

## Context

`navigateto` blocked 82 renewal NPC scripts, and the converted guides send `<NAVI>[name]<INFO>map,x,y,0,000,0</INFO></NAVI>` in dialog text, which the client showed as raw markup. Proposal 0006 section 5 designed the feature and the maintainer accepted it on 2026-10-27 (decisions 3 and 7 of "Decisions in review").

## Decision

### The graph and the route

- `server/navi.Graph` has one node per map and one edge per way out: `Edge{From, To, X, Y, ToX, ToY, Service}`. `Route(from, to, services)` is a breadth first search that returns the legs, each the cell to leave a map at, with the fewest map changes. Edges of one map keep the order they were added in, so the first of two parallel edges wins and a route is the same each time. A route from a map to itself has no legs.
- The edges are the visible warps of every zone that leave their map, and the extra edges of `data/navi.yml`. A hidden warp is a secret passage and is left out. Each zone keeps its edges in an atomic copy (set in `SetWarps`), because the graph is made from all zones and zones run on their own goroutines. The graph is made for each request (about 4,000 edges for 988 maps); requests are limited to one a second for each player.
- `services` is rAthena's `NAV_*` value, which holds one service in each decimal digit: ones for the airship, tens for scrolls, hundreds for Kafra. A route uses an edge of a service when its digit is not 0. Warps are always allowed. The flag of the converted guides is 0, so a guide's route walks.
- `data/navi.yml` (`mod.NaviKind`, records of `from`, `to`, `x`, `y`, `to_x`, `to_y` and `service` of `airship`, `kafra` or `scroll`) is read by `gamedata.modNavi` and set on each zone as `NaviExtra`. Core ships none, as decision 7 says: the airships and Kafra teleports are scripts in rAthena, not data. A record that breaks a rule is an error that names it.

### Lua and the converter

- `player:navigate_to(map, { x, y, monster, services, hide_window })` needs no permission. The defaults are rAthena's: no cell, `NAV_KAFRA_AND_AIRSHIP` (101) and a hidden window. It returns false when no map of the world has the name, which is every classic map name in a procedural world. The constants are `wa.navigation` (`none`, `airship_only`, `scroll_only`, `airship_and_scroll`, `kafra_only`, `kafra_and_airship`, `kafra_and_scroll`, `all`), read from the extract like the other constant modules.
- `navigateto "map"{,x,y,flag,hide,monster}` converts in both modes. A script that leaves an argument out leaves it out of the table. A call with the seventh argument, a character ID, does not convert. 78 of the 82 renewal rows convert; the other 4 stop on other commands. Pre-renewal has no `navigateto`.
- `F_Navi` was converted whole before. Its markup now renders.

### Protocol and server

- `NaviRoute` (event 138): the target (`map`, `x`, `y`, `monster`, `services`, `hide_window`), `found`, the `steps` and `cancel`. A target with no route comes with `found` unset, so the client shows the target alone. A route with no steps means the player is on the target's map.
- `NaviCommand` (command 34): the same target and `cancel`. The gateway drops values out of range (a map name over 64 bytes, a cell over 65,535, services over 255) with an acknowledgement, and the zone ignores a map that does not run and any request within a second of the last. Numbers are the next free at build time, as the contract says; they move with merge order.
- The player's target lives on the player and goes with it in the handoff of a warp. The new zone sends a route from its map, and drops the target when the player is on the target's map. A cancel drops it at once. Nothing is saved: a logout ends the navigation.

### The client

- `dialog.ts` reads `<NAVI>name<INFO>map,x,y,services,monster,hide</INFO></NAVI>` as a link that a click sends as a `NaviCommand`. The three trailing fields are read as `services`, `monster` and `hide`, as proposal 0006 says; the fork does not have the kRO client's format to check them against. A link that does not read (a bad map name, a cell or flag out of range) shows as its name, so it never shows raw markup. Link text is set with `textContent` only.
- `navi.ts` is the window: a summary line, the maps of the route, an arrow that turns toward the next warp (or the target's cell on its map) as the player walks, and Cancel. The client has no minimap yet, so the arrow stands in for the minimap's marker. `hide_window` keeps the window to the summary and the arrow. A route that ends on the player's map closes after five seconds.

## Consequences

- Routes over the warp graph only: airships, Kafra teleports and scrolls need a mod's `data/navi.yml`.
- A route is by fewest map changes, not by walking distance, since a warp's arrival cell and the next warp's cell are not measured.
- The client does not draw a path on the map. The marker on a minimap comes with the minimap.
- The format of the `<INFO>` fields is as proposal 0006 reads them. If a kRO capture shows another order, only `parseNaviInfo` changes.

## Tests

- `server/navi`: routes over a graph with all service kinds, the flag digits, records.
- `server/gamedata`: `data/navi.yml` reads, and a bad record names itself.
- `server/zone`: a route over the warps with a hidden warp skipped, the one-second limit, a map that does not run, a map with no route, cancel, the route made again on a warp to another map and dropped on arrival, the script call and the extra edges, and a world that cannot list its zones.
- `tools/convert`: the converted text of `navigateto` with its refusals, and a run of three calls against the real runtime.
- `client`: the link parser, the route helpers and the dialog's `<NAVI>` token.
