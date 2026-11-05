# Log: converter-names

- Started from c0f8584. Before: renewal reports 8 `assignment` NPCs, 6 `labels`, 8 `getarg` outside a function, 9 `sscanf`, getd 47 (24 convert), setd 32 (19 convert), variables with "is not a valid name" in 17 (renewal) and 5 (pre-renewal) NPCs. The brief's 221 and 47 predate decisions 0069 and 0070; the "is not a valid name" reasons were the digit-leading ones only.
- Nothing of the work existed (`go test ./tools/convert` passed on a clean tree).
- The report shows the three most common reasons of a row. I raised that to 3000 in a scratch copy (not committed) to see every reason.
- F1: variables.go regex is now `^[A-Za-z0-9_]{1,64}$`. mods/README.md line about names changed too (not in the brief's paths).
- F2: `assignValue` in npcexpr.go. rAthena pushes a variable operand as a reference and reads it when the operator runs, so `a + (a = 5)` is 10 there and 6 here. No script in the tree does this. Recorded as a difference.
- F3: `.@` table is `locals_N`, keys are the lower-case name without prefix, `$` kept for strings. A script with a computed `.` name (or one with no fixed start) makes its `.` variables NPC variables. A function cannot use computed `.` names (its `.` variables are refused anyway).
- Empty bare name (`getd("." + "")`): rAthena keeps a variable named by the prefix alone, which happens for an NPC whose name is cut to 24 characters and loses its # part (Phantasmagorika Spokesperson#prt). The helper maps it to `_`.
- Labels in blocks: AST rewrite in npcnames.go (flag plus a label before the top-level statement). Enters blocks, if branches and switch cases. Loops stay refused.
- getarg outside a function ends the script, as buildin_getarg does.
- sscanf: %d, %s, %% and literal text; fixed format; counts must match; types must match.
- E2: no "is talking" read exists, and one would not help: the touch runs after the talk ended. Added a `waited` parameter to the `touch` event (zone/touch.go, modscript Touch, prelude converse_touch). Touches that waited are dropped by rAthena (no click).
- E2 uses `npc.state == "shown"` for hidden NPCs, since npc_click refuses OPTION_HIDE.
- Test host fix: npc_runtime_test.go hidden_name for "::" names reads the record's display_name.

## Review round 1

- Fixed the eight review points: both sides of &&, || and ?: run when one has an effect (R.both, R.either, R.choose); an index with a call or effect is evaluated once (`pureTarget`, `luaCode`); a function passes no NPC to a computed name (R.getd_any raises for `.` there); dotShared only for names that can reach `.`; run-time names are tables first (order independent); E2 checks `npc:is_cloaked(player)`; end-to-end E2 test in the zone (`TestClickOnTouchConverted`); label tests for a sibling branch and switch cases.
- Item 5 (unbounded names) is recorded in 0071 as accepted.
- Row counts did not change except `rand` (1014 to 1015 scripts in renewal).
- `heard` in touch_test.go only matches text that starts with "touch".

## Review round 2

- Fix 4 of round 1 (run-time names always tables) clashed with a static declaration of the same variable in another script (`Medal Distributor#single`, `$@mr_1_tire1`). Run-time names are numbers or strings again, as static declarations are, and a table only for an index, or in a script that indexes any computed name (`indexed`, passed to R.getd, R.setd and the `_any` forms). The reverse conflict (one script indexes, another names it as a scalar) is recorded in 0071.
- Test: `OrdA` to `OrdE` in `TestRunNames`, a static and a computed use of one name in both orders and the x / x[2] order.
- Reports did not change.
