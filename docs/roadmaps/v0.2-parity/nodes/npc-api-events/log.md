# Log: npc-api-events

- 2026-10-31: built in the main checkout on feat/npc-api-events, no worktree, no worker. Decision 0068. No migration, proposal or review decision number.
- Review decision 60 is already in proposal 0002 (approved 2026-10-27), so no further amendment, as decision 0066 did for decision 59.
- A2 changed TestNPCsAcrossZones's expected fire_event result to false: its labels are built at run time ("OnE" .. i). The events still arrive.
- E1 changed TestTouchDialogConverted: the player already in the area now gets the dialog.
- A `git checkout -- server/zone/npcs.go` used to prove the E5 test reverted the uncommitted fix; re-applied before the commit.
- Remaining report rows for these commands fail on other commands in the same scripts; only removespecialeffect has its own reason (a computed effect, 1 script).
- 2026-10-31, stopped before the PR. Code, docs and the fresh review's fixes are committed (last commit 04700d7, docs(zone) label source cache). The branch is not pushed. Left to do:
  1. One full `scripts/check.sh quick queries` on the final commit, with its own log file and `WATHENA_TEST_DATABASE_URL` set (container `wa-castles-pg` in colima). The first run was killed by low system memory (8 GB RAM) while another session ran `go test ./...`. Run it when no other session runs tests, or run the slow packages one at a time (`go test -p 1 ./server/zone`, `./tools/convert`, `./tools/modkit`), then lint and queries. The review already passed; do not review again.
  2. Push feat/npc-api-events, open the PR "feat: NPC API: kill events, attached players and handles across maps" against main, mark it ready, and hand it over with `gh pr review <n> --approve`.
  3. After it merges, the brief's status change to done rides on the next node's branch.
