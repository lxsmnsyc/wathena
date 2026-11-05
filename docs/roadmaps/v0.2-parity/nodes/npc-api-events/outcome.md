# Outcome: npc-api-events

## Built

- Lua: `monster:show_emotion`, `"self"` for `player:say_aloud`'s `to`, `{ to }` on `npc:remove_effect` and `player:remove_effect`, `monster_id` and `monster_game_id` on a monster label's `wa.NpcEvent`, `npc:fire_touch(player)`.
- Zone rules: a broadcast starts an unstarted zone only when its NPC sources name the label (A2); answers to `sprite: none` NPCs skip the range check (E5).
- Converter: `killedrid`, `killedgid`, `unitexists`, `getunittype`, `getunitdata` (monsters), `emotion` and `unittalk` `bc_self` on other game IDs, `removespecialeffect` `SELF`/`ALL_SAMEMAP`, `OnInterIfInitOnce`, `unloadnpc` with duplicates, `R.touch_one` with `npc:fire_touch`.
- Decision record: docs/decisions/0068-npc-api-events.md. Reports regenerated.

## Deviations

- mods/README.md gains the `sprite: none` answer rule; the brief lists no docs outside generated files.
- `unloadnpc` keeps the running NPC when it is one of the duplicates.
- `getunitdata` fills six `UMOB_` indexes and leaves the rest, instead of refusing the others: which index a script reads is not known where the command converts.

## Stubs left

- None.

## Gotchas

- `npc:fire_event` to an NPC of an unstarted zone now returns false unless the zone's sources name the label in quotes. Labels built at run time do not count.
- Converted event functions take `(player, conversation, event)`; the touch event passes its own third argument, which `killedrid` reads as 0.
- `OnGuildBreak` and the `OnAgit*` labels are still system labels that the converter refuses (tools/convert/npcworld.go `systemLabels`). The castle registry sends `OnGuildBreak` (decision 0067), so converted castle NPCs need that label: sieges-war or sieges-content.

## Proposed learnings

- Never `git checkout -- <file>` to prove a test fails; use `git stash push <file>` | it drops the uncommitted fix | everywhere | no

## Verification

- Targeted tests: see decision 0068, "How to check it worked". All pass.
- `scripts/check.sh quick queries`: see the PR.

## Size

- Estimated L, actual L.
