# Outcome: review-npc-api

## Built

- docs/roadmaps/v0.2-parity/nodes/review-npc-api/review.md: 61 entries grouped by area, with a summary table, and "Draft review decision 60". Review decision number taken: 60 (draft only; proposal 0002 is not edited until the maintainer approves).
- Review decision 60 is recorded in docs/proposals/0002-lua-api.md ("Decisions taken in review"), approved by the maintainer on 2026-10-27, every recommendation as written. It changes review decision 56's `percentheal -100` rule.
- Counts: 35 accept, 15 defer, 1 refuse, 10 already built or settled (keep).
- Accepted items by node:
  - npc-api-events: A2, A5, A6, B4, B8, B9, B11, D6, E1, E5, G15 (`getunitdata`, `getunittype`).
  - npc-api-timers: A4, C1, C2, C3, C4.
  - converter-names: E2, F1, F2, F3.
  - converter-commands: E3, G5, G7, G10, G11, G12, G13, G14, G15 (`setunitdata`), G16.
  - Other existing nodes: A3 and D4 to sieges-war, B7 to instances-content, E4 to item-effects, E6 and E7 to item-skill-use.
- Proposed new node: `lua-cost-accounting` (modding milestone, size M): charge the Lua CPU budget by bytes for string compares, concatenation, `string.rep`, `find` and `gsub` in the C glue, with a new `lua.wasm` (H1).

## Deviations

- Added entries the brief's Steps do not name but its sources list: A6 (`unloadnpc` with duplicates, 0062:117), D5 (proposal step 18, which GOAL.md maps here), D6 (decision 0063's list of unnamed API, 0063:139), B11 (`unittalk` with `bc_self`, from the npc-api-events brief), G18 (step 11, already mapped to third-classes by GOAL.md).
- The HIDDEN_NPC item (0052:166) is reviewed as a rule for `sprite: none`, since decision 0062 changed what view -1 converts to.
- The temporary-skill half of 0053:106 is marked as matching rAthena: rAthena also shows no temporary levels from scripts. Only item skills are accepted.
- `schedules.yearly`, which npc-api-timers' brief lists, is recommended as deferred (0 users).
- converter-names' other rows (labels in blocks, `getarg` outside a function, `goto` across parts, `sscanf`) need no decision and are not entries.

## Stubs left

- None. Nothing was built.

## Gotchas

- F1 changes the name rule in server/modscript/variables.go:78 and the API docs. converter-names owns only tools/convert files; the orchestrator must add server/modscript/variables.go, spec.go and the generated Lua docs to its shared paths, or move F1 to converter-commands.
- E2 (click fallback) changes NPC placement in tools/convert/npc.go, which no build node owns. rAthena skips the click while the player talks (`sd->npc_id == 0`); wAthena queues touches while talking, so the converted handler must not click for a touch that waited. A `player` read of "is talking" may be needed; check before adding API.
- A3 (`npc:wake()`) depends on A4 (`timers.wait`). Build A4 in npc-api-timers before sieges-war.
- C1's `{ clock = "server" }` and A4's waits interact: a wait in a handler of a server clock timer should follow the server clock.
- npc-api-events' brief does not list A2, A5, A6, D6, E1 and E5; item-skill-use's and item-effects' briefs do not list E4, E6 and E7. The orchestrator needs to add them.
- E4 changes review decision 56's text ("never takes HP below 1"); the API doc of `heal_percent` (spec.go:565) must change with it.
- E6 is a protocol change (`SkillInfo.granted_level`); the release raises `schema.Version` anyway.
- Many command counts come from files in the fork, not from the reports, because the extract leaves out instances, guild, battleground, custom and mobs. extract-folders will change the report counts.

## Proposed learnings

- Check later decisions and the code before calling a review item open | decisions 0064 to 0066 settled several items listed in earlier "Details to review" | docs/decisions | no
- Count a command's users in the rAthena fork when the reports leave its folder out | the reports skip instances, guild and custom folders | tools/convert reports | no

## Verification

- Read-only review: grep and reads of the code, the generated reports and the rAthena fork. No build, tests or scripts/check.sh were run, as the brief says.

## Size

- Estimated M, actual M.
