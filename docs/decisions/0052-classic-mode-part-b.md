# 0052: Classic Mode, Part B

- Status: proposed.
- Date: 2026-10-19.
- Code: [server/modscript](../../server/modscript) (`npcs.go`, `prelude.lua`, `luaapi/spec.go`), [server/zone](../../server/zone) (`touch.go`, `zone.go`, `inventory.go`, `touch_test.go`, `npcs_test.go`), [server/item](../../server/item) (`heal.go`), [tools/convert](../../tools/convert) (`npcscript.go`, `npclua.go`, `npcexpr.go`, `npc.go`, `npcreport.go`, `classicnotes.go`, `rathena.lua`, `npc_touch_test.go`, `npc_test.go`), [tools/wactl](../../tools/wactl) (`convert.go`, `main.go`), [reference/rathena](../../reference/rathena) (`doc.go`, `embed.go`, `embed_gz.go`, `gzfs.go`, `internal/gzextracts`), [e2e](../../e2e) (`server.go`, `kro_test.go`), [client/scripts](../../client/scripts) (`kro-check.ts`, `game-check.ts`, `browser-check.ts`), [scripts/release.sh](../../scripts/release.sh), [scripts/check.sh](../../scripts/check.sh), [Dockerfile](../../Dockerfile).
- Docs: [mods/README.md](../../mods/README.md) ("Touch areas", "Core's NPCs", the player calls), [tools/convert/README.md](../../tools/convert/README.md), [server/README.md](../../server/README.md), [deploy/README.md](../../deploy/README.md) ("Classic mode"), [e2e/README.md](../../e2e/README.md) ("The kRO check"), [docs/generated/classic-coverage.md](../generated/classic-coverage.md), the NPC coverage reports and the API reference in [docs/generated](../generated).
- Phase 8 step 6, part B (strategy section 15, "Phase 8"), review decisions 55 and 56 of [proposal 0002](../proposals/0002-lua-api.md), after part A ([decision 0051](0051-classic-mode-npcs.md)).
- Lua API: the `touch` event's conversation, and negative amounts of `player:heal_percent`, as decision 56 approved. No other change. Protocol: none. Settings: none. New: the build tag `gzextracts` and `wactl convert classic`.

## Context

Part A placed the NPCs of the `quests`, `warps` and `events` folders, and added touch areas, bosses and Kafra pushcart rental. It left three review items, which decision 56 approved: a conversation for touch handlers, a way to take HP and SP, and gzip for the extracts. Decision 55 also asked for a full kRO run, a browser check of the same world, and a list of what does not convert, for the release notes.

## Decision

### Touch with a conversation

The `touch` event gets `(player, conversation)`, as `click` does:

- The zone runs an NPC's touch handlers in a conversation's thread, as a click's. A handler that shows nothing opens no window, as before.
- A touch while the player talks to an NPC waits until the conversation ends, as `npc_event_sub` queues it in `sd->eventqueue`:
  - At most 2 wait (`MAX_EVENTQUEUE`). Later ones are dropped, as rAthena drops them with a warning.
  - The first runs 100 ms after the conversation ended, as `npc_event_dequeue`'s `pc_addeventtimer`. The next one waits for that one's conversation.
  - A waiting touch runs only if the player is still in the area, and on the map, as `npc_event_sub` checks again.
- `untouch` runs at once, with no conversation, as before.
- A warp that a `touch` or `untouch` handler asks for now happens once the handler ran or paused, as in a conversation. In part A it waited until the player's next conversation or item use, so converted warp portals with `OnTouch` did not move the player. `TestTouchWarp` checks it.
- A converted touch label with a dialog converts now. `R.touch_one(fn, true)` marks an `OnTouch_` part with a dialog. When the player it ran for leaves, it does not run for the next player already in the area, who has no conversation. It runs for the next player who comes in. rAthena shows the dialog to the player already in the area (`npc_touchnext_areanpc`).

Coverage, from the reports, before and after:

| Mode | NPCs | Lines | Placed before | Placed after | With a touch area after | With a dialog on touch |
|---|---|---:|---:|---:|---:|---:|
| renewal | Scripts with OnTouch or OnTouch_ | 1,265 | 47 | 329 | 327 | 279 |
| renewal | Lines with OnTouch or OnTouch_, duplicates too | 4,935 | 448 | 795 | 791 | 334 |
| renewal | Script NPCs with a warp sprite, duplicates too | 2,089 | 143 | 259 | 170 | 116 |
| pre-renewal | Scripts with OnTouch or OnTouch_ | 682 | 28 | 210 | 209 | 179 |
| pre-renewal | Lines with OnTouch or OnTouch_, duplicates too | 3,974 | 588 | 828 | 825 | 227 |
| pre-renewal | Script NPCs with a warp sprite, duplicates too | 1,392 | 108 | 160 | 79 | 52 |

- The only touch reason left in the report is `OnTouchNPC` (18 renewal scripts), the label a monster fires. The other touch scripts fail on other commands.
- In all, renewal places 4,957 NPCs of 15,337 lines, up from 4,587, and pre-renewal 3,065 of 8,390, up from 2,805. The renewal job change NPCs of the jobs folders went from 187 to 259 of 710.

### Taking HP and SP

`player:heal_percent` takes a negative amount, as `percentheal`:

- A negative rate takes that percent of the maximum HP or SP, as `status_percent_damage` and `status_percent_change`: -100 or less takes the maximum, and a loss below 1 takes 1. A positive rate beside it still heals.
- HP never goes below 1, as decision 56 says. rAthena kills at -100 (`hp == -100` sets the kill flag). That is the one difference, and it changes the Poison Bottle of item scripts (`percentheal -100,-100` for jobs other than Assassin Cross and Guillotine Cross), which leaves 1 HP instead of killing.
- The loss is passive, as `status_damage`'s flag 1: no damage number, and no status ends. SP stops at 0.
- `item.PercentLoss` computes it, and the zone's `percentLoss` applies it.

The converter now writes Niflheim's `percentheal` of `F_KafStor` and `F_KafEnd` as `player:heal_percent { hp = -50, sp = -50 }` and the like. Every Kafra converts fully:

| Mode | Kafras | Placed | Full before | Full after | Partly before | Partly after |
|---|---:|---:|---:|---:|---:|---:|
| renewal | 55 | 54 | 0 | 54 | 54 | 0 |
| pre-renewal | 43 | 42 | 0 | 42 | 42 | 0 |

### Extract size

The extracts stay JSON in git. The release binaries embed gzipped copies:

- `reference/rathena/embed.go` embeds the JSON files, as before, in a plain build. `go build` and `go test` from a clean checkout need no extra step.
- The build tag `gzextracts` embeds the `gz` folder instead (`embed_gz.go`), and `Files` reads each file through `rathena.GzipFS`, which decompresses it when it is read. A build with the tag fails when the folder is missing.
- `go generate ./reference/rathena` runs `internal/gzextracts`, which writes a gzipped copy of each file that a plain build embeds, so both builds hold the same files. git and the Docker context ignore the folder.
- `scripts/release.sh` and the Dockerfile run `go generate` and build all binaries with the tag. `scripts/check.sh lua` does the same for its release builds, and runs `reference/rathena`'s tests with the tag.
- The 62 embedded files are 93.0 MB of JSON and 11.6 MB of gzip.

Release binaries for darwin/arm64, from `scripts/release.sh darwin/arm64`, before and after:

| Binary | Before | After |
|---|---:|---:|
| waserver | 131,378,450 bytes | 49,313,922 bytes |
| wactl | 131,491,106 bytes | 49,443,890 bytes |
| wabots | 117,325,378 bytes | 35,278,146 bytes |

After the change, `wactl mod validate` and `wactl mod test` of the Lantern sample ran from the release `wactl` outside the repository, so it read the gzipped copies.

### Job_Change

rAthena's `Job_Change` is `jobchange getarg(0),Upper`, which keeps the player's line. The converter refused any second argument but 0, so every job change NPC that calls it, such as the Taekwon job change, stopped at the change with "this NPC needs jobchange". It now converts `jobchange <job>,Upper` in a function:

- A normal player's change is `player:change_job(job, { force = true })`, as `jobchange <job>` was.
- A transcendent or baby player's change fails when it is reached, as before, and the function stays partly converted, with the same reason in the report.

No Lua API changed, and the report's counts did not change. `TestJobChangeUpper` checks it.

### The kRO run

`TestClassicKRO` in e2e is the run of decision 55 ("The kRO check" in e2e/README.md). It runs only when `WATHENA_KRO_ASSETS` names an asset build, and with PostgreSQL:

- `wactl world new --mode classic`, built with the `procedural` tag, makes the world, and `waserver` runs `classic: {start_map: prontera, maps: all}` with `mode: renewal`.
- The bot walks along routes over the build's cells, through rAthena's warps of `warps.json`, in legs short enough for one walk command each.
- The fixture mod's treasure hunt moves to the start map, since such a build has no `e2e_field`.

The full kRO build did not finish. `df` showed 12 GB free before the first attempt. Swap on the same APFS container grew while the build ran, so free space fell faster than the build wrote:

- The first attempt stopped at the 3 GB floor after 53 minutes, with 1.2 GB written.
- The second started at 7.8 GB free, after the partial output was removed, and stopped at the floor after 26 minutes, with 0.65 GB written.

So the run used a smaller build of the same client: the placeholder pack and the `.rsw`, `.gnd` and `.gat` files of `prontera`, `prt_fild08`, `payon` and `payon_in01`, read from `data/kro/client` without changes, as decision 0050's classic run did. That is all the server and the bot read of a map. With `maps: all`, the server ran those maps and the placeholder map. The steps, on PostgreSQL 18:

| Step | Result |
|---|---|
| Start in Prontera | At 156,196. Basic Skill 9, and 20,000 zeny from the fixture. |
| Save at a Kafra | kaf_prontera2: "Your Respawn Point has been saved here in the city of Prontera." |
| Storage at the Kafra | The fee was 40 zeny. 2 Jellopies went into storage, and it closed. |
| Buy from a converted shop | 3 Apples from the Fruit Gardener for 45 zeny. |
| Warp to the field | The warp at prontera 156,22 to prt_fild08. |
| Fight a spawned monster | A Poring of the field's spawns, killed with 56 damage. |
| Walk back | The warp at prt_fild08 170,378 to Prontera. |
| Kafra teleport | "Payon -> 1200 z", to payon 161,58, for 1,200 zeny. |
| Job change | Through the warp at payon 256,285 into payon_in01, three talks with the Phoenix, and the Novice became a Taekwon (4046) at job level 1. |

The run took 146 seconds, and the server logged no warning of the scripts. The job change needed the Job_Change change above: before it, the third talk ended at `Job_Change` with "this NPC needs jobchange".

The renewal first job changers in the towns, Swordman, Mage, Archer, Acolyte, Merchant and Thief, do not convert: they need the `skill` command, which grants quest skills. The Taekwon job change of `npc/re/jobs/1-1e/taekwon.txt` was the only first job change in reach that converts.

### The browser check

`client/scripts/kro-check.ts` is the browser check of the same world, as the `kro` check of browser-check.ts. It runs in Chromium only, and only when `WATHENA_KRO_ASSETS` is set:

```text
WATHENA_KRO_ASSETS=/path/to/build WATHENA_TEST_DATABASE_URL=... CHECKS=kro pnpm check:browsers
```

- `startGame` takes `classic`: a waserver built with the `procedural` tag, no `--map`, and the config's `classic` block. `startSeconds` gives a world of every map more time to start.
- The camera starts far out over a town this size, so the check zooms in with the wheel first. Otherwise a click on the ground lands on an NPC's sprite.

On the same smaller build:

| Check | Result |
|---|---|
| Prontera | `map/prontera` loaded: 5 files, 0.2 MB sent, 4.3 MB decoded, in 1.3 s. |
| Walk | Three walks with clicks on the ground, from 156,196 to 166,200. The camera went from 329.9 to 131.0. |
| Dialog | A click on the Man from Morocc at 163,204 opened his dialog: "[Morocc Traveler] Ah, it's very nice and refreshing to be near this fountain." |
| Errors | No page errors and no server errors. |

The smaller build has no textures, models or sprites of the client, so the page drew the ground with placeholder textures and the lightmaps.

### Release notes

`wactl convert classic --report docs/generated/classic-coverage.md` writes the release notes list from renewal's coverage, and `TestClassicNotesCurrent` checks that it is current:

- A summary: the NPC lines placed, the Kafras and the job change NPCs, and pre-renewal classic as experimental.
- The script NPC lines by folder, and the folders the extract leaves out.
- What blocks the rest: each script NPC that is not placed and is not a duplicate counts once, for the first part of its script that does not convert. The 30 most common reasons, with the most common entry of each. The NPC-to-NPC commands, which stay in phase 10, block 1,114 of the 4,152.
- The systems that are not built: instances, sieges, battlegrounds and the MVP tomb, with the script NPC lines of each.

deploy/README.md has a "Classic mode" section that links it.

## Consequences

- Touch areas with dialogs, such as quest notices and guards, run in classic worlds, and converted warp portals with `OnTouch` move the player.
- Mods' `touch` handlers may talk to the player. Old handlers that take only the player still work.
- The release binaries are smaller. Reading the extracts at start decompresses them first.
- The kRO run and the browser check are opt-in, behind `WATHENA_KRO_ASSETS`, since the build needs the owner's client and about 2.4 GB.

## Review items

- The kRO run and the browser check on the full kRO build, on a machine with the disk for it. Both passed on the smaller build of four maps, which has no textures, models or sprites of the client. [Decision 0053](0053-skill-command.md) ran both on the full build with `--fast`, and both passed.
- `R.touch_one` with a dialog and the next player already in the area. A way to start a conversation with another player, or a touch event the zone fires again, would match rAthena. Either is a Lua API addition.
- rAthena clicks an NPC that has a touch area and no touch label when a player steps into it (`npc_touch_areanpc`'s fallback to `npc_click`). Now that touch has a conversation, the converter could do the same. Decision 56 did not ask for it, so it is not made.
- The renewal first job changers (Swordman, Mage, Archer, Acolyte, Merchant and Thief) do not convert: they need the `skill` command, which grants quest skills. It is a Lua API addition. Review decision 57 approved `player:give_skill`, and [decision 0053](0053-skill-command.md) built it: all but the Thief convert now.
- `percentheal -100` kills in rAthena and leaves 1 HP in wAthena, which changes the Poison Bottle.
- A converted NPC without a sprite (-1) gets `HIDDEN_NPC`, a sprite of class 111. rAthena lets a player answer such an NPC from anywhere (`npc_checknear`). wAthena checks 15 cells, so a touch area wider than that with a dialog may refuse answers from its edge.
