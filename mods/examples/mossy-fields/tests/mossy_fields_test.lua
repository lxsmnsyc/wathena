-- Tests of the spawns. They run on the test world's field, gen:field:0:0,
-- whose spawn list the world draws from its secret and the hooks then
-- change. They need a wactl built with -tags procedural.

local testing = require "wa.testing"
local monsters = require "wa.monsters"
local maps = require "wa.maps"

local MOSS_PORING = monsters.id "example.mossy-fields:MOSS_PORING"
local MOSS_SPORE = monsters.id "example.mossy-fields:MOSS_SPORE"

return function()
  testing.case("the hook puts 12 Moss Porings on the field", function(world)
    world:new_player { map = "gen:field:0:0", x = 48, y = 48 }
    testing.expect(#world:get_monsters(MOSS_PORING)).to_be(12)
  end)

  testing.case("the spawn tags put Moss Spores on the field", function(world)
    world:new_player { map = "gen:field:0:0", x = 48, y = 48 }
    local spores = world:get_monsters(MOSS_SPORE)
    testing.expect(#spores > 0).to_be_true()
    testing.expect(spores[1].map).to_be("gen:field:0:0")
    testing.expect(spores[1].name).to_be("Moss Spore")
    testing.expect(maps.get_display_name(spores[1].map)).to_contain("Fields")
  end)

  testing.case("the field has other monsters too", function(world)
    world:new_player { map = "gen:field:0:0", x = 48, y = 48 }
    local all = world:get_monsters()
    testing.expect(#all > 12 + #world:get_monsters(MOSS_SPORE)).to_be_true()
  end)
end
