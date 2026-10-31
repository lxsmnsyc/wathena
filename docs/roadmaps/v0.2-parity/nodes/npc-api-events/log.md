# Log: npc-api-events

- 2026-10-31: built in the main checkout on feat/npc-api-events, no worktree, no worker. Decision 0068. No migration, proposal or review decision number.
- Review decision 60 is already in proposal 0002 (approved 2026-10-27), so no further amendment, as decision 0066 did for decision 59.
- A2 changed TestNPCsAcrossZones's expected fire_event result to false: its labels are built at run time ("OnE" .. i). The events still arrive.
- E1 changed TestTouchDialogConverted: the player already in the area now gets the dialog.
- A `git checkout -- server/zone/npcs.go` used to prove the E5 test reverted the uncommitted fix; re-applied before the commit.
- Remaining report rows for these commands fail on other commands in the same scripts; only removespecialeffect has its own reason (a computed effect, 1 script).
