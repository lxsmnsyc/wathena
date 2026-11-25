# Log: bonus-damage-taken

- Start check: `go test ./server/bonus ./tools/convert -p 1 -parallel 2` passed, but random-options-renewal.md showed 239 of 249. Not fulfilled.
- Before (reports on batch/v0.2-parity, from `go run ./tools/wactl convert items|random-options --mode <mode> --report ...`): 10 `RACE_WEAPON_TOLERACE_*` failures; `bonus3 bSubEle` 2 partly in renewal, 1 in pre-renewal; unknown monster 1900 (bAddDamageClass), 1900, 2048, 2050 (bAddDefMonster) in pre-renewal; bHPRegenRate and bSPRegenRate 1 failure each (interval is a function); bAutoSpell level 0, 1 item (Falconer_Flute, renewal).
- bSubRace/bSubEle with BF_WEAPON: the converter emits kind=physical alone, DamageGroup wanted a range or attack. The server already maps the list back with bfFlag. Fixed in DamageGroup.
- Monsters 1900, 2048, 2050 are commented out of pre-renewal mob_db.yml (renewal has them). rAthena's pc_bonus2 stores the ID unchecked, so the bonus never applies. The converter now drops the call.
- Regen interval: Sealed cards use `.@duration` from getrefine(). The runtime already runs computed args, so the checker and the renderer allow a function for "interval".
- bAutoSpell level: Falconer_Flute passes an unset `.@i` (0). rAthena casts `it.lv ? it.lv : 1`, so the converter writes 1.
- Regenerated item-scripts-*.md, random-options-renewal.md and the equipment parity testdata (more items compared, 0 mismatches).
