# Adversarial review (2026-10-26)

A fresh-context reviewer checked contracts and proposal briefs, shared files and dependency edges. All eight findings were checked against the code and accepted.

1. proposal-companions claimed a companion model in server/companion. Decision 0016 covers option-bit records only; egg drops are in server/zone/loot.go:213. Applied: rewrote the context, added the node `companion-units` (shared unit base) that pets, homunculus and mercenaries depend on.
2. Item converter nodes owned tools/convert/item.go, but use-item conversion is in use.go (itemskill line 442, bonus_script line 497), usestatus.go and output.go. Applied: use.go and output.go are shared by item nodes, usestatus.go owned by item-effects, npctimer.go owned by npc-api-timers; the item-effects to item-skill-use edge now names use.go.
3. Instance and siege nodes missed server/zone, server/modscript (variables.go:69) and guild code spread over server/zone and server/modscript/guild.go. Applied: server/zone and server/modscript shared by those nodes; the siege nodes gained proto, client and migration paths; both proposal briefs name the real code.
4. Nodes that persist data lacked migration paths. Applied: migrations, queries and generated DB code shared by every persisting node.
5. server/skill/testdata does not exist; goldens are in server/battle/testdata read by server/zone/skillgolden_test.go. Applied to expanded-class-goldens and homunculus.
6. check-hygiene was `could` while every node runs the checks on a small disk. Applied: raised to must; it runs first.
7. Missing edges. Applied: converter-commands depends on converter-names; instances-core depends on npc-api-events; converter-commands notes that castle and siege guild commands belong to the siege nodes.
8. homunculus to extract-sync looked like build order only. Kept, and the acceptance now requires new recordings. mercenaries to homunculus replaced by mercenaries to companion-units.

Rejected: none.
