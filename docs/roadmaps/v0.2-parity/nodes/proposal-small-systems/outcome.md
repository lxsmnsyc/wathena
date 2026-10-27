# Outcome: proposal-small-systems

## Built

- docs/proposals/0006-small-systems.md: design proposal 0006 (Problem, Change sections 1 to 23, Breaks, Alternatives, 14 questions for review).
- docs/roadmaps/v0.2-parity/contracts/small-systems.md: Interface, Behavior, Stub and Contract tests filled. Still `status: draft`, version 1, for the orchestrator to freeze after the maintainer accepts.
- Numbers: proposal 0006. Migrations named 00048 (`character_partners`), 00049 (`character_achievements`, `characters.title`), 00050 (`enchant_grade`, `enchants` on four item tables), after 0005's 00047; numbers are taken in merge order. Protocol events start at 128. No decision or review decision number used.
- No builds, tests or Docker. Read-only greps and reads of the repo and of the rAthena fork (branch wathena-reference, untouched).

## Deviations

- None from the brief. The proposal proposes node changes (splits, owned paths, scope cuts); the orchestrator owns the briefs.

## Stubs left

- `z.achieve` as a no-op in `server/zone/achieve.go` (the contract's Stub), added by the first node that needs it.

## Gotchas for each downstream node

For chat-rooms:

- An NPC room counts the NPC: it takes `limit - 1` players, but `getwaitingroomstate(32)` tests `users >= limit`. `enablewaitingroomevent` runs the trigger at once. `warpwaitingpc` writes account IDs.
- The room event is an NPC label with no player (`world.fire_npc_event`).
- Lift the refusals of `nochat` and `no_chat` in server/zone/maps.go and tools/convert/npcmaps.go.
- Most arenas and PvP rooms stay unplaced: PvP and battlegrounds are not ported.

For marriage:

- Rows are tied; insert both in one transaction. Divorce must remove rings 2634 and 2635 offline too.
- The five skills come from the rings' item scripts; `disable_call` does not exist.
- `marriage.txt` has a VIP branch (`VIP_SCRIPT` is 0 in the extract).
- No adoption: the baby classes and skills are v0.3.0.

For navigation:

- Guides already emit `<NAVI>` markup; check the trailing `<INFO>` fields against the kRO format, which the fork lacks.
- Route data is warp NPCs only; classic worlds only.

For achievements:

- 15 renewal records (`Eat`, `Chatting`) can never complete, as in rAthena.
- Titles at login include unclaimed completions. Item rewards are mail from "GM".
- Grep scripts with `-a`: quests_16_2.txt has non-UTF-8 bytes.
- Conditions use `ARGn`, `Class`, `BaseLevel`, `JobLevel`, `readparam`; the 93 renewal conditions and 51 reward scripts must convert.

For ui-windows:

- Both modes use the refine window; pre-renewal has Normal and Enriched only, max refine 10.
- The extended extract needs the fork's `build_refine`; the fork's branch is local only (GOAL open question).
- The bank is a currency, not a table.
- Kafra points in cash shops are out.

For item-windows (new):

- The enchant grade adds item fields: do it first. `Extra.Enchants` keeps positions; cards cannot hold them.
- All three windows are renewal only. Reform bases include fourth-class gear that nobody can wear yet.
- The grade bonus to refine ATK and DEF is renewal only (`status.cpp` 3936).

## Proposed learnings

- A grep with `-I` skips script files that have non-UTF-8 bytes, so counts of commands come out low | use `-a` when counting rAthena scripts | any node that counts commands in the fork's npc tree | yes
- A state that rAthena leaves unenforced because kRO clients do not report it can be enforced here: wAthena's client is ours | decide it per rule and say so in the decision record | server rules | yes

## Verification

- Design only. No builds, tests or `scripts/check.sh` runs, as the node asks.
- Checked the proposal, contract, log and outcome for em-dashes: none.

## Size

- Estimated M, actual L: eleven systems needed reads of chat.cpp, achievement.cpp, the clif window code and the item databases.
