---@meta wa.generation
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Generation layers (review decision 46): mod code that runs when wactl world new makes a procedural world, on the server only, and in the test worlds of wactl mod test. A layer adds named places, objects and bosses to each generated map. It cannot change terrain cells, which the client generates. Both calls need world.generate. In a running zone they record what they get, and nothing runs it.
---@class wa.generation
local generation = {}

---Adds a generation layer in a mod context. The mods' layers run in load order. A layer has no other source of randomness than its node: wa.random raises an error while it runs. A layer that fails is logged and left out.
---
---Needs the permission `world.generate`.
---@param event "map" The event: "map", once on each generated field, town and dungeon floor, in the layout's order.
---@param layer? fun(map: wa.GenerationMap, node: wa.GenerationNode): wa.GenerationResult Gets the map and the layer's node, and returns nil or what to add.
function generation.on(event, layer) end

---Adds a quest template, which the world's quest boards and job guilds draw from as from the records. Call it in an entry script or a layer. A bad record or a second template of the name raises an error.
---
---Needs the permission `world.generate`.
---@param record table A record of data/quest_templates.yml with its name, such as { name = "shrine_hunt", kind = "hunt", title = "Hunt {count} {monster}", count = { 5, 10 } }. The name gets the mod's prefix.
function generation.quest_template(record) end

return generation
