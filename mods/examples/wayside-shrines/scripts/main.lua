-- The mod script's generation layer runs when `wactl world new` makes a
-- procedural world, once on every generated map, on the server only. It
-- needs the permission world.generate. It returns what to add to the
-- map: named places, objects, and bosses. It cannot change the map's
-- cells.

local generation = require "wa.generation"
local monsters = require "wa.monsters"

local VAGABOND_WOLF = monsters.id "VAGABOND_WOLF"

-- STONES are a prop of each field biome's tileset, for the ring.
local STONES = { grassland = "rock", forest = "bush", desert = "rock", snow = "rock", swamp = "reed", beach = "rock" }

-- place_of returns the cell of a map's first place with a tag, or the
-- map's middle.
local function place_of(map, tag)
  for _, p in ipairs(map.places) do
    for _, t in ipairs(p.tags) do
      if t == tag then
        return p.x, p.y
      end
    end
  end
  return map.width // 2, map.height // 2
end

return function()
  generation.on("map", function(map, node)
    if map.kind == "town" then
      -- A shrine a few cells east of the square, where the Shrine
      -- Keeper stands. The server moves it to open ground.
      local x, y = place_of(map, "square")
      return { places = { { tags = { "shrine" }, x = x + 6, y = y } } }
    end
    if map.kind ~= "field" then
      return nil
    end
    -- A ring of stones around the field's first clearing, turned at
    -- random from the layer's own node, and a Vagabond Wolf to guard it.
    local x, y = place_of(map, "clearing")
    local turns = node:child("stones"):stream()
    local objects = {}
    for i, d in ipairs({ { 3, 0 }, { -3, 0 }, { 0, 3 }, { 0, -3 } }) do
      objects[i] = { model = STONES[map.biome] or "rock", x = x + d[1], y = y + d[2], turn = turns:integer(0, 3) }
    end
    return {
      places = { { tags = { "stone_ring" }, x = x, y = y } },
      objects = objects,
      bosses = { { monster = VAGABOND_WOLF, role = "mini_boss", x = x, y = y } },
    }
  end)

  -- Every quest board may ask for offerings: items that its nearby
  -- monsters drop.
  generation.quest_template {
    name = "offerings",
    kind = "collect",
    title = "Bring {count} {item} for the shrines",
    count = { 3, 6 },
    reward = 150,
    weight = 1000,
  }
end
