# 0071: Converter Names: Digit Names, Assignments as Values, Computed Names, Labels in Blocks, sscanf and Click on Touch

- Status: built. Details to review.
- Date: 2026-11-05.
- Code: [tools/convert](../../tools/convert) (`npcnames.go`, `npcexpr.go`, `npcbuiltins.go`, `npclua.go`, `npcscript.go`, `npc.go`, `npcreport.go`, `rathena.lua`), [server/modscript](../../server/modscript) (`variables.go`, `npcs.go`, `runtime.go`, `prelude.lua`, `luaapi/spec.go`), [server/zone](../../server/zone) (`touch.go`).
- Docs: [lua-api.md](../generated/lua-api.md), [mods/README.md](../../mods/README.md) ("Variables", "Touch areas"), [npc-scripts-renewal.md](../generated/npc-scripts-renewal.md), [npc-scripts-pre-renewal.md](../generated/npc-scripts-pre-renewal.md).
- Builds the converter-names items of review decision 60 of [proposal 0002](../proposals/0002-lua-api.md): E2, F1, F2 and F3, with the rows that needed no decision (`getarg` outside a function, labels inside a block, `sscanf`). Review decision 64 records the API details.
- Lua API: adds to API 1.0 (the name rule of `wa.variables`, and a third argument of the `touch` event). Protocol: none. Settings: none. Data: none. Store: none. Permission: none new.

## Context

- Every "is not a valid name" in the reports was a name that starts with a digit once its prefix is gone, such as `$@3rdgc_room01`. An assignment used as a value, a `getd` or `setd` of a computed `.@` or `.` name or of a name that is not fixed text, `getarg` in a script's own code, a label inside a block that a jump from outside reaches, and `sscanf` did not convert. An NPC with a touch area and no touch label was not clicked on touch.

## Decision

### Names (F1)

- `wa.variables` names are 1 to 64 letters, digits and `_`. A name may start with a digit. Names are strings and the converter makes Lua locals with its own names, so Lua identifiers do not matter.
- The rule for the curator: a name starts with a digit only once its prefix (`.@`, `.`, `@`, `$@`, `$`, `#`, `##`) is gone. A word without a prefix that starts with digits and then a letter or `_` is a word to rAthena's parser (`is_number`), such as `4_M_CACTUSMAN20`, and a character variable when it is no constant.

### Assignments as values (F2)

- An assignment used as a value is an inline function that assigns and returns the variable's new value: `(function() x = e return x end)()`. `x++` and `x--` return the old value, `++x` and `--x` the new one. A `.@` local is set as an upvalue. This covers `.@a = .@b = 2`, `(.@x = f()) > 2`, `while ((.@s = select(...)) ...)` and `.@a[.@i++] = 1`.
- The rule for the curator: Lua reads the operands of an expression from left to right, so a side effect runs where the script writes it, and the value an operand gives is the one when it is reached.

### Computed names (F3)

- A function whose statements have a computed name that may reach a `.@` variable keeps all its `.@` variables in one Lua table, `locals_N`, by lower-case name without the prefix, with a `$` at the end of a string's name. `R.getd_local` and `R.setd_local` read and write it. A name that the function never wrote is made on its first write.
- A script whose statements have a computed name that may reach a `.` variable makes all its `.` variables NPC variables of `wa.variables`, as `getvariableofnpc` does. `R.getd` and `R.setd` take the NPC as the owner of the scope `"npc"`. A function keeps refusing `.` variables.
- A name whose start is not fixed text, or does not fix the scope (`getd(.@var$)`, `getd("." + .@name$)`, `getd("$" + .@x$)`), goes through `R.getd_any` and `R.setd_any`. They read the prefix when the script runs, declare the variable then, and use the `.@` table and the NPC's variables for those prefixes.
- Core has no variable count limit (decision 0062), so declaring at run time is free there. A mod that converts its own scripts keeps the 256 limit, and computed names count toward it.

### getarg and getargcount outside a function

- In a script's own code `getarg` and `getargcount` end the script, as rAthena's, which logs "no callfunc or callsub" and stops. This is the code of a label that `callsub` runs, which the script also falls into. The conversion is `(R.stop() or 0)`.

### Labels inside blocks

- A jump to a label inside a block, from outside the block, sets a flag and jumps to a new label before the top-level statement that holds the label. The statements on the way to the label run only while the flag is clear, the conditions of the `if` branches on the way are skipped while it is set, and the label clears it. It enters blocks, `if` and `else` branches and `switch` cases (the `switch`'s value is skipped, and the case is picked). A label inside a loop stays refused, with its reason.

### sscanf

- `sscanf(text, format, variables...)` converts when the format is fixed text of `%d`, `%s`, `%%`, white space and literal text, when each conversion has its variable, and when `%d` goes into a number variable and `%s` into a string variable. `R.sscanf` returns the count, or -1 when the text or a conversion does not match, with the values it read. The variables before the failure are set, as rAthena's, which scans once for each conversion. As a statement the count is not read.

### Click on touch (E2)

- A script NPC with a touch area, a view, a click handler and no `OnTouch` or `OnTouch_` label clicks on touch: the script registers `touch` with a handler that runs the click handler, as `npc_touch_areanpc` does (npc.cpp 1943). The area is the line's span, or one of its duplicates', which run the same script.
- rAthena clicks only for a player who is not talking, and drops the touch otherwise. wAthena queues a touch while the player talks, so the `touch` event has a third argument, `waited`, true for a touch that waited. The converted handler does not click for it. No read of "is talking" exists, and one would not help: a queued touch runs after the conversation ended.
- A hidden NPC is not clicked (`npc_click` refuses OPTION_HIDE): the handler reads `npc.state`.

## Breaks

- Converted `.x` variables of a script that has a computed `.` name are NPC variables and no longer Lua locals of the setup function. They are not saved and every zone reads them.
- `Runtime.Touch` takes `waited` as its last argument.

## Alternatives

- **A prefix for digit names** (F1, option 2). A stored name that differs from rAthena's, and a collision if a script also has the prefixed name.
- **Left-to-right with lazy reads** (F2). rAthena reads a variable operand when the operator runs. See Details to review.
- **Closures for `.@` variables read by name** (F3). A table is what review decision 60 settled, and has no limit of 200 locals.
- **Moving the label's statements into a local function** (labels). It does not keep `.@` scope, `break` and jumps out of the block.
- **Clicking from the zone** (E2). A record field and a protocol of the data; the review settled a converter-only rule.

## Details to review

- rAthena pushes a variable as a reference and reads it when the operator runs, so `.@a + (.@a = 5)` is 10 there and 6 here, and `.@a == (.@a = 3)` compares the new value with itself there. No script in rAthena's tree writes this.
- `getd` of a name with no fixed start reads a variable even when the name is a constant, a parameter or a currency's variable (`getd("Zeny")`), where rAthena reads that. A fixed start that may be one of them is still refused. Names with other characters than letters, digits and `_` raise an error, where rAthena takes any text as a name. A name that is only a prefix (`.`, for an NPC whose name lost its `#` part) is the variable `_`.
- A computed `.` name in a function, and a `'` name, stay refused.
- A script with a computed `.` name keeps its `.` variables as NPC variables, which a `getvariableofnpc` of another NPC reads too.
- A failed `sscanf` returns -1 and the script goes on, with the variables before the failure set. rAthena's also returns -1 for a character variable when no player is attached; this conversion does not check that.
- `sscanf` with a computed format, a conversion other than `%d` and `%s`, or a width, is refused.
- An NPC whose state is `hidden` when it is touched does not click, as rAthena's. An NPC that `hideoffnpc` shows later does.
- A duplicate's touch area makes its source script click on touch for every NPC that runs it; only the NPCs with a touch area get touches.
- The `waited` flag is set only by the queue of touches (`maxTouchQueue`). A touch dropped there because two already wait is lost, as rAthena's.
- A label that a jump reaches from outside a loop, or from inside another loop, stays refused.

## How to check it worked

- `go test ./tools/convert -run 'TestNamesConvert|TestClickOnTouchConvert|TestRunNames|TestBuiltinsConvert|TestRunConvertedNPCs'`.
- `go test ./server/modscript -run TestVariableNameStartsWithDigit`.
- `go test ./server/zone -run 'TestTouchWaited|TestTouchQueue'`.
