# Hunting Board

A sample mod with a quest (proposal 0002, review decision 45).

- `data/quests.yml` adds `porings`, the Poring Patrol: three Porings within an hour. While the quest is in a player's log, each Poring the player kills also gives a Jellopy.
- `data/npcs.yml` places the Hunt Master in Prontera in a classic world, and on every town square in a procedural world.
- `scripts/npc/hunt_master.lua` gives the quest, counts the kills left, rewards 500 zeny and an Apple, and takes back a quest whose hour is up. Players within 14 cells see a quest icon over her: "!" while they can take the quest, and "?" once its kills are done.
- `tests/hunting_board_test.lua` takes the quest, kills Porings, collects the reward, and lets the hour run out, in Prontera. Its last case takes the quest on a generated town's square, with a `wactl` built with `-tags procedural`.

Run its tests:

```sh
go run ./tools/wactl mod test mods/examples/hunting-board
```
