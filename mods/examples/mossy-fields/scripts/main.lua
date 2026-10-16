-- The mod script runs when `wactl world new` makes a procedural world.
-- Its spawn_table hook gets each field's and dungeon floor's spawn list
-- and returns the list to keep. It needs the permission spawns.modify.

local hooks = require "wa.hooks"
local monsters = require "wa.monsters"

local MOSS_PORING = monsters.id "example.mossy-fields:MOSS_PORING"

return function()
  -- Every field that starts below level 20 gets 12 Moss Porings, in
  -- place of any line of them the world drew.
  hooks.on("spawn_table", function(map, spawns, next)
    local list = next(spawns)
    if map.kind ~= "field" or map.band[1] >= 20 then
      return list
    end
    local out = {}
    for _, s in ipairs(list) do
      if s.monster ~= MOSS_PORING then
        table.insert(out, s)
      end
    end
    table.insert(out, { monster = MOSS_PORING, count = 12, respawn = 5000, role = "common" })
    return out
  end)
end
