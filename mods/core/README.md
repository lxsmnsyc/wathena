# core mod

The base game as a mod: rulesets, converted scripts, and game data. See docs/strategy.md, section 6.

The server builds core's files when it starts:

- rAthena's item scripts, converted to `data/items.yml` and `scripts/items/*.lua` (mods/README.md, "Core's item scripts", and [decision 0007](../../docs/decisions/0007-converted-item-scripts.md)).
- rAthena's NPC scripts and shops, converted to `data/npcs.yml`, `scripts/npcs/*.lua`, `scripts/functions/*.lua` and `scripts/lib/rathena.lua` (mods/README.md, "Core's NPCs", and [decision 0008](../../docs/decisions/0008-converted-npcs.md)).

Core's own files, written for wAthena, live beside the converter in `tools/convert/core` and `tools/convert/procedural_npcs.yml`: the Job Master, and the NPCs of procedural worlds, which stand at named places (mods/README.md, "Core's NPCs", and [decision 0044](../../docs/decisions/0044-procedural-content.md)). Its quest templates are `server/content/core/data/quest_templates.yml`. This folder holds no files of its own yet.
